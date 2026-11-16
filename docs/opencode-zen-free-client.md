# OpenCode Zen 免费模型客户端适配

适配范围为 API Key 账号发往 `https://opencode.ai/zen/v1/chat/completions` 和
`https://opencode.ai/zen/v1/responses` 的 `*-free`、`big-pickle` 模型请求。
Go、付费模型、OAuth 和第三方中转继续使用原有请求及会话规则。

## 请求行为

- 官方 OpenCode 默认 User-Agent 更新为 `opencode/1.19.5`；账号显式配置的 User-Agent 仍然优先。
- 免费请求携带 `X-OpenCode-Client`、`X-OpenCode-Project`、`X-OpenCode-Request`，并同步两个 OpenCode 会话头及亲和性头。移除 Codex/Stainless 客户端标识。
- 会话使用 OpenCode 的 `ses_` 格式，请求使用 `msg_` 格式。已有合法会话 ID 原样使用；普通客户端会话标识按本地 API Key 和上游账号隔离映射，跨轮次及协议转换保持稳定。
- 映射使用进程内 LRU，最多 4096 项，空闲 24 小时过期；进程重启、淘汰或切换实例可能生成新 ID。没有会话标识时，仅在本次请求及重试期间保持稳定。
- 上游使用 SSE，Chat Completions 开启用量返回。下游非流式 Chat 请求聚合为 JSON，保留多候选、推理、工具调用和用量；不完整或错误 SSE 返回错误。Responses 沿用已有 SSE 转 JSON 处理。
- 保留原始消息、工具定义及工具选择。没有工具定义的请求补充 `bash`、`read` 兼容占位，并强制 `tool_choice: none`，避免生成客户端无法执行的工具调用。
- 免费请求默认使用内置 Bun TLS 指纹；账号已启用 TLS 指纹时优先使用账号配置。客户端连接池按完整指纹隔离，代理和传输失败回退继续使用原有链路。

连接测试和真实转发共用适配。适配不保证上游接受请求；`FreeUsageLimitError`、429 等仍进入现有错误处理，没有豁免、无限重试或伪造成功响应。

## 参考与验证

参考开源实现的协议观察和 Bun ClientHello 捕获：

- [azain47/zen-proxy](https://github.com/azain47/zen-proxy)
- [lutfi-zain/pi-opencode-fix](https://github.com/lutfi-zain/pi-opencode-fix)
- [Yee-h/dsh-zen-proxy](https://github.com/Yee-h/dsh-zen-proxy)
- OpenCode 官方标识格式：[anomalyco/opencode](https://github.com/anomalyco/opencode)

这些项目对上游识别条件的观察存在差异，本实现没有引入改写系统提示词或压缩为最后一条消息的 CLI 回退。
本地测试覆盖请求、会话、TLS ClientHello、连接池及 SSE 聚合；尚未使用真实账号验证免费请求是否被上游接受。
