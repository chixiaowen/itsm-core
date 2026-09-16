package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/chixiaowen/itsm-core/internal/config"
)

func TestGormZapLogger_LevelsAndTrace(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	z := zap.New(core)

	base := newGormZapLogger(z, 10*time.Millisecond).(*gormZapLogger)

	// 默认级别 Warn：Info 被丢弃
	base.Info(context.Background(), "skip-info")
	base.Warn(context.Background(), "warn-msg")
	base.Error(context.Background(), "err-msg")

	// 提升到 Info：Info/Debug 生效
	info := base.LogMode(gormlogger.Info).(*gormZapLogger)
	info.Info(context.Background(), "info %s", "x")

	// Trace：正常（debug）
	info.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	// Trace：慢查询（warn）
	info.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) { return "SELECT slow", 1 }, nil)
	// Trace：错误
	info.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT bad", 0 }, errors.New("boom"))
	// Trace：record not found 不应记 error
	info.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT none", 0 }, gorm.ErrRecordNotFound)

	// Silent 级别：全部丢弃
	silent := base.LogMode(gormlogger.Silent).(*gormZapLogger)
	silent.Info(context.Background(), "x")
	silent.Warn(context.Background(), "x")
	silent.Error(context.Background(), "x")
	silent.Trace(context.Background(), time.Now(), func() (string, int64) { return "q", 0 }, nil)

	if recorded.Len() == 0 {
		t.Fatalf("期望至少产生一条日志")
	}
}

func TestOpen_UnsupportedDriver(t *testing.T) {
	if _, err := Open(config.Database{Driver: "mysql"}, zap.NewNop()); err == nil {
		t.Fatalf("不支持的驱动应返回错误")
	}
}

func TestOpen_ConnectionFailure(t *testing.T) {
	// 指向不可达端口：gorm.Open 的自动 ping 会失败 -> Open 返回错误。
	cfg := config.Database{
		Driver: config.DriverPostgres,
		Postgres: config.PostgresConfig{
			Host: "127.0.0.1", Port: 1, User: "x", Password: "x",
			DBName: "x", SSLMode: "disable", TimeZone: "UTC",
			MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetimeMinutes: 1,
		},
	}
	if _, err := Open(cfg, zap.NewNop()); err == nil {
		t.Fatalf("不可达数据库应返回错误")
	}
}

func TestOpen_SuccessWithoutPing(t *testing.T) {
	// 关闭自动探活后，gorm.Open 不会真正连接，Open 成功后应正确配置连接池。
	cfg := config.Database{
		Driver:      config.DriverPostgres,
		DisablePing: true,
		Postgres: config.PostgresConfig{
			Host: "127.0.0.1", Port: 1, User: "x", Password: "x",
			DBName: "x", SSLMode: "disable", TimeZone: "UTC",
			MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetimeMinutes: 5,
		},
	}
	db, err := Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("关闭探活后 Open 应成功: %v", err)
	}
	if db == nil {
		t.Fatalf("db 不应为 nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() 失败: %v", err)
	}
	if sqlDB.Stats().MaxOpenConnections != 7 {
		t.Fatalf("连接池上限应被设置为 7，实际 %d", sqlDB.Stats().MaxOpenConnections)
	}
	// 未建立真实连接 -> Ping 应失败（覆盖 Ping 的错误分支）
	if err := Ping(context.Background(), db); err == nil {
		t.Fatalf("未连接数据库 Ping 应失败")
	}
}

func TestPing_UnconnectedDB(t *testing.T) {
	// 构造一个不建立实际连接的 *gorm.DB（禁用自动 ping），验证 Ping 的错误分支。
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:        "host=127.0.0.1 port=1 user=x password=x dbname=x sslmode=disable",
			DriverName: "pgx",
		}),
		&gorm.Config{DisableAutomaticPing: true, Logger: gormlogger.Discard},
	)
	if err != nil {
		t.Fatalf("构造 gorm.DB 失败: %v", err)
	}
	if err := Ping(context.Background(), db); err == nil {
		t.Fatalf("未连接数据库 Ping 应返回错误")
	}
}

func TestNewGormZapLogger_NilFallback(t *testing.T) {
	l := newGormZapLogger(nil, 0)
	if l == nil {
		t.Fatalf("nil logger 应回退为 no-op")
	}
}
