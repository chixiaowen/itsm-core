# itsm-core REST API 参考

> 本文档以 `docs/ARCHITECTURE.md` §5「REST API 接口清单」为基础，并按**真实代码注册的路由**补齐。
> 版本：v0.1.0　|　基础路径：`/api/v1`

---

## 与 `docs/ARCHITECTURE.md` §5 的差异说明

以下 3 个端点**已由代码真实注册**（见各域 `routes.go`），但 ARCHITECTURE §5 的接口表未列出。
为保证对外 API 参考与实现一致，本文件将其补入对应模块表，并说明其后加原因：

| 端点 | 注册位置 | 为何后加 |
| --- | --- | --- |
| `GET /api/v1/users/options` | `internal/domain/platform/routes.go:16` | 供前端各处「指派 / 选择处理人」下拉复用。属**只读、非敏感**的候选列表，故**不要求 admin**——任意已登录用户可访问（与 `/users` 的 admin 管控区分）。已在 SQL 层脱敏，仅返回 `id / display_name / role` 三字段，严禁扩展敏感字段。 |
| `POST /api/v1/service-items/:id/transition` | `internal/domain/catalog/routes.go:27` | 服务项状态机除 `publish / offline` 两个快捷入口外，还需一个**通用人工流转**入口以覆盖 `submit_review`、`reject`（驳回需填原因）等动作，故补注册。 |
| `DELETE /api/v1/incidents/:id/cis/:ciId` | `internal/domain/incident/routes.go:30` | 事件与 CI 的关联需可**解除**（PRD REQ-INC-004「关联关系可解除」）。§5.3 仅列了建立关联的 `POST /incidents/:id/cis`，遗漏了解除接口。 |

> 除上表 3 条外，其余所有端点与 ARCHITECTURE §5 完全一致（未增删）。

**信息性说明（前端接入情况）**：QA 双向比对（前端调用 97 条 / 后端注册 101 条）结论——前端调用了但后端**不存在**的路由为 **0**；
以下 **4 条**端点后端已注册但**当前前端未接入**（不影响后端契约有效性）：
`DELETE /incidents/:id/cis/:ciId`、`GET /incidents/stats`、`POST /service-items/:id/transition`、`GET /healthz`。

---

## 0. 通用约定

- **前缀**：所有业务接口位于 `/api/v1` 之下（`/healthz` 除外）。
- **鉴权**：除 `POST /api/v1/auth/login` 与 `GET /healthz` 外，**全部**接口需要请求头
  `Authorization: Bearer <jwt>`。
- **实体范围访问控制（`GET /audit-logs`）**：审计查询的鉴权分两档——
  - 带**实体范围**（`entity_type`/别名 `biz_type` **与** `entity_id`/别名 `biz_id` **同时提供**）：
    任意已登录用户可读。实体自身的状态流转历史与「详情接口」同级可见，供前端详情页组装时间线。
  - **不带实体范围**的全局审计浏览：要求权限点 `perm.audit.view`（当前仅 `admin` 拥有），否则 `403 / 20003`。
  - 安全红线：实体范围必须「类型 + ID 同时提供」，仅传 `entity_type` 不足以放宽到任意登录用户。
- **内容类型**：请求 / 响应均为 `application/json`（附件上传为 `multipart/form-data`）。
- **时间格式**：RFC3339 UTC。
- **字段命名**：`snake_case`。

### 0.1 统一响应体

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

- `code`：业务错误码，`0` 表示成功。
- `message`：可直接展示给用户的提示。
- `data`：业务数据；无内容时为 `null`。

### 0.2 统一分页结构

列表类接口的 `data` 固定为：

```json
{
  "total": 100,
  "page": 1,
  "page_size": 20,
  "items": []
}
```

### 0.3 分页与排序参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `page` | `1` | 页码，从 1 开始 |
| `page_size` | `20` | 每页条数，**上限 100（超出自动截断为 100）** |
| `sort_by` | 由各接口约定 | 排序字段（白名单校验） |
| `order` | 由各接口约定 | `asc` / `desc` |

### 0.4 错误响应体与错误码 ↔ HTTP 映射

错误响应沿用统一响应体，且 **`data` 恒为 `null`**——错误详情**不**以结构化字段（如 `current_status` / `target_status`）返回，
状态信息只出现在 `message` 文本中：

```json
{ "code": 40001, "message": "非法流转 current=new -> target=in_progress: 跳过分派", "data": null }
```

- **409（`code=40001`，状态冲突 / 非法流转 / 重复操作）**：`message` 文本包含「当前状态 → 目标状态」状态对，
  统一格式为 `非法流转 current=<from> -> target=<to>: <原因>`。
- **422（`code=40002`，业务前置条件不满足）**：`message` 为具体前置条件说明（如「解决方案必填」「驳回必须填写原因」）。
- 除 `code` / `message` / `data` 外，**不存在**其他字段（尤其**没有** `current_status` / `target_status`）。

| code | HTTP | 含义 | `message` 是否含状态对 |
| --- | --- | --- | --- |
| 0 | 200 | 成功 | — |
| 10001 | 400 | 参数校验失败 | 否 |
| 10002 | 400 | 自环 / 非法关系等业务参数错误 | 否 |
| 20001 | 401 | 未登录 / token 无效或过期 | 否 |
| 20003 | 403 | 越权 | 否 |
| 30001 | 404 | 资源不存在 | 否 |
| 40001 | 409 | 状态冲突 / 非法流转 / 重复操作 | **是**（非法流转类） |
| 40002 | 422 | 业务前置条件不满足 | 否 |
| 50000 | 500 | 服务器内部错误 | 否 |

> 上表中 `data` 一列对错误响应一律为 `null`。`DELETE` 成功响应为 **HTTP 200 + `data:null`**（非 204）。

### 0.5 角色与权限

内置角色：`requestor`（终端用户）/ `agent`（服务台坐席）/ `resolver`（二线工程师）/
`problem_manager`（问题经理）/ `change_manager`（变更经理）/ `cmdb_manager`（配置管理员）/ `admin`（系统管理员）。

接口级鉴权按下表「权限」列校验；资源级约束（`requestor` 仅本人数据、`resolver` 仅被指派数据、
职责分离）由 service 层实现，越权统一返回 `403 / 20003`。

- **登录**：已登录用户即可访问。
- **agent / resolver / admin 等**：表示要求对应角色（`admin` 通常等价拥有该权限）。
- **状态机**：流转权限由状态机定义（`internal/pkg/statemachine`，详见 `docs/ARCHITECTURE.md` §6）。

---

## 1. auth / platform（SYS）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 响应 data | 需求 |
| --- | --- | --- | --- | --- | --- | --- |
| POST | `/auth/login` | 登录签发 JWT | 公开 | `{username,password}` | `{token,expires_at,user}` | SYS-001 |
| GET | `/auth/me` | 当前用户 | 登录 | — | `User` | SYS-001 |
| POST | `/auth/logout` | 登出（前端清 token） | 登录 | — | `null` | SYS-001 |
| GET | `/users/options` | 候选用户下拉（指派/选处理人） | **登录**（不限 admin） | `?role=` | `[{id,display_name,role}]`（SQL 层脱敏，仅 3 字段；仅返回 active） | 见差异表 |
| GET | `/users` | 用户列表 | admin | `?page&page_size&role&keyword` | 分页 `User` | SYS-002 |
| POST | `/users` | 新建用户 | admin | `{username,display_name,role,password,email}` | `User` | SYS-002 |
| GET | `/users/:id` | 用户详情 | admin | — | `User` | SYS-002 |
| PUT | `/users/:id` | 编辑用户（含改角色/密码） | admin | `{display_name?,role?,password?,email?,status?}` | `User` | SYS-002 |
| DELETE | `/users/:id` | 软删除用户 | admin | — | `null` | SYS-002/004 |
| GET | `/roles` | 角色与权限点矩阵 | admin | — | `[{role,name,permissions[]}]` | SYS-002 |
| GET | `/sla-policies` | SLA 策略列表 | admin | — | `[SLAPolicy]` | TKT-005 |
| POST | `/sla-policies` | 新建策略 | admin | `{name,priority,response_minutes,resolve_minutes,pause_on_pending}` | `SLAPolicy` | TKT-005 |
| PUT | `/sla-policies/:id` | 编辑策略 | admin | 同上 | `SLAPolicy` | TKT-005 |
| DELETE | `/sla-policies/:id` | 删除策略 | admin | — | `null` | TKT-005 |
| GET | `/audit-logs` | 审计查询 | 带实体范围→登录；全局→admin(`perm.audit.view`) | `?actor_id&biz_type&entity_type&biz_id&entity_id&action&from&to&page` | 分页 `AuditLog` | SYS-003 |
| GET | `/comments` | 评论/时间线（按 biz） | 登录 | `?biz_type&biz_id&include_internal` | `[Comment]` | TKT-006 |
| POST | `/comments` | 新增评论/内部备注 | 登录 | `{biz_type,biz_id,content,is_internal}` | `Comment` | TKT-006 |
| POST | `/attachments` | 上传附件（multipart） | 登录 | `file` + `biz_type,biz_id` | `Attachment`（≤20MB） | TKT-010 |
| GET | `/attachments/:id/download` | 下载附件 | 登录（软删除实体拦截） | — | 文件流 | TKT-010 |
| DELETE | `/attachments/:id` | 删除附件 | 上传者/admin | — | `null` | TKT-010 |

---

## 2. ticket（TKT）

| 方法 | 路径 | 用途 | 权限 | 请求体（关键） | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/tickets` | 列表（状态/优先级/分类/指派/时间/超期/关键字） | 登录（requestor 仅本人） | `?status&priority&category_id&assignee_id&sla_status&from&to&keyword&page&sort_by&order` | TKT-011 |
| POST | `/tickets` | 创建工单 | 登录 | `{title,description,category_id,priority?,requester_id?,type?,source_incident_id?,service_item_id?,form_data?}` → 返回 `code` | TKT-001 |
| GET | `/tickets/:id` | 详情（含时间线/CI/附件/服务项） | 登录（本人或坐席） | — | TKT-003/005/006 |
| PUT | `/tickets/:id` | 编辑（draft/new/assigned 可改字段） | 坐席/admin/本人 | `{title?,description?,category_id?,priority?}` | TKT-002/003 |
| DELETE | `/tickets/:id` | 软删除 | admin | — | SYS-004 |
| POST | `/tickets/:id/transition` | **统一状态流转**（action） | 状态机（ARCHITECTURE §6） | `{action, solution?, reason?, assignee_id?, priority?}` | TKT-007/008/012 |
| POST | `/tickets/:id/assign` | 指派（快捷） | agent/admin | `{assignee_id}` | TKT-004 |
| POST | `/tickets/:id/rating` | 满意度评价（仅一次） | requestor | `{rating,comment?}` | TKT-009 |
| POST | `/tickets/:id/comment` | 公开回复（触发首次响应时间） | 登录 | `{content,is_internal}` | TKT-006 |
| POST | `/tickets/:id/cis` | 关联 CI | agent/resolver | `{ci_ids[]}` | CMDB-006 |
| DELETE | `/tickets/:id/cis/:ciId` | 解除 CI | agent/resolver | — | CMDB-006 |
| GET | `/ticket-categories` | 分类树 | 登录 | — | TKT-002 |
| POST | `/ticket-categories` | 新建分类 | admin | `{name,parent_id?,sort_order}` | TKT-002 |
| PUT | `/ticket-categories/:id` | 编辑分类 | admin | 同上 | TKT-002 |
| DELETE | `/ticket-categories/:id` | 删除分类（含子/含工单拒绝 409） | admin | — | TKT-002 |

> `POST /tickets` 的 `type` 取值枚举：`manual`（手动）/ `service`（服务目录下单）/ `incident`（事件转单）；`priority` 为 `P1`~`P4`。

---

## 3. incident（INC）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/incidents` | 列表 | 登录 | `?status&priority&impact&urgency&escalation_level&from&to&page` | INC-008 |
| POST | `/incidents` | 上报 | 登录 | `{title,description,impact,urgency,occurred_at?,reporter_id?,ci_ids[]}` | INC-001 |
| GET | `/incidents/:id` | 详情（含升级历史/关联工单/CI） | 登录 | — | INC-003/004/005 |
| PUT | `/incidents/:id` | 编辑 | agent/admin | `{title?,description?,impact?,urgency?}` | INC-001 |
| DELETE | `/incidents/:id` | 软删除 | admin | — | SYS-004 |
| POST | `/incidents/:id/transition` | 状态流转 | 状态机（ARCHITECTURE §6） | `{action, solution?, reason?, assignee_id?, review_conclusion?}` | INC-006 |
| POST | `/incidents/:id/priority` | 人工覆盖优先级 | agent+ | `{priority}` → `priority_overridden=true` | INC-002 |
| POST | `/incidents/:id/escalate` | 升级（功能/层级，+1） | agent/resolver/problem_manager/admin | `{type, reason, to_assignee_id?, level?}` | INC-005 |
| POST | `/incidents/:id/convert-to-ticket` | 一键转工单 | agent/resolver/admin | `{title?}` → 返回 ticket | INC-003 |
| POST | `/incidents/:id/link-ticket` | 关联已有工单 | agent/resolver | `{ticket_id}` | INC-004 |
| POST | `/incidents/:id/cis` | 关联 CI | agent/resolver | `{ci_ids[]}` | CMDB-006 |
| DELETE | `/incidents/:id/cis/:ciId` | 解除 CI 关联 | agent/resolver/admin | — | CMDB-006（见差异表） |
| GET | `/incidents/priority-matrix` | 9 宫格矩阵（前端展示） | 登录 | — | INC-002 |
| GET | `/incidents/stats` | 看板统计（P2） | 登录 | — | INC-008 |

---

## 4. problem（PRB）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/problems` | 列表 | problem_manager/admin（△ resolver 只读） | `?status&assignee_id&known_error&page` | PRB-003 |
| POST | `/problems` | 新建/聚合 | problem_manager/admin | `{title,description,source,incident_ids[]}` | PRB-001/005 |
| GET | `/problems/:id` | 详情（RCA/关联事件/关联变更） | 登录只读 | — | PRB-002/004 |
| PUT | `/problems/:id` | 编辑/RCA | problem_manager/admin | `{title?,description?,symptom?,analysis?,root_cause?,workaround?}` | PRB-002 |
| DELETE | `/problems/:id` | 软删除 | admin | — | SYS-004 |
| POST | `/problems/:id/transition` | 流转 | 状态机（ARCHITECTURE §6） | `{action, workaround?, no_change_reason?, reason?}` | PRB-003/006 |
| POST | `/problems/:id/known-error` | 标记已知错误 | problem_manager/admin | `{root_cause,workaround}` → 422 if 空 | PRB-003 |
| POST | `/problems/:id/changes` | 关联变更 | problem_manager/admin | `{change_ids[]}` | PRB-004 |
| DELETE | `/problems/:id/changes/:changeId` | 解除关联 | problem_manager/admin | — | PRB-004 |
| GET | `/problems/aggregate-suggestions` | 「建议聚合」提示 | problem_manager/admin | `?ci_id&days=30` | PRB-001 |

---

## 5. change（CHG）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/changes` | 列表 | 登录 | `?status&change_type&risk_level&manager_id&window_from&window_to&page` | CHG-003 |
| POST | `/changes` | 提交申请 | 登录（工程师） | `{title,description,change_type,risk_level}` | CHG-001/002 |
| GET | `/changes/:id` | 详情（含审批记录/时间线） | 登录 | — | CHG-005 |
| PUT | `/changes/:id` | 编辑（仅 draft） | requester/change_manager | `{title?,description?,change_type?,risk_level?,impact_analysis?,plan?,rollback_plan?,window_start?,window_end?}` | CHG-002/003/004/006 |
| DELETE | `/changes/:id` | 软删除 | admin | — | SYS-004 |
| POST | `/changes/:id/transition` | 流转 | 状态机（ARCHITECTURE §6） | `{action, result?, reason?, comment?, conclusion?, window_start?, window_end?, confirm_out_of_window?}` | CHG-004~009 |
| POST | `/changes/:id/approvals` | CAB 审批 | change_manager/admin | `{decision,comment}` | CHG-005 |
| GET | `/changes/:id/approvals` | 审批记录 | 登录 | — | CHG-005 |

---

## 6. catalog（CAT）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/service-categories` | 分类树 | 登录 | — | CAT-002 |
| POST | `/service-categories` | 新建分类 | admin | `{name,parent_id?,sort_order}` | CAT-002 |
| PUT | `/service-categories/:id` | 编辑/排序 | admin | 同上 | CAT-002 |
| DELETE | `/service-categories/:id` | 删除（含子/含服务项 409） | admin | — | CAT-002 |
| GET | `/service-items` | 服务项列表（管理台） | admin | `?status&category_id&page` | CAT-001 |
| POST | `/service-items` | 新建服务项 | admin | `{name,description,category_id,sla_policy_id,default_priority,requires_approval,form_schema}` | CAT-001 |
| GET | `/service-items/:id` | 详情 | 登录 | — | CAT-001 |
| PUT | `/service-items/:id` | 编辑（published 编辑即退回 draft） | admin | 同 POST | CAT-003 |
| DELETE | `/service-items/:id` | 归档（draft/offline→archived；否则 409） | admin | — | CAT-003 |
| POST | `/service-items/:id/publish` | 发布/重新上架 | admin | — | CAT-003 |
| POST | `/service-items/:id/offline` | 下线 | admin | — | CAT-003 |
| POST | `/service-items/:id/transition` | 服务项状态机人工流转（`submit_review`/`reject`/`edit`/`archive`/`republish` 等；`reject` 须填 `reason`） | admin(`perm.catalog.manage`) | `{action,reason?}` | CAT-003（见差异表） |
| GET | `/catalog/categories` | 用户侧分类（仅含 published） | 登录 | — | CAT-003 |
| GET | `/catalog/items` | 用户侧服务项（仅 published） | 登录 | `?category_id&keyword` | CAT-003 |
| POST | `/catalog/items/:id/order` | 下单（动态表单校验→生成工单） | 登录 | `{form_data:{...}, title?}` → 返回 ticket | CAT-004/005/006 |

---

## 7. cmdb / asset（CMDB）

| 方法 | 路径 | 用途 | 权限 | 请求体 | 需求 |
| --- | --- | --- | --- | --- | --- |
| GET | `/cis` | CI 列表 | cmdb_manager/admin | `?ci_type&status&owner_id&keyword&page` | CMDB-001 |
| POST | `/cis` | 新建 CI | cmdb_manager/admin | `{code,name,ci_type,status,owner_id,attrs{}}` | CMDB-001 |
| GET | `/cis/:id` | CI 详情（含关系/关联工单事件） | 登录只读 | — | CMDB-002 |
| PUT | `/cis/:id` | 编辑 CI | cmdb_manager/admin | 同 POST | CMDB-001 |
| DELETE | `/cis/:id` | 软删除（有未清关系 409） | cmdb_manager/admin | — | CMDB-003 |
| GET | `/cis/:id/topology` | 拓扑展开 | 登录 | `?depth=2&direction=both` | CMDB-007 |
| POST | `/cis/:id/relations` | 新增关系（自环 400 / 重复 409） | cmdb_manager/admin | `{target_ci_id,relation_type}` | CMDB-002/003 |
| DELETE | `/cis/:id/relations/:relId` | 删除关系 | cmdb_manager/admin | — | CMDB-002 |
| GET | `/ci-types` | CI 类型枚举 | 登录 | — | CMDB-001 |
| GET | `/assets` | 资产台账 | cmdb_manager/admin | `?category&status&user_id&warranty_before&page` | CMDB-003 |
| POST | `/assets` | 新建资产 | cmdb_manager/admin | `{asset_no,name,category,vendor,purchase_date,warranty_end}` | CMDB-003 |
| GET | `/assets/:id` | 详情（含生命周期历史/绑定 CI） | cmdb_manager/admin | — | CMDB-003 |
| PUT | `/assets/:id` | 编辑资产 | cmdb_manager/admin | 同上 | CMDB-003 |
| DELETE | `/assets/:id` | 软删除 | cmdb_manager/admin | — | SYS-004 |
| POST | `/assets/:id/transition` | 生命周期流转 | cmdb_manager/admin | `{action, remark?, user_id?, location?, ci_id?}` | CMDB-004 |
| GET | `/assets/:id/history` | 生命周期历史 | cmdb_manager/admin | — | CMDB-004 |
| POST | `/assets/:id/bind-ci` | 绑定 CI（1:1） | cmdb_manager/admin | `{ci_id}` | CMDB-005 |
| DELETE | `/assets/:id/bind-ci` | 解绑 | cmdb_manager/admin | — | CMDB-005 |
| GET | `/healthz` | 探活（含 DB ping） | 公开 | — | SYS-009 |

---

## 8. 示例

### 登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'
```

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "eyJhbGciOi...",
    "expires_at": "2026-09-17T02:00:00Z",
    "user": { "id": 1, "username": "admin", "display_name": "系统管理员", "role": "admin" }
  }
}
```

### 创建工单

```bash
curl -X POST http://localhost:8080/api/v1/tickets \
  -H 'Authorization: Bearer <jwt>' \
  -H 'Content-Type: application/json' \
  -d '{"title":"无法连接 VPN","description":"报错超时","category_id":3,"priority":"P3"}'
```

### 状态流转（解决工单）

```bash
curl -X POST http://localhost:8080/api/v1/tickets/1/transition \
  -H 'Authorization: Bearer <jwt>' \
  -H 'Content-Type: application/json' \
  -d '{"action":"resolve","solution":"重置账号密码"}'
```

非法流转返回（409 / `40001`；状态对在 `message` 文本中，`data` 恒为 `null`）：

```json
{ "code": 40001, "message": "非法流转 current=new -> target=in_progress: 跳过分派", "data": null }
```

> `message` 中「当前状态 → 目标状态」对的格式为 `非法流转 current=<from> -> target=<to>: <原因>`；
> 响应体**没有** `current_status` / `target_status` 字段。
