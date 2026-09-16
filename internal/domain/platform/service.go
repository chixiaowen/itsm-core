package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

const (
	defaultMaxUploadBytes int64 = 20 * 1024 * 1024 // 20MB
	defaultUploadDir            = "./data/uploads"
)

// Operator 是执行操作的当前用户（含审计所需 IP）。
type Operator struct {
	ID       uint64
	Role     string
	ClientIP string
}

// Deps 是 platform service 的依赖集合。
type Deps struct {
	Users          UserRepository
	SLAPolicies    SLAPolicyRepository
	Audits         AuditRepository
	Comments       CommentRepository
	Attachments    AttachmentRepository
	Logger         *zap.Logger
	MaxUploadBytes int64
	UploadDir      string
	Now            func() time.Time
}

// Service 承载用户/角色、SLA 策略、审计、评论、附件业务。
type Service struct {
	users          UserRepository
	slaPolicies    SLAPolicyRepository
	audits         AuditRepository
	comments       CommentRepository
	attachments    AttachmentRepository
	log            *zap.Logger
	maxUploadBytes int64
	uploadDir      string
	now            func() time.Time
}

// NewService 构造 platform service，未提供的可选依赖会回退到安全默认值。
func NewService(d Deps) *Service {
	s := &Service{
		users:          d.Users,
		slaPolicies:    d.SLAPolicies,
		audits:         d.Audits,
		comments:       d.Comments,
		attachments:    d.Attachments,
		log:            d.Logger,
		maxUploadBytes: d.MaxUploadBytes,
		uploadDir:      d.UploadDir,
		now:            d.Now,
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	if s.maxUploadBytes <= 0 {
		s.maxUploadBytes = defaultMaxUploadBytes
	}
	if strings.TrimSpace(s.uploadDir) == "" {
		s.uploadDir = defaultUploadDir
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// MaxUploadBytes 返回附件上传字节上限。
func (s *Service) MaxUploadBytes() int64 { return s.maxUploadBytes }

// ------------------------- 用户 -------------------------

// ListUsers 分页查询用户。
func (s *Service) ListUsers(ctx context.Context, q UserListQuery) ([]User, int64, error) {
	if s.users == nil {
		return nil, 0, httpx.ErrInternal("用户仓储未初始化")
	}
	return s.users.List(ctx, q)
}

// ListUserOptions 返回候选用户下拉项（任意登录用户可用）。
//
// 仅返回 active 且未软删除的用户，且只含 id/display_name/role 三个非敏感字段；
// role 为空表示不按角色过滤，否则按精确角色过滤。
func (s *Service) ListUserOptions(ctx context.Context, role string) ([]UserOption, error) {
	if s.users == nil {
		return nil, httpx.ErrInternal("用户仓储未初始化")
	}
	opts, err := s.users.ListOptions(ctx, role)
	if err != nil {
		return nil, httpx.ErrInternal("查询候选用户失败: " + err.Error())
	}
	if opts == nil {
		opts = make([]UserOption, 0)
	}
	return opts, nil
}

// GetUser 按 ID 查询用户。
func (s *Service) GetUser(ctx context.Context, id uint64) (*User, error) {
	if s.users == nil {
		return nil, httpx.ErrInternal("用户仓储未初始化")
	}
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "用户不存在")
	}
	return u, nil
}

// CreateUser 新建用户（密码 bcrypt 存储）。
func (s *Service) CreateUser(ctx context.Context, actorID uint64, req CreateUserRequest) (*User, error) {
	if s.users == nil {
		return nil, httpx.ErrInternal("用户仓储未初始化")
	}
	if !role.Valid(req.Role) {
		return nil, httpx.ErrBadRequest("非法角色: " + req.Role)
	}
	if _, err := s.users.GetByUsername(ctx, req.Username); err == nil {
		return nil, httpx.ErrConflict("用户名已存在: " + req.Username)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrInternal(err.Error())
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, httpx.ErrInternal("密码哈希失败: " + err.Error())
	}

	now := s.now()
	u := &User{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		Role:         req.Role,
		PasswordHash: hash,
		Email:        req.Email,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, httpx.ErrConflict("创建用户失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "create", "user", u.ID, "", "", nil, u)
	return u, nil
}

// UpdateUser 编辑用户（含角色/密码/状态）。
func (s *Service) UpdateUser(ctx context.Context, actorID, id uint64, req UpdateUserRequest) (*User, error) {
	if s.users == nil {
		return nil, httpx.ErrInternal("用户仓储未初始化")
	}
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "用户不存在")
	}
	before := *u

	if req.DisplayName != nil {
		if strings.TrimSpace(*req.DisplayName) == "" {
			return nil, httpx.ErrBadRequest("展示名不得为空")
		}
		u.DisplayName = *req.DisplayName
	}
	if req.Email != nil {
		u.Email = *req.Email
	}
	if req.Role != nil {
		if !role.Valid(*req.Role) {
			return nil, httpx.ErrBadRequest("非法角色: " + *req.Role)
		}
		u.Role = *req.Role
	}
	if req.Status != nil {
		if *req.Status != UserStatusActive && *req.Status != UserStatusDisabled {
			return nil, httpx.ErrBadRequest("非法状态: " + *req.Status)
		}
		u.Status = *req.Status
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := security.HashPassword(*req.Password)
		if err != nil {
			return nil, httpx.ErrInternal("密码哈希失败: " + err.Error())
		}
		u.PasswordHash = hash
	}
	u.UpdatedAt = s.now()

	if err := s.users.Update(ctx, u); err != nil {
		return nil, httpx.ErrInternal("更新用户失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "update", "user", u.ID, "", "", before, u)
	return u, nil
}

// DeleteUser 软删除用户。
func (s *Service) DeleteUser(ctx context.Context, actorID, id uint64) error {
	if s.users == nil {
		return httpx.ErrInternal("用户仓储未初始化")
	}
	if actorID == id {
		return httpx.ErrPrecondition("不能删除当前登录用户")
	}
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return mapRepoErr(err, "用户不存在")
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除用户失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "delete", "user", id, "", "", u, nil)
	return nil
}

// Roles 返回角色与权限点矩阵。
func (s *Service) Roles() []role.RoleInfo { return role.AllRoles() }

// ------------------------- SLA 策略 -------------------------

// ListSLAPolicies 返回全部 SLA 策略。
func (s *Service) ListSLAPolicies(ctx context.Context) ([]SLAPolicy, error) {
	if s.slaPolicies == nil {
		return nil, httpx.ErrInternal("SLA 仓储未初始化")
	}
	return s.slaPolicies.List(ctx)
}

// GetSLAPolicy 按 ID 查询 SLA 策略。
func (s *Service) GetSLAPolicy(ctx context.Context, id uint64) (*SLAPolicy, error) {
	if s.slaPolicies == nil {
		return nil, httpx.ErrInternal("SLA 仓储未初始化")
	}
	p, err := s.slaPolicies.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "SLA 策略不存在")
	}
	return p, nil
}

// CreateSLAPolicy 新建 SLA 策略（(priority) 唯一：先查后写，禁用 ON CONFLICT）。
func (s *Service) CreateSLAPolicy(ctx context.Context, actorID uint64, req SLAPolicyRequest) (*SLAPolicy, error) {
	if s.slaPolicies == nil {
		return nil, httpx.ErrInternal("SLA 仓储未初始化")
	}
	if _, err := s.slaPolicies.GetByPriority(ctx, req.Priority); err == nil {
		return nil, httpx.ErrConflict("该优先级已存在 SLA 策略: " + req.Priority)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrInternal(err.Error())
	}

	now := s.now()
	p := &SLAPolicy{
		Name:            req.Name,
		Priority:        req.Priority,
		ResponseMinutes: req.ResponseMinutes,
		ResolveMinutes:  req.ResolveMinutes,
		PauseOnPending:  req.PauseOnPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.slaPolicies.Create(ctx, p); err != nil {
		return nil, httpx.ErrConflict("创建 SLA 策略失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "create", "sla_policy", p.ID, "", "", nil, p)
	return p, nil
}

// UpdateSLAPolicy 编辑 SLA 策略。
func (s *Service) UpdateSLAPolicy(ctx context.Context, actorID, id uint64, req SLAPolicyRequest) (*SLAPolicy, error) {
	if s.slaPolicies == nil {
		return nil, httpx.ErrInternal("SLA 仓储未初始化")
	}
	p, err := s.slaPolicies.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "SLA 策略不存在")
	}
	before := *p

	if req.Priority != p.Priority {
		if _, err := s.slaPolicies.GetByPriority(ctx, req.Priority); err == nil {
			return nil, httpx.ErrConflict("该优先级已存在 SLA 策略: " + req.Priority)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrInternal(err.Error())
		}
	}
	p.Name = req.Name
	p.Priority = req.Priority
	p.ResponseMinutes = req.ResponseMinutes
	p.ResolveMinutes = req.ResolveMinutes
	p.PauseOnPending = req.PauseOnPending
	p.UpdatedAt = s.now()

	if err := s.slaPolicies.Update(ctx, p); err != nil {
		return nil, httpx.ErrInternal("更新 SLA 策略失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "update", "sla_policy", p.ID, "", "", before, p)
	return p, nil
}

// DeleteSLAPolicy 软删除 SLA 策略。
func (s *Service) DeleteSLAPolicy(ctx context.Context, actorID, id uint64) error {
	if s.slaPolicies == nil {
		return httpx.ErrInternal("SLA 仓储未初始化")
	}
	p, err := s.slaPolicies.GetByID(ctx, id)
	if err != nil {
		return mapRepoErr(err, "SLA 策略不存在")
	}
	if err := s.slaPolicies.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除 SLA 策略失败: " + err.Error())
	}
	s.audit(ctx, Operator{ID: actorID}, "delete", "sla_policy", id, "", "", p, nil)
	return nil
}

// ------------------------- 审计 -------------------------

// AuditEntry 是审计写入入参（供其它业务域调用）。
type AuditEntry struct {
	ActorID     uint64
	Action      string
	BizType     string
	BizID       uint64
	FromStatus  string
	ToStatus    string
	BeforeValue string
	AfterValue  string
	ClientIP    string
}

// AppendAudit 追加一条审计日志（供其它业务域在写操作/状态流转时调用）。
func (s *Service) AppendAudit(ctx context.Context, e AuditEntry) error {
	if s.audits == nil {
		return nil
	}
	entry := &AuditLog{
		ActorID:     e.ActorID,
		Action:      e.Action,
		BizType:     e.BizType,
		BizID:       e.BizID,
		FromStatus:  e.FromStatus,
		ToStatus:    e.ToStatus,
		BeforeValue: e.BeforeValue,
		AfterValue:  e.AfterValue,
		ClientIP:    e.ClientIP,
		CreatedAt:   s.now(),
	}
	return s.audits.Append(ctx, entry)
}

// ListAuditLogs 分页查询审计日志。
func (s *Service) ListAuditLogs(ctx context.Context, q AuditListQuery) ([]AuditLog, int64, error) {
	if s.audits == nil {
		return nil, 0, httpx.ErrInternal("审计仓储未初始化")
	}
	return s.audits.List(ctx, q)
}

// audit 是内部审计helper；审计失败只告警，不影响主流程。
func (s *Service) audit(ctx context.Context, op Operator, action, bizType string, bizID uint64, from, to string, before, after any) {
	if s.audits == nil {
		return
	}
	entry := &AuditLog{
		ActorID:     op.ID,
		Action:      action,
		BizType:     bizType,
		BizID:       bizID,
		FromStatus:  from,
		ToStatus:    to,
		BeforeValue: toJSON(before),
		AfterValue:  toJSON(after),
		ClientIP:    op.ClientIP,
		CreatedAt:   s.now(),
	}
	if err := s.audits.Append(ctx, entry); err != nil {
		s.log.Warn("写入审计日志失败", zap.String("biz_type", bizType), zap.Uint64("biz_id", bizID), zap.Error(err))
	}
}

// ------------------------- 评论 -------------------------

// ListComments 按业务对象查询评论。
func (s *Service) ListComments(ctx context.Context, bizType string, bizID uint64, includeInternal bool) ([]Comment, error) {
	if s.comments == nil {
		return nil, httpx.ErrInternal("评论仓储未初始化")
	}
	if !validBizType(bizType) {
		return nil, httpx.ErrBadRequest("非法 biz_type: " + bizType)
	}
	return s.comments.ListByBiz(ctx, bizType, bizID, includeInternal)
}

// CreateComment 新增评论/内部备注。
func (s *Service) CreateComment(ctx context.Context, op Operator, req CommentRequest) (*Comment, error) {
	if s.comments == nil {
		return nil, httpx.ErrInternal("评论仓储未初始化")
	}
	if !validBizType(req.BizType) {
		return nil, httpx.ErrBadRequest("非法 biz_type: " + req.BizType)
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, httpx.ErrBadRequest("评论内容不得为空")
	}

	now := s.now()
	c := &Comment{
		BizType:    req.BizType,
		BizID:      req.BizID,
		AuthorID:   op.ID,
		Content:    req.Content,
		IsInternal: req.IsInternal,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.comments.Create(ctx, c); err != nil {
		return nil, httpx.ErrInternal("创建评论失败: " + err.Error())
	}
	s.audit(ctx, op, "create", req.BizType, req.BizID, "", "", nil, map[string]any{"comment_id": c.ID})
	return c, nil
}

// ------------------------- 附件 -------------------------

// ListAttachments 按业务对象查询附件。
func (s *Service) ListAttachments(ctx context.Context, bizType string, bizID uint64) ([]Attachment, error) {
	if s.attachments == nil {
		return nil, httpx.ErrInternal("附件仓储未初始化")
	}
	return s.attachments.ListByBiz(ctx, bizType, bizID)
}

// GetAttachment 按 ID 查询附件（用于下载，含软删除实体拦截）。
func (s *Service) GetAttachment(ctx context.Context, id uint64) (*Attachment, error) {
	if s.attachments == nil {
		return nil, httpx.ErrInternal("附件仓储未初始化")
	}
	a, err := s.attachments.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "附件不存在")
	}
	return a, nil
}

// SaveAttachment 落盘并登记附件（上限 maxUploadBytes）。
func (s *Service) SaveAttachment(ctx context.Context, op Operator, bizType string, bizID uint64,
	filename, mimeType string, size int64, r io.Reader) (*Attachment, error) {

	if s.attachments == nil {
		return nil, httpx.ErrInternal("附件仓储未初始化")
	}
	if !validBizType(bizType) {
		return nil, httpx.ErrBadRequest("非法 biz_type: " + bizType)
	}
	if bizID == 0 {
		return nil, httpx.ErrBadRequest("biz_id 必填")
	}
	if strings.TrimSpace(filename) == "" {
		return nil, httpx.ErrBadRequest("文件名必填")
	}
	if size <= 0 {
		return nil, httpx.ErrBadRequest("附件内容为空")
	}
	if size > s.maxUploadBytes {
		return nil, httpx.ErrBadRequest(fmt.Sprintf("附件超过上限 %d MB", s.maxUploadBytes/(1024*1024)))
	}
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, httpx.ErrInternal("创建上传目录失败: " + err.Error())
	}

	stored := filepath.Join(s.uploadDir, uuid.NewString()+filepath.Ext(filename))
	f, err := os.Create(stored)
	if err != nil {
		return nil, httpx.ErrInternal("创建文件失败: " + err.Error())
	}
	written, copyErr := io.Copy(f, io.LimitReader(r, s.maxUploadBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(stored)
		return nil, httpx.ErrInternal("写入文件失败: " + copyErr.Error())
	}
	if closeErr != nil {
		_ = os.Remove(stored)
		return nil, httpx.ErrInternal("关闭文件失败: " + closeErr.Error())
	}
	if written > s.maxUploadBytes {
		_ = os.Remove(stored)
		return nil, httpx.ErrBadRequest(fmt.Sprintf("附件超过上限 %d MB", s.maxUploadBytes/(1024*1024)))
	}

	now := s.now()
	a := &Attachment{
		BizType:    bizType,
		BizID:      bizID,
		Filename:   filepath.Base(filename),
		FilePath:   stored,
		Size:       written,
		MimeType:   mimeType,
		UploaderID: op.ID,
		CreatedAt:  now,
	}
	if err := s.attachments.Create(ctx, a); err != nil {
		_ = os.Remove(stored)
		return nil, httpx.ErrInternal("保存附件失败: " + err.Error())
	}
	s.audit(ctx, op, "create", bizType, bizID, "", "", nil, map[string]any{"attachment_id": a.ID})
	return a, nil
}

// DeleteAttachment 删除附件（仅上传者或 admin）。
func (s *Service) DeleteAttachment(ctx context.Context, op Operator, id uint64) error {
	if s.attachments == nil {
		return httpx.ErrInternal("附件仓储未初始化")
	}
	a, err := s.attachments.GetByID(ctx, id)
	if err != nil {
		return mapRepoErr(err, "附件不存在")
	}
	if a.UploaderID != op.ID && op.Role != role.Admin {
		return httpx.ErrForbidden("仅上传者或管理员可删除附件")
	}
	if err := s.attachments.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除附件失败: " + err.Error())
	}
	if err := os.Remove(a.FilePath); err != nil && !os.IsNotExist(err) {
		s.log.Warn("删除附件文件失败", zap.String("path", a.FilePath), zap.Error(err))
	}
	s.audit(ctx, op, "delete", a.BizType, a.BizID, "", "", a, nil)
	return nil
}

// ------------------------- helpers -------------------------

func validBizType(bizType string) bool {
	switch bizType {
	case BizTypeTicket, BizTypeIncident, BizTypeProblem, BizTypeChange:
		return true
	default:
		return false
	}
}

func mapRepoErr(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound(notFoundMsg)
	}
	return httpx.ErrInternal(err.Error())
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
