package cmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
)

// 审计 biz_type 常量。
const (
	bizTypeCI    = "ci"
	bizTypeCIRel = "ci_relation"
)

// Operator 是执行操作的当前用户。
type Operator struct {
	ID       uint64
	Role     string
	ClientIP string
}

// AuditWriter 复用 platform 的审计能力（consumer 侧接口）。
type AuditWriter interface {
	AppendAudit(ctx context.Context, e platform.AuditEntry) error
}

// Deps 是 cmdb service 的依赖集合。
type Deps struct {
	CIs       CIRepository
	Relations RelationRepository
	Audit     AuditWriter
	Logger    *zap.Logger
	Now       func() time.Time
}

// Service 承载 CMDB 业务。
type Service struct {
	cis       CIRepository
	relations RelationRepository
	audit     AuditWriter
	log       *zap.Logger
	now       func() time.Time
}

// NewService 构造 cmdb service。
func NewService(d Deps) *Service {
	s := &Service{
		cis:       d.CIs,
		relations: d.Relations,
		audit:     d.Audit,
		log:       d.Logger,
		now:       d.Now,
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// ------------------------- CI -------------------------

// ListCIs 分页查询 CI。
func (s *Service) ListCIs(ctx context.Context, q CIListQuery) ([]CI, int64, error) {
	if s.cis == nil {
		return nil, 0, httpx.ErrInternal("CI 仓储未初始化")
	}
	return s.cis.List(ctx, q)
}

// GetCIDetail 返回 CI 详情（含直接关系）。
func (s *Service) GetCIDetail(ctx context.Context, id uint64) (*CIDetail, error) {
	if s.cis == nil || s.relations == nil {
		return nil, httpx.ErrInternal("CMDB 仓储未初始化")
	}
	ci, err := s.cis.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "CI 不存在")
	}
	rels, err := s.relations.ListByCI(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	if rels == nil {
		rels = []CIRelation{}
	}
	return &CIDetail{CI: *ci, Relations: rels}, nil
}

// CreateCI 新建 CI（同域 code 唯一；attrs 序列化为 JSON 字符串）。
func (s *Service) CreateCI(ctx context.Context, op Operator, req CIRequest) (*CI, error) {
	if s.cis == nil {
		return nil, httpx.ErrInternal("CI 仓储未初始化")
	}
	status, attrs, err := normalizeCIReq(req)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if _, err := s.cis.GetByCode(ctx, code); err == nil {
		return nil, httpx.ErrConflict("CI 编码已存在: " + code)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrInternal(err.Error())
	}

	now := s.now()
	ci := &CI{
		Code:      code,
		Name:      strings.TrimSpace(req.Name),
		CIType:    req.CIType,
		Status:    status,
		Attrs:     attrs,
		OwnerID:   req.OwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.cis.Create(ctx, ci); err != nil {
		return nil, httpx.ErrConflict("创建 CI 失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "create", bizTypeCI, ci.ID, "", ci.Status)
	return ci, nil
}

// UpdateCI 编辑 CI。
func (s *Service) UpdateCI(ctx context.Context, op Operator, id uint64, req CIRequest) (*CI, error) {
	if s.cis == nil {
		return nil, httpx.ErrInternal("CI 仓储未初始化")
	}
	ci, err := s.cis.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "CI 不存在")
	}
	status, attrs, err := normalizeCIReq(req)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if code != ci.Code {
		if other, err := s.cis.GetByCode(ctx, code); err == nil && other.ID != ci.ID {
			return nil, httpx.ErrConflict("CI 编码已存在: " + code)
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrInternal(err.Error())
		}
	}

	from := ci.Status
	ci.Code = code
	ci.Name = strings.TrimSpace(req.Name)
	ci.CIType = req.CIType
	ci.Status = status
	ci.Attrs = attrs
	ci.OwnerID = req.OwnerID
	ci.UpdatedAt = s.now()
	if err := s.cis.Update(ctx, ci); err != nil {
		return nil, httpx.ErrInternal("更新 CI 失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "update", bizTypeCI, ci.ID, from, ci.Status)
	return ci, nil
}

// DeleteCI 软删除 CI；存在未清理关系时返回 409 并附带冲突关系清单。
func (s *Service) DeleteCI(ctx context.Context, op Operator, id uint64) error {
	if s.cis == nil || s.relations == nil {
		return httpx.ErrInternal("CMDB 仓储未初始化")
	}
	if _, err := s.cis.GetByID(ctx, id); err != nil {
		return mapRepoErr(err, "CI 不存在")
	}
	rels, err := s.relations.ListByCI(ctx, id)
	if err != nil {
		return httpx.ErrInternal(err.Error())
	}
	if len(rels) > 0 {
		return relationsConflictError(rels)
	}
	if err := s.cis.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除 CI 失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "delete", bizTypeCI, id, "", "")
	return nil
}

// ------------------------- 关系 -------------------------

// AddRelation 新增关系：自环 400、重复 409、目标不存在 400。
func (s *Service) AddRelation(ctx context.Context, op Operator, sourceID uint64, req RelationRequest) (*CIRelation, error) {
	if s.cis == nil || s.relations == nil {
		return nil, httpx.ErrInternal("CMDB 仓储未初始化")
	}
	if sourceID == req.TargetCIID {
		return nil, httpx.ErrInvalidRelation("禁止自环关系：源与目标 CI 相同")
	}
	if !ValidRelationType(req.RelationType) {
		return nil, httpx.ErrBadRequest("非法关系类型: " + req.RelationType)
	}
	if _, err := s.cis.GetByID(ctx, sourceID); err != nil {
		return nil, mapRepoErr(err, "源 CI 不存在")
	}
	if _, err := s.cis.GetByID(ctx, req.TargetCIID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrBadRequest("目标 CI 不存在")
		}
		return nil, httpx.ErrInternal(err.Error())
	}
	if _, err := s.relations.Find(ctx, sourceID, req.TargetCIID); err == nil {
		return nil, httpx.ErrConflict("该关系已存在")
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrInternal(err.Error())
	}

	rel := &CIRelation{
		SourceCIID:   sourceID,
		TargetCIID:   req.TargetCIID,
		RelationType: req.RelationType,
		CreatedAt:    s.now(),
	}
	if err := s.relations.Create(ctx, rel); err != nil {
		return nil, httpx.ErrConflict("创建关系失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "create", bizTypeCIRel, rel.ID, "", "")
	return rel, nil
}

// DeleteRelation 删除关系。
func (s *Service) DeleteRelation(ctx context.Context, op Operator, ciID, relID uint64) error {
	if s.relations == nil {
		return httpx.ErrInternal("关系仓储未初始化")
	}
	rel, err := s.relations.GetByID(ctx, relID)
	if err != nil {
		return mapRepoErr(err, "关系不存在")
	}
	if rel.SourceCIID != ciID && rel.TargetCIID != ciID {
		return httpx.ErrNotFound("关系不存在")
	}
	if err := s.relations.Delete(ctx, relID); err != nil {
		return httpx.ErrInternal("删除关系失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "delete", bizTypeCIRel, relID, "", "")
	return nil
}

// ------------------------- 拓扑 -------------------------

// Topology 从指定 CI 出发做 N 层拓扑展开（默认 2 层）。
func (s *Service) Topology(ctx context.Context, id uint64, depth int, direction string) (*TopologyGraph, error) {
	if s.cis == nil || s.relations == nil {
		return nil, httpx.ErrInternal("CMDB 仓储未初始化")
	}
	if _, err := s.cis.GetByID(ctx, id); err != nil {
		return nil, mapRepoErr(err, "CI 不存在")
	}
	allCIs, err := s.cis.ListAll(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	allRels, err := s.relations.ListAll(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	ciMap := make(map[uint64]CI, len(allCIs))
	for _, ci := range allCIs {
		ciMap[ci.ID] = ci
	}
	g := BuildTopology(id, depth, direction, ciMap, allRels)
	return &g, nil
}

// ------------------------- helpers -------------------------

// normalizeCIReq 校验并归一化 CI 请求（返回归一化状态与 attrs JSON 字符串）。
func normalizeCIReq(req CIRequest) (status string, attrs string, err error) {
	if !ValidCIType(req.CIType) {
		return "", "", httpx.ErrBadRequest("非法 CI 类型: " + req.CIType)
	}
	status = strings.TrimSpace(req.Status)
	if status == "" {
		status = StatusPlanned
	}
	if !ValidCIStatus(status) {
		return "", "", httpx.ErrBadRequest("非法 CI 状态: " + status)
	}
	attrs = "{}"
	if req.Attrs != nil {
		b, jerr := json.Marshal(req.Attrs)
		if jerr != nil {
			return "", "", httpx.ErrBadRequest("attrs 序列化失败: " + jerr.Error())
		}
		attrs = string(b)
	}
	return status, attrs, nil
}

// relationsConflictError 构造「存在未清理关系」的 409 错误，message 内含冲突关系清单（JSON）。
func relationsConflictError(rels []CIRelation) error {
	payload, err := json.Marshal(rels)
	if err != nil {
		payload = []byte("[]")
	}
	return httpx.ErrConflict(fmt.Sprintf("存在 %d 条未清理的 CI 关系，请先解除后再删除；冲突关系: %s", len(rels), string(payload)))
}

// auditWrite 写入审计（best-effort）。
func (s *Service) auditWrite(ctx context.Context, op Operator, action, bizType string, bizID uint64, from, to string) {
	if s.audit == nil {
		return
	}
	if err := s.audit.AppendAudit(ctx, platform.AuditEntry{
		ActorID:    op.ID,
		Action:     action,
		BizType:    bizType,
		BizID:      bizID,
		FromStatus: from,
		ToStatus:   to,
		ClientIP:   op.ClientIP,
	}); err != nil {
		s.log.Warn("写入审计日志失败", zap.String("biz_type", bizType), zap.Uint64("biz_id", bizID), zap.Error(err))
	}
}

// mapRepoErr 把仓储哨兵错误映射为 404 或 500。
func mapRepoErr(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound(notFoundMsg)
	}
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		return err
	}
	return httpx.ErrInternal(err.Error())
}
