# syntax=docker/dockerfile:1

# ── 阶段 1：构建前端 ─────────────────────────────────────────────
# 前端产物通过 go:embed 打进 Go 二进制，必须先构建 web/dist 再编译 Go。
FROM node:24-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable
# 先只复制依赖清单，利用层缓存：依赖不变时 pnpm install 不会重跑
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# ── 阶段 2：编译 Go 二进制 ───────────────────────────────────────
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod ./
# 无第三方依赖，无需 go mod download；若有依赖在此预热缓存
COPY . .
# 用阶段 1 的前端产物满足 go:embed all:web/dist
COPY --from=web /src/web/dist ./web/dist
# 固定 CGO 关闭，产出静态二进制，运行镜像不依赖构建工具链
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -trimpath -ldflags="-s -w" -o /out/dst-panel .

# ── 阶段 3：运行镜像 ─────────────────────────────────────────────
# DST 专用服务器是 glibc 动态链接的原生 Linux 二进制，需要完整的 Debian/Ubuntu
# 运行时依赖；不用 Alpine（musl 不兼容）也不用 Wine。
FROM debian:bookworm-slim AS runtime

# DST 64 位服务器运行所需的共享库；SteamCMD 首次运行也需要这些。
# 具体清单以 `ldd bin64/dontstarve_dedicated_server_nullrenderer_x64` 为准，
# 这里覆盖官方 Linux 专用服务器常见的 glibc/图形/音频/压缩依赖。
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        lib32gcc-s1 \
        lib32stdc++6 \
        libc6-i386 \
        libcurl4 \
        # DST 的 bin64 二进制链接的是 GnuTLS 版 libcurl（libcurl-gnutls.so.4），
        # 与 OpenSSL 版 libcurl4 是两个不同 soname，缺了启动就报找不到库。
        # 这一项是对真实服务端二进制跑 ldd 核出来的。
        libcurl3-gnutls \
        libstdc++6 \
        libgl1 \
        libglu1-mesa \
        libx11-6 \
        libxext6 \
        libxrender1 \
        libxi6 \
        libfontconfig1 \
        libfreetype6 \
        libpulse0 \
        libasound2 \
        libdbus-1-3 \
        curl \
        tini \
    && rm -rf /var/lib/apt/lists/*

# 固定非 root 用户运行。UID/GID 可由 compose 覆盖以匹配宿主挂载卷属主。
ARG PANEL_UID=1000
ARG PANEL_GID=1000
RUN groupadd -g ${PANEL_GID} dst && \
    useradd -u ${PANEL_UID} -g ${PANEL_GID} -m -s /bin/bash dst

# 程序只读安装在镜像里；所有可变数据走 /data 挂载卷。
WORKDIR /opt/dst-panel
COPY --from=build /out/dst-panel /opt/dst-panel/dst-panel
# 预建数据目录并授权给非 root 用户，首次运行即可写
RUN mkdir -p /data && chown -R ${PANEL_UID}:${PANEL_GID} /data

USER dst
ENV DST_DATA_DIR=/data
EXPOSE 16800

# tini 作为 init：正确转发 SIGTERM 并回收僵尸子进程（面板会 fork DST 分片进程）
ENTRYPOINT ["/usr/bin/tini", "--"]
# 容器内绑 0.0.0.0（bridge 网络下必须如此，否则端口映射收不到包）；
# 「不对外暴露」由 compose 的端口映射限制在宿主 127.0.0.1 来保证，
# 而不是靠这里的 bind 地址。
# 当前 API 无鉴权：远程访问请用 SSH 隧道，或前置带 TLS+认证的反向代理。
CMD ["/opt/dst-panel/dst-panel", "-bind", "0.0.0.0", "-port", "16800", "-data-dir", "/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://127.0.0.1:16800/healthz || exit 1
