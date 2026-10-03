import { describe, expect, it } from 'vitest'
import { CODEX_GATEWAY_BLACKLIST_EXTRA_KEY, parseCodexGatewayBlacklist, readCodexGatewayBlacklist } from '../codexGatewayBlacklist'

describe('Codex gateway blacklist', () => {
  it('normalizes aliases, commas, newlines and duplicates', () => {
    expect(parseCodexGatewayBlacklist(' UNIFIED-12, unified-35\nunified12\r\n35 ')).toEqual(['unified-12', 'unified-35'])
    expect(parseCodexGatewayBlacklist(['12', 'chat.gateway.unified-35.api.openai.com'])).toEqual(['unified-12', 'unified-35'])
    expect(parseCodexGatewayBlacklist(' ,\n')).toEqual([])
  })
  it('validates names and safely reads missing settings', () => {
    for (const value of ['any', 'unified-x', [12], {}, 'unified-12\r\nX-Foo: injected']) {
      expect(() => parseCodexGatewayBlacklist(value)).toThrow()
    }
    expect(readCodexGatewayBlacklist()).toEqual([])
    expect(readCodexGatewayBlacklist({ [CODEX_GATEWAY_BLACKLIST_EXTRA_KEY]: ['unified-12'] })).toEqual(['unified-12'])
  })
})
