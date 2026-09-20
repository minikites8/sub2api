# Codex 门票接入

## 启用入口

管理员后台 → 系统设置 → 网关服务 → Codex 设置：

1. 填写「Codex 门票代理」，格式为 `http://USER:PASSWORD@HOST:PORT`，也支持 HTTPS、SOCKS5、SOCKS5h。此代理专用于后台门票请求，每次探测使用独立 HTTP/1.1 连接；业务请求继续使用账号原有代理与连接池。
2. 填写「门票目标模型」，使用实际上游模型名，以逗号分隔。留空采用部署配置，默认值沿用参考代码的 `gpt-6-astra,gpt-5.6-sol`。管理员应选择账号实际支持的模型。
3. 打开「Codex 门票（个人 292 / Team 332）」并保存。

设置保存后刷新运行时缓存。首次启动的后台循环约 1 秒后执行，后续循环间隔默认 6 秒。模型请求需等待对应账号获得有效门票。

## 行为

- 功能默认关闭。开启后按账号、模型缓存 `x-codex-turn-state`。个人账号校验 292 字符，Team/Business 校验 332 字符，同时校验 `gAAAAA` 前缀、有效期与凭证摘要。
- HTTP Responses、透传、Messages 转换后的 Responses 以及 WebSocket 握手共用注入逻辑。调度门控采用映射后的模型，compact 使用实际 compact 上游模型。
- 票据仅保存在当前进程内存中，默认有效期一小时，剩余十分钟时尝试刷新。重启后重新获取。多实例分别维护缓存。
- 默认 `fail_closed: true`：目标模型缺少有效票据时跳过该账号，转发入口再次校验。API Key、影子账号与其他模型沿用现有流程。
- 后台最多四个账号并行，每个账号依次处理模型。账号停调、过期、限流、额度暂停时跳过。请求超时默认 25 秒；服务退出会取消请求并等待循环结束。
- 普通失败按模型退避；401 清除该账号票据并暂停，凭证更新后恢复；429 按 `Retry-After` 冷却，最低一分钟。凭证刷新保留 429 冷却。失败退避上限十分钟，服务端更长的 `Retry-After` 优先。
- 代理地址回显隐藏密码，回传隐藏值保留原配置。日志只记录账号 ID、模型、HTTP 状态和长度。票据内容与代理密码保留在内部。
- 「防降智」是参考教程的功能称呼，长度校验属于参考实现规则；推理质量和上游 429 改善程度需要实际观测。后台探测可能消耗账号额度和代理流量。

## 部署配置

```yaml
gateway:
  openai_codex_ticket:
    enabled: false
    harvest_proxy_url: ""
    models: ["gpt-6-astra", "gpt-5.6-sol"]
    ttl_seconds: 3600
    refresh_before_seconds: 600
    harvest_probe_interval_seconds: 6
    harvest_attempt_timeout_seconds: 25
    fail_closed: true
```

环境变量前缀：`GATEWAY_OPENAI_CODEX_TICKET_`，例如 `GATEWAY_OPENAI_CODEX_TICKET_ENABLED=true`。模型环境变量使用逗号分隔。后台开关优先于部署开关；后台代理、模型留空时使用部署值。

管理 API 新增字段：

```json
{
  "openai_codex_ticket_enabled": false,
  "openai_codex_ticket_harvest_proxy_url": "http://USER:PASSWORD@HOST:PORT",
  "openai_codex_ticket_models": "MODEL_A,MODEL_B"
}
```

本次变更包含源码接入和本地 mock 测试。运行中的服务继续使用其已加载版本；部署新构建并配置代理后，可通过 `codex ticket acquired` 和 `codex ticket probe deferred` 日志观测状态。

## 本地验证

服务测试：`go test ./internal/service -run TestCodexTicket -count=1`。
配置测试：`go test ./internal/config -run TestCodexTicket -count=1`。
管理接口测试：`go test -tags=unit ./internal/handler/admin -run TestCodexTicket -count=1`。
界面测试：`pnpm exec vitest run src/views/admin/__tests__/SettingsView.spec.ts`。

本次工作区已有的账号到期测试引用了缺失方法，因此服务测试使用独立 overlay 排除该文件；原测试文件保持原样。完整命令、原始结果与回滚校验保存在任务的 `VERIFICATION.txt` 中。

## 账号列表与打票日志

- 启用门票功能后，OpenAI OAuth / setup-token 账号的用量单元格按配置模型显示门票剩余时间、打票中 / 冷却 / 暂停等状态，以及本轮打票次数。有效门票每秒更新倒计时，可见账号每 5 秒批量刷新状态。
- 点击模型状态行打开该账号、该模型的日志。日志按最新在前排列，包含时间、轮内次数、结果、原因、HTTP 状态、出口 IP / 国家、实际与目标长度、耗时。弹窗每次请求完成后等待 2 秒刷新，关闭弹窗或隐藏页面时暂停轮询。
- 当前进程最多保留 512 个账号 / 模型日志流，每流保留最近 200 条事件；较旧日志流按使用顺序淘汰，服务重启清空。成功取票后的新一轮请求重新计数，多实例分别记录。
- 出口诊断在本次打票的独立 HTTP/1.1 连接上匿名读取 `/cdn-cgi/trace`，并校验票据请求复用了同一连接。匿名诊断头部保持独立，账号认证仅用于票据请求。诊断预算为 3 秒，失败后继续打票并记录原因；出口国家取自诊断的 `loc` 字段。
- 管理接口仅返回状态、计数和诊断摘要；门票正文、账号认证、代理密码始终保留在内部。

管理 API：

- `POST /api/v1/admin/accounts/codex-tickets/batch`，请求体 `{"account_ids":[1,2]}`，最多 200 个正整数账号 ID，返回 `statuses` 与 `fetched_at`。
- `GET /api/v1/admin/accounts/:id/codex-ticket-logs?model=MODEL`，返回 `entries`、`status`、`limit`、`fetched_at`。
- 账号列表和单账号响应增加 `codex_tickets`。以上接口继续使用管理员认证并设置 `Cache-Control: no-store`。

账号显示专项验证见 `artifacts/codex-ticket-display/VERIFICATION.txt`；预览截图使用本地模拟账号数据。
