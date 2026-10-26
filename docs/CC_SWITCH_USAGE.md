# CC Switch 多周期用量查询

新版一键导入使用 `GET /v1/usage`，Bearer API Key 认证，`User-Agent: cc-switch/1.0`。只发送一个 HTTPS 同源请求；站点根地址、`/v1`、尾部斜杠及原有供应商协议后缀会归一化。已导入的供应商需重新导入或手动替换脚本；保留 `usageEnabled=true`、Base URL、API Key、客户端与模型配置。

## Key 页面选择导入端点

点击某个 Key 的“导入到 CCS”后，如果站点配置了多个不同的 API 端点，会先显示单选弹窗，列出默认端点和自定义端点的名称、地址及说明。默认预选默认端点，选择后点击“继续导入”。所选地址用于这一次导入，不修改站点默认地址，也不沿用其他 Key 上次取消的选择。

只有一个端点时直接沿用原导入流程；未配置默认端点但有自定义端点时使用自定义端点，完全没有配置时回退当前站点地址。空地址会忽略，仅尾部斜杠不同的重复地址视为同一端点。公开设置尚未加载时暂不允许导入，避免提前使用错误的默认地址。

Antigravity 在端点选择后继续选择 Claude Code 或 Gemini CLI，选中的端点会保留到最终配置。OpenAI/Codex、Claude、Gemini 和 Grok 继续使用已有客户端及路径规则。取消端点或客户端弹窗均不会发起导入；弹窗不展示 API Key 明文。最终导入仍使用所点击行的 Key，并同步设置所选 Base URL 及原用量脚本。

2026-10-26 回归：Key 页面、导入链接、端点展示及语言检查共 6 个文件 / 145 项测试通过，前端类型检查和生产构建（18.87s）通过。测试拦截 `window.open`，只核对生成的配置，没有启动真实 CCS、导入真实密钥或发送模型请求；实际客户端协议注册和跨系统唤起仍需发布后使用测试 Key 人工核验。

## 响应与额度含义

`/v1/usage` 返回未包裹的 JSON。未添加数据库字段，未修改计费倍率、扣减、准入或窗口维护规则。

| 字段 | 含义 |
| --- | --- |
| `isValid`、`invalidMessage`、`status` | Key 状态；过期/独立总额度耗尽或缺少有效订阅时为无效。订阅单周期耗尽保留 `isValid=true`，其剩余为零。禁用/不存在 Key 由原认证中间件返回 401，extractor 转为无效结果。 |
| `planName`、`unit`、`mode` | 套餐名、USD 和既有模式。 |
| `remaining` | 兼容旧单对象脚本：有 Key 总额度时为 Key 总额度剩余，否则为订阅最小周期剩余或钱包余额。完全无限额仍保留旧值 -1；新脚本不将其显示为金额。 |
| `subscription.{daily,weekly,monthly}_limit_usd` | 订阅真实周期限额；null/非正数代表该周期不限额。 |
| `subscription.{daily,weekly,monthly}_usage_usd` | 该周期真实已用 USD；零值明确返回。 |
| `subscription.{daily,weekly,monthly}_remaining_usd` | 配置正数限额时返回 `max(0, limit-usage)`。 |
| `subscription.{daily,weekly,monthly}_percentage` | 配置正数限额时返回使用百分比，最大 100。超额时保留真实已用数值，剩余为零。 |
| `subscription.{daily,weekly,monthly}_window_start` | 既有实际窗口起点；未经激活或已过期的展示窗口为 null。保留原 `weekly_window_start` 字段。 |
| `subscription.{daily,weekly,monthly}_reset_at` | 仅有实际有效窗口且未来确有一次重置时返回。单日一次性卡到期、最后一个周期到期、无窗口及刚重置但尚未持久化锚点时不编造时间。 |
| `subscription.expires_at`、`status`、`unlimited` | 订阅期限、状态及是否全部周期无限额。 |
| `quota.{limit,used,remaining,unit}` | 当前 API Key 的独立总配额，与订阅额度同时返回，互不替代。 |
| `rate_limits[]` | 当前 API Key 的 5h/1d/7d USD 速率窗口，含 `window/limit/used/remaining/window_start/reset_at`，不会标记成订阅的日/周/月额度。 |
| `balance` | 钱包模式余额；Key 只配置速率额度时仍保留钱包余额展示。 |
| `usage`、`daily_usage`、`model_stats` | 原有当前 Key 的统计字段，继续保留。 |

认证照常执行用户、Key、分组权限及 IP 检查。随后按已认证的 Key ID/用户 ID 只读刷新 Key 配额与选中绑定组，订阅查询严格限定当前用户和该组。不会返回其他 Key、其他用户或内部上游账号数据。轮询不更新 `last_used`，响应为 `Cache-Control: no-store`。

订阅金额继续使用实际扣减的 USD 计数，不根据顶层最小余额反推，也不再除以分组倍率。多个绑定组沿用 `/v1/usage` 已有元数据选号逻辑；不会为了展示而合并全部分组或跳过某个已耗尽周期。日窗口按既有配置时区日历日、周/月按既有 7/30 天规则展示；过期窗口只在返回快照中归零，不写数据库。

## 可直接复制的查询脚本

每个配置了正数限额的订阅周期为一个数组项，套餐名附带「日限额 / 周限额 / 月限额」。`percentage` 是扩展信息，同时用 `extra` 展示百分比以兼容只识别官方基础字段的客户端。单独钱包或 Key 总配额仍可返回单对象。CC Switch 可能显示为“N 个套餐”，展开即可查看各周期。

```javascript
({
  request: {
    url: (function(base) {
      if (!/^https:\/\//i.test(base)) throw new Error("Usage queries require an HTTPS base URL");
      return base.replace(/\/+$/, "").replace(/\/(v1|v1beta)$/, "").replace(/\/antigravity$/, "") + "/v1/usage";
    })("{{baseUrl}}"),
    method: "GET",
    headers: { "Authorization": "Bearer {{apiKey}}", "User-Agent": "cc-switch/1.0" }
  },
  extractor: function(response) {
    var r = response || {};
    var unit = r.unit || "USD";
    var name = r.planName || "套餐";
    var items = [];
    function number(value) {
      if (typeof value !== "number" && typeof value !== "string") return null;
      if (typeof value === "string" && value.trim() === "") return null;
      var n = Number(value);
      return isFinite(n) ? n : null;
    }
    function invalid(message) {
      return { isValid: false, invalidMessage: message || "用量查询失败", planName: name, unit: unit };
    }
    if (r.error || r.isValid !== true || r.status === "expired" || r.status === "disabled") {
      return invalid(r.invalidMessage || (r.error && r.error.message) || r.message || "API Key 无效或已过期");
    }
    function add(label, limit, usage, reset) {
      var total = number(limit);
      if (total === null || total <= 0) return;
      var used = number(usage);
      if (used === null || used < 0) {
        var bad = invalid("该周期的用量数据不可用");
        bad.planName = name + " · " + label;
        items.push(bad);
        return;
      }
      var percent = Math.round(Math.min(100, used / total * 100) * 100) / 100;
      var extra = "已用 " + percent + "%";
      if (typeof reset === "string" && isFinite(Date.parse(reset))) extra += " · 重置 " + reset;
      items.push({ isValid: true, planName: name + " · " + label, remaining: Math.max(0, total - used), used: used, total: total, unit: unit, percentage: percent, extra: extra });
    }
    var sub = r.subscription;
    if (sub != null && (typeof sub !== "object" || Array.isArray(sub))) return invalid("订阅数据格式异常");
    if (sub && typeof sub === "object" && !Array.isArray(sub)) {
      add("日限额", sub.daily_limit_usd, sub.daily_usage_usd, sub.daily_reset_at);
      add("周限额", sub.weekly_limit_usd, sub.weekly_usage_usd, sub.weekly_reset_at);
      add("月限额", sub.monthly_limit_usd, sub.monthly_usage_usd, sub.monthly_reset_at);
      if (!items.length) {
        if (sub.unlimited === true || r.unlimited === true || r.remaining === -1) {
          items.push({ isValid: true, planName: name, unit: unit, extra: "无限额" });
        } else {
          items.push(invalid("订阅周期额度数据不可用"));
        }
      }
    }
    var quota = r.quota;
    if (quota && typeof quota === "object") add("API Key 总额度", quota.limit, quota.used);
    if (!sub && !(quota && number(quota.limit) > 0)) {
      var balance = number(r.balance);
      if (balance === null) balance = number(r.remaining);
      if (r.unlimited === true || balance === -1) {
        items.push({ isValid: true, planName: name, unit: unit, extra: "无限额" });
      } else if (balance !== null && balance >= 0) {
        items.push({ isValid: true, planName: name, remaining: balance, unit: unit });
      } else {
        items.push(invalid("余额数据不可用"));
      }
    }
    var rates = Array.isArray(r.rate_limits) ? r.rate_limits : [];
    var labels = { "5h": "5小时", "1d": "24小时", "7d": "7天" };
    for (var i = 0; i < rates.length; i++) {
      var rate = rates[i];
      if (rate && (rate.window === "5h" || rate.window === "1d" || rate.window === "7d")) add("API Key " + labels[rate.window] + "速率额度", rate.limit, rate.used, rate.reset_at);
    }
    return sub || items.length > 1 ? items : items[0];
  }
})
```

脚本仅使用标准 JavaScript。数值字段缺失/null/异常时不会虚构用量；有效正数限额对应的用量不可用会产生带原因的无效项。全部无限额仅显示套餐和「无限额」，不输出 -1 或伪造 0 USD。接口错误、Key 失效返回 `isValid:false` 与 `invalidMessage`。

## 兼容性与本地验证

- `/api/usage` 的 GET/POST 旧协议保留，仍是首个可用绑定组的单余额；本次不修改它的语义。
- `/v1/usage` 的顶层兼容字段和 `quota/rate_limits` 保留，订阅数据补充到有 Key 配额/速率额度的响应中。修正过去过期 Key 仍显示 `isValid:true` 的结果。
- 不支持数组的客户端可继续使用原单对象 extractor；新脚本需要客户端支持对象数组。
- `go test -p 2 -tags=unit ./internal/server/routes ./internal/server/middleware -count=1`：全部通过，含 52 个新增的标准/simple 模式认证场景及旧 `/api/usage` 回归；763 个测试/子测试通过。
- `go test -p 2 -tags=unit ./internal/service ./internal/handler -run 'Test(SubscriptionUsageSnapshot|APIKeyUsageSubscriptionScope|RefreshUsageKey|UsageUnrestrictedIncludesWeeklyWindowStart)' -count=1`：7 项通过，涵盖周期快照、只读滚动、一次性日卡、用户归属与缓存计数刷新。
- `vitest run src/utils/__tests__/ccswitchImport.spec.ts src/utils/__tests__/ccswitchImport.antigravityUrl.spec.ts src/views/user/__tests__/KeysView.spec.ts`：125 项通过，包含文档与导入脚本一致性。
- Python QuickJS 1.19.4 临时隔离运行时：17 种 extractor 响应及 HTTPS/同源 GET 配置断言通过；脚本不依赖 Node、浏览器或网络全局对象。
- Vue 类型检查、变更文件 ESLint、后端 `go build ./cmd/server`、前端 `vite build`：通过。构建产物写入临时目录；仅有已有 Browserslist 数据及大 chunk 提示。
- 所有上述检查使用本次当前工作树，保留其他会话的未提交改动。验证未改动数据库或部署服务；提交推送按后续明确授权执行。
- 未连接真实 CC Switch 桌面客户端或生产服务；没有声称桌面视觉验收、真实数据库端到端或生产部署完成。未运行完整仓库所有测试或 race；本次执行了相关路由/认证全套和用量服务定向回归。
