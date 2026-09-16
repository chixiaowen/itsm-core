package database

import (
	"context"
	"strings"
	"testing"

	"github.com/godoes/gorm-dameng"
	"gorm.io/driver/postgres"

	"github.com/chixiaowen/itsm-core/internal/config"
)

// TestDialector_Selection 覆盖 Dialector 选择分支（不连库，仅构造并断言类型）。
func TestDialector_Selection(t *testing.T) {
	t.Run("postgres", func(t *testing.T) {
		cfg := config.Database{
			Driver: config.DriverPostgres,
			Postgres: config.PostgresConfig{
				Host: "127.0.0.1", Port: 5432, User: "itsm",
				Password: "itsm", DBName: "itsm_core", SSLMode: "disable", TimeZone: "UTC",
			},
		}
		d, err := Dialector(cfg)
		if err != nil {
			t.Fatalf("Dialector(postgres) 返回错误: %v", err)
		}
		if _, ok := d.(*postgres.Dialector); !ok {
			t.Fatalf("期望 *postgres.Dialector，实际 %T", d)
		}
	})

	t.Run("dameng", func(t *testing.T) {
		cfg := config.Database{
			Driver: config.DriverDameng,
			Dameng: config.DamengConfig{
				Host: "127.0.0.1", Port: 5236, User: "SYSDBA",
				Password: "SYSDBA", Schema: "SYSDBA", VarcharSizeIsChar: true,
			},
		}
		d, err := Dialector(cfg)
		if err != nil {
			t.Fatalf("Dialector(dameng) 返回错误: %v", err)
		}
		dm, ok := d.(*dameng.Dialector)
		if !ok {
			t.Fatalf("期望 *dameng.Dialector，实际 %T", d)
		}
		if !dm.VarcharSizeIsCharLength {
			t.Fatalf("期望 VarcharSizeIsCharLength=true")
		}
		if dm.DriverName != dameng.DriverName {
			t.Fatalf("期望 DriverName=%q，实际 %q", dameng.DriverName, dm.DriverName)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		if _, err := Dialector(config.Database{Driver: "mysql"}); err == nil {
			t.Fatalf("期望不支持的驱动返回错误")
		}
	})
}

// TestBuildDSN 覆盖 DSN 拼接与显式 DSN 覆盖。
func TestBuildDSN(t *testing.T) {
	t.Run("postgres-default", func(t *testing.T) {
		dsn := buildPostgresDSN(config.Database{Postgres: config.PostgresConfig{
			Host: "db", Port: 5433, User: "u", Password: "p", DBName: "d", SSLMode: "require", TimeZone: "UTC",
		}})
		for _, want := range []string{"host=db", "port=5433", "user=u", "dbname=d", "sslmode=require", "TimeZone=UTC"} {
			if !strings.Contains(dsn, want) {
				t.Fatalf("DSN %q 缺少 %q", dsn, want)
			}
		}
	})

	t.Run("postgres-explicit-dsn", func(t *testing.T) {
		dsn := buildPostgresDSN(config.Database{DSN: "postgres://x"})
		if dsn != "postgres://x" {
			t.Fatalf("期望显式 DSN 生效，实际 %q", dsn)
		}
	})

	t.Run("dameng-default", func(t *testing.T) {
		dsn := buildDamengDSN(config.Database{Dameng: config.DamengConfig{
			Host: "127.0.0.1", Port: 5236, User: "SYSDBA", Password: "SYSDBA", Schema: "SYSDBA",
		}})
		if !strings.HasPrefix(dsn, "dm://") {
			t.Fatalf("达梦 DSN 应以 dm:// 开头，实际 %q", dsn)
		}
		if !strings.Contains(dsn, "schema=SYSDBA") {
			t.Fatalf("达梦 DSN 应含 schema，实际 %q", dsn)
		}
	})

	t.Run("dameng-explicit-dsn", func(t *testing.T) {
		if got := buildDamengDSN(config.Database{DSN: "dm://custom"}); got != "dm://custom" {
			t.Fatalf("期望显式 DSN 生效，实际 %q", got)
		}
	})
}

// TestAutoMigrate_NilDB 覆盖 nil 与空模型边界。
func TestAutoMigrate_NilDB(t *testing.T) {
	if err := AutoMigrate(nil); err == nil {
		t.Fatalf("nil db 应返回错误")
	}
}

// TestPing_NilDB 覆盖 Ping 的空 db 分支。
func TestPing_NilDB(t *testing.T) {
	if err := Ping(context.Background(), nil); err == nil {
		t.Fatalf("nil db 的 Ping 应返回错误")
	}
}

// TestNewGormZapLogger 覆盖日志适配器构造分支。
func TestNewGormZapLogger(t *testing.T) {
	if l := newGormZapLogger(nil, 0); l == nil {
		t.Fatalf("logger 不应为 nil")
	}
	if l := newGormZapLogger(nil, defaultSlowThreshold); l.LogMode(0) == nil {
		t.Fatalf("LogMode 不应返回 nil")
	}
}
