package database

import (
	"errors"

	"gorm.io/gorm"
)

// AutoMigrate 是通用迁移入口：仅转发到 gorm 的 AutoMigrate，不硬编码任何业务实体。
//
// 设计约定（为避免跨批次共享编辑点，支持业务模块并行开发）：
//   - 本函数保持通用，**不列出任何业务实体**；
//   - 每个业务域各自提供 `func Migrate(db *gorm.DB) error`（置于该域包内，
//     例如 internal/domain/platform/migrate.go），只迁移本域实体；
//   - 由 internal/bootstrap/bootstrap.go（批次 T11）按拓扑序调用各域 Migrate。
func AutoMigrate(db *gorm.DB, models ...any) error {
	if db == nil {
		return errors.New("database.AutoMigrate: db 为 nil")
	}
	if len(models) == 0 {
		return nil
	}
	return db.AutoMigrate(models...)
}
