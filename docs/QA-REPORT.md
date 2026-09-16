# itsm-core 独立验证报告（QA-REPORT）

> 验证人：QA 工程师 严过关（software-qa-engineer）
> 验证日期：2026-09-16
> 验证对象：`itsm-core`（Go 1.22 + Gin + GORM + Viper + zap / Vue3 + TS + Vite）
> 判定基准：`docs/PRD.md`（§2.2 权限矩阵、§5 六大状态机、§5.2.1 SLA 口径、§8.6 双库约束）、`docs/ARCHITECTURE.md`、`docs/API.md`、`README.md`
> 证据文件：`internal/bootstrap/qa_verify_test.go`（QA 独立用例，未改动 `integration_test.go` 与任何业务源码）

---

## 1. 执行摘要

**结论：有条件发布。** 未发现 P0（阻断性/安全致命）缺陷；架构分层、构建卫生、双库静态合规、前后端路由一致性均通过独立验证；核心六大模块端到端主链路（HTTP → 中间件 → handler → service → repository → GORM → DB）实测走通。

本次独立验证**未采用「重跑开发自测看是否全绿」的方式**，而是自建 10 个对抗性用例（含 12 个子用例）攻击被测实现，并另做前后端路由交叉比对、权限矩阵逐格审计、六张状态机逐行映射、双库 10 条硬约束静态扫描、文档可检验声明实测。

**关键数字（均为本次实测，非转述）**

| 指标 | 实测值 | 命令 |
| --- | --- | --- |
| `go build ./...` | 通过 | `go build ./...` |
| `go vet ./...` | 通过（0 输出） | `go vet ./...` |
| `gofmt -l .` | 空（格式化合规） | `gofmt -l .` |
| `go test ./... -count=1 -cover` | 20 个包全绿（除下述 QA 用例） | `go test ./... -count=1 -cover` |
| **总覆盖率** | **73.1%**（`go tool cover -func` 语句总覆盖） | `go test ./... -coverprofile=/tmp/qa_cover.out` |
| `go test -race ./... -count=1` | **干净**，无 data race（各包 OK） | `go test -race ./... -count=1` |
| 前端 `npm run build`（含 `vue-tsc --noEmit` 类型检查） | **通过**，零 TS 报错，`✓ built in 7.29s` | `cd frontend && npm run build` |
| 生产二进制依赖含 sqlite | **0**（测试依赖未污染生产） | `go list -deps ./cmd/server | grep -c sqlite` |
| 前后端路由一致性（A） | **前端调用 97 条，后端注册 101 条，前端→后端缺失 = 0** | 见 §2.A |
| 本次 QA 用例 | 10 个用例（`TestQA_PRD_Deviations` 内含 3 个子用例）：9 通过、1 失败（3 子用例失败，均指向真实缺陷，见 §3） | `go test ./internal/bootstrap/ -run TestQA -v` |

**缺陷计数：P0 = 0，P1 = 2，P2 = 6。** 3 个失败断言分别对应 P1-1、P1-2、P2-1，有据可查、可复现。

---

## 2. 验证矩阵（A–G）

### A. 前后端路由一致性交叉核对（最高价值项）

**方法**：脚本从 `frontend/src/api/*.ts` 提取全部 `method + path`（模板参数归一为 `:p`），从 `internal/domain/*/routes.go` 与 `internal/bootstrap/routes.go` 提取全部真实注册路由，双向比对（含参数名/路径形状归一）。

**实际结果**：

| 表 | 数量 | 明细 |
| --- | --- | --- |
| 前端调用但后端**不存在**（生产 404，P0） | **0** | — |
| 后端注册但前端**从未使用**（中性） | **4** | `DELETE /incidents/:id/cis/:ciId`（incident/routes.go:30）、`GET /incidents/stats`（incident/routes.go:16）、`POST /service-items/:id/transition`（catalog/routes.go:27）、`GET /healthz`（bootstrap/routes.go:50，探活非业务，正常） |
| path 参数名/形状不一致 | **0** | 全部 RESTful 复数资源，参数占位符仅命名不同（前端 `${id}` ↔ 后端 `:id/:relId/:ciId`），无 `/ticket/:id` vs `/tickets/:id` 类形状冲突 |

**判定**：✅ **通过。无 P0 路由缺陷。** 4 条未被前端调用的后端路由中，3 条为后端已注册的能力（事件解除 CI、事件看板、服务项通用流转），前端未接线——属**中性**（后端能力冗余，不影响生产可用性），建议前端补接或后端按需保留。

**证据**：`frontend/src/api/{ticket,incident,change,problem,catalog,cmdb,asset,platform,auth}.ts` vs `internal/domain/{ticket,incident,change,problem,catalog,cmdb,asset,platform,auth}/routes.go`；`docs/API.md` 端点清单经比对与真实注册**完全一致**（含此前补登的 3 条：`GET /users/options`、`POST /service-items/:id/transition`、`DELETE /incidents/:id/cis/:ciId`）。

### B. 权限矩阵审计（对齐 PRD §2.2 + §5「触发角色」）

**方法 1（矩阵逐格）**：把 `internal/pkg/role/role.go` 的「角色→权限点」矩阵，逐格对照 PRD §2.2（23 行 × 7 角色）。

**实际结果**：**22/23 行完全吻合**；唯一不符：

| PRD 行 | PRD | 代码 | 判定 |
| --- | --- | --- | --- |
| 满意度评价（PRD.md:77） | admin **✗** | admin 拥有 `perm.ticket.rate`（role.go:75-84）+ `ticket/service.go:377` 显式放行 admin | ⚠️ 不符 → **P2-1** |

（`△` 在接口级表现为「授予权限点」，资源级约束由 service 兜底，符合 role.go 设计说明。）

**方法 2（路由装饰器 vs §5 触发角色）**：逐条核对 `RequirePerm/RequireRole`。

**实际结果**：路由级授权与 §5「触发角色」**大体一致**，发现 2 处**授权不一致**（路由放行但状态机拒绝）：

| 操作 | 路由授权 | 状态机实际允许 | PRD §5 | 判定 |
| --- | --- | --- | --- | --- |
| 事件升级 | `RequirePerm(perm.incident.escalate)`，`problem_manager` 拥有 → 放行（incident/routes.go:26） | `incident/machine.go:22` 仅 `role.Resolvers`(agent/resolver/admin) | §5.3「agent, resolver, **problem_manager**, admin」 | ❌ → **P1-1** |
| 变更提交风险评估 | 路由无角色限制 | `change/machine.go:13` 仅 `role.Requestor/Admin` | §5.5「**requester(工程师)**」 | ❌ → **P1-2** |

**失效权限点**（声明于矩阵但无任何路由/service 引用）：`PermTicketViewOwn`、`PermTicketClose`、`PermTicketReopen`、`PermChangeImplement`（`grep -rn "role.Perm" internal --include=*.go` 命中清单中缺失）。功能实际由状态机角色表兜底，**不造成越权**，但矩阵具误导性 → **P2-4**。

**资源级约束（仅本人/被指派/职责分离）**：

| 约束 | 是否有 service/guard 兜底 | 证据 |
| --- | --- | --- |
| 工单「仅本人查看/编辑」 | ✅ 有 | `ticket/service.go:708 canView`、`:716 canEdit` |
| 工单「撤销仅 requestor 本人」 | ✅ 有 | `ticket/machine.go:25 guardOwner` + `:68` |
| 工单「重开仅 requestor 且 ≤7 天」 | ✅ 有 | `ticket/machine.go:41 guardReopenWindow` |
| 工单「开始/退回仅被指派人」 | ✅ 有 | `ticket/machine.go:28-29 guardAssigneeOrAdmin` |
| 工单「挂起/恢复/解决仅被指派人」 | ❌ 仅校角色（Resolvers），未校 assignee | `ticket/machine.go:32-37`（无 assignee guard）→ **P2-2** |
| 事件「撤销仅本人」 | ✅ 有 | `incident/machine.go:14 guardOwner` |
| 变更「change_manager 仅审批不实施同一变更」 | ✅ 结构性成立 | 审批=`ChangeManagers`，实施=`Resolver/Admin`（`change/machine.go:22 vs :32,36`），角色集合互斥 |
| CMDB 自环/重复/退役清关系 | ✅ 有 | `cmdb/service.go:200/215/183` |

**判定**：⚠️ 矩阵级 1 处不符 + 路由/状态机授权 2 处不一致 + 4 个失效权限点 + 工单流转资源级校验偏宽。

### C. 状态机完备性审计（对齐 PRD §5 六张表）

**方法**：将 PRD §5 每张表「当前状态 + 允许操作 + 目标状态」逐行映射到 `*/machine.go`。

**实际结果**：

| 模块 | PRD 表行数 | PRD 有但代码缺失 | 代码有但 PRD 未定义 | 非法流转拒绝 |
| --- | --- | --- | --- | --- |
| 工单 §5.2 | 14 | 0 | 0 | ✅ `new→in_progress`、`resolved→assigned`、`draft→resolved` 均 409（`closed/cancelled` 不在表 → 全部 409） |
| 事件 §5.3 | 13 | 0 | 0 | ✅ 越级升级 409（`incident/machine.go:88`） |
| 问题 §5.4 | 9 | 0 | 0 | ✅ 终态拒绝 |
| 变更 §5.5 | 17 | 0 | 0 | ✅ 未审批不可实施；标准免审仅 `guardStandardOnly` |
| 服务目录 §5.6 | 9 | 0 | 0 | ✅ `archived` 终态 409 |
| 资产 §5.7 | 8 | 0 | 0 | ✅ `disposed` 终态 409 |

**触发角色的状态机内校验差异**（非缺失流转，而是「允许角色集合」与 §5 细微不符）：

| 流转 | 代码 Roles | PRD §5 触发角色 | 判定 |
| --- | --- | --- | --- |
| 工单 `assigned→new`（退回） | Resolvers + guardAssigneeOrAdmin（仅 assignee/admin） | assignee, **agent** | ⚠️ 非指派坐席被拒（偏严，P2 级，未单列） |
| 工单 挂起/恢复/解决 | Resolvers（含任意 resolver） | assignee, agent | ⚠️ 偏宽 → 并入 **P2-2** |
| 事件 `in_progress→escalated` | Resolvers（缺 problem_manager） | agent, resolver, **problem_manager**, admin | ❌ → **P1-1** |
| 事件 挂起/解决/接手 | Resolvers | assignee/agent | ⚠️ 偏宽（同 P2-2 性质） |
| 变更 `draft→assessment` | Requestor/Admin | requester(工程师) | ❌ → **P1-2** |

**判定**：✅ 六张状态机**无缺失流转、无未定义放行**；PRD §5 明确列出的非法流转**全部被 409 拒绝**（已独立复现）。⚠️ 但 4 处「允许角色集合」与 §5 不符（2 处偏严、2 处偏宽）。

### D. 双库兼容静态审计（PRD §8.6 十条硬约束）

**方法**：全 Go 源码（排除 `_test.go`）大小写不敏感扫描 `JSONB / jsonb / ILIKE / ON CONFLICT / now() / CURRENT_TIMESTAMP / SERIAL / BIGSERIAL / RETURNING / text[] / ::`；扫描全部 gorm tag 提取索引名长度；确认 UUID 存储形态；确认迁移未启用自动外键。

**实际结果**：

| 检查项 | 结果 | 证据 |
| --- | --- | --- |
| `JSONB/jsonb` | ✅ 无（仅注释提及） | 命中均为注释，如 `ticket/repository_gorm.go:3` |
| `ILIKE` | ✅ 无（注释说明改用 `LIKE`+应用层小写） | `catalog/repository_gorm.go:85`、`cmdb/repository_gorm.go:62`、`platform/repository_gorm.go:63` |
| `ON CONFLICT` | ✅ 无（改用「先查后写」） | `change/service.go:315`、`platform/service.go:260` |
| `now() / CURRENT_TIMESTAMP` | ✅ 无（时间由 Go 层传入） | 全源码无 SQL 时间函数 |
| `SERIAL/BIGSERIAL/RETURNING/text[]/::` | ✅ 无 | — |
| PG 数组类型 | ✅ 无 | 无非标量 gorm `type` tag |
| **索引名长度 ≤ 30** | ✅ 全部满足（最长 `idx_apv_change_approver`=23） | 全量 gorm tag 扫描；`ARCHITECTURE.md:874` 自检表 |
| UUID 存储 | ✅ 无 PG 原生 `uuid`（主键统一 `uint64 autoIncrement`） | 各 `model.go` 主键定义 |
| 迁移禁用自动外键 | ✅ | `pkg/database/database.go:73 DisableForeignKeyConstraintWhenMigrating: true`；测试侧 `integration_test.go:57` 同 |
| Dialector 切换 | ✅ 代码路径存在 | `pkg/database/database.go:26-39`（postgres/dameng），`config.go:253` 校验合法值 |

**判定**：✅ **通过**。代码层严格遵守 PRD §8.6 十条硬约束，**无方言专属构造**。⚠️ 但**未在真实达梦实例验证**（见 §5）。

### E. 对抗性功能用例（自建 `internal/bootstrap/qa_verify_test.go`）

**方法**：同包复用 `integration_test.go` 脚手架（真实 HTTP + 真实 GORM + 内存 SQLite），自建独立断言，**未修改** `integration_test.go` 与任何业务源码。

| # | 用例 | 方法 | 实际结果 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | 种子幂等回归（D1 锁） | `Migrate`+`Seed` **各连跑 2 次**；直查库断言用户数=8、`username=''`=0；admin 可登录 | 无重复键错误；`users=8`；`blank=0`；admin 登录成功 | ✅ 通过 |
| 2 | 8 账号真实登录 + 权限边界 | 逐一直登；requestor01→`/users`；cmdb01→变更审批 | 8/8 登录 200 且有 token；requestor01→**403**；cmdb01→**403** | ✅ 通过 |
| 3 | CAB 单人绕过（独立复现） | cm01 连投 2 票；再换 cm02 | 第 1 票 200；第 2 票（同人）**409**；状态仍 `pending_approval`；cm02 投→`approved` | ✅ 通过 |
| 4 | 工单 SLA 落库 | P1/P4 建单比对 `resolve_due_at-created_at`；挂起回拨 `paused_at` 后恢复 | P1≈**240min**、P4≈**4320min**；`paused_minutes` 增长、`due_at` 回补（均达标） | ✅ 通过 |
| 5 | 边界与错误码 | 不存在资源/非法 JSON/`page_size=100000`/无 token/不存在路径 | **404 / 400 / 钳制为100 / 401 / 404** | ✅ 通过 |
| 6 | 审计留痕 | 工单流转后 requestor 带实体范围查审计；再查全局 | 实体范围 **200** 且含 `from_status=new/to_status=assigned`；全局 **403** | ✅ 通过 |
| 7 | 事件优先级矩阵 | 5 组（高×高、低×低、中×中、高×低、中×高） | **P1 / P4 / P3 / P3 / P2**，与 §5.3.1 一致 | ✅ 通过 |
| 8 | CMDB 关系约束 | 自环；重复关系 | **400 / 409** | ✅ 通过 |
| 9 | 软删除 | 删除 CI 后查列表/详情/审计 | 列表不再返回、详情 **404**、审计仍可查 | ✅ 通过 |
| 10 | **PRD 偏差固化** | 按 PRD 期望断言 3 项 | 3 项全部失败 → 真实缺陷（P1-1、P1-2、P2-1） | ❌ 3 子用例失败（预期即缺陷） |

### F. 文档真实性审计

| 声明 | 方法 | 实际结果 | 判定 |
| --- | --- | --- | --- |
| README 8 演示账号（用户名/密码/角色） | 对照 `bootstrap/seed.go:97-105` + 实测登录 | `admin/requestor01/agent01/resolver01/pm01/cm01/cm02/cmdb01`，密码统一 `admin123`，角色一一对应 | ✅ 一致 |
| README/CONTRIBUTING 文件路径 | 逐一 `test -e` | 全部存在（LICENSE/CONTRIBUTING/CHANGELOG/.env.example/.golangci.yml/.editorconfig/Dockerfile/docker-compose.yml/.github/workflows/ci.yml/docs/*/configs/config.yaml 等） | ✅ 一致 |
| README 配置项（字段名/默认值） | 对照 `configs/config.yaml` + `internal/config/config.go` | 30+ 配置项字段名与默认值**全部吻合** | ✅ 一致 |
| README 快速开始命令 | `make help` 目标存在性 + compose 服务名/端口/环境变量 | Makefile 全 17 个 target 存在且 `make help` 正常；compose `postgres`/`app`、端口 5432/8080、`DB_DSN` host=postgres 与 README 一致（`docker compose` 本机无 Docker 未实跑） | ✅ 一致 |
| `docs/API.md` 端点清单 vs 真实路由 | 全文比对 | **完全一致**（含 3 条补登端点、`/audit-logs` 实体范围访问控制描述） | ✅ 一致 |
| README 工单状态机图 | 对照 `ticket/model.go` | README.md:77 写 `new → assigned → **processing** → pending`，实际状态常量为 **`in_progress`** | ❌ 文档过时 → **P2-3** |
| API.md §0.4「409 响应附带 `current_status/target_status`」 | 实测非法流转响应 | 实际 `data:null`，状态信息仅在 `message` 文本（`"非法流转 new -> close"`） | ❌ 文档不实 → **P2-5** |
| API.md §8 非法流转示例响应体 | 实测比对 | 示例 `{"code":40001,"message":"非法的状态流转","data":{"current_status":"new","target_status":"closed"}}` 与实现**不符**（message 与 data 均不同） | ❌ 文档不实 → **P2-6** |

### G. 独立复算覆盖率与构建卫生

| 检查 | 命令 | 实测结果 | 判定 |
| --- | --- | --- | --- |
| 总覆盖率 | `go tool cover -func=/tmp/qa_cover.out | tail -1` | **73.1%** | ✅ 与宣称一致（本次独立复算） |
| 竞态 | `go test -race ./... -count=1` | **干净**（20 包全 OK，含 `idgen` 并发） | ✅ |
| `go build ./...` | — | 通过 | ✅ |
| `go vet ./...` | — | 通过（0 输出） | ✅ |
| `gofmt -l .` | — | 空 | ✅ |
| 前端构建（含 TS 类型检查） | `npm run build`（`vue-tsc --noEmit && vite build`） | 通过，零 TS 报错 | ✅ |
| 生产依赖无 sqlite | `go list -deps ./cmd/server | grep -c sqlite` | **0** | ✅ |

---

## 3. 缺陷清单（P0 / P1 / P2）

> 复现统一入口：`export GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct && go test ./internal/bootstrap/ -run TestQA_PRD_Deviations -v`

### P0（阻断性 / 安全致命）— 0 条

本次独立验证**未发现 P0 缺陷**：无前端调用后端不存在的路由、无非法状态流转被放行、无越权致数据泄露、无构建/迁移失败。

### P1（高）— 2 条

**QA-01｜problem_manager 无法升级事件（路由放行、状态机拒绝）**
- 复现：登录 `pm01` → 对 `in_progress` 事件 `POST /api/v1/incidents/:id/escalate` → **403** `{"code":20003,"message":"当前角色无权执行该流转"}`
- 实际 vs 期望：实际 403；PRD §2.2「事件升级」problem_manager=✓、§5.3 触发角色含 `problem_manager` → 期望 200/`escalated`
- 根因：`incident/machine.go:22` `ActionEscalate` 的 `Roles` 为 `role.Resolvers`（=agent/resolver/admin），**漏 problem_manager**；而路由 `incident/routes.go:26` 挂 `perm.incident.escalate`（problem_manager 拥有）→ 路由授权与状态机授权**不一致**
- 建议转交：**software-engineer**（incident 域）
- 证据：`internal/bootstrap/qa_verify_test.go`（`problem_manager_escalate_PRD_allows`）；`internal/domain/incident/machine.go:22`；`internal/pkg/role/role.go:106-112`

**QA-02｜变更「提交风险评估」仅 admin 可执行，非 admin 走不通变更主流程**
- 复现：`agent01`（工程师，拥有 `perm.change.submit`）创建 normal 变更 → PUT 计划/回滚 → `POST /changes/:id/transition {"action":"submit_assessment"}` → **403**
- 实际 vs 期望：实际 403；PRD §5.5 `draft`→提交风险评估→触发角色「requester(工程师)」→ 期望 200/`assessment`
- 根因：`change/machine.go:13` `ActionSubmitAssessment` 的 `Roles` 仅 `{role.Requestor, role.Admin}`；而**创建变更者**是 agent/resolver/change_manager（`role.requestor` 无 `perm.change.submit`，无法建单）→ 真正的申请人与 change_manager 均被拒 → 该必经步骤**实际仅 admin 可推动**（开发自测正因如此用 admin 执行本步，掩盖了该缺陷）
- 影响：非 admin 无法完成「创建 → 风险评估 → 审批 → 排期」变更闭环
- 建议转交：**software-engineer**（change 域）
- 证据：`qa_verify_test.go`（`change_requester_submit_assessment_PRD_allows`）；`internal/domain/change/machine.go:13`；`internal/bootstrap/integration_test.go:431`（开发用 admin 执行本步）

### P2（中/低）— 6 条

**QA-03｜admin 可对工单满意度评分（PRD §2.2 明确 admin ✗）**
- 复现：`admin` 对 `resolved` 工单 `POST /tickets/:id/rating {"rating":5}` → **200**（写入成功）
- 实际 vs 期望：实际 200；PRD §2.2「满意度评价」admin=✗ → 期望 403
- 根因：`role.go:75-84` admin=`AllPermissions` 含 `perm.ticket.rate`；`ticket/service.go:377` 显式 `|| actor.Role == role.Admin` 放行
- 建议转交：**software-engineer**（role/ticket）
- 证据：`qa_verify_test.go`（`admin_cannot_rate_PRD_forbids`）；`internal/pkg/role/role.go:77`；`internal/domain/ticket/service.go:377`

**QA-04｜失效权限点（矩阵声明但无任何校验引用）**
- 明细：`PermTicketViewOwn`、`PermTicketClose`、`PermTicketReopen`、`PermChangeImplement`
- 影响：不造成越权（功能由状态机角色表兜底），但权限矩阵具误导性，且 `PermTicketViewOwn` 语义被 `!Has(PermTicketViewAll)` 隐式替代
- 建议转交：**software-engineer**（role.go）/ **software-architect**（矩阵语义确认）
- 证据：`grep -rn "role.Perm" internal --include=*.go` 命中清单；`internal/pkg/role/role.go:73-126`

**QA-05｜工单/事件「挂起·恢复·解决」缺乏「仅被指派人」资源级校验**
- 实际 vs 期望：任意 `resolver/agent/admin` 均可挂起/解决**他人被指派**的工单/事件（未校 `assignee`）；PRD §5.2/§5.3 触发角色为「assignee, agent」
- 根因：`ticket/machine.go:32-37`、`incident/machine.go:22-31` 仅 `role.Resolvers`，未挂 `guardAssigneeOrAdmin`
- 建议转交：**software-engineer**
- 证据：`internal/domain/ticket/machine.go:32-37`；`internal/domain/incident/machine.go:23-31`

**QA-06｜文档不实：README 工单状态机写 `processing`（应为 `in_progress`）**
- 证据：`README.md:77` vs `internal/domain/ticket/model.go:18`
- 建议转交：**software-engineer-5**（文档）

**QA-07｜文档不实：API.md 声称 409 响应附带 `current_status/target_status`（实际无）**
- 证据：`docs/API.md:87`、`docs/API.md:298` vs `internal/domain/ticket/service.go:332`（message 文本 + `data:null`）
- 建议转交：**software-engineer-5**（文档）；若坚持结构化字段则**software-engineer**（改 `ErrConflict` 携带 data）

**QA-08｜文档不实：API.md 非法流转示例响应体与实际不符**
- 证据：`docs/API.md:298`（编造 message/data）vs 实测 `{"code":40001,"message":"非法流转 new -> close","data":null}`
- 建议转交：**software-engineer-5**（文档）

> 说明：`docs/ARCHITECTURE.md` 附录 A（D1–D9）已**主动登记**若干实现期偏差（迁移方式、跨域适配器、新增 3 端点、审计访问控制、CAB 加固、8 账号、SQLite 测试依赖、资产 1:1），本次核对**属实**，不另列为缺陷（见 §4）。

---

## 4. 与 PRD 的偏差清单

### 4.1 实现缺陷（需修复，见 §3）
- **P1-1** problem_manager 无法升级事件（QA-01）
- **P1-2** 变更提交风险评估仅 admin 可执行（QA-02）
- **P2-1** admin 可评分工单（QA-03）
- **P2-2** 工单/事件挂起·解决缺 assignee 资源级校验（QA-05）

### 4.2 有意裁剪（**非缺陷**，建议文档显式声明）
| 项 | 说明 | 依据 |
| --- | --- | --- |
| `resolved → closed` 3 天自动关闭 | 无常驻调度器，未实现 | PRD §5.2 mermaid 注释，无独立 REQ 编号 |
| 登录失败锁定（5 次/10 分钟） | 配置项存在但未实现逻辑 | PRD §8.3 标 P1 |
| 事件看板统计（`GET /incidents/stats`） | 路由已注册，前端未接线 | PRD REQ-INC-008 标 P2 |
| 服务项目录需审批流 / 动态表单必填后端二次校验 | 部分实现 | PRD REQ-CAT-006/007 |
| 站内信/邮件通知 | 明确不做 | PRD §0 + Q3 默认方案 |
| 多租户 / SSO / 工作日历 | 明确不做 | PRD §0 范围边界 |
| 资产 CSV 批量导入 / 审计导出 | 未做 | PRD P2 |

### 4.3 与 §5 状态机的角色集合偏差（偏严/偏宽，已并入缺陷或以 P2 观察）
- 偏严：工单「退回」非指派坐席被拒（`ticket/machine.go:29`）——与 §5「assignee, agent」有出入，影响低。
- 偏宽：见 QA-05。

---

## 5. 未覆盖 / 无法验证项（**诚实清单**）

1. **达梦 DM8 方言级未验证**：本机无 DM8 实例。仅验证了 `Dialector` 构造、编译通过与 §8.6 静态合规；**DDL 方言、30 字符标识符限制、大小写/时区差异、真实建表与查询**均**未**实测。
2. **PostgreSQL 方言级未本地验证**：本机无 PG/Docker。CI 含 `integration-postgres` job（真实 `postgres:16`），但我无法在本机执行；本地端到端全部基于嵌入式 SQLite。
3. **SQLite 不等于 PG/DM**：SQLite 仅作 GORM 行为代理，**无法**代表生产库的并发、类型、时区、排序规则、JSON 处理差异。
4. **并发/性能未做压测**：PRD §8.1「列表 P95 ≤ 300ms、100 并发无明显错误」**未验证**；`-race` 仅覆盖测试用例并发，非生产负载。
5. **前端运行期未做 E2E**：仅验证 `npm run build`（类型+打包）通过；未在浏览器跑通交互、路由守卫、权限显隐、动态表单渲染。
6. **附件上传/下载（multipart）、文件大小边界**：未做端到端用例。
7. **JWT 过期/时钟偏斜、bcrypt 成本、CORS 实际跨域**：仅静态阅读，未做运行期攻击。
8. **达梦索引名 ≤ 30 的真实建表验证**：仅静态扫描 tag 长度，未在 DM8 建表确认。
9. **审计写入「best-effort」降级**：`ticket/service.go:681` 等审计失败仅告警，未验证审计不丢的强一致保证。
10. **种子跨域数据（service_categories/cis/ticket_categories）为 best-effort**：对应表未迁移时仅告警，未逐一验证边界。

---

## 6. 发布建议

**建议：有条件发布（Conditional Go）。**

**理由**
- ✅ 无 P0；核心六模块端到端主链路、鉴权/RBAC、状态机非法流转拒绝、SLA 计时、审计、软删除、种子幂等**均通过独立验证**；
- ✅ 构建卫生优秀（build/vet/gofmt/race/前端构建全绿；总覆盖 73.1%；生产二进制无 sqlite 污染）；
- ✅ 前后端路由一致（0 缺失）、双库硬约束静态合规、文档大体真实；
- ⚠️ 存在 2 个 P1 功能缺陷（变更主流程受 admin 单点制约；problem_manager 无法升级事件），影响变更/事件模块的角色可用性。

**发布前置条件**
1. **修复 P1-1、P1-2**（`software-engineer`：incident/change 域 `machine.go` 角色集合对齐 PRD §5）；或经产品确认后**降级为已知限制并写入 README「已知限制」**。
2. **修复/确认 P2-1（admin 评分）、P2-2（挂起/解决缺 assignee 校验）**。
3. **订正文档 P2-3/P2-4/P2-5/P2-6**（README 状态名、API.md 409 字段与示例）。
4. **达梦 DM8 上线前必须在真实实例执行一次完整人工验证**（清单见 `ARCHITECTURE.md §11.2`），并完成 PG 方言级回归。
5. 合并本次 `internal/bootstrap/qa_verify_test.go` 作为**回归锁**；其中 `TestQA_PRD_Deviations` 的 3 个失败断言在缺陷修复后应转绿。

> **附：关于失败用例的处理建议** —— `TestQA_PRD_Deviations` 的失败**不是用例缺陷，而是被测实现的真实缺陷**（已按 PRD 严格断言，未放宽）。修复对应源码后该用例即恢复通过；**不建议**通过放宽断言/删除用例使其变绿。当前状态：`go test ./...` 仅 `internal/bootstrap` 包因该用例失败，其余 19 包全绿。

---

### 附录：本次验证新增/变更文件
- 新增：`internal/bootstrap/qa_verify_test.go`（QA 独立用例，同包复用脚手架，未改 `integration_test.go`）
- 新增：`docs/QA-REPORT.md`（本报告）
- 未改动任何业务源码、`cmd/**`、`configs/**`、`frontend/**`、`docs/{PRD,ARCHITECTURE,TASKS,API}.md`、`README.md`、`CHANGELOG.md`、`.github/**`、`go.mod`、`go.sum`、`Makefile`
