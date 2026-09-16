package asset

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/idgen"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// 审计 biz_type 常量。
const bizTypeAsset = "asset"

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

// Deps 是 asset service 的依赖集合。
type Deps struct {
	Assets    AssetRepository
	Histories HistoryRepository
	CIs       CIReader
	Audit     AuditWriter
	Gen       *idgen.Generator
	Logger    *zap.Logger
	Now       func() time.Time
}

// Service 承载资产域业务。
type Service struct {
	assets    AssetRepository
	histories HistoryRepository
	cis       CIReader
	audit     AuditWriter
	gen       *idgen.Generator
	log       *zap.Logger
	now       func() time.Time
}

// NewService 构造 asset service，未提供的可选依赖回退到安全默认值。
func NewService(d Deps) *Service {
	s := &Service{
		assets:    d.Assets,
		histories: d.Histories,
		cis:       d.CIs,
		audit:     d.Audit,
		gen:       d.Gen,
		log:       d.Logger,
		now:       d.Now,
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	if s.gen == nil {
		s.gen = idgen.NewGenerator()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// ListAssets 分页查询资产台账。
func (s *Service) ListAssets(ctx context.Context, q AssetListQuery) ([]Asset, int64, error) {
	if s.assets == nil {
		return nil, 0, httpx.ErrInternal("资产仓储未初始化")
	}
	return s.assets.List(ctx, q)
}

// GetAssetDetail 返回资产详情（含历史与绑定 CI）。
func (s *Service) GetAssetDetail(ctx context.Context, id uint64) (*AssetDetail, error) {
	if s.assets == nil || s.histories == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	a, err := s.assets.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	history, err := s.histories.ListByAsset(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	if history == nil {
		history = []AssetHistory{}
	}
	detail := &AssetDetail{Asset: *a, History: history}
	if a.CIID != nil && s.cis != nil {
		ci, err := s.cis.GetByID(ctx, *a.CIID)
		if err == nil {
			detail.CI = ci
		} else if !errors.Is(err, cmdb.ErrNotFound) {
			// 读取 CI 失败不阻断详情；仅记录。
			s.log.Warn("读取绑定 CI 失败", zap.Uint64("ci_id", *a.CIID), zap.Error(err))
		}
	}
	return detail, nil
}

// ListHistory 返回资产生命周期历史。
func (s *Service) ListHistory(ctx context.Context, id uint64) ([]AssetHistory, error) {
	if s.assets == nil || s.histories == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	if _, err := s.assets.GetByID(ctx, id); err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	items, err := s.histories.ListByAsset(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	if items == nil {
		items = []AssetHistory{}
	}
	return items, nil
}

// CreateAsset 新建资产（初始状态 planned；编号留空则自动生成）。
func (s *Service) CreateAsset(ctx context.Context, op Operator, req AssetRequest) (*Asset, error) {
	if s.assets == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	assetNo := strings.TrimSpace(req.AssetNo)
	if assetNo == "" {
		assetNo = s.gen.Next(AssetPrefix, s.now(), 0)
	}
	if _, err := s.assets.GetByAssetNo(ctx, assetNo); err == nil {
		return nil, httpx.ErrConflict("资产编号已存在: " + assetNo)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.ErrInternal(err.Error())
	}

	now := s.now()
	a := &Asset{
		AssetNo:      assetNo,
		Name:         strings.TrimSpace(req.Name),
		Category:     strings.TrimSpace(req.Category),
		Status:       StatusPlanned,
		UserID:       req.UserID,
		Location:     req.Location,
		Vendor:       req.Vendor,
		PurchaseDate: req.PurchaseDate,
		WarrantyEnd:  req.WarrantyEnd,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.assets.Create(ctx, a); err != nil {
		return nil, httpx.ErrConflict("创建资产失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "create", a.ID, "", a.Status)
	return a, nil
}

// UpdateAsset 编辑资产台账字段。
func (s *Service) UpdateAsset(ctx context.Context, op Operator, id uint64, req AssetRequest) (*Asset, error) {
	if s.assets == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	a, err := s.assets.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	if assetNo := strings.TrimSpace(req.AssetNo); assetNo != "" && assetNo != a.AssetNo {
		if other, err := s.assets.GetByAssetNo(ctx, assetNo); err == nil && other.ID != a.ID {
			return nil, httpx.ErrConflict("资产编号已存在: " + assetNo)
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrInternal(err.Error())
		}
		a.AssetNo = assetNo
	}
	a.Name = strings.TrimSpace(req.Name)
	a.Category = strings.TrimSpace(req.Category)
	a.Vendor = req.Vendor
	a.Location = req.Location
	a.UserID = req.UserID
	a.PurchaseDate = req.PurchaseDate
	a.WarrantyEnd = req.WarrantyEnd
	a.UpdatedAt = s.now()
	if err := s.assets.Update(ctx, a); err != nil {
		return nil, httpx.ErrInternal("更新资产失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "update", a.ID, a.Status, a.Status)
	return a, nil
}

// DeleteAsset 软删除资产（仅 cmdb_manager/admin）。
func (s *Service) DeleteAsset(ctx context.Context, op Operator, id uint64) error {
	if s.assets == nil {
		return httpx.ErrInternal("资产仓储未初始化")
	}
	if _, err := s.assets.GetByID(ctx, id); err != nil {
		return mapRepoErr(err, "资产不存在")
	}
	if err := s.assets.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除资产失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "delete", id, "", "")
	return nil
}

// TransitionParams 是资产生命周期流转的附加参数。
type TransitionParams struct {
	Remark       string
	Reason       string
	UserID       *uint64
	Location     *string
	CIID         *uint64
	PurchaseDate *time.Time
}

// TransitionAsset 执行一次生命周期流转：非法流转 409、越权 403、前置不满足 422。
//
// 每次成功流转追加一条 AssetHistory。
func (s *Service) TransitionAsset(ctx context.Context, op Operator, id uint64, action string, p TransitionParams) (*Asset, error) {
	if s.assets == nil || s.histories == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	a, err := s.assets.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	tr, ok := assetMachine.Resolve(a.Status, action)
	if !ok {
		return nil, httpx.ErrConflict(illegalTransitionMsg(a.Status, resolveAssetTarget(action), "当前状态不允许该动作"))
	}
	if !tr.Allows(op.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该操作")
	}

	// 应用流转附加参数（guard 之前，便于校验使用人/位置/采购信息等）。
	if p.PurchaseDate != nil {
		a.PurchaseDate = p.PurchaseDate
	}
	if p.UserID != nil {
		a.UserID = p.UserID
	}
	if p.Location != nil {
		a.Location = *p.Location
	}
	if p.CIID != nil && action != ActionRetire {
		if err := s.bindCIInternal(ctx, a, *p.CIID); err != nil {
			return nil, err
		}
	}

	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx:    ctx,
			Actor:  statemachine.Actor{UserID: op.ID, Role: op.Role},
			Entity: a,
			Params: map[string]any{"remark": p.Remark, "reason": p.Reason},
			Now:    s.now(),
		}); err != nil {
			return nil, err
		}
	}

	from := a.Status
	a.Status = tr.To
	a.UpdatedAt = s.now()
	if err := s.assets.Update(ctx, a); err != nil {
		return nil, httpx.ErrInternal("资产流转失败: " + err.Error())
	}

	remark := strings.TrimSpace(p.Remark)
	if remark == "" {
		remark = strings.TrimSpace(p.Reason)
	}
	if err := s.histories.Append(ctx, &AssetHistory{
		AssetID:    a.ID,
		FromStatus: from,
		ToStatus:   tr.To,
		Remark:     remark,
		ActorID:    op.ID,
		CreatedAt:  s.now(),
	}); err != nil {
		return nil, httpx.ErrInternal("写入资产历史失败: " + err.Error())
	}

	s.auditWrite(ctx, op, "transition", a.ID, from, tr.To)
	return a, nil
}

// BindCI 绑定 CI（ci_id 全局 1:1 唯一、可空）。
//
// 一个 CI 至多被一个资产绑定：已绑定的 CI 再被第二个资产绑定时返回 409
// （与 ci_id 上的 uniqueIndex idx_ast_ci 一致；不区分资产状态，见 PRD §5 Q4）。
func (s *Service) BindCI(ctx context.Context, op Operator, assetID, ciID uint64) (*Asset, error) {
	if s.assets == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	if ciID == 0 {
		return nil, httpx.ErrBadRequest("ci_id 必填")
	}
	a, err := s.assets.GetByID(ctx, assetID)
	if err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	if err := s.bindCIInternal(ctx, a, ciID); err != nil {
		return nil, err
	}
	a.UpdatedAt = s.now()
	if err := s.assets.Update(ctx, a); err != nil {
		return nil, httpx.ErrInternal("绑定 CI 失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "bind_ci", a.ID, a.Status, a.Status)
	return a, nil
}

// UnbindCI 解绑 CI。
func (s *Service) UnbindCI(ctx context.Context, op Operator, assetID uint64) (*Asset, error) {
	if s.assets == nil {
		return nil, httpx.ErrInternal("资产仓储未初始化")
	}
	a, err := s.assets.GetByID(ctx, assetID)
	if err != nil {
		return nil, mapRepoErr(err, "资产不存在")
	}
	if a.CIID == nil {
		return nil, httpx.ErrPrecondition("该资产未绑定 CI")
	}
	a.CIID = nil
	a.UpdatedAt = s.now()
	if err := s.assets.Update(ctx, a); err != nil {
		return nil, httpx.ErrInternal("解绑 CI 失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "unbind_ci", a.ID, a.Status, a.Status)
	return a, nil
}

// bindCIInternal 校验并设置资产的 CIID（不落库）。
//
// 校验三件事：CI 存在（否则 400）、目标 CI 未被其它资产绑定（否则 409）、幂等放行。
// 唯一性为**全局**约束（不看资产状态），与 ci_id 上的 uniqueIndex 一致。
func (s *Service) bindCIInternal(ctx context.Context, a *Asset, ciID uint64) error {
	if s.cis != nil {
		if _, err := s.cis.GetByID(ctx, ciID); err != nil {
			if errors.Is(err, cmdb.ErrNotFound) {
				return httpx.ErrBadRequest("CI 不存在")
			}
			return httpx.ErrInternal(err.Error())
		}
	}
	if a.CIID != nil && *a.CIID == ciID {
		return nil // 幂等
	}
	existing, err := s.assets.GetByCIID(ctx, ciID)
	if err == nil && existing.ID != a.ID {
		return httpx.ErrConflict("该 CI 已被其他资产绑定")
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return httpx.ErrInternal(err.Error())
	}
	id := ciID
	a.CIID = &id
	return nil
}

// AssetActions 返回资产某状态下允许的动作。
func (s *Service) AssetActions(status string) []string { return AssetActions(status) }

// ------------------------- helpers -------------------------

// auditWrite 写入审计（best-effort）。
func (s *Service) auditWrite(ctx context.Context, op Operator, action string, bizID uint64, from, to string) {
	if s.audit == nil {
		return
	}
	if err := s.audit.AppendAudit(ctx, platform.AuditEntry{
		ActorID:    op.ID,
		Action:     action,
		BizType:    bizTypeAsset,
		BizID:      bizID,
		FromStatus: from,
		ToStatus:   to,
		ClientIP:   op.ClientIP,
	}); err != nil {
		s.log.Warn("写入审计日志失败", zap.String("biz_type", bizTypeAsset), zap.Uint64("biz_id", bizID), zap.Error(err))
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
