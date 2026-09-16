// Package bootstrap 负责应用装配（批次 T11）：按拓扑序构造 repository -> service -> handler，
// 注入全部跨域消费者接口，并注册业务路由。
//
// 装配层（cmd + bootstrap）唯一允许直接接触 *gorm.DB（经 database.DB 别名）的地方；
// 从此层向下，业务代码一律不得依赖 gorm。所有 9 个域的 service 在此一次性注入真实实现，
// 使此前各域「内存 fake 注入单测」覆盖不到的 GORM 模型 / AutoMigrate / repository_gorm 查询，
// 得以在集成测试中被真实验证。
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/config"
	"github.com/chixiaowen/itsm-core/internal/domain/asset"
	"github.com/chixiaowen/itsm-core/internal/domain/auth"
	"github.com/chixiaowen/itsm-core/internal/domain/catalog"
	"github.com/chixiaowen/itsm-core/internal/domain/change"
	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
	"github.com/chixiaowen/itsm-core/internal/domain/incident"
	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/domain/problem"
	"github.com/chixiaowen/itsm-core/internal/domain/ticket"
	"github.com/chixiaowen/itsm-core/internal/pkg/database"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/idgen"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

// Handlers 汇总 9 个域的 HTTP handler（供路由注册使用）。
type Handlers struct {
	Platform *platform.Handler
	Auth     *auth.Handler
	Ticket   *ticket.Handler
	Incident *incident.Handler
	Change   *change.Handler
	Problem  *problem.Handler
	Catalog  *catalog.Handler
	CMDB     *cmdb.Handler
	Asset    *asset.Handler
}

// App 是一次完整装配的结果：全部 service 实例 + handler 集合 + 共享组件（JWT）。
//
// DB 仅作为装配层持有引用（供 /healthz 探活），业务代码不得读取。
type App struct {
	Config *config.Config
	Logger *zap.Logger
	DB     *database.DB
	JWT    *security.JWTManager

	Platform *platform.Service
	Auth     *auth.Service
	Ticket   *ticket.Service
	Incident *incident.Service
	Change   *change.Service
	Problem  *problem.Service
	Catalog  *catalog.Service
	CMDB     *cmdb.Service
	Asset    *asset.Service

	Handlers Handlers
}

// New 装配全部域：repo -> service -> handler，并注入跨域消费者接口。
//
// 构造顺序遵循依赖拓扑：
//
//	platform -> ticket -> incident -> change -> problem -> cmdb -> catalog -> asset -> auth
//
// 任一强依赖缺失立即返回错误，绝不产出半装配的可运行实例。
func New(cfg *config.Config, log *zap.Logger, db *database.DB) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("bootstrap.New: cfg 为 nil")
	}
	if db == nil {
		return nil, fmt.Errorf("bootstrap.New: db 为 nil")
	}
	if log == nil {
		log = zap.NewNop()
	}

	jwt := security.NewJWTManager(cfg.Auth.JWTSecret, time.Duration(cfg.Auth.JWTTTLHours)*time.Hour)

	// ---- platform（共享内核）：用户 / SLA / 审计 / 评论 / 附件 ----
	userRepo := platform.NewUserRepository(db)
	slaRepo := platform.NewSLAPolicyRepository(db)
	auditRepo := platform.NewAuditRepository(db)
	commentRepo := platform.NewCommentRepository(db)
	attachRepo := platform.NewAttachmentRepository(db)

	platformSvc := platform.NewService(platform.Deps{
		Users:          userRepo,
		SLAPolicies:    slaRepo,
		Audits:         auditRepo,
		Comments:       commentRepo,
		Attachments:    attachRepo,
		Logger:         log,
		MaxUploadBytes: cfg.UploadLimitBytes(),
		UploadDir:      cfg.App.UploadDir,
	})

	// ---- ticket：提供 incident.TicketCreator、catalog.TicketCreator ----
	ticketRepo := ticket.NewRepository(db)
	ticketCatRepo := ticket.NewCategoryRepository(db)

	ticketSvc := ticket.NewService(ticket.Deps{
		Repo:       ticketRepo,
		Categories: ticketCatRepo,
		Users:      platformSvc, // ticket.UserDirectory
		SLAs:       platformSvc, // ticket.SLAPolicyReader
		Comments:   platformSvc, // ticket.CommentStore（含 AuditWriter）
		IDGen:      idgen.NewGenerator(),
		Logger:     log,
	})

	// ---- incident：注入 ticket.TicketCreator 与 platform 审计/用户/SLA ----
	incidentRepo := incident.NewRepository(db)
	escalationRepo := incident.NewEscalationRepository(db)

	incidentSvc := incident.NewService(incident.Deps{
		Repo:        incidentRepo,
		Escalations: escalationRepo,
		Users:       platformSvc, // incident.UserDirectory
		SLAs:        platformSvc, // incident.SLAPolicyReader
		Auditor:     platformSvc, // incident.Auditor
		Tickets:     ticketSvc,   // incident.TicketCreator
		IDGen:       idgen.NewGenerator(),
		Logger:      log,
	})

	// ---- change：注入 platform 审计/用户 ----
	changeRepo := change.NewRepository(db)
	approvalRepo := change.NewApprovalRepository(db)

	changeSvc := change.NewService(change.Deps{
		Repo:          changeRepo,
		Approvals:     approvalRepo,
		Users:         platformSvc, // change.UserDirectory
		Auditor:       platformSvc, // change.Auditor
		IDGen:         idgen.NewGenerator(),
		EnforceWindow: cfg.Change.EnforceWindow,
		Logger:        log,
	})

	// ---- problem：注入 incident.IncidentReader 与 change.ChangeReader ----
	problemRepo := problem.NewRepository(db)
	problemChangeRepo := problem.NewProblemChangeRepository(db)

	problemSvc := problem.NewService(problem.Deps{
		Repo:       problemRepo,
		Changes:    problemChangeRepo,
		Incidents:  incidentSvc, // problem.IncidentReader
		ChangeRead: changeSvc,   // problem.ChangeReader
		Users:      platformSvc, // problem.UserDirectory
		Auditor:    platformSvc, // problem.Auditor
		IDGen:      idgen.NewGenerator(),
		Logger:     log,
	})

	// ---- cmdb：注入 platform 审计 ----
	ciRepo := cmdb.NewCIRepository(db)
	relationRepo := cmdb.NewRelationRepository(db)

	cmdbSvc := cmdb.NewService(cmdb.Deps{
		CIs:       ciRepo,
		Relations: relationRepo,
		Audit:     platformSvc, // cmdb.AuditWriter
		Logger:    log,
	})

	// ---- catalog：注入 platform 审计 + ticket.TicketCreator（经适配器）----
	catalogCatRepo := catalog.NewCategoryRepository(db)
	catalogItemRepo := catalog.NewItemRepository(db)

	catalogSvc := catalog.NewService(catalog.Deps{
		Categories: catalogCatRepo,
		Items:      catalogItemRepo,
		Audit:      platformSvc,                      // catalog.AuditWriter
		Tickets:    &ticketCreatorAdapter{ticketSvc}, // catalog.TicketCreator
		Logger:     log,
	})

	// ---- asset：注入 cmdb（CIRepository 满足 asset.CIReader）+ platform 审计 ----
	assetRepo := asset.NewAssetRepository(db)
	assetHistRepo := asset.NewHistoryRepository(db)

	assetSvc := asset.NewService(asset.Deps{
		Assets:    assetRepo,
		Histories: assetHistRepo,
		CIs:       ciRepo,      // asset.CIReader（GetByID）
		Audit:     platformSvc, // asset.AuditWriter
		Gen:       idgen.NewGenerator(),
		Logger:    log,
	})

	// ---- auth ----
	authSvc := auth.NewService(userRepo, jwt)

	app := &App{
		Config:   cfg,
		Logger:   log,
		DB:       db,
		JWT:      jwt,
		Platform: platformSvc,
		Auth:     authSvc,
		Ticket:   ticketSvc,
		Incident: incidentSvc,
		Change:   changeSvc,
		Problem:  problemSvc,
		Catalog:  catalogSvc,
		CMDB:     cmdbSvc,
		Asset:    assetSvc,
		Handlers: Handlers{
			Platform: platform.NewHandler(platformSvc, log),
			Auth:     auth.NewHandler(authSvc),
			Ticket:   ticket.NewHandler(ticketSvc, log),
			Incident: incident.NewHandler(incidentSvc, log),
			Change:   change.NewHandler(changeSvc, log),
			Problem:  problem.NewHandler(problemSvc, log),
			Catalog:  catalog.NewHandler(catalogSvc, log),
			CMDB:     cmdb.NewHandler(cmdbSvc, log),
			Asset:    asset.NewHandler(assetSvc, log),
		},
	}
	return app, nil
}

// Migrate 按拓扑序调用全部 9 个域自带的 Migrate（禁用 GORM 自动外键，故顺序不敏感，
// 但必须全部调用，缺一不可）。
//
// 拓扑序：platform -> cmdb -> asset -> catalog -> ticket -> incident -> change -> problem。
func Migrate(db *database.DB) error {
	if db == nil {
		return fmt.Errorf("bootstrap.Migrate: db 为 nil")
	}
	migrations := []struct {
		name string
		fn   func(*database.DB) error
	}{
		{"platform", platform.Migrate},
		{"cmdb", cmdb.Migrate},
		{"asset", asset.Migrate},
		{"catalog", catalog.Migrate},
		{"ticket", ticket.Migrate},
		{"incident", incident.Migrate},
		{"change", change.Migrate},
		{"problem", problem.Migrate},
	}
	for _, m := range migrations {
		if err := m.fn(db); err != nil {
			return fmt.Errorf("迁移 %s 域失败: %w", m.name, err)
		}
	}
	return nil
}

// ticketCreatorAdapter 把 ticket.Service 适配为 catalog.TicketCreator。
//
// 背景（T11 集成期发现的接口漂移）：
//
//	catalog.TicketCreator 定义 CreateFromServiceItem(ctx, req catalog.TicketFromServiceItem) (uint64, error)
//	而 ticket.Service.CreateFromServiceItem 的签名为位置参数
//	  CreateFromServiceItem(ctx, title, description string, requesterID, categoryID, serviceItemID uint64, priority, formData string) (uint64, string, error)
//
// 二者不匹配。为遵守「严禁改动 domain 业务代码」的约束，适配逻辑收敛在装配层，
// 不改动任一域。该漂移已在集成报告中单列。
type ticketCreatorAdapter struct {
	svc *ticket.Service
}

// CreateFromServiceItem 实现 catalog.TicketCreator。
func (a *ticketCreatorAdapter) CreateFromServiceItem(ctx context.Context, req catalog.TicketFromServiceItem) (uint64, error) {
	if a == nil || a.svc == nil {
		return 0, httpx.ErrInternal("工单创建组件未初始化")
	}
	var categoryID uint64
	if req.CategoryID != nil {
		categoryID = *req.CategoryID
	}
	ticketID, _, err := a.svc.CreateFromServiceItem(
		ctx,
		req.Title,
		req.Description,
		req.RequesterID,
		categoryID,
		req.ServiceItemID,
		req.Priority,
		req.FormData,
	)
	if err != nil {
		return 0, err
	}
	return ticketID, nil
}

// 编译期断言：确保装配注入满足各域消费者接口（接口漂移会在编译期暴露）。
var (
	_ catalog.TicketCreator  = (*ticketCreatorAdapter)(nil)
	_ incident.TicketCreator = (*ticket.Service)(nil)
	_ problem.IncidentReader = (*incident.Service)(nil)
	_ problem.ChangeReader   = (*change.Service)(nil)
	_ asset.CIReader         = (cmdb.CIRepository)(nil)
	_ cmdb.AuditWriter       = (*platform.Service)(nil)
	_ catalog.AuditWriter    = (*platform.Service)(nil)
	_ asset.AuditWriter      = (*platform.Service)(nil)
	_ incident.Auditor       = (*platform.Service)(nil)
	_ change.Auditor         = (*platform.Service)(nil)
	_ problem.Auditor        = (*platform.Service)(nil)
	_ ticket.CommentStore    = (*platform.Service)(nil)
)
