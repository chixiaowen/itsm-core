# itsm-core Makefile
#
# 环境硬约束（本机实测）：
#   - GOPROXY 默认 proxy.golang.org 不可达，必须使用中国镜像 https://goproxy.cn,direct
#   - 必须 GOTOOLCHAIN=local + go.mod go 1.22，严禁 go get -u / 工具链自动升级
#
# 所有 Go 命令均前置 `GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct` 以保证生效。

export GOTOOLCHAIN=local
export GOPROXY=https://goproxy.cn,direct

GO  ?= go
NPM ?= npm

.DEFAULT_GOAL := help

.PHONY: help run build test test-cover test-race vet fmt lint tidy \
        frontend-install frontend-dev frontend-build frontend \
        up up-db down down-purge status logs clean

help: ## 显示可用目标（默认目标）
	@echo "itsm-core 常用命令："
	@grep -E '^[a-zA-Z0-9_-]+:.*## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## 本地启动后端（无库时降级告警启动）
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) run ./cmd/server

build: ## 编译后端到 bin/server
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) build -o bin/server ./cmd/server

test: ## 运行全部单测（零外部依赖）
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) test ./... -count=1

test-cover: ## 覆盖率报告（写入 coverage.out 并打印函数级覆盖）
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) test ./... -count=1 -coverprofile=coverage.out
	GOTOOLCHAIN=local $(GO) tool cover -func=coverage.out

test-race: ## 竞态检测（idgen 并发）
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) test ./... -race -count=1

vet: ## go vet 静态检查
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) vet ./...

fmt: ## gofmt 格式化
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) fmt ./...

lint: ## golangci-lint（.golangci.yml）；未安装时回退 go vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint run"; \
		GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct golangci-lint run; \
	else \
		echo "golangci-lint 未安装，回退执行 go vet ./..."; \
		GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) vet ./...; \
	fi

tidy: ## 整理依赖（禁止 go get -u）
	GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct $(GO) mod tidy

frontend-install: ## 安装前端依赖（npm install）
	cd frontend && $(NPM) install

frontend-dev: ## 启动前端开发服务器（vite dev，http://127.0.0.1:5173）
	cd frontend && $(NPM) run dev

frontend-build: ## 构建前端产物到 frontend/dist
	cd frontend && $(NPM) run build

frontend: ## 前端安装依赖 + 构建（CI 约定：npm ci）
	cd frontend && $(NPM) ci && $(NPM) run build

up: ## 一键构建并启动全部（PostgreSQL + 后端 + 前端）—— 推荐
	./scripts/start.sh

up-db: ## 仅启动 PostgreSQL（配合本地 go run 开发）
	./scripts/start.sh --db-only

down: ## 停止全部服务（保留数据库数据卷）
	./scripts/stop.sh

down-purge: ## 停止全部服务并删除数据卷（清空数据库，谨慎）
	./scripts/stop.sh --purge

status: ## 查看服务状态、健康检查与接口探测
	./scripts/status.sh

logs: ## 跟踪全部服务日志（make logs S=app 可指定服务）
	docker compose logs -f $(S)

clean: ## 清理构建产物
	rm -rf bin coverage.out
