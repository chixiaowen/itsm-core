// Package database 提供数据库 Dialector 工厂、连接管理与通用迁移入口。
//
// 这是全工程唯一（与各域 repository_gorm.go 一起）允许直接使用 *gorm.DB 的包之一。
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/godoes/gorm-dameng"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/config"
)

// DB 是 *gorm.DB 的类型别名。
//
// 通过别名暴露，使 cmd/bootstrap 等装配层无需直接 import gorm.io/gorm，
// 从而把 GORM 耦合收敛在 repository_gorm.go 与本包内。
type DB = gorm.DB

// Dialector 根据配置构造 gorm.Dialector（不建立连接，便于单元测试断言类型）。
func Dialector(cfg config.Database) (gorm.Dialector, error) {
	switch cfg.Driver {
	case config.DriverPostgres:
		return postgres.Open(buildPostgresDSN(cfg)), nil
	case config.DriverDameng:
		return dameng.New(dameng.Config{
			DriverName:              dameng.DriverName,
			DSN:                     buildDamengDSN(cfg),
			VarcharSizeIsCharLength: cfg.Dameng.VarcharSizeIsChar,
		}), nil
	default:
		return nil, fmt.Errorf("不支持的 DB_DRIVER: %q（可选 postgres | dameng）", cfg.Driver)
	}
}

// buildPostgresDSN 拼接 PostgreSQL DSN；若显式配置了 DSN 则直接使用。
func buildPostgresDSN(cfg config.Database) string {
	if cfg.DSN != "" {
		return cfg.DSN
	}
	p := cfg.Postgres
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.SSLMode, p.TimeZone,
	)
}

// buildDamengDSN 拼接达梦 DSN；若显式配置了 DSN 则直接使用。
func buildDamengDSN(cfg config.Database) string {
	if cfg.DSN != "" {
		return cfg.DSN
	}
	d := cfg.Dameng
	return dameng.BuildUrl(d.User, d.Password, d.Host, d.Port, map[string]string{"schema": d.Schema})
}

// Open 建立数据库连接并配置连接池。
//
// 注意：连接失败会返回错误（由调用方决定是否降级告警），本函数不自作退出。
func Open(cfg config.Database, log *zap.Logger) (*DB, error) {
	dialector, err := Dialector(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		// 禁用 GORM 自动外键迁移：避免达梦 30 字符外键名限制，外键语义由 service 保证。
		DisableForeignKeyConstraintWhenMigrating: true,
		// 可选关闭启动探活（受限网络/启动即降级场景）。
		DisableAutomaticPing: cfg.DisablePing,
		Logger:               newGormZapLogger(log, defaultSlowThreshold),
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层 sql.DB 失败: %w", err)
	}

	// 连接池参数对 postgres / dameng 通用（取自 postgres 配置块）。
	if cfg.Postgres.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	}
	if cfg.Postgres.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}

	return db, nil
}

// Ping 探活底层连接（供 /healthz 使用）。
func Ping(ctx context.Context, db *DB) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层 sql.DB 失败: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("数据库探活失败: %w", err)
	}
	return nil
}
