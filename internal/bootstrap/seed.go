// Package bootstrap 负责装配（批次 T11 完成）与幂等种子数据写入。
package bootstrap

import (
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
	"github.com/chixiaowen/itsm-core/internal/pkg/sla"
)

// 默认管理员账号（交付文档要求：首次登录后修改密码）。
const (
	SeedAdminUsername = "admin"
	SeedAdminPassword = "admin123"
)

// Seed 幂等写入种子数据。
//
// platform 域（admin/演示账号、SLA 策略）为强依赖，失败即返回错误；
// 其余跨域示例数据为 best-effort：对应表尚未迁移时仅告警，不中断启动。
func Seed(db *gorm.DB, log *zap.Logger) error {
	if db == nil {
		return fmt.Errorf("bootstrap.Seed: db 为 nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	now := time.Now().UTC()

	if err := seedAdminUser(db, log, now); err != nil {
		return fmt.Errorf("种子数据: admin 账号: %w", err)
	}
	// platform 域强依赖：用户表必然已迁移，失败直接返回错误（非 best-effort）。
	if err := seedDemoUsers(db, log, now); err != nil {
		return fmt.Errorf("种子数据: 演示账号: %w", err)
	}
	if err := seedSLAPolicies(db, log, now); err != nil {
		return fmt.Errorf("种子数据: SLA 策略: %w", err)
	}

	bestEffort(log, "service_categories", func() error { return seedServiceCategories(db, now) })
	bestEffort(log, "cis/ci_relations", func() error { return seedCMDB(db, now) })
	bestEffort(log, "ticket_categories", func() error { return seedTicketCategories(db, now) })

	return nil
}

// seedAdminUser 幂等创建 admin 账号。
func seedAdminUser(db *gorm.DB, log *zap.Logger, now time.Time) error {
	hash, err := security.HashPassword(SeedAdminPassword)
	if err != nil {
		return err
	}
	created, err := firstOrCreate(db,
		db.Where("username = ?", SeedAdminUsername),
		&platform.User{},
		func() any {
			return &platform.User{
				Username:     SeedAdminUsername,
				DisplayName:  "系统管理员",
				Role:         role.Admin,
				PasswordHash: hash,
				Email:        "admin@example.com",
				Status:       platform.UserStatusActive,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
		})
	if err != nil {
		return err
	}
	if created {
		log.Info("已创建默认 admin 账号（请首次登录后修改密码）", zap.String("username", SeedAdminUsername))
	}
	return nil
}

// demoAccount 描述一个分角色演示账号（README 对外承诺的清单项）。
type demoAccount struct {
	Username    string
	Role        string
	DisplayName string
	Email       string
}

// demoAccounts 是 README「演示账号」表格逐条对应的分角色账号（密码统一 admin123）。
//
// admin 由 seedAdminUser 单独创建，此处只列其余 7 个角色。cm01 / cm02 同为
// change_manager，专门用于演示 CAB「会签」（普通变更需 2 名审批人全部通过），故不可省。
var demoAccounts = []demoAccount{
	{Username: "requestor01", Role: role.Requestor, DisplayName: "终端用户", Email: "requestor01@example.com"},
	{Username: "agent01", Role: role.Agent, DisplayName: "服务台坐席", Email: "agent01@example.com"},
	{Username: "resolver01", Role: role.Resolver, DisplayName: "二线工程师", Email: "resolver01@example.com"},
	{Username: "pm01", Role: role.ProblemManager, DisplayName: "问题经理", Email: "pm01@example.com"},
	{Username: "cm01", Role: role.ChangeManager, DisplayName: "变更经理", Email: "cm01@example.com"},
	{Username: "cm02", Role: role.ChangeManager, DisplayName: "变更经理", Email: "cm02@example.com"},
	{Username: "cmdb01", Role: role.CmdbManager, DisplayName: "配置管理员", Email: "cmdb01@example.com"},
}

// seedDemoUsers 幂等创建分角色演示账号（密码统一 admin123）。
//
// 属 platform 域强依赖：用户表必然已迁移，任一失败直接返回错误，不做 best-effort 降级。
func seedDemoUsers(db *gorm.DB, log *zap.Logger, now time.Time) error {
	for _, acc := range demoAccounts {
		hash, err := security.HashPassword(SeedAdminPassword)
		if err != nil {
			return err
		}
		acc := acc
		created, err := firstOrCreate(db,
			db.Where("username = ?", acc.Username),
			&platform.User{},
			func() any {
				return &platform.User{
					Username:     acc.Username,
					DisplayName:  acc.DisplayName,
					Role:         acc.Role,
					PasswordHash: hash,
					Email:        acc.Email,
					Status:       platform.UserStatusActive,
					CreatedAt:    now,
					UpdatedAt:    now,
				}
			})
		if err != nil {
			return err
		}
		if created {
			log.Info("已创建演示账号",
				zap.String("username", acc.Username), zap.String("role", acc.Role))
		}
	}
	return nil
}

// seedSLAPolicies 幂等创建四条默认 SLA 策略（P1~P4）。
func seedSLAPolicies(db *gorm.DB, log *zap.Logger, now time.Time) error {
	for _, p := range sla.DefaultPolicies() {
		p := p
		if _, err := firstOrCreate(db,
			db.Where("priority = ?", p.Priority),
			&platform.SLAPolicy{},
			func() any {
				return &platform.SLAPolicy{
					Name:            fmt.Sprintf("%s 默认策略", p.Priority),
					Priority:        p.Priority,
					ResponseMinutes: p.ResponseMinutes,
					ResolveMinutes:  p.ResolveMinutes,
					PauseOnPending:  p.PauseOnPending,
					CreatedAt:       now,
					UpdatedAt:       now,
				}
			}); err != nil {
			return err
		}
	}
	log.Info("默认 SLA 策略已就绪", zap.Int("count", len(sla.DefaultPolicies())))
	return nil
}

// seedServiceCategories 幂等创建服务分类示例（service_categories）。
func seedServiceCategories(db *gorm.DB, now time.Time) error {
	names := []string{"办公支持", "账号与权限", "网络与连接", "硬件与设备"}
	for i, name := range names {
		i, name := i, name
		if _, err := firstOrCreate(db,
			db.Where("name = ? AND parent_id IS NULL", name),
			&seedServiceCategory{},
			func() any {
				return &seedServiceCategory{Name: name, SortOrder: i + 1, CreatedAt: now, UpdatedAt: now}
			}); err != nil {
			return err
		}
	}
	return nil
}

// seedTicketCategories 幂等创建工单分类树示例（ticket_categories）。
func seedTicketCategories(db *gorm.DB, now time.Time) error {
	names := []string{"硬件故障", "软件问题", "账号问题"}
	for i, name := range names {
		i, name := i, name
		if _, err := firstOrCreate(db,
			db.Where("name = ? AND parent_id IS NULL", name),
			&seedTicketCategory{},
			func() any {
				return &seedTicketCategory{Name: name, SortOrder: i + 1, CreatedAt: now, UpdatedAt: now}
			}); err != nil {
			return err
		}
	}
	return nil
}

// seedCMDB 幂等创建示例 CI 与两条关系（cis / ci_relations）。
//
// 注：CI 类型为 Go 侧枚举常量，无需建表；此处仅写示例 CI 与关系。
func seedCMDB(db *gorm.DB, now time.Time) error {
	samples := []seedCI{
		{Code: "srv-app-01", Name: "应用服务器 01", CIType: "server", Status: "in_use"},
		{Code: "db-pg-01", Name: "PostgreSQL 主库 01", CIType: "database", Status: "in_use"},
		{Code: "net-core-01", Name: "核心交换机 01", CIType: "network", Status: "in_use"},
	}

	ids := make(map[string]uint64, len(samples))
	for _, s := range samples {
		s := s
		var ci seedCI
		if err := db.Where("code = ?", s.Code).First(&ci).Error; err == nil {
			ids[s.Code] = ci.ID
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		ci = seedCI{
			Code: s.Code, Name: s.Name, CIType: s.CIType, Status: s.Status,
			Attrs: "{}", CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&ci).Error; err != nil {
			return err
		}
		ids[s.Code] = ci.ID
	}

	relations := []struct {
		source, target, rel string
	}{
		{"srv-app-01", "db-pg-01", "depends_on"},
		{"srv-app-01", "net-core-01", "connects_to"},
	}
	for _, r := range relations {
		sourceID, ok1 := ids[r.source]
		targetID, ok2 := ids[r.target]
		if !ok1 || !ok2 {
			continue
		}
		r := r
		if _, err := firstOrCreate(db,
			db.Where("source_ci_id = ? AND target_ci_id = ?", sourceID, targetID),
			&seedCIRelation{},
			func() any {
				return &seedCIRelation{SourceCIID: sourceID, TargetCIID: targetID, RelationType: r.rel, CreatedAt: now}
			}); err != nil {
			return err
		}
	}
	return nil
}

// firstOrCreate 通用「先查后建」幂等写入原语。
//
// 背景（集成期缺陷修复）：原实现使用 `Where("k = ?", v).Attrs(...).FirstOrCreate(&dst)`，
// 其中 WHERE 为**字符串条件**——GORM 不会把字符串条件里的字段值写入待插入记录，
// 导致被查询字段（如 username）落为空串，第二条起触发唯一索引冲突。
// 这里改为显式「SELECT -> 未命中则 Create(完整实体)」，语义确定、幂等且可读。
//
//	query 为定位唯一键的查询（已带 Where）；dest 为查询目标；entity 为未命中时插入的完整实体。
func firstOrCreate(db *gorm.DB, query *gorm.DB, dest any, entity func() any) (created bool, err error) {
	err = query.First(dest).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if err := db.Create(entity()).Error; err != nil {
		return false, err
	}
	return true, nil
}

// bestEffort 执行跨域种子并在失败时仅告警（依赖表可能尚未迁移）。
func bestEffort(log *zap.Logger, section string, fn func() error) {
	if err := fn(); err != nil {
		log.Warn("跨域种子数据写入跳过（依赖表可能尚未迁移）",
			zap.String("section", section), zap.Error(err))
	}
}

// ---- 跨域种子投影结构（仅用于种子插入，表结构由对应业务域 Migrate 创建）----

type seedServiceCategory struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"type:varchar(128);not null"`
	ParentID  *uint64
	SortOrder int `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (seedServiceCategory) TableName() string { return "service_categories" }

type seedTicketCategory struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"type:varchar(128);not null"`
	ParentID  *uint64
	SortOrder int `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (seedTicketCategory) TableName() string { return "ticket_categories" }

type seedCI struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Code      string `gorm:"type:varchar(64);not null;uniqueIndex:idx_ci_code"`
	Name      string `gorm:"type:varchar(255);not null"`
	CIType    string `gorm:"type:varchar(32);not null"`
	Status    string `gorm:"type:varchar(16);not null"`
	Attrs     string `gorm:"type:text"`
	OwnerID   *uint64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (seedCI) TableName() string { return "cis" }

type seedCIRelation struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	SourceCIID   uint64 `gorm:"not null;uniqueIndex:idx_cirel_uq"`
	TargetCIID   uint64 `gorm:"not null;uniqueIndex:idx_cirel_uq"`
	RelationType string `gorm:"type:varchar(16);not null"`
	CreatedAt    time.Time
}

func (seedCIRelation) TableName() string { return "ci_relations" }
