package platform

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// 本文件是 platform 域唯一允许直接使用 *gorm.DB 的文件之一（另一个是 migrate.go）。

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// gormUserRepository 是 UserRepository 的 GORM 实现。
type gormUserRepository struct{ db *gorm.DB }

// NewUserRepository 构造用户仓储。
func NewUserRepository(db *gorm.DB) UserRepository { return &gormUserRepository{db: db} }

func (r *gormUserRepository) Create(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *gormUserRepository) Update(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Save(u).Error
}

func (r *gormUserRepository) GetByID(ctx context.Context, id uint64) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &u, nil
}

func (r *gormUserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &u, nil
}

func (r *gormUserRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&User{}, id).Error
}

func (r *gormUserRepository) List(ctx context.Context, q UserListQuery) ([]User, int64, error) {
	tx := r.db.WithContext(ctx).Model(&User{})
	if q.Role != "" {
		tx = tx.Where("role = ?", q.Role)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		// 双库约束：不使用 ILIKE；在应用层转小写后用 LIKE 匹配。
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("username LIKE ? OR display_name LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []User
	if err := tx.Order("id DESC").Limit(q.Limit).Offset(q.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListOptions 返回候选用户下拉项：仅 active 且未软删除。
//
// 双库约束：只 SELECT 三个非敏感列（id/display_name/role），
// 从 SQL 层杜绝 password_hash / email 外泄。role 为空表示不过滤。
func (r *gormUserRepository) ListOptions(ctx context.Context, role string) ([]UserOption, error) {
	tx := r.db.WithContext(ctx).Model(&User{}).Where("status = ?", UserStatusActive)
	if role != "" {
		tx = tx.Where("role = ?", role)
	}
	opts := make([]UserOption, 0)
	if err := tx.Select("id, display_name, role").Order("id ASC").Find(&opts).Error; err != nil {
		return nil, err
	}
	return opts, nil
}

// gormSLAPolicyRepository 是 SLAPolicyRepository 的 GORM 实现。
type gormSLAPolicyRepository struct{ db *gorm.DB }

// NewSLAPolicyRepository 构造 SLA 策略仓储。
func NewSLAPolicyRepository(db *gorm.DB) SLAPolicyRepository {
	return &gormSLAPolicyRepository{db: db}
}

func (r *gormSLAPolicyRepository) Create(ctx context.Context, p *SLAPolicy) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *gormSLAPolicyRepository) Update(ctx context.Context, p *SLAPolicy) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *gormSLAPolicyRepository) GetByID(ctx context.Context, id uint64) (*SLAPolicy, error) {
	var p SLAPolicy
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &p, nil
}

func (r *gormSLAPolicyRepository) GetByPriority(ctx context.Context, priority string) (*SLAPolicy, error) {
	var p SLAPolicy
	if err := r.db.WithContext(ctx).Where("priority = ?", priority).First(&p).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &p, nil
}

func (r *gormSLAPolicyRepository) List(ctx context.Context) ([]SLAPolicy, error) {
	var items []SLAPolicy
	if err := r.db.WithContext(ctx).Order("priority ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormSLAPolicyRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&SLAPolicy{}, id).Error
}

// gormAuditRepository 是 AuditRepository 的 GORM 实现（只追加，不软删除）。
type gormAuditRepository struct{ db *gorm.DB }

// NewAuditRepository 构造审计仓储。
func NewAuditRepository(db *gorm.DB) AuditRepository { return &gormAuditRepository{db: db} }

func (r *gormAuditRepository) Append(ctx context.Context, entry *AuditLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *gormAuditRepository) List(ctx context.Context, q AuditListQuery) ([]AuditLog, int64, error) {
	tx := r.db.WithContext(ctx).Model(&AuditLog{})
	if q.ActorID != 0 {
		tx = tx.Where("actor_id = ?", q.ActorID)
	}
	// biz_type/biz_id 与 entity_type/entity_id 归一到同一组列。
	bizType, bizID := q.auditBizFilter()
	if bizType != "" {
		tx = tx.Where("biz_type = ?", bizType)
	}
	if bizID != 0 {
		tx = tx.Where("biz_id = ?", bizID)
	}
	if q.Action != "" {
		tx = tx.Where("action = ?", q.Action)
	}
	if q.From != nil {
		tx = tx.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		tx = tx.Where("created_at <= ?", *q.To)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []AuditLog
	if err := tx.Order("id DESC").Limit(q.Limit).Offset(q.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// gormCommentRepository 是 CommentRepository 的 GORM 实现。
type gormCommentRepository struct{ db *gorm.DB }

// NewCommentRepository 构造评论仓储。
func NewCommentRepository(db *gorm.DB) CommentRepository { return &gormCommentRepository{db: db} }

func (r *gormCommentRepository) Create(ctx context.Context, c *Comment) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormCommentRepository) ListByBiz(ctx context.Context, bizType string, bizID uint64, includeInternal bool) ([]Comment, error) {
	tx := r.db.WithContext(ctx).Model(&Comment{}).Where("biz_type = ? AND biz_id = ?", bizType, bizID)
	if !includeInternal {
		tx = tx.Where("is_internal = ?", false)
	}
	var items []Comment
	if err := tx.Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// gormAttachmentRepository 是 AttachmentRepository 的 GORM 实现。
type gormAttachmentRepository struct{ db *gorm.DB }

// NewAttachmentRepository 构造附件仓储。
func NewAttachmentRepository(db *gorm.DB) AttachmentRepository {
	return &gormAttachmentRepository{db: db}
}

func (r *gormAttachmentRepository) Create(ctx context.Context, a *Attachment) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormAttachmentRepository) GetByID(ctx context.Context, id uint64) (*Attachment, error) {
	var a Attachment
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &a, nil
}

func (r *gormAttachmentRepository) ListByBiz(ctx context.Context, bizType string, bizID uint64) ([]Attachment, error) {
	var items []Attachment
	if err := r.db.WithContext(ctx).Where("biz_type = ? AND biz_id = ?", bizType, bizID).
		Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormAttachmentRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Attachment{}, id).Error
}
