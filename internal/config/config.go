// Package config 负责加载、默认值填充与校验应用配置。
//
// 配置优先级：环境变量 > 配置文件 > 内置默认值。
// 支持两类环境变量：
//   - ITSM_ 前缀 + `.` -> `_`（如 ITSM_DATABASE_DRIVER）；
//   - 直读常用变量：DB_DRIVER / DB_DSN / JWT_SECRET / LOG_LEVEL（优先级最高）。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// 支持的数据库驱动。
const (
	DriverPostgres = "postgres"
	DriverDameng   = "dameng"
)

// 运行环境。
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// 分页默认值（供 httpx 与配置共享口径）。
const (
	DefaultHTTPAddr = ":8080"
	DefaultUploadMB = 20
)

// Config 是全量应用配置。
type Config struct {
	App      AppConfig    `mapstructure:"app"`
	Log      LogConfig    `mapstructure:"log"`
	Database Database     `mapstructure:"database"`
	Auth     AuthConfig   `mapstructure:"auth"`
	Change   ChangeConfig `mapstructure:"change"`
	CORS     CORSConfig   `mapstructure:"cors"`
}

// AppConfig 应用级配置。
type AppConfig struct {
	Name                  string `mapstructure:"name"`
	Env                   string `mapstructure:"env"`
	HTTPAddr              string `mapstructure:"http_addr"`
	RequestTimeoutSeconds int    `mapstructure:"request_timeout_seconds"`
	MaxUploadMB           int    `mapstructure:"max_upload_mb"`
	UploadDir             string `mapstructure:"upload_dir"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// Database 数据库配置（支持 postgres / dameng 双库切换）。
type Database struct {
	Driver      string         `mapstructure:"driver"`
	DSN         string         `mapstructure:"dsn"`
	Postgres    PostgresConfig `mapstructure:"postgres"`
	Dameng      DamengConfig   `mapstructure:"dameng"`
	AutoMigrate bool           `mapstructure:"auto_migrate"`
	Seed        bool           `mapstructure:"seed"`
	// DisablePing 关闭 gorm.Open 时的自动探活（受限网络/启动即降级场景使用）。
	DisablePing bool `mapstructure:"disable_ping"`
}

// PostgresConfig PostgreSQL 连接配置。连接池参数对两种驱动通用。
type PostgresConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	User                   string `mapstructure:"user"`
	Password               string `mapstructure:"password"`
	DBName                 string `mapstructure:"dbname"`
	SSLMode                string `mapstructure:"sslmode"`
	TimeZone               string `mapstructure:"timezone"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `mapstructure:"conn_max_lifetime_minutes"`
}

// DamengConfig 达梦 DM8 连接配置。
type DamengConfig struct {
	Host              string `mapstructure:"host"`
	Port              int    `mapstructure:"port"`
	User              string `mapstructure:"user"`
	Password          string `mapstructure:"password"`
	Schema            string `mapstructure:"schema"`
	VarcharSizeIsChar bool   `mapstructure:"varchar_size_is_char"`
}

// AuthConfig 认证配置。
type AuthConfig struct {
	JWTSecret        string `mapstructure:"jwt_secret"`
	JWTTTLHours      int    `mapstructure:"jwt_ttl_hours"`
	LoginMaxAttempts int    `mapstructure:"login_max_attempts"`
	LoginLockMinutes int    `mapstructure:"login_lock_minutes"`
}

// ChangeConfig 变更模块配置。
type ChangeConfig struct {
	EnforceWindow         bool `mapstructure:"enforce_window"`
	EmergencyNeedsConfirm bool `mapstructure:"emergency_needs_confirm"`
}

// CORSConfig 跨域配置。
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// Load 加载配置。path 为空时按默认搜索路径（./configs、/etc/itsm-core）查找。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	// ITSM_ 前缀 + `.` -> `_`（如 ITSM_DATABASE_DRIVER）。
	v.SetEnvPrefix("ITSM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/itsm-core")
	}

	setDefaults(v)
	bindEnvs(v)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) || path != "" {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		// 未提供配置文件时，允许仅使用默认值 + 环境变量。
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	applyDirectEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// setDefaults 为所有已知键设置默认值，使 Unmarshal 能感知嵌套键。
func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "itsm-core")
	v.SetDefault("app.env", EnvDevelopment)
	v.SetDefault("app.http_addr", DefaultHTTPAddr)
	v.SetDefault("app.request_timeout_seconds", 30)
	v.SetDefault("app.max_upload_mb", DefaultUploadMB)
	v.SetDefault("app.upload_dir", "./data/uploads")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.output", "stdout")

	v.SetDefault("database.driver", DriverPostgres)
	v.SetDefault("database.dsn", "")
	v.SetDefault("database.auto_migrate", true)
	v.SetDefault("database.seed", true)

	v.SetDefault("database.postgres.host", "127.0.0.1")
	v.SetDefault("database.postgres.port", 5432)
	v.SetDefault("database.postgres.user", "itsm")
	v.SetDefault("database.postgres.password", "itsm")
	v.SetDefault("database.postgres.dbname", "itsm_core")
	v.SetDefault("database.postgres.sslmode", "disable")
	v.SetDefault("database.postgres.timezone", "UTC")
	v.SetDefault("database.postgres.max_open_conns", 50)
	v.SetDefault("database.postgres.max_idle_conns", 10)
	v.SetDefault("database.postgres.conn_max_lifetime_minutes", 60)

	v.SetDefault("database.dameng.host", "127.0.0.1")
	v.SetDefault("database.dameng.port", 5236)
	v.SetDefault("database.dameng.user", "SYSDBA")
	v.SetDefault("database.dameng.password", "SYSDBA")
	v.SetDefault("database.dameng.schema", "SYSDBA")
	v.SetDefault("database.dameng.varchar_size_is_char", true)

	v.SetDefault("auth.jwt_secret", "change-me-in-production")
	v.SetDefault("auth.jwt_ttl_hours", 24)
	v.SetDefault("auth.login_max_attempts", 5)
	v.SetDefault("auth.login_lock_minutes", 10)

	v.SetDefault("change.enforce_window", false)
	v.SetDefault("change.emergency_needs_confirm", true)

	v.SetDefault("cors.allowed_origins", []string{"http://localhost:5173"})
}

// bindEnvs 显式绑定环境变量，保证嵌套键可被环境变量覆盖。
func bindEnvs(v *viper.Viper) {
	// 直读常用变量（优先级最高，显式命名；首个存在的变量生效）。
	_ = v.BindEnv("database.driver", "DB_DRIVER", "ITSM_DATABASE_DRIVER")
	_ = v.BindEnv("database.dsn", "DB_DSN", "ITSM_DATABASE_DSN")
	_ = v.BindEnv("auth.jwt_secret", "JWT_SECRET", "ITSM_AUTH_JWT_SECRET")
	_ = v.BindEnv("log.level", "LOG_LEVEL", "ITSM_LOG_LEVEL")

	// ITSM_ 前缀键（BindEnv 单参数时按 ITSM_ 前缀 + `.`->`_` 映射）。
	keys := []string{
		"app.name", "app.env", "app.http_addr", "app.request_timeout_seconds",
		"app.max_upload_mb", "app.upload_dir",
		"log.format", "log.output",
		"database.auto_migrate", "database.seed", "database.disable_ping",
		"database.postgres.host", "database.postgres.port", "database.postgres.user",
		"database.postgres.password", "database.postgres.dbname", "database.postgres.sslmode",
		"database.postgres.timezone", "database.postgres.max_open_conns",
		"database.postgres.max_idle_conns", "database.postgres.conn_max_lifetime_minutes",
		"database.dameng.host", "database.dameng.port", "database.dameng.user",
		"database.dameng.password", "database.dameng.schema", "database.dameng.varchar_size_is_char",
		"auth.jwt_ttl_hours", "auth.login_max_attempts", "auth.login_lock_minutes",
		"change.enforce_window", "change.emergency_needs_confirm",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
}

// applyDirectEnv 兜底：再次直读四个常用变量，确保其优先级与生效性。
func applyDirectEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("DB_DRIVER")); v != "" {
		cfg.Database.Driver = v
	}
	if v := strings.TrimSpace(os.Getenv("DB_DSN")); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		cfg.Log.Level = v
	}
}

// Validate 校验关键配置并回填安全默认值。
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return errors.New("配置校验失败: auth.jwt_secret（JWT_SECRET）不得为空")
	}
	switch c.Database.Driver {
	case DriverPostgres, DriverDameng:
	default:
		return fmt.Errorf("配置校验失败: database.driver 非法值 %q（可选 postgres | dameng）", c.Database.Driver)
	}

	if strings.TrimSpace(c.App.HTTPAddr) == "" {
		c.App.HTTPAddr = DefaultHTTPAddr
	}
	if c.App.MaxUploadMB <= 0 {
		c.App.MaxUploadMB = DefaultUploadMB
	}
	if strings.TrimSpace(c.App.UploadDir) == "" {
		c.App.UploadDir = "./data/uploads"
	}
	if c.App.RequestTimeoutSeconds <= 0 {
		c.App.RequestTimeoutSeconds = 30
	}
	if c.Database.Postgres.MaxOpenConns <= 0 {
		c.Database.Postgres.MaxOpenConns = 50
	}
	if c.Database.Postgres.MaxIdleConns <= 0 {
		c.Database.Postgres.MaxIdleConns = 10
	}
	if c.Database.Postgres.ConnMaxLifetimeMinutes <= 0 {
		c.Database.Postgres.ConnMaxLifetimeMinutes = 60
	}
	if c.Auth.JWTTTLHours <= 0 {
		c.Auth.JWTTTLHours = 24
	}
	return nil
}

// UploadLimitBytes 返回附件上传字节上限。
func (c *Config) UploadLimitBytes() int64 {
	return int64(c.App.MaxUploadMB) * 1024 * 1024
}
