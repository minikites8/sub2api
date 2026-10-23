import type { GroupPlatform } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

export type CcSwitchClientType = 'claude' | 'gemini'

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
}

/** CC Switch substitutes the provider URL/key before evaluating this object. */
export const CC_SWITCH_USAGE_SCRIPT = `({
  request: {
    url: (function(base) {
      if (!/^https:\\/\\//i.test(base)) throw new Error("Usage queries require an HTTPS base URL");
      return base.replace(/\\/+$/, "").replace(/\\/(v1|v1beta)$/, "").replace(/\\/antigravity$/, "") + "/v1/usage";
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
})`

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

function withoutTrailingSlashes(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '')
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: `${baseUrl.replace(/\/+$/, '')}/antigravity`
      }
    case 'openai':
      return {
        app: 'codex',
        // CC Switch's Codex provider appends the OpenAI-compatible path itself.
        // Passing /v1 here can make the client request /v1/v1/....
        endpoint: withoutTrailingSlashes(baseUrl),
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl
      }
    case 'grok':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: GROK_CC_SWITCH_MODEL
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', btoa(Array.from(new TextEncoder().encode(input.usageScript), byte => String.fromCharCode(byte)).join(''))],
    ['usageAutoInterval', '30']
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
