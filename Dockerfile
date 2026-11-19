# syntax=docker/dockerfile:1.7
# =============================================================================
# Sub2API Multi-Stage Dockerfile
# =============================================================================
# Stage 1: Build frontend
# Stage 2: Build Go backend with embedded frontend
# Stage 3: Final minimal image
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
ARG GOLANG_IMAGE=golang:1.27.4-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

FROM node:22.22.2-bookworm-slim AS node
FROM python:3.12.12-slim-bookworm AS runtime
ARG TOSUB2_COMMIT=8548397e89bf80e508eda64a87e0d556d43abc84
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libstdc++6 curl \
    && rm -rf /var/lib/apt/lists/*
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm
WORKDIR /build/tosub2
RUN curl -fsSL --retry 3 "https://codeload.github.com/poxiao33/toSub2/tar.gz/$TOSUB2_COMMIT" \
    | tar -xz --strip-components=1 -C /build/tosub2
COPY tools/reauth-runtime/package*.json ./
RUN npm ci --omit=dev --ignore-scripts --no-audit --no-fund
COPY tools/reauth-runtime/requirements.txt /build/requirements.txt
RUN python -m pip install --no-cache-dir --only-binary=:all: -r /build/requirements.txt
COPY tools/openai_oauth_reauth_worker.py /build/worker.py
COPY tools/openai_totp_rotation.py tools/openai_totp_journal.mjs tools/test_openai_totp_rotation.py tools/test_openai_totp_journal.mjs /build/tools/
COPY tools/test_openai_oauth_reauth_worker.py tools/test_openai_oauth_reauth_engines.py tools/test_openai_oauth_reauth_runtime_controls.py /build/tools/
COPY tools/openai_codex_oauth_adapter.py tools/openai_codex_diagnostics.mjs tools/test_openai_codex_oauth_adapter.py tools/test_openai_codex_diagnostics.mjs /build/tools/
COPY tools/openai_excel_oauth_adapter.py tools/openai_excel_2fa_login.py tools/test_openai_excel_oauth_adapter.py /build/tools/
COPY tools/openai_excel_password_flow.mjs tools/test_openai_excel_password_flow.mjs /build/tools/
COPY tools/openai_siwc_oauth_adapter.py tools/openai_siwc_password_flow.mjs tools/test_openai_siwc_oauth_adapter.py tools/test_openai_siwc_password_flow.mjs /build/tools/
COPY tools/fixtures/siwc-http-transport.mjs /build/tools/fixtures/
COPY tools/fixtures/codex-http-transport.mjs /build/tools/fixtures/
COPY tools/fixtures/session-studio/ /build/tools/fixtures/session-studio/
COPY tools/fixtures/credential-operation-errors.json /build/tools/fixtures/credential-operation-errors.json
RUN cp /build/worker.py /build/tools/openai_oauth_reauth_worker.py \
    && cd /build && EXCEL_TEST_TOSUB2_ROOT=/build/tosub2 python -m unittest tools.test_openai_totp_rotation tools.test_openai_oauth_reauth_worker tools.test_openai_oauth_reauth_engines tools.test_openai_oauth_reauth_runtime_controls tools.test_openai_excel_oauth_adapter tools.test_openai_siwc_oauth_adapter tools.test_openai_codex_oauth_adapter -q \
    && node --test /build/tools/test_openai_codex_diagnostics.mjs \
    && node --test /build/tools/test_openai_totp_journal.mjs \
    && node /build/tools/test_openai_excel_password_flow.mjs \
    && node --test /build/tools/test_openai_siwc_password_flow.mjs \
    && cd /build/tosub2 && node test/password-mfa-workspace-smoke.mjs
COPY tools/reauth-runtime/package.py /build/package.py
RUN python /build/package.py /out
RUN OPENAI_REAUTH_WORKER_TOKEN=00000000000000000000000000000000 \
    TOSUB2_ROOT=/out/tosub2 NODE_EXECUTABLE=/out/node \
    /out/python /out/worker.py --check \
    && /out/python -c 'import curl_cffi; from ssl import SSLContext' \
    && /out/node -e 'require("/out/tosub2/node_modules/jsdom")'
# Validate on the same libc family as the main Alpine image, with no
# system Python or Node available to mask missing bundled dependencies.
FROM alpine:3.21 AS portable-test
COPY --from=runtime /out/ /runtime/
COPY tools/reauth-runtime/smoke.mjs /smoke.mjs
RUN NODE_EXECUTABLE=/runtime/node TOSUB2_PYTHON=/runtime/python /runtime/node /smoke.mjs \
    && OPENAI_TOTP_JOURNAL_DIR=/tmp/totp-smoke NODE_EXECUTABLE=/runtime/node /runtime/python -c 'import sys; sys.path.insert(0,"/runtime"); from openai_totp_rotation import Journal; j=Journal(); j.save(dict(task_id=1,account_id=1,login_email="demo@example.test",password="synthetic",totp_secret="OLD"),"NEW","session"); assert j.call({"action":"list"})[0]["candidate"]=="NEW"; j.acknowledge(1); assert Journal().call({"action":"list"})[0]["database_acknowledged"]' \
    && touch /verified

FROM ubuntu:24.04 AS ubuntu-test
COPY --from=runtime /out/ /runtime/
COPY tools/reauth-runtime/smoke.mjs /smoke.mjs
RUN ln -s /runtime /current \
    && NODE_EXECUTABLE=/current/node TOSUB2_PYTHON=/current/python /current/node /smoke.mjs \
    && OPENAI_TOTP_JOURNAL_DIR=/tmp/totp-smoke NODE_EXECUTABLE=/current/node /current/python -c 'import sys; sys.path.insert(0,"/current"); from openai_totp_rotation import Journal; j=Journal(); j.save(dict(task_id=1,account_id=1,login_email="demo@example.test",password="synthetic",totp_secret="OLD"),"NEW","session"); assert j.call({"action":"list"})[0]["candidate"]=="NEW"' \
    && touch /ubuntu-verified

FROM scratch AS reauth-artifact
COPY --from=runtime /out/ /
COPY --from=portable-test /verified /portable-verified
COPY --from=ubuntu-test /ubuntu-verified /ubuntu-verified

FROM ${ALPINE_IMAGE} AS reauth-bundle
COPY --from=reauth-artifact / /runtime/
RUN tar -czf /runtime.bundle -C /runtime .

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: the frontend output is JS (arch-neutral), so build
# it on the native host arch instead of under QEMU emulation for the target.
FROM --platform=${BUILDPLATFORM} ${NODE_IMAGE} AS frontend-builder
ARG NPM_CONFIG_REGISTRY
ENV NPM_CONFIG_REGISTRY=${NPM_CONFIG_REGISTRY}

WORKDIR /app/frontend

# Install pnpm (pinned to v9 to match CI and keep builds reproducible)
RUN npm install -g pnpm@9 --registry="${NPM_CONFIG_REGISTRY}"

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,id=sub2api-pnpm-store,target=/root/.local/share/pnpm/store \
    if [ -n "${NPM_CONFIG_REGISTRY}" ]; then pnpm config set registry "${NPM_CONFIG_REGISTRY}"; fi && \
    pnpm install --frozen-lockfile --prefer-offline --ignore-scripts

# Copy frontend source and build.
# LegalDocumentView.vue (admin-compliance gate) build-time imports
# ../../../../docs/legal/*.md?raw, so docs/legal/ must sit beside frontend/
# in the image (WORKDIR /app/frontend -> resolves to /app/docs/legal/*.md).
# Copy only that subtree to keep the build dependency minimal.
COPY frontend/ ./
COPY docs/legal/ /app/docs/legal/
# vue-tsc exceeds Node's default ~2 GiB heap on this source tree. Keep the
# extra heap scoped to the build, and allow smaller/larger builders to override.
ARG FRONTEND_NODE_OPTIONS=--max-old-space-size=4096
RUN NODE_OPTIONS="${FRONTEND_NODE_OPTIONS}" pnpm run build

# -----------------------------------------------------------------------------
# Stage 2: Backend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: run the Go toolchain on the native host arch and
# cross-compile to the target arch below. The binary is CGO_ENABLED=0, so this
# is a clean pure-Go cross-compile — no QEMU emulation of go mod download / go
# build (emulated networking here was dropping module fetches with EOF).
FROM --platform=${BUILDPLATFORM} ${GOLANG_IMAGE} AS backend-builder

# Build arguments for version info (set by CI)
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB
# Populated by buildx from the --platform target (e.g. linux/amd64).
ARG TARGETOS
ARG TARGETARCH

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/backend

# Copy go mod files first (better caching)
COPY backend/go.mod backend/go.sum ./
COPY backend/third_party ./third_party
# Cache mount keeps the module cache across builds so a transient CDN blip on
# retry resumes instead of re-fetching every zip from scratch.
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    go mod download

# Copy backend source first
COPY backend/ ./
COPY --from=reauth-bundle /runtime.bundle ./internal/reauthruntime/${TARGETARCH}.bundle

# Copy frontend dist from previous stage (must be after backend copy to avoid being overwritten)
COPY --from=frontend-builder /app/backend/internal/web/dist ./internal/web/dist

# Build the binary (BuildType=release for CI builds, embed frontend)
# Version precedence: build arg VERSION > exact git tag > cmd/server/VERSION
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=sub2api-gobuild,target=/root/.cache/go-build \
    VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(./scripts/resolve-version.sh)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 3: PostgreSQL Client (version-matched with docker-compose)
# -----------------------------------------------------------------------------
FROM ${POSTGRES_IMAGE} AS pg-client

# -----------------------------------------------------------------------------
# Stage 4: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${ALPINE_IMAGE}

# Labels
LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    su-exec \
    libpq \
    zstd-libs \
    lz4-libs \
    krb5-libs \
    libldap \
    libedit \
    && rm -rf /var/cache/apk/*

# Copy pg_dump and psql from the same postgres image used in docker-compose
# This ensures version consistency between backup tools and the database server
COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/

# Create non-root user
RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

# Set working directory
WORKDIR /app

# Copy binary/resources with ownership to avoid extra full-layer chown copy
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api /app/sub2api
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources

COPY THIRD_PARTY_NOTICES.md /app/THIRD_PARTY_NOTICES.md
COPY LICENSES/BEpusdt-GPL-3.0.txt /app/LICENSES/BEpusdt-GPL-3.0.txt

# Create data directory
RUN mkdir -p /app/data && chown sub2api:sub2api /app/data

# Copy entrypoint script (fixes volume permissions then drops to sub2api)
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Expose port (can be overridden by SERVER_PORT env var)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Run the application (entrypoint fixes /app/data ownership then execs as sub2api)
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/sub2api"]
