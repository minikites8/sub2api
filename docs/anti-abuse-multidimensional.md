# 多维反滥用风控

## 信号

- 浏览器指纹：前端使用 FingerprintJS v4.6.2（兼容 FingerprintJS v3+ API）采集 `visitorId`，并叠加设备特征指纹；支持 `browser_fingerprints` 多值输入，服务端保存 SHA-256 精确哈希与数字归一化后的模糊桶哈希。FingerprintJS 加载失败时继续使用设备特征指纹。
- TLS 指纹：从 WAF/反向代理注入 `X-JA3`、`X-JA3-Fingerprint`、`X-JA3-Hash`、`X-TLS-JA3`、`X-Client-JA3`、`CF-JA3-*` 与对应 JA4 请求头；服务端仅采信来自 `server.trusted_proxies` 的直连代理，并校验格式与账号复用速度。JA3/JA4 原值不落库，只保存带命名空间的 SHA-256 哈希。
- IP 信誉：默认使用 IPPure 页面实际调用的上游，聚合 `/api/info/ip-risk/{ip}`、`/api/info/ip-basic/{ip}`、`/api/info/asn/botclass/{asn}` 与 `ipinfo.io/widget/demo/{ip}`。服务端实现 `x-k`/`x-t` 握手和 HMAC-SHA256 请求签名，结果缓存 15 分钟。
- 邮箱信誉：识别一次性邮箱域名、自动化命名模式和邮箱域名注册速度。
- 调用 UA：识别缺失 UA、脚本客户端、无浏览器标识的客户端；网关继续保留 IP+UA 兼容条件。
- 风控中心事件：注册评估、网关评估和实际赠金扣除均写入 `anti_abuse_events`，面板展示动作、评分、因子、网络摘要、指纹哈希存在性和扣除金额，并支持按动作和“仅扣除”筛选。

## 评分与处置

`score = sum(signal_contribution * weight)`。默认阈值为 60，权重均为 1；达到阈值进入 `restrict`，执行既有免费赠金扣除流程，付费账户保持原有保护逻辑。达到阈值 75% 进入 `review`，保留审计因子明细并继续使用旧阈值决定自动扣除。

## 配置

管理员设置新增：

- `anti_abuse_enabled`
- `anti_abuse_score_threshold`
- `anti_abuse_fingerprint_weight`
- `anti_abuse_ip_weight`
- `anti_abuse_email_weight`
- `anti_abuse_user_agent_weight`
- `anti_abuse_tls_fingerprint_weight`
- `anti_abuse_ip_reputation_endpoint`
- `anti_abuse_ip_reputation_api_key`

旧的注册 IP、API IP+UA 配置保持有效，用作兼容处置阈值。`anti_abuse_ip_reputation_endpoint` 留空时使用 IPPure 多源聚合；填写自定义地址时调用 `GET <endpoint>?ip=<client-ip>` 并读取 JSON 内的 `score`、`risk_score` 或 `reputation`。上游请求异常时使用本地评分。

关闭 `anti_abuse_enabled` 后，系统停止多维信誉查询、指纹持久化和 `anti_abuse_events` 新事件写入；旧的注册 IP、API IP+UA 兼容风控开关与阈值独立生效。关闭前已经产生的事件会继续保留在风控中心历史列表中。

## 发布顺序

1. 应用 migration `231_anti_abuse_multidimensional.sql`。
2. 在管理员设置填写 provider 地址和密钥，先保持 `anti_abuse_enabled=true`、阈值 60。
3. 观察风控扣款审计中的 `score` 与 `factors`，按误报率调整权重和阈值。
4. 对前端客户端逐步升级；旧客户端继续使用 IP、UA 和服务端请求上下文完成兼容判定。

风控中心页面 `/admin/risk-control` 的“多维反滥用事件”区域读取 `/admin/risk-control/anti-abuse/events`。赠金扣除同时保留现有 `risk_control_balance` 余额历史记录，事件流提供评分和因子上下文。
