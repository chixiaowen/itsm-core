// Package role 定义 7 个内置角色、权限点常量，以及「角色 -> 权限点」矩阵。
//
// 矩阵逐条对齐 PRD §2.2；权限点命名遵循 `perm.<domain>.<action>`。
// 说明：PRD 中的「△ 部分允许」在接口级表现为**授予该权限点**，
// 资源级约束（仅本人 / 仅被指派 / 职责分离）由各域 service 的 canAccess 兜底。
//
// 权限点的覆盖范围：**仅用于「独立端点」的路由级鉴权**（如 POST /tickets、POST /tickets/:id/rating）。
// 工单/事件/变更/问题等状态机类动作统一走通用端点 `POST /xxx/:id/transition`，
// 无法按单个动作挂不同权限点，其**角色约束由各域状态机表（machine.go）的 Roles 表达**。
// 因此不应为 transition 动作单列权限点（避免出现「已声明但无任何引用」的失效权限点）。
//
// 权限点全集与 admin 授权（防退化规则）：
//   - AllPermissions 是「系统定义的全部权限点」全集（不变量：新增权限点必须加入），≠ admin 实际持有集；
//   - AdminPermissions 是 admin 实际持有集 = AllPermissions 去掉 withheld 项；
//   - 新增权限点**默认授予 admin**；仅当 PRD §2.2 判定 admin 为 ✗ 时，才将其加入 withheld，
//     而**不是**从 AllPermissions 中删除（否则 AllPermissions 名不副实）。
package role

import "sort"

// 7 个内置角色常量。
const (
	Requestor      = "requestor"
	Agent          = "agent"
	Resolver       = "resolver"
	ProblemManager = "problem_manager"
	ChangeManager  = "change_manager"
	CmdbManager    = "cmdb_manager"
	Admin          = "admin"
)

// 角色组合：供各域状态机声明「允许角色」使用（只读，勿修改）。
var (
	// Agents = agent + admin（受理/指派等）。
	Agents = []string{Agent, Admin}
	// Resolvers = agent + resolver + admin（处理/流转/解决）。
	Resolvers = []string{Agent, Resolver, Admin}
	// ProblemManagers = problem_manager + admin。
	ProblemManagers = []string{ProblemManager, Admin}
	// ChangeManagers = change_manager + admin。
	ChangeManagers = []string{ChangeManager, Admin}
	// CmdbManagers = cmdb_manager + admin。
	CmdbManagers = []string{CmdbManager, Admin}
	// Requestors = requestor + admin。
	Requestors = []string{Requestor, Admin}
	// Everyone = 全部 7 个角色。
	Everyone = []string{Requestor, Agent, Resolver, ProblemManager, ChangeManager, CmdbManager, Admin}
)

// 权限点常量（perm.<domain>.<action>）。
const (
	// 工单
	PermTicketCreate  = "perm.ticket.create"
	PermTicketViewAll = "perm.ticket.view_all"
	PermTicketAssign  = "perm.ticket.assign"
	PermTicketHandle  = "perm.ticket.handle"
	PermTicketRate    = "perm.ticket.rate"
	// 事件
	PermIncidentReport   = "perm.incident.report"
	PermIncidentEscalate = "perm.incident.escalate"
	PermIncidentConvert  = "perm.incident.convert"
	// 问题
	PermProblemCreate = "perm.problem.create"
	PermProblemRCA    = "perm.problem.rca"
	// 变更
	PermChangeSubmit  = "perm.change.submit"
	PermChangeApprove = "perm.change.approve"
	// 服务目录
	PermCatalogManage = "perm.catalog.manage"
	PermCatalogOrder  = "perm.catalog.order"
	// 配置管理
	PermCmdbManage = "perm.cmdb.manage"
	// 资产
	PermAssetManage = "perm.asset.manage"
	// 平台
	PermSlaManage  = "perm.sla.manage"
	PermUserManage = "perm.user.manage"
	PermAuditView  = "perm.audit.view"
)

// AllPermissions 是系统中定义的全部权限点（不变量：任何新增权限点都必须加入此列表）。
//
// 注意：它**不等于** admin 实际拥有的权限集——个别权限点会依 PRD §2.2 刻意不授予 admin，
// 见 AdminPermissions。
var AllPermissions = []string{
	PermTicketCreate, PermTicketViewAll, PermTicketAssign,
	PermTicketHandle, PermTicketRate,
	PermIncidentReport, PermIncidentEscalate, PermIncidentConvert,
	PermProblemCreate, PermProblemRCA,
	PermChangeSubmit, PermChangeApprove,
	PermCatalogManage, PermCatalogOrder,
	PermCmdbManage, PermAssetManage,
	PermSlaManage, PermUserManage, PermAuditView,
}

// AdminPermissions 是 admin 实际拥有的权限点 = AllPermissions 去掉刻意保留项。
//
// 目前刻意保留（admin 不持有）的仅有 PermTicketRate：
// PRD §2.2 中 admin 对「满意度评价」为 ✗（评价语义为「请求人对解决结果的确认」，不应由管理员代持）。
var AdminPermissions = func() []string {
	withheld := map[string]bool{PermTicketRate: true}
	out := make([]string, 0, len(AllPermissions))
	for _, p := range AllPermissions {
		if !withheld[p] {
			out = append(out, p)
		}
	}
	return out
}()

// rolePermissions 是「角色 -> 权限点」矩阵（对齐 PRD §2.2）。
var rolePermissions = map[string][]string{
	Requestor: {
		PermTicketCreate, PermTicketRate,
		PermIncidentReport, PermCatalogOrder,
	},
	Agent: {
		PermTicketCreate, PermTicketViewAll, PermTicketAssign,
		PermTicketHandle,
		PermIncidentReport, PermIncidentEscalate, PermIncidentConvert,
		PermProblemCreate, PermChangeSubmit, PermCatalogOrder,
	},
	Resolver: {
		PermTicketCreate, PermTicketViewAll, PermTicketAssign,
		PermTicketHandle,
		PermIncidentReport, PermIncidentEscalate, PermIncidentConvert,
		PermProblemCreate, PermProblemRCA,
		PermChangeSubmit,
		PermCatalogOrder, PermCmdbManage,
	},
	ProblemManager: {
		PermTicketCreate, PermTicketViewAll,
		PermIncidentReport, PermIncidentEscalate,
		PermProblemCreate, PermProblemRCA,
		PermChangeSubmit, PermChangeApprove,
		PermCatalogOrder, PermCmdbManage,
	},
	ChangeManager: {
		PermTicketCreate, PermTicketViewAll,
		PermIncidentReport,
		PermChangeSubmit, PermChangeApprove,
		PermCatalogOrder,
	},
	CmdbManager: {
		PermTicketCreate, PermTicketViewAll,
		PermIncidentReport,
		PermChangeSubmit, PermCatalogOrder,
		PermCmdbManage, PermAssetManage,
	},
	Admin: AdminPermissions,
}

// roleNames 是角色中文名（对齐 PRD §2.1）。
var roleNames = map[string]string{
	Requestor:      "终端用户",
	Agent:          "服务台坐席",
	Resolver:       "二线工程师",
	ProblemManager: "问题经理",
	ChangeManager:  "变更经理",
	CmdbManager:    "配置管理员",
	Admin:          "系统管理员",
}

// roleOrder 固定角色展示顺序。
var roleOrder = []string{Requestor, Agent, Resolver, ProblemManager, ChangeManager, CmdbManager, Admin}

// permissionSet 是矩阵的 O(1) 索引。
var permissionSet = func() map[string]map[string]bool {
	set := make(map[string]map[string]bool, len(rolePermissions))
	for r, perms := range rolePermissions {
		m := make(map[string]bool, len(perms))
		for _, p := range perms {
			m[p] = true
		}
		set[r] = m
	}
	return set
}()

// RoleInfo 是角色及其权限点（供 GET /roles 返回）。
type RoleInfo struct {
	Role        string   `json:"role"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// Valid 判断角色是否为内置角色之一。
func Valid(r string) bool {
	_, ok := rolePermissions[r]
	return ok
}

// Has 判断角色是否拥有某权限点。
func Has(r, perm string) bool {
	perms, ok := permissionSet[r]
	if !ok {
		return false
	}
	return perms[perm]
}

// Permissions 返回角色权限点的副本（已排序）。
func Permissions(r string) []string {
	perms, ok := rolePermissions[r]
	if !ok {
		return nil
	}
	out := make([]string, len(perms))
	copy(out, perms)
	sort.Strings(out)
	return out
}

// Name 返回角色中文名。
func Name(r string) string { return roleNames[r] }

// AllRoles 返回全部角色及其权限点（顺序固定），供 GET /roles 使用。
func AllRoles() []RoleInfo {
	out := make([]RoleInfo, 0, len(roleOrder))
	for _, r := range roleOrder {
		out = append(out, RoleInfo{Role: r, Name: roleNames[r], Permissions: Permissions(r)})
	}
	return out
}

// All 返回全部角色常量（顺序固定）。
func All() []string {
	out := make([]string, len(roleOrder))
	copy(out, roleOrder)
	return out
}
