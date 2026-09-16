# Changelog

本项目的所有重要变更都会记录在此文件。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本（Semantic Versioning）](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Planned

- 工单模板、批量指派 / 批量关闭（TKT-013/014）。
- 事件看板统计（INC-008）。
- 资产批量导入 CSV（CMDB-008）。
- 审计日志导出 CSV（SYS-010）。

---

## [0.1.0] - 2026-09-16

首个版本。交付 ITSM/ITIL 六大核心模块的 P0 闭环、双数据库支持、前端管理台与完整工程化文档。

### Added

**工单管理（Ticket）**

- 工单创建 / 编辑 / 软删除，支持多级分类、优先级（P1~P4）、请求人、来源。
- 状态机驱动流转（`new → assigned → processing → pending → resolved → closed`，支持 `reopened`），非法流转返回 409。
- SLA 计时：按优先级匹配策略，输出 `normal / warning / breached`，挂起暂停计时、恢复回补。
- 公开评论与内部备注时间线、附件上传下载（≤ 20MB）、满意度评价（1~5 星，仅一次）。

**事件管理（Incident）**

- 故障上报，影响度 × 紧急度 9 宫格优先级矩阵自动映射，支持人工覆盖并留痕。
- 一键转工单 / 关联已有工单（双向引用，重复转单 409）。
- 功能升级与层级升级，`escalation_level` 递增并记录升级历史，越级被拒绝。
- 进入 `resolved` 前强制填写解决方案。

**问题管理（Problem）**

- 由 ≥1 个事件聚合创建或手动创建问题。
- RCA 根因分析（现象 / 分析 / 根本原因）。
- 已知错误标记需附带临时规避方案（否则 422）。
- 关联事件与变更；`resolved` 约束（须关联已关闭变更或填写免变更原因）。

**变更管理（Change）**

- 变更申请（标准 / 普通 / 紧急）与风险评估、影响分析。
- CAB 审批流、审批记录；标准变更预授权免 CAB 路径。
- 变更窗口校验；窗口外实施可配置强制拒绝。
- 实施与回滚闭环（`implemented` / `rolled_back`，失败原因必填）。

**服务目录（Service Catalog）**

- 服务项定义（含动态表单 `form_schema`）与服务分类多级树。
- 服务项 `draft / published / offline / archived` 生命周期，终端用户仅可见 `published`。
- 用户侧浏览下单，动态表单校验通过后自动生成工单并继承 SLA 策略。

**资产与配置管理（CMDB + Asset）**

- CI 建模（服务器 / 网络 / 数据库 / 应用 / 终端 + 自定义属性）与关系（`depends_on` / `contains` / `connects_to`）。
- 关系完整性校验：禁止自环（400）、退役前须清理关系（409）。
- CI 拓扑展开（上下游 2 层）。
- 资产生命周期流转（采购 → 入库 → 在用 → 维修 → 退役 → 报废）与生命周期历史；资产与 CI 1:1 绑定。

**平台与支撑能力**

- JWT 鉴权 + RBAC：7 个内置角色、`perm.<domain>.<action>` 权限点矩阵，接口级 + 资源级鉴权。
- 审计日志：所有写操作（含状态流转 / 删除 / 审批 / 升级）追加留痕。
- 统一响应体 `{code, message, data}` 与统一错误码 ↔ HTTP 映射。
- 统一分页规范（`page` 默认 1，`page_size` 默认 20、上限 100）。
- 软删除（`deleted_at`，默认查询过滤）。
- `/healthz` 健康检查（含数据库探活）。
- 无数据库时**降级模式**启动并打印 WARN。

**双数据库支持**

- `DB_DRIVER=postgres|dameng` 运行时切换 GORM Dialector。
- PostgreSQL 主库与达梦 DM8（`github.com/godoes/gorm-dameng v0.7.2`，纯 Go 无 CGO）。
- 模型与查询遵循双库兼容约束（无 SERIAL / jsonb / ILIKE / ON CONFLICT / now()；索引名 ≤ 30 字符）。

**前端管理台（Vue 3 + TS + Vite + Pinia + Element Plus）**

- 登录鉴权、路由守卫、axios 统一封装（JWT 注入、401 跳登录、错误提示）。
- 六大模块 List / Detail / Form 页面与工作台看板。
- 通用组件：状态色板、优先级标签、SLA 倒计时、时间线、动态表单渲染器、CI 拓扑图、列表页骨架。
- 服务目录门户与下单流程、后台用户 / 角色 / SLA 策略 / 审计日志管理页。

**工程化与文档**

- `Makefile` 自文档化常用命令（run / build / test / test-cover / test-race / vet / fmt / lint / tidy / docker-up / docker-down / frontend-*）。
- 多阶段 `Dockerfile`（非 root 运行）与 `docker-compose.yml`（PostgreSQL + app，含健康检查）。
- GitHub Actions CI（后端测试 + 覆盖率、前端构建）。
- `.golangci.yml`、`.editorconfig`、`.gitignore`、`.env.example`。
- 文档：`README.md`、`CONTRIBUTING.md`、`CHANGELOG.md`、`LICENSE`，以及 `docs/` 下的 PRD、架构设计、任务分解、API 参考。
- 测试分两层，`go test ./...` 均无需数据库即可全绿：**单元测试**（repository 接口 + 内存 fake，覆盖业务规则）与**端到端测试**（嵌入式 SQLite，覆盖跨模块闭环）。注：达梦 / PostgreSQL 方言级行为未覆盖（见下 Known Issues）。

### Changed

- 无（首个版本）。

### Fixed

- **CAB 会签加固**（集成验证阶段发现并修复）：为变更审批增加**复合唯一索引**（防重复投票）、重复投票返回 `409`、会签计数**按审批人去重**。该问题在集成验证时暴露，当版修复。

### Known Issues

首个版本已知限制（完整版见 `README.md` 的「已知限制 / Known Limitations」章节）：

- **达梦 DM8 未做方言级验证**：代码已完成 `DB_DRIVER` 双库 Dialector 切换并遵守 PRD §8.6 十条兼容约束，但**未在真实达梦实例上跑过集成测试**；上生产前必须人工验证（建表 / 核心流程 / 分页 / 关键字查询）。
- **嵌入式 SQLite 仅测试用，是方言行为代理**：`github.com/glebarez/sqlite` 为**仅测试依赖**，不进入服务端二进制；不能替代 PostgreSQL / 达梦的方言级验证。
- **SLA 采用 7×24 自然时间**：不扣除非工作时段与节假日（PRD Q2 默认方案）；工作日历为 P1 路线图项。
- **工单满意度评价无乐观锁**：并发双提交理论上存在竞态（同一用户自我覆盖，影响低）；为 P2 路线图项。

---

[Unreleased]: https://github.com/chixiaowen/itsm-core/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chixiaowen/itsm-core/releases/tag/v0.1.0
