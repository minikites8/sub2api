# Sub2API 公开资料出口增强版

本仓库基于 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 最新主线整理，额外加入一个面向中转站站长和第三方采集器的“公开资料出口”功能。

它的目标不是开放后台，也不是暴露账号池，而是把站点本来适合公开的信息标准化输出，方便 PriceAI 或其他采集器自动识别模型价格、分组倍率、缓存命中和可用性状态。

## 新增能力

- 公开发现接口：`/.well-known/ai-transit.json`
- 公开快照接口：`/api/public/transit/v1/snapshot`
- 可选公开页面：`/public/transit`
- 后台开关：`系统设置 -> 功能开关 -> 公开资料出口`
- 分组价格：充值倍率、分组倍率、平台、模型数量
- 模型明细：模型名、计费模式、输入价、输出价、缓存输入价、缓存创建价、按次/尺寸价
- 缓存指标：累计缓存命中率、缓存命中量、缓存创建量
- 可用性监测：复用现有渠道状态监测数据，展示延迟、可用率和状态

## 隐私边界

公开资料出口只输出标准化聚合信息，不公开以下内容：

- 上游账号、Cookie、Access Token、Refresh Token
- API Key、密钥、代理配置
- 内部渠道 ID、账号池调度细节
- 用户身份、用户余额、用户请求日志

站长可以只开放机器可读接口，不开放公开页面。

## 开关说明

公开资料出口拆成两个开关：

- `public_transit_enabled`：公开资料接口开关，默认开启。
- `public_transit_page_enabled`：公开资料页面开关，默认关闭。

如果只想让 PriceAI 或其他采集器自动抓取数据，开启接口即可；如果希望访客也能在站点页面上查看模型价格和可用性，再开启公开页面。

## 接口示例

发现接口：

```bash
curl https://your-domain.example/.well-known/ai-transit.json
```

返回示例：

```json
{
  "schema_version": "ai-transit.v1",
  "system": "sub2api",
  "snapshot_url": "https://your-domain.example/api/public/transit/v1/snapshot",
  "homepage_url": "https://your-domain.example/public/transit",
  "generated_at": "2026-07-07T00:00:00Z"
}
```

快照接口：

```bash
curl https://your-domain.example/api/public/transit/v1/snapshot
```

快照会包含站点基础信息、公开分组、模型价格、缓存指标和可用性监测摘要。

### 渠道监控 V1 兼容输出

发现接口继续声明 `schema_version: "ai-transit.v1"`，并指向 `/api/public/transit/v1/snapshot`。
快照中的 `monitoring` 使用渠道监控 V2 的真实请求聚合数据，提供 V1 爬虫所需字段：

- 每个 V2 平台/模型聚合行对应一个监控项，包含 `name`、`provider`、`primary_model`、`primary_status`、`models`、`extra_models` 和 `timeline`；`models` 包含该主模型，`extra_models` 为 `[]`。分组监控同时提供 `group_name`。
- `availability_7d`、`availability_15d`、`availability_30d` 延用 V2 对应窗口成功率，数值范围为 0–100；主模型明细同步这些数值。
- 状态映射为 `healthy → operational`、`warning → degraded`、`critical → failed`、`unknown → unknown`。主模型状态取最近有请求的聚合桶；仅有窗口汇总时采用窗口健康状态，空流量状态为 `unknown`。
- `timeline` 优先使用 V2 最近 90 分钟的五分钟聚合桶，并按时间倒序列出有请求的观测；近期窗口空闲时保留七天历史中的最近观测。`checked_at` 与 `last_checked_at` 使用桶起始时间。`avg_latency_7d_ms` 使用 V2 七天请求耗时均值并四舍五入到毫秒，`latest_latency_ms` 与时间线 `latency_ms` 使用对应桶的耗时 P50；缺失延迟、Ping 字段保持省略。
- `platform`、`model`、`status`、`metrics`、`health`、`buckets`、`windows` 等扩展字段继续服务模型广场。监控数据源和开关沿用 V2 配置。

## 接入方式

如果你的 Sub2API 版本接近上游主线，推荐直接 cherry-pick 本仓库的功能提交：

```bash
git remote add public-transit https://github.com/dimthink/sub2api-public-transit.git
git fetch public-transit main
git cherry-pick <public-transit-feature-commit>
```

如果你的仓库改动较多，也可以下载 patch 后手动应用：

```bash
curl -L https://github.com/dimthink/sub2api-public-transit/commit/<public-transit-feature-commit>.patch -o public-transit.patch
git am public-transit.patch
```

遇到冲突时，优先检查这些区域：

- 分组管理：`backend/internal/handler/admin/group_handler.go`
- 用量统计：`backend/internal/repository/usage_log_repo.go`
- 设置开关：`backend/internal/service/setting_service.go`
- 前端分组页：`frontend/src/views/admin/GroupsView.vue`
- 前端设置页：`frontend/src/views/admin/SettingsView.vue`

## 发给 Codex / Agent 的接入提示词

如果你希望让自己的 Codex、Claude Code、Cursor Agent 或其他代码助手自动接入，可以把下面这段直接发给它：

```text
请在当前 Sub2API 仓库中接入“公开资料出口”功能。

参考仓库：
https://github.com/dimthink/sub2api-public-transit

目标：
1. 不要把我的仓库直接替换成参考仓库。
2. 先确认当前仓库的上游来源、当前分支、未提交改动和 Sub2API 版本。
3. 从参考仓库中找出它相对 Wei-Shaw/sub2api 最新主线新增的唯一功能提交。
4. 优先用 cherry-pick 接入这个提交；如果我的仓库已经深度二改导致冲突较多，就改用 patch 方式手动合并。
5. 合并时保留我的现有数据、配置、部署文件和本地改动，不要重置仓库，不要删除数据库或数据卷。
6. 接入后确认至少存在这些能力：
   - /.well-known/ai-transit.json
   - /api/public/transit/v1/snapshot
   - /public/transit
   - 后台“系统设置 -> 功能开关 -> 公开资料出口”中的接口开关和页面开关
7. 接入后运行关键验证：
   - go test ./internal/service ./internal/handler ./internal/server ./internal/repository ./internal/web -tags embed
   - pnpm --dir frontend run build
8. 如果测试失败，先定位是否是合并冲突或本地二改导致，不要盲目升级依赖。
9. 最后给我输出：
   - 接入的 commit hash
   - 冲突文件和处理方式
   - 验证命令结果
   - 部署前需要我确认的风险点

注意：公开资料出口只能公开模型价格、分组倍率、缓存命中和可用性等聚合信息，不能公开账号、密钥、Cookie、Token、内部渠道 ID 或用户数据。
```

如果你的站点已经有大量二改，建议先让 Agent 创建一个临时分支再接入：

```bash
git switch -c feature/public-transit-snapshot
```

## 验证建议

应用后建议至少执行：

```bash
cd backend
go test ./internal/service ./internal/handler ./internal/server ./internal/repository ./internal/web -tags embed

cd ..
pnpm --dir frontend run build
```

启动服务后检查：

```bash
curl -I https://your-domain.example/public/transit
curl https://your-domain.example/.well-known/ai-transit.json
curl https://your-domain.example/api/public/transit/v1/snapshot
```

## 给 PriceAI 的站点准入建议

站点如果希望被自动收录，建议至少满足：

- `/.well-known/ai-transit.json` 可公开访问
- `/api/public/transit/v1/snapshot` 可公开访问
- 模型价格字段尽量完整
- 分组倍率和充值倍率真实可用
- 可用性监测保持启用
- 不在公开接口中泄露账号、密钥、Cookie 或内部渠道 ID

## 与原版 Sub2API 的关系

本仓库不是重新发行一个独立网关项目，而是基于原版 Sub2API 增加“公开资料出口”能力。原项目的部署方式、数据库结构和后台使用习惯尽量保持不变。

如果上游后续合并了类似功能，建议优先回到上游主线；如果上游暂未合并，可以继续基于本仓库的单一功能提交做兼容接入。
