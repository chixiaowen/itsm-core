package cmdb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// errFakeDuplicate 是内存仓储的「唯一约束冲突」哨兵。
var errFakeDuplicate = errors.New("fake: duplicate")

type testEnv struct {
	cis   *fakeCIRepo
	rels  *fakeRelationRepo
	audit *fakeAuditWriter
	svc   *Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		cis:   newFakeCIRepo(),
		rels:  newFakeRelationRepo(),
		audit: newFakeAuditWriter(),
	}
	e.svc = NewService(Deps{
		CIs:       e.cis,
		Relations: e.rels,
		Audit:     e.audit,
		Now:       func() time.Time { return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) },
	})
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

func adminOp() Operator { return Operator{ID: 1, Role: role.CmdbManager} }

func (e *testEnv) seedCI(t *testing.T, code, name, ciType string) *CI {
	t.Helper()
	ci, err := e.svc.CreateCI(context.Background(), adminOp(), CIRequest{
		Code: code, Name: name, CIType: ciType,
	})
	if err != nil {
		t.Fatalf("seedCI(%s) 失败: %v", code, err)
	}
	return ci
}

// ---------------- CI CRUD ----------------

func TestCICreate_SuccessAndDuplicate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	ci, err := e.svc.CreateCI(ctx, adminOp(), CIRequest{
		Code: "srv-01", Name: "应用服务器", CIType: CITypeServer, Attrs: map[string]any{"cpu": 8},
	})
	if err != nil {
		t.Fatalf("CreateCI 失败: %v", err)
	}
	if ci.Status != StatusPlanned {
		t.Fatalf("默认状态应为 planned，实际 %s", ci.Status)
	}
	if !strings.Contains(ci.Attrs, "\"cpu\":8") {
		t.Fatalf("attrs 应序列化为 JSON 字符串，实际 %q", ci.Attrs)
	}

	if _, err := e.svc.CreateCI(ctx, adminOp(), CIRequest{Code: "srv-01", Name: "重复", CIType: CITypeServer}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复 code 应 409，实际 %v", err)
	}
}

func TestCICreate_InvalidEnums(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if _, err := e.svc.CreateCI(ctx, adminOp(), CIRequest{Code: "x", Name: "x", CIType: "vm"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法类型应 400，实际 %v", err)
	}
	if _, err := e.svc.CreateCI(ctx, adminOp(), CIRequest{Code: "x", Name: "x", CIType: CITypeServer, Status: "zombie"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法状态应 400，实际 %v", err)
	}
}

func TestCIUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ci := e.seedCI(t, "srv-01", "服务器", CITypeServer)
	other := e.seedCI(t, "srv-02", "服务器2", CITypeServer)

	// 改成已存在的 code → 409
	if _, err := e.svc.UpdateCI(ctx, adminOp(), ci.ID, CIRequest{Code: other.Code, Name: "x", CIType: CITypeServer}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("改重复 code 应 409，实际 %v", err)
	}
	got, err := e.svc.UpdateCI(ctx, adminOp(), ci.ID, CIRequest{
		Code: ci.Code, Name: "服务器改", CIType: CITypeServer, Status: StatusInUse, Attrs: map[string]any{"rack": "A1"},
	})
	if err != nil {
		t.Fatalf("UpdateCI 失败: %v", err)
	}
	if got.Name != "服务器改" || got.Status != StatusInUse || !strings.Contains(got.Attrs, "rack") {
		t.Fatalf("更新字段不符: %+v", got)
	}
	if _, err := e.svc.UpdateCI(ctx, adminOp(), 999, CIRequest{Code: "z", Name: "z", CIType: CITypeServer}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestCIDelete_WithRelations409(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedCI(t, "srv-01", "A", CITypeServer)
	b := e.seedCI(t, "db-01", "B", CITypeDatabase)
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn}); err != nil {
		t.Fatalf("AddRelation 失败: %v", err)
	}

	err := e.svc.DeleteCI(ctx, adminOp(), a.ID)
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("带关系删除应 409，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "冲突关系") {
		t.Fatalf("错误信息应含冲突关系清单，实际 %v", err)
	}

	// 解除关系后可删除
	rels, _ := e.rels.ListByCI(ctx, a.ID)
	if err := e.svc.DeleteRelation(ctx, adminOp(), a.ID, rels[0].ID); err != nil {
		t.Fatalf("解除关系失败: %v", err)
	}
	if err := e.svc.DeleteCI(ctx, adminOp(), a.ID); err != nil {
		t.Fatalf("无关系后删除应成功，实际 %v", err)
	}
}

// ---------------- 关系 ----------------

func TestAddRelation_SelfLoopAndDuplicate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedCI(t, "srv-01", "A", CITypeServer)
	b := e.seedCI(t, "db-01", "B", CITypeDatabase)

	// 自环 → 400
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: a.ID, RelationType: RelationDependsOn}); appErrCode(t, err) != httpx.CodeInvalidRelation {
		t.Fatalf("自环应 400/10002，实际 %v", err)
	}
	// 非法关系类型 → 400
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: "owns"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法关系类型应 400，实际 %v", err)
	}
	// 目标不存在 → 400
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: 999, RelationType: RelationDependsOn}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("目标不存在应 400，实际 %v", err)
	}
	// 成功
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn}); err != nil {
		t.Fatalf("AddRelation 失败: %v", err)
	}
	// 重复 → 409
	if _, err := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复关系应 409，实际 %v", err)
	}
}

func TestDeleteRelation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedCI(t, "srv-01", "A", CITypeServer)
	b := e.seedCI(t, "db-01", "B", CITypeDatabase)
	c := e.seedCI(t, "net-01", "C", CITypeNetwork)
	rel, _ := e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationConnectsTo})

	// 与路径 CI 无关 → 404
	if err := e.svc.DeleteRelation(ctx, adminOp(), c.ID, rel.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("无关 CI 删除应 404，实际 %v", err)
	}
	// 不存在 → 404
	if err := e.svc.DeleteRelation(ctx, adminOp(), a.ID, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在关系应 404，实际 %v", err)
	}
	// 成功
	if err := e.svc.DeleteRelation(ctx, adminOp(), a.ID, rel.ID); err != nil {
		t.Fatalf("删除关系应成功，实际 %v", err)
	}
}

func TestGetCIDetail(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedCI(t, "srv-01", "A", CITypeServer)
	b := e.seedCI(t, "db-01", "B", CITypeDatabase)
	_, _ = e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn})

	d, err := e.svc.GetCIDetail(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetCIDetail 失败: %v", err)
	}
	if d.CI.ID != a.ID || len(d.Relations) != 1 {
		t.Fatalf("详情不符: %+v", d)
	}
	if _, err := e.svc.GetCIDetail(ctx, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestListCIs_Filters(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.seedCI(t, "srv-01", "应用服务器", CITypeServer)
	e.seedCI(t, "db-01", "数据库", CITypeDatabase)

	items, total, err := e.svc.ListCIs(ctx, CIListQuery{CIType: CITypeServer, Offset: 0, Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("按类型筛选失败: total=%d items=%d err=%v", total, len(items), err)
	}
	items, _, _ = e.svc.ListCIs(ctx, CIListQuery{Keyword: "数据库", Limit: 10})
	if len(items) != 1 || items[0].Code != "db-01" {
		t.Fatalf("关键字筛选失败: %+v", items)
	}
}

// ---------------- 拓扑（service 层） ----------------

func TestTopology_Service(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedCI(t, "a", "A", CITypeServer)
	b := e.seedCI(t, "b", "B", CITypeDatabase)
	c := e.seedCI(t, "c", "C", CITypeApp)
	d := e.seedCI(t, "d", "D", CITypeOther)
	_, _ = e.svc.AddRelation(ctx, adminOp(), a.ID, RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn})
	_, _ = e.svc.AddRelation(ctx, adminOp(), b.ID, RelationRequest{TargetCIID: c.ID, RelationType: RelationDependsOn})
	_, _ = e.svc.AddRelation(ctx, adminOp(), c.ID, RelationRequest{TargetCIID: d.ID, RelationType: RelationDependsOn})

	// 默认 2 层：a,b,c
	g, err := e.svc.Topology(ctx, a.ID, 0, "")
	if err != nil {
		t.Fatalf("Topology 失败: %v", err)
	}
	if len(g.Nodes) != 3 || g.Depth != DefaultTopologyDepth {
		t.Fatalf("默认 2 层展开错误: nodes=%d depth=%d", len(g.Nodes), g.Depth)
	}
	// 3 层：a,b,c,d
	g, _ = e.svc.Topology(ctx, a.ID, 3, DirectionOut)
	if len(g.Nodes) != 4 {
		t.Fatalf("3 层展开应为 4 节点，实际 %d", len(g.Nodes))
	}
	// 根不存在 → 404
	if _, err := e.svc.Topology(ctx, 999, 0, ""); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("根不存在应 404，实际 %v", err)
	}
}

func TestCIAudit(t *testing.T) {
	e := newTestEnv(t)
	e.seedCI(t, "x", "X", CITypeServer)
	if e.audit.count() == 0 {
		t.Fatalf("应写入审计")
	}
}
