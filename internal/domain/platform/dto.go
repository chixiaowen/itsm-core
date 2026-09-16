package platform

import "time"

// CreateUserRequest 是新建用户请求体。
type CreateUserRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	DisplayName string `json:"display_name" binding:"required,max=128"`
	Role        string `json:"role" binding:"required"`
	Password    string `json:"password" binding:"required,min=6,max=64"`
	Email       string `json:"email" binding:"omitempty,email,max=128"`
}

// UpdateUserRequest 是编辑用户请求体（字段均为可选）。
type UpdateUserRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=128"`
	Role        *string `json:"role" binding:"omitempty"`
	Password    *string `json:"password" binding:"omitempty,min=6,max=64"`
	Email       *string `json:"email" binding:"omitempty,email,max=128"`
	Status      *string `json:"status" binding:"omitempty,oneof=active disabled"`
}

// UserOption 是候选用户（下拉用）：仅暴露非敏感字段。
//
// 严禁在此结构体增加 password_hash / email / 手机号 等字段。
type UserOption struct {
	ID          uint64 `json:"id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// SLAPolicyRequest 是 SLA 策略新建/编辑请求体。
type SLAPolicyRequest struct {
	Name            string `json:"name" binding:"required,max=64"`
	Priority        string `json:"priority" binding:"required,oneof=P1 P2 P3 P4"`
	ResponseMinutes int    `json:"response_minutes" binding:"required,min=1,max=1000000"`
	ResolveMinutes  int    `json:"resolve_minutes" binding:"required,min=1,max=1000000"`
	PauseOnPending  bool   `json:"pause_on_pending"`
}

// CommentRequest 是新增评论请求体。
type CommentRequest struct {
	BizType    string `json:"biz_type" binding:"required,oneof=ticket incident problem change"`
	BizID      uint64 `json:"biz_id" binding:"required,min=1"`
	Content    string `json:"content" binding:"required,max=5000"`
	IsInternal bool   `json:"is_internal"`
}

// AuditLogResponse 是审计日志响应体（直接复用实体字段，此处仅作类型别名以便文档化）。
//
// 返回字段含 from_status / to_status，供前端详情页组装「状态流转时间线」。
type AuditLogResponse = AuditLog

// CommentResponse 是评论响应体。
type CommentResponse = Comment

// AttachmentResponse 是附件响应体。
type AttachmentResponse = Attachment

// UserResponse 是用户响应体。
type UserResponse = User

// TimeNow 便于测试替换的时间源类型。
type TimeNow = func() time.Time
