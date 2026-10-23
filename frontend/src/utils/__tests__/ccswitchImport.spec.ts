import { describe, expect, it } from 'vitest'
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
  // Mirrors CC Switch: substitute the template vars as text, evaluate, read request.url.
  function usageUrlFor(baseUrl: string): string {
    const script = CC_SWITCH_USAGE_SCRIPT.split('{{baseUrl}}').join(baseUrl).split('{{apiKey}}').join('sk-test')
    // eslint-disable-next-line no-new-func
    const config = new Function(`return ${script}`)() as { request: { url: string } }
    return config.request.url
  }

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('queries the authenticated /api/usage endpoint for base URL %s', (baseUrl) => {
    expect(usageUrlFor(baseUrl)).toBe('https://api.example.com/api/usage')
  })

  it('works against the endpoint every platform import stores', () => {
    for (const platform of ['anthropic', 'openai', 'grok', 'gemini', 'antigravity'] as GroupPlatform[]) {
      const endpoint = paramsFromDeeplink(
        buildCcSwitchImportDeeplink({
          baseUrl: 'https://api.example.com',
          platform,
          clientType: platform === 'gemini' ? 'gemini' : 'claude',
          providerName: 'Sub2API',
          apiKey: 'sk-test',
          usageScript: CC_SWITCH_USAGE_SCRIPT
        })
      ).get('endpoint') as string
      expect(usageUrlFor(endpoint)).toBe('https://api.example.com/api/usage')
    }
  })

  function usageConfig() {
    const script = CC_SWITCH_USAGE_SCRIPT.replaceAll('{{baseUrl}}', 'https://api.example.com/v1').replaceAll('{{apiKey}}', 'sk-test')
    // eslint-disable-next-line no-new-func
    return new Function(`return ${script}`)() as {
      request: { method: string; headers: Record<string, string> }
      extractor: (response: unknown) => Record<string, unknown>
    }
  }

  it('uses the supplied POST/Bearer contract without a request body or dashboard JWT', () => {
    expect(usageConfig().request).toMatchObject({
      method: 'POST', headers: { Authorization: 'Bearer sk-test', 'User-Agent': 'cc-switch/1.0' }
    })
  })

  it('also supports the original CC Switch extractor unchanged', () => {
    // eslint-disable-next-line no-new-func
    const config = new Function(`return ({
      request: {
        url: "{{baseUrl}}/api/usage",
        method: "POST",
        headers: { "Authorization": "Bearer {{apiKey}}", "User-Agent": "cc-switch/1.0" }
      },
      extractor: function(response) {
        return { isValid: !response.error, remaining: response.balance, unit: "USD" };
      }
    })`)() as { extractor: (response: unknown) => Record<string, unknown> }
    expect(config.extractor({ balance: 3, isValid: true })).toEqual({ isValid: true, remaining: 3, unit: 'USD' })
    expect(config.extractor({ balance: 0, error: { code: 'NO_AVAILABLE_GROUP', message: 'No quota' } }))
      .toEqual({ isValid: false, remaining: 0, unit: 'USD' })
    expect(config.extractor({ error: { code: 'QUOTA_UNAVAILABLE', message: 'Failed to query group quota' } }).isValid).toBe(false)
  })

  it('extracts only the selected group quota and plan metadata', () => {
    expect(usageConfig().extractor({ isValid: true, balance: 6, unit: 'USD', planName: 'Second group', total: 40, used: 34 }))
      .toMatchObject({ isValid: true, remaining: 6, unit: 'USD', planName: 'Second group', total: 40, used: 34 })
  })

  it('reports unavailable or unauthorized keys instead of assuming success', () => {
    expect(usageConfig().extractor({ isValid: false, balance: 0, invalidMessage: 'No available bound group' }))
      .toMatchObject({ isValid: false, invalidMessage: 'No available bound group', remaining: 0 })
    expect(usageConfig().extractor({ error: { message: 'Invalid API key' } }))
      .toMatchObject({ isValid: false, invalidMessage: 'Invalid API key' })
    expect(usageConfig().extractor({ message: 'Access denied' }))
      .toMatchObject({ isValid: false, invalidMessage: 'Access denied' })
  })

  it('represents unlimited plans without displaying a negative dollar balance', () => {
    expect(usageConfig().extractor({ isValid: true, balance: -1, unlimited: true, planName: 'Unlimited plan' }))
      .toMatchObject({ isValid: true, remaining: undefined, extra: 'Unlimited' })
  })

  it('preserves a deployment prefix while stripping provider protocol suffixes', () => {
    expect(usageUrlFor('https://api.example.com/prefix/antigravity/v1/')).toBe('https://api.example.com/prefix/api/usage')
    expect(usageUrlFor('https://api.example.com/v1beta/')).toBe('https://api.example.com/api/usage')
  })

})
