import { describe, expect, it } from 'vitest'
import usageDocumentation from '../../../../docs/CC_SWITCH_USAGE.md?raw'
import {
  CC_SWITCH_USAGE_SCRIPT,
  GROK_CC_SWITCH_MODEL,
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.5')
  })

  it('defaults Grok Build imports to the current Grok model', () => {
    expect(GROK_CC_SWITCH_MODEL).toBe('grok-4.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it.each([
    ['https://api.example.com', 'https://api.example.com'],
    ['https://api.example.com/', 'https://api.example.com'],
    ['https://api.example.com/v1', 'https://api.example.com/v1'],
    ['https://api.example.com/v1/', 'https://api.example.com/v1']
  ])('keeps Codex imports on the configured endpoint for base URL %s', (baseUrl, endpoint) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'grok',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe(GROK_CC_SWITCH_MODEL)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
  })

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/antigravity`)
    expect(params.has('model')).toBe(false)
  })
})

describe('CC Switch usage script', () => {
  function usageConfig(baseUrl = 'https://api.example.com/v1') {
    const script = CC_SWITCH_USAGE_SCRIPT.split('{{baseUrl}}').join(baseUrl).split('{{apiKey}}').join('sk-test')
    // eslint-disable-next-line no-new-func
    return new Function(`return ${script}`)() as {
      request: { url: string; method: string; headers: Record<string, string> }
      extractor: (response: unknown) => Record<string, unknown> | Record<string, unknown>[]
    }
  }
  function rows(response: unknown) {
    const result = usageConfig().extractor(response)
    return Array.isArray(result) ? result : [result]
  }
  function subscription(overrides = {}) {
    return { isValid: true, planName: 'Pro稳定组', unit: 'USD', remaining: 58,
      subscription: { daily_limit_usd: 90, daily_usage_usd: 32, weekly_limit_usd: 500, weekly_usage_usd: 285, monthly_limit_usd: 1300, monthly_usage_usd: 720, ...overrides } }
  }

  it.each(['https://api.example.com', 'https://api.example.com/', 'https://api.example.com/v1', 'https://api.example.com/v1/'])('normalizes one GET /v1/usage for %s', (baseUrl) => {
    expect(usageConfig(baseUrl).request).toEqual({ url: 'https://api.example.com/v1/usage', method: 'GET', headers: { Authorization: 'Bearer sk-test', 'User-Agent': 'cc-switch/1.0' } })
  })
  it('keeps all import platforms on the same origin and handles deployment prefixes', () => {
    for (const platform of ['anthropic', 'openai', 'grok', 'gemini', 'antigravity'] as GroupPlatform[]) {
      const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ baseUrl: 'https://api.example.com', platform, clientType: 'claude', providerName: 'Sub2API', apiKey: 'sk-test', usageScript: CC_SWITCH_USAGE_SCRIPT }))
      expect(usageConfig(params.get('endpoint')!).request.url).toBe('https://api.example.com/v1/usage')
      expect(params.get('usageEnabled')).toBe('true')
      const decoded = new TextDecoder().decode(Uint8Array.from(atob(params.get('usageScript')!), c => c.charCodeAt(0)))
      expect(decoded).toBe(CC_SWITCH_USAGE_SCRIPT)
    }
    expect(usageConfig('https://api.example.com/prefix/antigravity/v1/').request.url).toBe('https://api.example.com/prefix/v1/usage')
  })
  it('requires HTTPS without redirecting a key to a different origin', () => {
    expect(() => usageConfig('http://api.example.com')).toThrow('HTTPS')
    const configured = 'https://api.example.com:9443/prefix/v1/'
    expect(new URL(usageConfig(configured).request.url).origin).toBe(new URL(configured).origin)
  })
  it('preserves the old single-object /v1/usage extractor fields', () => {
    const oldExtractor = (r: { isValid: boolean; remaining: number; planName: string; unit: string }) => ({ isValid: r.isValid, remaining: r.remaining, planName: r.planName, unit: r.unit })
    expect(oldExtractor(subscription())).toEqual({ isValid: true, remaining: 58, planName: 'Pro稳定组', unit: 'USD' })
  })
  it('returns the three real periods instead of deriving them from the minimum', () => {
    const input = subscription()
    input.remaining = 1
    expect(rows(input)).toEqual([
      expect.objectContaining({ isValid: true, planName: 'Pro稳定组 · 日限额', used: 32, total: 90, remaining: 58, unit: 'USD', percentage: 35.56 }),
      expect.objectContaining({ isValid: true, planName: 'Pro稳定组 · 周限额', used: 285, total: 500, remaining: 215, percentage: 57 }),
      expect.objectContaining({ isValid: true, planName: 'Pro稳定组 · 月限额', used: 720, total: 1300, remaining: 580 })
    ])
    for (const row of rows(input)) expect(Number(row.used) + Number(row.remaining)).toBe(row.total)
  })
  it('shows only configured positive periods and preserves zero usage', () => {
    expect(rows(subscription({ daily_usage_usd: 0, weekly_limit_usd: null, monthly_limit_usd: 0 })))
      .toEqual([expect.objectContaining({ used: 0, total: 90, remaining: 90, percentage: 0 })])
  })
  it.each([90, 100])('does not call a day-exhausted plan expired: used=%s', (used) => {
    const result = rows(subscription({ daily_usage_usd: used }))
    expect(result[0]).toMatchObject({ isValid: true, used, remaining: 0, percentage: 100 })
    expect(result[1]).toMatchObject({ isValid: true, remaining: 215 })
    expect(result[2]).toMatchObject({ isValid: true, remaining: 580 })
  })
  it('keeps monthly quota visible after weekly exhaustion', () => {
    const result = rows(subscription({ weekly_usage_usd: 600 }))
    expect(result[1]).toMatchObject({ used: 600, remaining: 0, isValid: true })
    expect(result[2].remaining).toBe(580)
  })
  it.each([undefined, null, '', 'invalid', NaN, Infinity, -1])('does not invent missing or malformed usage: %s', (used) => {
    expect(rows(subscription({ daily_usage_usd: used }))[0]).toMatchObject({ isValid: false, invalidMessage: '该周期的用量数据不可用' })
    expect(rows(subscription({ daily_usage_usd: used }))[0]).not.toHaveProperty('remaining')
  })
  it.each([undefined, null, 0, -1, 'invalid', Infinity])('does not display an invalid/unlimited limit as dollars: %s', (limit) => {
    expect(rows(subscription({ daily_limit_usd: limit }))).toHaveLength(2)
  })
  it('accepts finite numeric strings without treating null or booleans as zero', () => {
    expect(rows(subscription({ daily_limit_usd: '90', daily_usage_usd: '0' }))[0]).toMatchObject({ total: 90, used: 0, remaining: 90 })
    expect(rows(subscription({ daily_usage_usd: false }))[0].isValid).toBe(false)
  })
  it('represents an entirely unlimited subscription without a dollar amount', () => {
    expect(rows({ isValid: true, planName: 'Unlimited', remaining: -1, subscription: { daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null, unlimited: true } }))
      .toEqual([{ isValid: true, planName: 'Unlimited', unit: 'USD', extra: '无限额' }])
  })
  it('does not infer unlimited status from missing period data', () => {
    expect(rows({ isValid: true, subscription: {} })[0]).toMatchObject({ isValid: false, invalidMessage: '订阅周期额度数据不可用' })
  })
  it.each([[], 'invalid', 1])('rejects a malformed subscription object: %j', (subscription) => {
    expect(rows({ isValid: true, subscription })[0]).toMatchObject({ isValid: false, invalidMessage: '订阅数据格式异常' })
  })
  it('preserves wallet and independent key quota modes', () => {
    expect(usageConfig().extractor({ isValid: true, planName: '钱包余额', balance: 8, unit: 'USD' })).toMatchObject({ remaining: 8, planName: '钱包余额' })
    expect(usageConfig().extractor({ isValid: true, remaining: 9, quota: { limit: 50, used: 12 } })).toMatchObject({ used: 12, total: 50, remaining: 38, planName: '套餐 · API Key 总额度' })
    expect(usageConfig().extractor({ isValid: true, balance: 0 })).toMatchObject({ isValid: true, remaining: 0 })
  })
  it('keeps key total and rate limits separate from subscription windows', () => {
    const result = rows({ ...subscription(), quota: { limit: 100, used: 10 }, rate_limits: [{ window: '5h', limit: 10, used: 0 }, { window: '1d', limit: 20, used: 4 }, { window: '7d', limit: 30, used: 5 }] })
    expect(result).toHaveLength(7)
    expect(result[3]).toMatchObject({ planName: 'Pro稳定组 · API Key 总额度', remaining: 90 })
    expect(result[4]).toMatchObject({ planName: 'Pro稳定组 · API Key 5小时速率额度', used: 0, total: 10, remaining: 10 })
    expect(result[5].planName).toContain('24小时速率额度')
    expect(result[6].planName).toContain('7天速率额度')
  })
  it('shows a real backend reset only and does not synthesize dates', () => {
    const result = rows(subscription({ daily_reset_at: '2026-11-01T00:00:00+08:00', weekly_reset_at: null, monthly_reset_at: 'bad' }))
    expect(result[0].extra).toContain('2026-11-01T00:00:00+08:00')
    expect(result[1].extra).not.toContain('重置')
    expect(result[2].extra).not.toContain('重置')
  })
  it.each([null, {}, { error: { message: 'Invalid key' } }, { code: 401, message: 'Disabled' }, { isValid: false, invalidMessage: 'Expired' }, { isValid: true, status: 'expired' }])('rejects failed or invalid-key responses: %j', (response) => {
    expect(rows(response)[0].isValid).toBe(false)
    expect(rows(response)[0].invalidMessage).toBeTruthy()
  })
  it('uses only a single request and standard sandbox JavaScript', () => {
    expect(CC_SWITCH_USAGE_SCRIPT).not.toMatch(/\b(?:const|let|document|Buffer)\b|\b(?:fetch|require)\s*\(|\bwindow\s*[.(]|\?\.|=>/)
    expect(usageConfig().request.method).toBe('GET')
  })
  it('keeps the documented copyable script identical to the imported configuration', () => {
    const example = usageDocumentation.split('```javascript\n')[1]?.split('\n```')[0]
    expect(example).toBe(CC_SWITCH_USAGE_SCRIPT)
  })
})
