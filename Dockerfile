# ============================================================
# itsm-core 多阶段构建 Dockerfile
# ============================================================
# 构建镜像：docker build -t itsm-core:latest .
# 运行容器：docker run --rm -p 8080:8080 \
#             -e DB_DRIVER=postgres \
#             -e DB_DSN="host=<db> port=5432 user=itsm password=itsm dbname=itsm_core sslmode=disable TimeZone=UTC" \
#             -e JWT_SECRET="change-me" \
#             itsm-core:latest
#
# 环境硬约束：
#   - 默认 proxy.golang.org 不可达，故 ARG GOPROXY 默认走 https://goproxy.cn,direct
#   - 必须 GOTOOLCHAIN=local，避免构建期下载外部工具链

# ---------- 阶段 1：构建 ----------
FROM golang:1.22-alpine AS builder

# 依赖下载走中国镜像（可在构建时用 --build-arg GOPROXY=... 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV GOTOOLCHAIN=local \
    GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

WORKDIR /src

# 先拷贝依赖清单，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译（静态链接，无 CGO）
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- 阶段 2：运行 ----------
FROM alpine:3.20

# 运行期必需：CA 证书与时区数据
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S itsm \
    && adduser -S -G itsm -h /app itsm

WORKDIR /app

# 以非 root 用户运行
USER itsm

# 拷贝二进制与配置
COPY --from=builder /out/server /app/server
COPY --chown=itsm:itsm configs /app/configs

# 上传目录（可挂载卷持久化）
RUN mkdir -p /app/data/uploads

ENV GOTOOLCHAIN=local \
    ITSM_APP_HTTP_ADDR=":8080"

EXPOSE 8080

# 直接启动服务（配置默认搜索 ./configs/config.yaml，可用 -config 覆盖）
ENTRYPOINT ["/app/server"]
