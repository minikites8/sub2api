import { describe, expect, it } from 'vitest'
import {
  applyUpstreamRetrySettings,
  isValidUpstreamMaxRetries,
  readUpstreamRetrySettings
} from '../upstreamRetry'

describe('upstream retry credentials', () => {
  it('persists enabled, explicit zero and the upstream default distinctly', () => {
    const credentials = { api_key: 'test-key', unrelated: true }
    applyUpstreamRetrySettings(credentials, true, 0, 'create')
    expect(readUpstreamRetrySettings(credentials)).toEqual({ enabled: true, maxRetries: 0 })
    applyUpstreamRetrySettings(credentials, true, '', 'edit')
    expect(credentials).toMatchObject({ openai_upstream_retry_account: true, openai_upstream_max_retries: null })
    expect(readUpstreamRetrySettings(credentials)).toEqual({ enabled: true, maxRetries: '' })
    applyUpstreamRetrySettings(credentials, false, 3, 'edit')
    expect(credentials).toEqual({ api_key: 'test-key', unrelated: true, openai_upstream_retry_account: false, openai_upstream_max_retries: null })
  })

  it('does not add settings to default creates or enable malformed saved settings', () => {
    const credentials = {}
    applyUpstreamRetrySettings(credentials, false, '', 'create')
    expect(credentials).toEqual({})
    expect(readUpstreamRetrySettings({ openai_upstream_retry_account: 'true', openai_upstream_max_retries: -1 })).toEqual({ enabled: false, maxRetries: '' })
  })

  it('accepts only bounded nonnegative integer limits or blank', () => {
    for (const value of ['', 0, 2, 2147483647]) expect(isValidUpstreamMaxRetries(value)).toBe(true)
    for (const value of [-1, 1.5, 2147483648, NaN, Infinity, '2', null]) expect(isValidUpstreamMaxRetries(value)).toBe(false)
  })
})
