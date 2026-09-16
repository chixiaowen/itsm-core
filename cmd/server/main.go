// Command server 是 itsm-core 后端的进程入口。
//
// 职责：加载配置 -> 建 zap logger -> 连接数据库（失败仅告警）-> 按拓扑序迁移全部域
// -> 写入种子数据 -> 经 internal/bootstrap 装配全部业务域 -> 挂载路由 -> 优雅退出。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/bootstrap"
	"github.com/chixiaowen/itsm-core/internal/config"
	"github.com/chixiaowen/itsm-core/internal/pkg/database"
	"github.com/chixiaowen/itsm-core/internal/pkg/logger"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（缺省搜索 ./configs/config.yaml）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	zlog, err := logger.New(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer func() { _ = zlog.Sync() }()

	// 连接数据库：失败仅告警、不 panic，保证无库也能启动并输出日志。
	var (
		db  *database.DB
		app *bootstrap.App
	)
	db, dbErr := database.Open(cfg.Database, zlog)
	if dbErr != nil {
		db = nil
		zlog.Warn("数据库连接失败，服务以降级模式启动（业务接口不可用）",
			zap.String("driver", cfg.Database.Driver), zap.Error(dbErr))
	} else {
		zlog.Info("数据库连接成功", zap.String("driver", cfg.Database.Driver))
		initDatabase(db, cfg, zlog)

		assembled, aErr := bootstrap.New(cfg, zlog, db)
		if aErr != nil {
			// 装配失败视为致命：回退为降级启动（仅 /healthz），避免半可用实例对外服务。
			zlog.Error("业务域装配失败，降级为仅健康检查启动", zap.Error(aErr))
			app = nil
		} else {
			app = assembled
			zlog.Info("业务域装配完成（9 个域、全部跨域接口已注入）")
		}
	}

	engine := bootstrap.BuildEngine(cfg, zlog, db, app)

	srv := &http.Server{
		Addr:              cfg.App.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		zlog.Info("HTTP 服务启动", zap.String("addr", cfg.App.HTTPAddr), zap.String("env", cfg.App.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zlog.Error("HTTP 服务异常退出", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	zlog.Info("收到退出信号，开始优雅关闭…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		zlog.Error("优雅关闭失败", zap.Error(err))
		return
	}
	zlog.Info("服务已优雅退出")
}

// initDatabase 按拓扑序迁移全部域表并写入种子数据（best-effort，失败仅告警不中断启动）。
func initDatabase(db *database.DB, cfg *config.Config, zlog *zap.Logger) {
	if cfg.Database.AutoMigrate {
		if err := bootstrap.Migrate(db); err != nil {
			zlog.Warn("业务域表迁移失败", zap.Error(err))
		} else {
			zlog.Info("全部域表迁移完成")
		}
	}
	if cfg.Database.Seed {
		if err := bootstrap.Seed(db, zlog); err != nil {
			zlog.Warn("种子数据初始化失败", zap.Error(err))
		} else {
			zlog.Info("种子数据初始化完成")
		}
	}
}
