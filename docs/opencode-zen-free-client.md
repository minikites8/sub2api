# OpenCode Zen 免费模型客户端适配

适配范围为 API Key 账号发往 `https://opencode.ai/zen/v1/chat/completions` 和
`https://opencode.ai/zen/v1/responses` 的 `*-free`、`big-pickle` 模型请求。
Go、付费模型、OAuth 和第三方中转继续使用原有请求及会话规则。

## 请求行为

- 官方 OpenCode 默认 User-Agent 为实测的 `opencode/latest/2.2.0/cli`；账号显式配置的 User-Agent 仍然优先。
- 免费请求携带 `X-OpenCode-Client`、`X-OpenCode-Project`、`X-OpenCode-Request`，并同步两个 OpenCode 会话头及亲和性头。移除 Codex/Stainless 客户端标识。
- 会话使用 OpenCode 的 `ses_` 格式，请求使用 `msg_` 格式。已有合法会话 ID 原样使用；普通客户端会话标识按本地 API Key 和上游账号隔离映射，跨轮次及协议转换保持稳定。
- 映射使用进程内 LRU，最多 4096 项，空闲 24 小时过期；进程重启、淘汰或切换实例可能生成新 ID。没有会话标识时，仅在本次请求及重试期间保持稳定。
- 上游使用 SSE，Chat Completions 开启用量返回。下游非流式 Chat 请求聚合为 JSON，保留多候选、推理、工具调用和用量；不完整或错误 SSE 返回错误。Responses 沿用已有 SSE 转 JSON 处理。
- 保留原始消息、工具定义及工具选择。没有工具定义的请求补充 `bash`、`read` 兼容占位，并强制 `tool_choice: none`，避免生成客户端无法执行的工具调用。
- 免费请求缺少系统级 `<env>` 时，追加网关上下文，明确客户端工作区未知。原有系统/开发者指令及分段内容保留；已有环境块不重复添加；不发送服务器真实路径。Responses 使用 `instructions`。
- 免费请求默认使用从 OpenCode 2.2.0 内置 Bun 1.4.4 重新采集的 TLS 指纹，包含 ALPN、Session Ticket 和 PSK 扩展；旧版 1.19.x 捕获不能替代。账号已启用 TLS 指纹时优先使用账号配置。客户端连接池按完整指纹隔离，代理和传输失败回退继续使用原有链路。

连接测试和真实转发共用适配。适配不保证上游接受请求；`FreeUsageLimitError`、429 等仍进入现有错误处理，没有豁免、无限重试或伪造成功响应。

## 参考与验证

参考开源实现的协议观察和 Bun ClientHello 捕获：

- [azain47/zen-proxy](https://github.com/azain47/zen-proxy)
- [lutfi-zain/pi-opencode-fix](https://github.com/lutfi-zain/pi-opencode-fix)
- [Yee-h/dsh-zen-proxy](https://github.com/Yee-h/dsh-zen-proxy)
- OpenCode 官方标识格式：[anomalyco/opencode](https://github.com/anomalyco/opencode)

这些项目对上游识别条件的观察存在差异。TLS 参数以实际安装的官方客户端捕获为准；没有引入压缩为最后一条消息的 CLI 回退。

### 2026-11-17 真实调用对照

使用用户授权的临时 key，仅发送简短测试提示词，未将 key 写入文件或测试夹具。

| 请求 | 结果 |
| --- | --- |
| 官方 OpenCode 2.2.0，完整配置，`hy4-flash-free` | 200，输出 `OK` |
| 相同客户端请求通过 Node fetch | 429，`FreeUsageLimitError` |
| Go 使用旧版 Bun 指纹及相同客户端请求 | 429，`FreeUsageLimitError` |
| Go 使用 Bun 1.4.4 捕获及相同客户端请求 | 200，输出 `OK` |
| Go 使用新指纹、网关环境块及禁用的占位工具，`hy4-flash-free` | 200，输出 `OK` |
| 同上，`beam-free` / `mimo-v2.6-flash-free` | 均为 200，输出 `OK` |
| Go 使用新指纹、环境指令，原生 `hy4-flash-free` Responses | 200，输出 `OK`，收到 `response.completed` |

在该次对照中，使用新指纹但移除系统环境上下文仍返回 429。因此不能仅凭该错误名或较长的 `Retry-After` 就认定真实免费额度已耗尽。服务端也可能存在真实额度限制，仍须保留原有 429 处理。

最终验证直接使用仓库的指纹工厂和环境块处理代码；没有启动完整网关服务，也未重编整个 service 测试包。验证期间发生过一次 TLS 握手 EOF，在尚无 HTTP 响应时仅重试一次后成功。
