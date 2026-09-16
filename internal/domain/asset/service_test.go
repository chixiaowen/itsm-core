package asset

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

type testEnv struct {
	assets  *fakeAssetRepo
	history *fakeHistoryRepo
	cis     *fakeCIReader
	audit   *fakeAuditWriter
	svc     *Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		assets:  newFakeAssetRepo(),
		history: newFakeHistoryRepo(),
		cis:     newFakeCIReader(),
		audit:   newFakeAuditWriter(),
	}
	e.svc = NewService(Deps{
		Assets:    e.assets,
		Histories: e.history,
		CIs:       e.cis,
		Audit:     e.audit,
		Now:       func() time.Time { return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) },
	})
	e.cis.put(cmdb.CI{ID: 100, Code: "srv-01", Name: "服务器", CIType: cmdb.CITypeServer, Status: cmdb.StatusInUse})
	e.cis.put(cmdb.CI{ID: 200, Code: "db-01", Name: "数据库", CIType: cmdb.CITypeDatabase, Status: cmdb.StatusInUse})
	return e
}

func appErrCode(t *testing.T, err error) int {
	t.Helper()
	var ae *httpx.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *httpx.AppError，实际 %T (%v)", err, err)
	}
	return ae.Code
}

func op() Operator                   { return Operator{ID: 1, Role: role.CmdbManager} }
func u64(v uint64) *uint64           { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func (e *testEnv) seedAsset(t *testing.T, no, name string) *Asset {
	t.Helper()
	a, err := e.svc.CreateAsset(context.Background(), op(), AssetRequest{AssetNo: no, Name: name, Category: "laptop"})
	if err != nil {
		t.Fatalf("seedAsset 失败: %v", err)
	}
	return a
}

// ---------------- CRUD ----------------

func TestCreateAsset(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	a, err := e.svc.CreateAsset(ctx, op(), AssetRequest{AssetNo: "AST-0001", Name: "笔记本", Category: "laptop", Vendor: "Dell"})
	if err != nil {
		t.Fatalf("CreateAsset 失败: %v", err)
	}
	if a.Status != StatusPlanned {
		t.Fatalf("初始应为 planned，实际 %s", a.Status)
	}
	if _, err := e.svc.CreateAsset(ctx, op(), AssetRequest{AssetNo: "AST-0001", Name: "重复", Category: "laptop"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复编号应 409，实际 %v", err)
	}

	// 编号留空自动生成
	auto, err := e.svc.CreateAsset(ctx, op(), AssetRequest{Name: "自动编号", Category: "desktop"})
	if err != nil {
		t.Fatalf("自动编号创建失败: %v", err)
	}
	if auto.AssetNo == "" || auto.AssetNo[:4] != "AST-" {
		t.Fatalf("自动编号格式不符: %s", auto.AssetNo)
	}
}

func TestGetAssetDetailAndHistory(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本")
	// 走一次流转产生历史
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{PurchaseDate: timePtr(time.Now())}); err != nil {
		t.Fatalf("入库失败: %v", err)
	}

	d, err := e.svc.GetAssetDetail(ctx, a.ID)
	if err != nil {
		t.Fatalf("详情失败: %v", err)
	}
	if len(d.History) != 1 || d.History[0].ToStatus != StatusInStock {
		t.Fatalf("历史不符: %+v", d.History)
	}
	if _, err := e.svc.GetAssetDetail(ctx, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestUpdateAndDeleteAsset(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本")
	b := e.seedAsset(t, "AST-0002", "台式机")

	// 改成已存在编号 → 409
	if _, err := e.svc.UpdateAsset(ctx, op(), a.ID, AssetRequest{AssetNo: "AST-0002", Name: "x", Category: "laptop"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("改重复编号应 409，实际 %v", err)
	}
	got, err := e.svc.UpdateAsset(ctx, op(), a.ID, AssetRequest{AssetNo: "AST-0001", Name: "笔记本改", Category: "laptop", Location: "A1"})
	if err != nil || got.Name != "笔记本改" || got.Location != "A1" {
		t.Fatalf("更新失败: %+v %v", got, err)
	}
	// 删除
	if err := e.svc.DeleteAsset(ctx, op(), b.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := e.svc.DeleteAsset(ctx, op(), b.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("重复删除应 404，实际 %v", err)
	}
}

// ---------------- 生命周期状态机 ----------------

func TestLifecycle_FullFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本")

	// planned -> deploy 非法（跳过 stock_in）→ 409
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDeploy, TransitionParams{}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("planned 直接部署应 409，实际 %v", err)
	}
	// stock_in 缺采购信息 → 422
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("缺采购信息应 422，实际 %v", err)
	}
	// 补采购日期 → in_stock
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{PurchaseDate: timePtr(time.Now())}); err != nil || got.Status != StatusInStock {
		t.Fatalf("入库失败: %+v %v", got, err)
	}
	// deploy 缺 user/location → 422
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDeploy, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("缺使用人/位置应 422，实际 %v", err)
	}
	// 提供 user/location → in_use
	loc := "A1-工位"
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDeploy, TransitionParams{UserID: u64(7), Location: &loc}); err != nil || got.Status != StatusInUse {
		t.Fatalf("部署失败: %+v %v", got, err)
	}
	// maintain 缺 remark → 422；补 remark → maintenance
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionMaintain, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("送修缺备注应 422，实际 %v", err)
	}
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionMaintain, TransitionParams{Remark: "屏幕故障"}); err != nil || got.Status != StatusMaintenance {
		t.Fatalf("送修失败: %+v %v", got, err)
	}
	// finish_maintain 缺结论 → 422；补结论 → in_use
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionFinishMaintain, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("维护完成缺结论应 422，实际 %v", err)
	}
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionFinishMaintain, TransitionParams{Remark: "更换屏幕"}); err != nil || got.Status != StatusInUse {
		t.Fatalf("维护完成失败: %+v %v", got, err)
	}
	// in_use -> 绑定 CI 后退役 → 409
	if _, err := e.svc.BindCI(ctx, op(), a.ID, 100); err != nil {
		t.Fatalf("绑定 CI 失败: %v", err)
	}
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionRetire, TransitionParams{Reason: "老化"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("带 CI 退役应 409，实际 %v", err)
	}
	// 解绑后退役 → retired
	if _, err := e.svc.UnbindCI(ctx, op(), a.ID); err != nil {
		t.Fatalf("解绑失败: %v", err)
	}
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionRetire, TransitionParams{Reason: "老化"}); err != nil || got.Status != StatusRetired {
		t.Fatalf("退役失败: %+v %v", got, err)
	}
	// dispose 缺处置方式 → 422；补 → disposed
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDispose, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("报废缺处置方式应 422，实际 %v", err)
	}
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDispose, TransitionParams{Remark: "环保报废"}); err != nil || got.Status != StatusDisposed {
		t.Fatalf("报废失败: %+v %v", got, err)
	}
	// disposed 终态 → 任何流转 409
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("终态流转应 409，实际 %v", err)
	}

	// 历史应记录全部 6 次成功流转
	if e.history.count() != 6 {
		t.Fatalf("历史记录应为 6 条，实际 %d", e.history.count())
	}
}

func TestRetireFromStockRequiresReason(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本")
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{PurchaseDate: timePtr(time.Now())}); err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	if _, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionRetire, TransitionParams{}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("未用直接退役缺原因应 422，实际 %v", err)
	}
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionRetire, TransitionParams{Reason: "采购错误"}); err != nil || got.Status != StatusRetired {
		t.Fatalf("退役失败: %+v %v", got, err)
	}
}

func TestTransition_RoleForbidden(t *testing.T) {
	e := newTestEnv(t)
	a := e.seedAsset(t, "AST-0001", "笔记本")
	_, err := e.svc.TransitionAsset(context.Background(), Operator{ID: 2, Role: role.Requestor}, a.ID, ActionStockIn, TransitionParams{PurchaseDate: timePtr(time.Now())})
	if appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("越权流转应 403，实际 %v", err)
	}
}

// TestTransition_Illegal409CarriesStatePair 校验资产生命周期非法流转 409 消息统一含「当前状态 -> 目标状态」。
func TestTransition_Illegal409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本") // 初始 planned

	// 负向：planned 直接 deploy（跳过 stock_in）→ 409，消息含 current=planned -> target=in_use
	_, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionDeploy, TransitionParams{})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusPlanned) || !strings.Contains(msg, "target="+StatusInUse) {
		t.Fatalf("非法流转消息应含 current=planned -> target=in_use，实际 %q", msg)
	}

	// 正向：合法流转（入库）成功
	if got, err := e.svc.TransitionAsset(ctx, op(), a.ID, ActionStockIn, TransitionParams{PurchaseDate: timePtr(time.Now())}); err != nil || got.Status != StatusInStock {
		t.Fatalf("入库应成功: %+v %v", got, err)
	}
}

// ---------------- CI 绑定 ----------------

func TestBindCI_Uniqueness(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "A")
	b := e.seedAsset(t, "AST-0002", "B")

	// CI 不存在 → 400
	if _, err := e.svc.BindCI(ctx, op(), a.ID, 999); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("CI 不存在应 400，实际 %v", err)
	}
	// 成功绑定
	got, err := e.svc.BindCI(ctx, op(), a.ID, 100)
	if err != nil || got.CIID == nil || *got.CIID != 100 {
		t.Fatalf("绑定失败: %+v %v", got, err)
	}
	// 幂等重复绑定
	if _, err := e.svc.BindCI(ctx, op(), a.ID, 100); err != nil {
		t.Fatalf("重复绑定同一资产应幂等，实际 %v", err)
	}
	// 另一资产绑定同一 CI → 409
	if _, err := e.svc.BindCI(ctx, op(), b.ID, 100); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("同 CI 多资产应 409，实际 %v", err)
	}
	// 解绑后再绑定新 CI
	if _, err := e.svc.UnbindCI(ctx, op(), a.ID); err != nil {
		t.Fatalf("解绑失败: %v", err)
	}
	if _, err := e.svc.UnbindCI(ctx, op(), a.ID); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("重复解绑应 422，实际 %v", err)
	}
	if _, err := e.svc.BindCI(ctx, op(), a.ID, 200); err != nil {
		t.Fatalf("重新绑定失败: %v", err)
	}
}

func TestBindCI_ZeroAndNotFoundAsset(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if _, err := e.svc.BindCI(ctx, op(), 1, 0); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("ci_id=0 应 400，实际 %v", err)
	}
	if _, err := e.svc.BindCI(ctx, op(), 999, 100); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("资产不存在应 404，实际 %v", err)
	}
}

func TestListAssets_Filters(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.seedAsset(t, "AST-0001", "A")
	b := e.seedAsset(t, "AST-0002", "B")
	if _, err := e.svc.UpdateAsset(ctx, op(), b.ID, AssetRequest{AssetNo: "AST-0002", Name: "B", Category: "desktop"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	items, total, err := e.svc.ListAssets(ctx, AssetListQuery{Category: "desktop", Offset: 0, Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("筛选失败: total=%d items=%d err=%v", total, len(items), err)
	}
}

func TestValidStatusAndActions(t *testing.T) {
	if !ValidStatus(StatusDisposed) || ValidStatus("zombie") {
		t.Fatalf("状态校验异常")
	}
	if acts := AssetActions(StatusInStock); len(acts) != 2 {
		t.Fatalf("in_stock 应有 2 个动作，实际 %v", acts)
	}
}
