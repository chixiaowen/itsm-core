package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfig(t, "auth:\n  jwt_secret: test-secret\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Database.Driver != DriverPostgres {
		t.Fatalf("默认驱动应为 postgres，实际 %s", cfg.Database.Driver)
	}
	if cfg.App.HTTPAddr != DefaultHTTPAddr {
		t.Fatalf("默认地址不符: %s", cfg.App.HTTPAddr)
	}
	if cfg.Database.Postgres.MaxIdleConns != 10 {
		t.Fatalf("默认连接池不符: %d", cfg.Database.Postgres.MaxIdleConns)
	}
	if cfg.App.MaxUploadMB != DefaultUploadMB {
		t.Fatalf("默认上传上限不符: %d", cfg.App.MaxUploadMB)
	}
	if cfg.UploadLimitBytes() != int64(DefaultUploadMB)*1024*1024 {
		t.Fatalf("UploadLimitBytes 不符")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("DB_DRIVER", "dameng")
	t.Setenv("JWT_SECRET", "env-secret")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ITSM_APP_HTTP_ADDR", ":9090")

	path := writeConfig(t, "auth:\n  jwt_secret: file-secret\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Database.Driver != DriverDameng {
		t.Fatalf("DB_DRIVER 应覆盖为 dameng，实际 %s", cfg.Database.Driver)
	}
	if cfg.Auth.JWTSecret != "env-secret" {
		t.Fatalf("JWT_SECRET 应覆盖，实际 %s", cfg.Auth.JWTSecret)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("LOG_LEVEL 应覆盖，实际 %s", cfg.Log.Level)
	}
	if cfg.App.HTTPAddr != ":9090" {
		t.Fatalf("ITSM_APP_HTTP_ADDR 应覆盖，实际 %s", cfg.App.HTTPAddr)
	}
}

func TestLoad_NoConfigFileUsesDefaults(t *testing.T) {
	// 未指定路径且搜索路径下无配置文件时，应回退到默认值（jwt_secret 有默认值）。
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("无配置文件时应可用默认值: %v", err)
	}
	if cfg.Database.Driver != DriverPostgres {
		t.Fatalf("应回退默认驱动")
	}
}

func TestLoad_MissingExplicitFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatalf("指定不存在的配置文件应报错")
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{JWTSecret: ""}, Database: Database{Driver: DriverPostgres}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("空 jwt_secret 应报错")
	}

	cfg2 := &Config{Auth: AuthConfig{JWTSecret: "s"}, Database: Database{Driver: "mysql"}}
	if err := cfg2.Validate(); err == nil {
		t.Fatalf("非法驱动应报错")
	}

	// 合法但缺省值回填
	cfg3 := &Config{Auth: AuthConfig{JWTSecret: "s"}, Database: Database{Driver: DriverDameng}}
	if err := cfg3.Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	if cfg3.App.HTTPAddr == "" || cfg3.Database.Postgres.MaxOpenConns == 0 {
		t.Fatalf("Validate 应回填默认值")
	}
}
