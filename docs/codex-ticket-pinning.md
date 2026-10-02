# Codex 门票固定出口与会话身份

移植来源：用户提供的 mracry/sub2api 源码快照（ZIP SHA-256 `2151721700fe475982e594cf47fb94ffdbe18d26e9aebdd3216d7584665b73f6`）及《Codex门票钉住原理.md》。原作者的 30 分钟实测属于其报告，本分支尚未复现。

本版将票据绑定到采票时的出口节点和会话身份。打票成功后，HTTP/SSE 与 WebSocket 业务请求复用相同出口；请求体会移除采票时不存在的 `client_metadata`、`prompt_cache_key` 和设备身份字段，并沿用上游返回的 `x-codex-turn-state`。

同一账号在业务请求期间暂停采票，避免后台探针轮换或消耗当前会话。票据过期、被 312/响应形态拒绝或出口节点不可用时，当前会话失效，后续请求需要新开会话并重新采票。


292/312 只是 `x-codex-turn-state` 的形态，不能单独证明模型质量；发布验收仍需查看实际 `response.model`。

## SUCK_MY_ASTRA 云端取票

Sub2api 支持把 780 票据探测交给 SUCK_MY_ASTRA 兼容 relay。Sub2api 继续负责账号级、模型级票据缓存、`__cflb/__oailb` 路由缓存、模型声明校验和过期裁剪；relay 负责使用账号凭据向 `chatgpt.com` 铸造票据并返回路由 pair。

```yaml
gateway:
  openai_codex_ticket:
    enabled: true
    target_length: 780
    cloud_mint:
      enabled: true
      url: https://your-relay.example/
      key_env: SUB2API_CODEX_CLOUD_MINT_KEY
      proxy_url: ""
      transport: sse
      gateway: any
      timeout_seconds: 25
```

```powershell
$env:SUB2API_CODEX_CLOUD_MINT_KEY = "your-relay-key"
```

`cloud_mint.url` 使用 relay 的 HTTPS 地址；本机 relay 可使用 `http://127.0.0.1:<port>/`。relay 返回的 `served_model`、票据长度、签发时间、过期时间和路由 Cookie 会全部经过 Sub2api 校验。relay 请求失败时沿用现有 `fail_closed` 行为。
