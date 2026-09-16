package platform

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 platform 域的 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 platform handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// ------------------------- 用户 -------------------------

// ListUsers 处理 GET /users。
func (h *Handler) ListUsers(c *gin.Context) {
	page := httpx.ParsePage(c)
	items, total, err := h.svc.ListUsers(c.Request.Context(), UserListQuery{
		Role:    c.Query("role"),
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Offset:  page.Offset(),
		Limit:   page.PageSize,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// CreateUser 处理 POST /users。
func (h *Handler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	u, err := h.svc.CreateUser(c.Request.Context(), actorOf(c).ID, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, u)
}

// GetUser 处理 GET /users/:id。
func (h *Handler) GetUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	u, err := h.svc.GetUser(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, u)
}

// UpdateUser 处理 PUT /users/:id。
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req UpdateUserRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	u, err := h.svc.UpdateUser(c.Request.Context(), actorOf(c).ID, id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, u)
}

// DeleteUser 处理 DELETE /users/:id。
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteUser(c.Request.Context(), actorOf(c).ID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ListRoles 处理 GET /roles。
func (h *Handler) ListRoles(c *gin.Context) {
	httpx.OK(c, h.svc.Roles())
}

// ListUserOptions 处理 GET /users/options。
//
// 任意已登录用户可访问（不挂 RequirePerm）；返回仅含 id/display_name/role 的下拉项，
// status 固定过滤为 active 且排除软删除，可选 role= 精确过滤。
func (h *Handler) ListUserOptions(c *gin.Context) {
	opts, err := h.svc.ListUserOptions(c.Request.Context(), strings.TrimSpace(c.Query("role")))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, opts)
}

// ------------------------- SLA 策略 -------------------------

// ListSLAPolicies 处理 GET /sla-policies。
func (h *Handler) ListSLAPolicies(c *gin.Context) {
	items, err := h.svc.ListSLAPolicies(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// CreateSLAPolicy 处理 POST /sla-policies。
func (h *Handler) CreateSLAPolicy(c *gin.Context) {
	var req SLAPolicyRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.CreateSLAPolicy(c.Request.Context(), actorOf(c).ID, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, p)
}

// UpdateSLAPolicy 处理 PUT /sla-policies/:id。
func (h *Handler) UpdateSLAPolicy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req SLAPolicyRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.UpdateSLAPolicy(c.Request.Context(), actorOf(c).ID, id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

// DeleteSLAPolicy 处理 DELETE /sla-policies/:id。
func (h *Handler) DeleteSLAPolicy(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteSLAPolicy(c.Request.Context(), actorOf(c).ID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ------------------------- 审计 -------------------------

// ListAuditLogs 处理 GET /audit-logs。
//
// 支持按业务对象过滤：biz_type/biz_id 与 entity_type/entity_id 两套参数等价
// （前端详情页用 entity_type+entity_id 组装「状态流转时间线」）。
// 返回项含 from_status/to_status 字段。
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := AuditListQuery{
		Action: c.Query("action"),
		Offset: page.Offset(),
		Limit:  page.PageSize,
	}
	// biz_* 与 entity_* 互为别名；两者都传时 biz_* 优先（在仓储层归一）。
	q.BizType = c.Query("biz_type")
	q.EntityType = c.Query("entity_type")
	if v := c.Query("actor_id"); v != "" {
		q.ActorID, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := c.Query("biz_id"); v != "" {
		q.BizID, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := c.Query("entity_id"); v != "" {
		q.EntityID, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := strings.TrimSpace(c.Query("from")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.From = &t
		}
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.To = &t
		}
	}

	items, total, err := h.svc.ListAuditLogs(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// ------------------------- 评论 -------------------------

// ListComments 处理 GET /comments。
func (h *Handler) ListComments(c *gin.Context) {
	bizType := c.Query("biz_type")
	bizID, err := strconv.ParseUint(c.Query("biz_id"), 10, 64)
	if err != nil || bizID == 0 {
		httpx.Fail(c, httpx.ErrBadRequest("biz_id 必填且为数字"))
		return
	}
	includeInternal := c.Query("include_internal") == "true"
	items, err := h.svc.ListComments(c.Request.Context(), bizType, bizID, includeInternal)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// CreateComment 处理 POST /comments。
func (h *Handler) CreateComment(c *gin.Context) {
	var req CommentRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	cm, err := h.svc.CreateComment(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, cm)
}

// ------------------------- 附件 -------------------------

// UploadAttachment 处理 POST /attachments（multipart，≤20MB）。
func (h *Handler) UploadAttachment(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrBadRequest("缺少文件字段 file"))
		return
	}
	if fileHeader.Size > h.svc.MaxUploadBytes() {
		httpx.Fail(c, httpx.ErrBadRequest("附件超过大小上限"))
		return
	}
	bizType := c.PostForm("biz_type")
	bizID, _ := strconv.ParseUint(c.PostForm("biz_id"), 10, 64)

	f, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, httpx.ErrInternal("打开上传文件失败"))
		return
	}
	defer func() { _ = f.Close() }()

	a, err := h.svc.SaveAttachment(c.Request.Context(), actorOf(c), bizType, bizID,
		fileHeader.Filename, fileHeader.Header.Get("Content-Type"), fileHeader.Size, f)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, a)
}

// DownloadAttachment 处理 GET /attachments/:id/download。
func (h *Handler) DownloadAttachment(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	a, err := h.svc.GetAttachment(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+a.Filename+"\"")
	if a.MimeType != "" {
		c.Header("Content-Type", a.MimeType)
	}
	c.File(a.FilePath)
}

// DeleteAttachment 处理 DELETE /attachments/:id。
func (h *Handler) DeleteAttachment(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteAttachment(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ------------------------- helpers -------------------------

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, httpx.ErrBadRequest("非法的路径参数 id")
	}
	return id, nil
}

// actorOf 从 context 提取当前操作者（含客户端 IP）。
func actorOf(c *gin.Context) Operator {
	a, _ := middleware.GetActor(c)
	return Operator{ID: a.UserID, Role: a.Role, ClientIP: c.ClientIP()}
}
