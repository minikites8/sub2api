# AGENT.md

面向后续 coding agent 的项目工作手册。此文件根据当前仓库状态整理；若与旧 README 或 DEV_GUIDE 冲突，优先信任当前源码、Makefile、CI workflow 和 lockfile。

## 项目速览

Sub2API 是一个 AI API gateway，用于把上游 AI 产品订阅额度封装成平台 API Key，并处理认证、计费、调度、限流、用量记录和管理后台。当前仓库是长期维护 Kiro 支持的 fork。

技术栈：

- 后端：Go + Gin + Ent + Wire，模块名 `github.com/Wei-Shaw/sub2api`
- 前端：Vue 3 + Vite + TypeScript + Pinia + Vue Router + TailwindCSS
- 数据：PostgreSQL + Redis
- 部署：Docker、systemd、GoReleaser，配置样例在 `deploy/config.example.yaml`

当前 CI 以 `backend/go.mod` 为准，要求 Go `1.26.4`。部分旧文档仍写 Go `1.25.7`，不要沿用旧版本。前端 CI 使用 Node 20、pnpm 9；必须用 pnpm，不要用 npm/yarn 改 lockfile。

## 目录地图

- `backend/cmd/server/`：主服务入口；`main.go` 支持 `-setup` 和 `-version`
- `backend/cmd/jwtgen/`：JWT secret 工具
- `backend/internal/config/`：配置结构、默认值、加载与校验
- `backend/internal/server/`：Gin router、middleware、route 注册
- `backend/internal/handler/`：HTTP handler；管理员 handler 在 `handler/admin/`
- `backend/internal/service/`：业务逻辑和网关核心流程
- `backend/internal/repository/`：Ent client、数据访问、migration runner
- `backend/internal/payment/`：支付 provider 抽象与实现
- `backend/internal/pkg/`：协议兼容、OAuth、tokenizer、proxy 等底层包
- `backend/ent/schema/`：Ent schema；`backend/ent/` 其余大多是生成代码
- `backend/migrations/`：前向 SQL migration，启动时自动执行
- `backend/internal/web/`：前端嵌入服务；`dist/` 是前端 build 产物
- `frontend/src/api/`：Axios API client 与接口封装
- `frontend/src/router/`：路由、导航守卫和标题逻辑
- `frontend/src/stores/`：Pinia stores
- `frontend/src/views/`：页面
- `frontend/src/components/`：组件
- `frontend/src/composables/`：组合式逻辑
- `frontend/src/i18n/`：中英文 locale，默认 locale 是 `en`
- `frontend/src/types/`：前端共享类型
- `skills/`：本项目自带 Codex/agent skill 资料

## 常用命令

根目录：

```bash
make build
make test
make test-backend
make test-frontend
make test-frontend-critical
make secret-scan
```

Windows 没有 make 时，直接运行 Makefile 里的原始命令。

后端：

```bash
cd backend
go run ./cmd/server/
go test ./...
go test -tags=unit ./...
go test -tags=integration ./...
go test -tags=e2e -v -timeout=300s ./internal/integration/...
golangci-lint run ./...
go generate ./ent
go generate ./cmd/server
go build -o bin/server ./cmd/server
```

前端：

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm dev
pnpm build
pnpm lint:check
pnpm typecheck
pnpm exec vitest run
pnpm exec vitest run src/path/to/file.spec.ts
```

`pnpm build` 会把产物写入 `backend/internal/web/dist/`，这是生成目录，不要手改。CI 的前端检查等价于 lint check、typecheck、以及根 Makefile 中列出的 critical vitest 文件。

## 本地运行与配置

后端配置文件名为 `config.yaml`。`LoadForBootstrap` 的搜索顺序大致是：

1. `DATA_DIR` 环境变量指向的目录
2. `/app/data`
3. 当前工作目录
4. `./config`
5. `/etc/sub2api`

配置 key 支持环境变量覆盖，`.` 会替换为 `_`。setup server 的地址可用 `SERVER_HOST`、`SERVER_PORT` 覆盖，默认 `0.0.0.0:8080`。

本地常用方式：

```bash
cd backend
go run ./cmd/server/
```

首次运行可能进入 setup wizard。也可以参考 `deploy/config.example.yaml` 创建本地 `backend/config.yaml`；该文件含敏感信息，已被 `.gitignore` 忽略。

前端开发服务器默认端口来自 `VITE_DEV_PORT`，默认 `3000`。`VITE_DEV_PROXY_TARGET` 默认 `http://localhost:8080`，会代理 `/api`、`/v1`、`/setup` 到后端。

## 后端改动规则

- 按现有分层走：`routes -> handler -> service -> repository/ent`。不要把业务逻辑塞进 route 注册或 handler。
- 新增 API 时，优先在 `backend/internal/server/routes/` 注册路由，再接到对应 handler/service。
- 管理端 API 通常在 `/api/v1/admin/...`，用户端 API 在 `/api/v1/...`，网关兼容层在 `/v1`、`/v1beta`、`/responses`、`/backend-api/codex` 等路径。
- Wire 依赖注入变更后运行 `go generate ./cmd/server`，并提交生成的 `wire_gen.go`。
- Ent schema 变更后运行 `go generate ./ent`，并提交生成的 Ent 文件。
- 数据库结构或数据迁移必须新增 `backend/migrations/NNN_description.sql`，不要修改已发布/已应用 migration。
- migration runner 会记录 SHA256 checksum。修改已应用 migration 会触发 checksum mismatch。
- 含 `CREATE INDEX CONCURRENTLY` 或 `DROP INDEX CONCURRENTLY` 的 migration 必须用 `*_notx.sql` 后缀，且只放并发索引语句。
- 现有 `backend/migrations/README.md` 中的 `make migrate-up/down` 与当前 Makefile 不匹配；当前迁移由服务启动时自动应用。
- 改 interface 后，用 `rg "type.*Stub.*struct"`、`rg "type.*Mock.*struct"` 找测试 stub/mock 并补齐方法。

生成/派生文件注意：

- `backend/ent/` 中除 `schema/` 和 `generate.go` 外大多是生成文件。
- `backend/cmd/server/wire_gen.go` 是 Wire 生成文件。
- `backend/internal/web/dist/` 是前端构建产物，只由 build 生成。

## 前端改动规则

- 使用 `frontend/src/api/client.ts` 的 `apiClient`，它会自动附加 auth token、`Accept-Language`、GET 请求 timezone，并解包 `{ code, message, data }`。
- 新接口类型优先放在 `frontend/src/types/` 或邻近 API 文件已有类型位置，保持现有命名风格。
- 全局状态放 Pinia stores；页面级临时状态留在 view/component/composable。
- 可复用逻辑放 `composables/`，不要复制粘贴到多个 view。
- 可见文案通常需要同时更新 `frontend/src/i18n/locales/en.ts` 和 `frontend/src/i18n/locales/zh.ts`。
- 样式优先复用 `frontend/src/style.css` 里的 `.btn`、`.input`、`.card` 等组件类，以及 `tailwind.config.js` 中的主题色。
- 前端路径 alias 是 `@ -> frontend/src`。
- 路由标题、权限和菜单元信息集中在 `frontend/src/router/index.ts` 及相关 helper。

## 测试策略

优先跑与改动最接近的测试，再按风险扩大范围。

- 纯后端小改：`go test ./path/to/package`
- 后端业务/接口改动：`go test -tags=unit ./...`
- 涉及数据库、Redis、迁移、端到端流程：`go test -tags=integration ./...`，可能需要 Docker/Testcontainers 或本地依赖
- Wire/Ent/全局共享逻辑改动：再跑 `go test ./...` 和 `golangci-lint run ./...`
- 纯前端组件/逻辑改动：`pnpm exec vitest run <spec>`
- 前端类型或路由/store/API 改动：`pnpm lint:check`、`pnpm typecheck`
- 发 PR 前尽量跑：后端 unit + integration + lint，前端 lint + typecheck + critical vitest

文档或注释-only 改动一般不需要跑完整测试，但结束时要说明未跑测试的原因。

## 常见坑

- 不要用 npm 安装前端依赖；改 `frontend/package.json` 后必须同步 `frontend/pnpm-lock.yaml`。
- PowerShell 会把 bcrypt hash 里的 `$` 当变量解析。涉及这类 SQL 时写入 `.sql` 文件再用 `psql -f`。
- Windows 上连接 PostgreSQL 建议用 `127.0.0.1`，避免 `localhost` 优先 IPv6 造成干扰。
- 本地 DEV_GUIDE 记录的 PostgreSQL 账号常见为 `sub2api/sub2api`、数据库 `sub2api`，Redis 默认无密码；实际以本机配置为准。
- root `Makefile` 里有 `datamanagement` 目标，但当前仓库没有 `datamanagement/` 目录，使用前先确认。
- OpenAI/Kiro/Gemini 等账号不要在批量编辑中跨平台混选，模型白名单/映射可能被覆盖。排查 “Service temporarily unavailable/无可用账号” 时优先检查账号平台、分组、模型映射和默认透传。
- `.gitignore` 已忽略 `AGENTS.md`、`CLAUDE.md`、本地配置、构建产物和 `.codex/`。本文件命名为 `AGENT.md`，用于保留在仓库中。

## 结束前检查

提交或交付前快速确认：

- `git status --short`，不要误改无关文件。
- 是否改了生成源却忘了生成文件：Ent schema、Wire provider、前端 build 输出。
- 是否改了 DB schema 却忘了新增 migration。
- 是否改了前端依赖却忘了 `pnpm-lock.yaml`。
- 是否新增用户可见文案却只改了一种语言。
- 是否引入 secret、真实 token、私有配置或本地路径。
- 最终回复里说明改了什么、验证了什么、哪些测试没有跑。
