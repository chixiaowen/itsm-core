// 本文件负责路由装配：把 9 个域的路由挂到 /api/v1 已鉴权分组，并挂好中间件链与 /healthz。
package bootstrap

import (
	"github.com/gin-gonic/gin"
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
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// loginPath 是唯一的免鉴权业务路径（Auth 中间件按完整 URL path 白名单放行）。
const loginPath = "/api/v1/auth/login"

// BuildEngine 构造完整的 gin 引擎：全局中间件链 + /healthz + /api/v1 业务路由。
//
// 中间件链（顺序即执行顺序）：RequestID -> 访问日志 -> panic 恢复 -> CORS；
// 业务分组额外挂 JWT Auth（登录路径走白名单免鉴权）。
//
// app 为 nil 表示数据库不可用时的降级启动：仅保留中间件与 /healthz，不注册业务路由，
// 从而「无库也能起进程并输出日志」。
func BuildEngine(cfg *config.Config, log *zap.Logger, db *database.DB, app *App) *gin.Engine {
	if cfg != nil && cfg.App.Env == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}
	if log == nil {
		log = zap.NewNop()
	}

	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.Logger(log),
		middleware.Recovery(log),
		middleware.CORS(corsOrigins(cfg)),
	)

	// /healthz 免鉴权（置于 Auth 之外）。
	r.GET("/healthz", func(c *gin.Context) {
		if db == nil {
			httpx.Fail(c, httpx.ErrInternal("数据库不可用"))
			return
		}
		if err := database.Ping(c.Request.Context(), db); err != nil {
			httpx.Fail(c, httpx.ErrInternal("数据库探活失败"))
			return
		}
		httpx.OK(c, gin.H{"status": "ok"})
	})

	if app == nil {
		return r
	}

	// /api/v1 业务分组：统一挂 JWT Auth（登录路径免鉴权）。
	api := r.Group("/api/v1", middleware.Auth(app.JWT, loginPath))

	// auth：登录免鉴权（走白名单），me/logout 需登录。
	auth.Register(api, app.Handlers.Auth)
	auth.RegisterProtected(api, app.Handlers.Auth)

	// 其余 8 个业务域：全部挂到同一已鉴权分组，域内路由各自声明权限点/角色。
	platform.Register(api, app.Handlers.Platform)
	ticket.Register(api, app.Handlers.Ticket)
	incident.Register(api, app.Handlers.Incident)
	change.Register(api, app.Handlers.Change)
	problem.Register(api, app.Handlers.Problem)
	catalog.Register(api, app.Handlers.Catalog)
	cmdb.Register(api, app.Handlers.CMDB)
	asset.Register(api, app.Handlers.Asset)

	return r
}

// corsOrigins 安全读取 CORS 允许来源（cfg 为 nil 时返回 nil → CORS 中间件放行所有来源）。
func corsOrigins(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return cfg.CORS.AllowedOrigins
}
