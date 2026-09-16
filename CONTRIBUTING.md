# 贡献指南（Contributing Guide）

感谢你愿意为 **itsm-core** 做出贡献！本文档说明分支模型、提交规范、代码风格、测试要求与 PR 流程。

---

## 1. 行为准则

- 尊重所有参与者，保持友善与专业的沟通。
- 聚焦技术问题，不做人身评价。
- 提交前先搜索已有 Issue / PR，避免重复劳动。

---

## 2. 分支模型

采用简化的 **Trunk-Based + Feature 分支** 模型：

| 分支 | 用途 | 说明 |
| --- | --- | --- |
| `main` | 主干，始终保持可构建、可测试 | **受保护分支**，仅通过 PR 合并 |
| `feature/<name>` | 新功能开发 | 例如 `feature/ticket-reopen` |
| `fix/<name>` | 缺陷修复 | 例如 `fix/sla-pause-calc` |
| `docs/<name>` | 文档修订 | 例如 `docs/api-refresh` |
| `chore/<name>` | 构建 / 依赖 / 工程化杂项 | 例如 `chore/ci-cache` |

工作流：

```bash
git checkout main && git pull
git checkout -b feature/your-feature
# ... 开发与提交 ...
git push -u origin feature/your-feature
# 在 GitHub 上发起 PR → main
```

---

## 3. 提交规范（Conventional Commits）

所有提交信息必须遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <subject>
```

### 类型（type）

| type | 含义 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | 缺陷修复 |
| `docs` | 仅文档改动 |
| `test` | 新增 / 修改测试 |
| `refactor` | 重构（不改变外部行为） |
| `perf` | 性能优化 |
| `style` | 代码格式（不影响逻辑） |
| `chore` | 构建、依赖、脚手架、CI 等杂项 |
| `revert` | 回滚某次提交 |

### scope（可选，建议填写业务域）

`ticket` / `incident` / `problem` / `change` / `catalog` / `cmdb` / `asset` / `platform` / `auth` / `config` / `ci` / `docs` 等。

### 示例

```
feat(ticket): 支持关闭后 7 天内重开
fix(sla): 修正挂起时长回补的边界计算
docs(readme): 补充达梦 DM8 双库切换说明
test(catalog): 增加动态表单必填校验用例
chore(ci): 缓存 Go modules 与 npm 依赖
```

> 提交信息请使用中文或英文皆可，但需语义明确、一句话说清「做了什么」。

---

## 4. 代码风格

### 后端（Go）

- 必须通过 `gofmt` 与 `goimports` 格式化：`make fmt`。
- 遵循 Google Go Style，包注释 / 导出符号必须有 `//` 注释。
- 分层铁律（**硬约束，PR 会重点检查**）：
  - `*gorm.DB` **只允许**出现在 repository 实现层（`internal/domain/*/repository_gorm.go`）与 `internal/pkg/database`。
  - **Handler 层禁止** import `gorm.io/gorm`；**Service 层只依赖 repository 接口**，禁止 import GORM。
  - Service 构造函数接收 **interface**，便于注入内存 fake。
- 命名：Go 导出字段 `PascalCase`，JSON 字段 `snake_case`。
- 时间统一 RFC3339 UTC 存储与传输。

### 前端（Vue 3 + TypeScript）

- 使用 TypeScript，`npm run build` 内含 `vue-tsc --noEmit` 类型检查，必须零类型错误。
- 组件 / 目录命名遵循既有约定（`PascalCase.vue`）。
- 若配置了 ESLint / Prettier，提交前请本地运行。
- 与后端常量对齐的联合类型统一放 `frontend/src/types/`。

---

## 5. 测试要求

- **新功能必须附带单元测试**；缺陷修复应补充能复现该缺陷的用例。
- 后端测试遵循「**零外部依赖**」原则：使用 repository 接口 + 内存 fake，**禁止**在测试中连接真实数据库或 Docker。
- 提交前请本地确保以下命令全绿：

  ```bash
  make test        # go test ./... -count=1
  make vet         # go vet ./...
  make test-race   # 竞态检测
  ```

- 前端改动请确保 `npm run build` 通过。

---

## 6. Pull Request 流程

1. 从最新的 `main` 切出分支。
2. 完成开发，本地跑通 `make test` / `make vet`（前端改动加 `npm run build`）。
3. 提交信息遵循 Conventional Commits。
4. 发起 PR 到 `main`，PR 描述请包含：
   - **变更动机**（关联 Issue 编号，如 `Closes #12`）；
   - **变更内容**（做了什么、影响哪些模块）；
   - **验证方式**（跑了哪些测试、手工验证步骤）。
5. 等待 CI 通过 + 至少一位维护者 Review。
6. 合并策略：Squash Merge，保持 `main` 历史整洁。

---

## 7. Issue 模板指引

提交 Issue 时请选择合适的类型并尽量填写模板字段：

- **Bug Report**：环境（Go / Node / 数据库版本）、复现步骤、期望结果、实际结果、日志（含 `request_id` 更佳）。
- **Feature Request**：需求背景、期望能力、验收标准、优先级建议（P0/P1/P2）。
- **Question**：先查阅 `docs/` 下的 PRD / 架构 / API 文档。

---

## 8. 环境约定（重要，务必遵守）

本项目的工具链有严格约束，**违反会导致构建失败**：

- **必须** `GOTOOLCHAIN=local`；`go.mod` 声明 `go 1.22`。
- **必须**使用 `GOPROXY=https://goproxy.cn,direct`（默认 `proxy.golang.org` 不可达）。
- **严禁 `go get -u`**：不得将 `golang.org/x/crypto` 升级到 `≥ v0.32`（需 go ≥ 1.23）、不得将 `gin` 升级到 `≥ v1.11`、不得将 GORM 降到 `gorm.io/gorm < v1.30.1`（达梦驱动要求）。
- 新增依赖前请**先实测** `GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct go build ./...` 通过，且依赖的 `go` directive ≤ 1.22。
- 本地推荐使用项目内置 `Makefile` 命令（已内置上述环境变量）。

```bash
export GOTOOLCHAIN=local
export GOPROXY=https://goproxy.cn,direct
```

---

再次感谢你的贡献！
