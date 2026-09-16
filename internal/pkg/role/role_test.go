package role

import "testing"

func TestMatrix_AlignedWithPRD(t *testing.T) {
	cases := []struct {
		role string
		perm string
		want bool
	}{
		// 提交工单/上报事件：全部角色
		{Requestor, PermTicketCreate, true},
		{Admin, PermTicketCreate, true},
		{CmdbManager, PermIncidentReport, true},
		// 查看工单（全部）
		{Requestor, PermTicketViewAll, false},
		{Agent, PermTicketViewAll, true},
		// 受理/指派
		{Requestor, PermTicketAssign, false},
		{Agent, PermTicketAssign, true},
		{Resolver, PermTicketAssign, true}, // △ 仅被指派
		{ChangeManager, PermTicketAssign, false},
		// 处理
		{Agent, PermTicketHandle, true},
		// 评价（PRD §2.2：仅 requestor ✓；agent / admin ✗）
		{Requestor, PermTicketRate, true},
		{Agent, PermTicketRate, false},
		{Admin, PermTicketRate, false},
		// 事件升级 / 转单
		{Agent, PermIncidentEscalate, true},
		{ProblemManager, PermIncidentEscalate, true},
		{ChangeManager, PermIncidentEscalate, false},
		{Agent, PermIncidentConvert, true},
		{ProblemManager, PermIncidentConvert, false},
		// 问题
		{Agent, PermProblemCreate, true}, // △
		{ProblemManager, PermProblemCreate, true},
		{Requestor, PermProblemCreate, false},
		{Resolver, PermProblemRCA, true}, // △
		{Agent, PermProblemRCA, false},
		{ProblemManager, PermProblemRCA, true},
		// 变更
		{Requestor, PermChangeSubmit, false},
		{Agent, PermChangeSubmit, true},
		{CmdbManager, PermChangeSubmit, true},
		{ProblemManager, PermChangeApprove, true}, // △
		{ChangeManager, PermChangeApprove, true},
		{Agent, PermChangeApprove, false},
		// 服务目录
		{Admin, PermCatalogManage, true},
		{Agent, PermCatalogManage, false},
		{Requestor, PermCatalogOrder, true},
		// CMDB / 资产管理
		{CmdbManager, PermCmdbManage, true},
		{Resolver, PermCmdbManage, true}, // △
		{Agent, PermCmdbManage, false},
		{CmdbManager, PermAssetManage, true},
		{ProblemManager, PermAssetManage, false},
		// 平台
		{Admin, PermSlaManage, true},
		{CmdbManager, PermSlaManage, false},
		{Admin, PermUserManage, true},
		{Admin, PermAuditView, true},
		{Requestor, PermAuditView, false},
	}
	for _, c := range cases {
		if got := Has(c.role, c.perm); got != c.want {
			t.Fatalf("Has(%s,%s)=%v，期望 %v", c.role, c.perm, got, c.want)
		}
	}
}

// TestAdminPermissions_WithheldRate 锁定「全集 vs admin 实授权」的防退化不变量。
//
// 关键：AllPermissions 是全集（必须包含 PermTicketRate），而 admin 实际不持有该权限点
// （PRD §2.2「满意度评价」admin=✗）。将来若有人从 AllPermissions 中删除它，断言 1/3 会失败。
func TestAdminPermissions_WithheldRate(t *testing.T) {
	contains := func(list []string, p string) bool {
		for _, x := range list {
			if x == p {
				return true
			}
		}
		return false
	}

	// 1) 全集不变量：AllPermissions 必须包含 PermTicketRate。
	if !contains(AllPermissions, PermTicketRate) {
		t.Fatalf("AllPermissions 全集应包含 %s", PermTicketRate)
	}
	// 2) admin 实际不持有 PermTicketRate。
	if Has(Admin, PermTicketRate) {
		t.Fatalf("admin 不应持有 %s（PRD §2.2 满意度评价 admin=✗）", PermTicketRate)
	}
	// 3) AdminPermissions = 全集 - 刻意保留项（当前仅 rate，故长度恰差 1）。
	if got, want := len(AdminPermissions), len(AllPermissions)-1; got != want {
		t.Fatalf("len(AdminPermissions)=%d，期望 len(AllPermissions)-1=%d", got, want)
	}
	// 4) 请求人持有 PermTicketRate。
	if !Has(Requestor, PermTicketRate) {
		t.Fatalf("requestor 应持有 %s", PermTicketRate)
	}
	// 反向：AdminPermissions 不含 rate。
	if contains(AdminPermissions, PermTicketRate) {
		t.Fatalf("AdminPermissions 不应包含 %s", PermTicketRate)
	}
}

// TestAdminHoldsAdminPermissions 断言 admin 持有 AdminPermissions 中的每一个权限点。
func TestAdminHoldsAdminPermissions(t *testing.T) {
	for _, p := range AdminPermissions {
		if !Has(Admin, p) {
			t.Fatalf("admin 应拥有 %s", p)
		}
	}
}

func TestValidAndName(t *testing.T) {
	for _, r := range All() {
		if !Valid(r) {
			t.Fatalf("%s 应为合法角色", r)
		}
		if Name(r) == "" {
			t.Fatalf("%s 应有中文名", r)
		}
	}
	if Valid("ghost") {
		t.Fatalf("ghost 不应合法")
	}
	if Has("ghost", PermTicketCreate) {
		t.Fatalf("未知角色不应有任何权限")
	}
	if Permissions("ghost") != nil {
		t.Fatalf("未知角色权限应为 nil")
	}
}

func TestAllRoles(t *testing.T) {
	rs := AllRoles()
	if len(rs) != 7 {
		t.Fatalf("应 7 个角色，实际 %d", len(rs))
	}
	if rs[0].Role != Requestor || rs[6].Role != Admin {
		t.Fatalf("顺序不符: %s..%s", rs[0].Role, rs[6].Role)
	}
	for _, r := range rs {
		if len(r.Permissions) == 0 {
			t.Fatalf("%s 权限不应为空", r.Role)
		}
		for i := 1; i < len(r.Permissions); i++ {
			if r.Permissions[i-1] > r.Permissions[i] {
				t.Fatalf("%s 权限未排序", r.Role)
			}
		}
	}
	if len(Everyone) != 7 {
		t.Fatalf("Everyone 应 7 个")
	}
}

func TestPermissions_ReturnsCopy(t *testing.T) {
	a := Permissions(Agent)
	if len(a) == 0 {
		t.Fatalf("agent 权限不应为空")
	}
	a[0] = "mutated"
	b := Permissions(Agent)
	if b[0] == "mutated" {
		t.Fatalf("Permissions 应返回副本")
	}
}
