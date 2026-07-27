import { describe, expect, it } from 'vitest'
import {
  apiIntervalsToForm,
  creditsToUSD,
  formIntervalsToAPI,
  mTokToPerToken,
  perTokenToMTok,
  usdToCreditsValue,
  validateIntervals,
  type IntervalFormEntry,
} from '../types'

function makeInterval(over: Partial<IntervalFormEntry>): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: 0,
    ...over,
  }
}

function t(key: string, params?: Record<string, unknown>): string {
  return `${key}${params ? ` ${JSON.stringify(params)}` : ''}`
}

describe('validateIntervals', () => {
  describe('token mode', () => {
    it('rejects unbounded interval that is not last', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })

    it('accepts unbounded interval at the end', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: null, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toBeNull()
    })

    it('rejects overlapping intervals', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 250000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('overlap')
    })

    it('rejects unbounded interval in token mode', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 100, max_tokens: 200, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })
  })

  describe('image / per_request / video mode', () => {
    it('allows multiple unbounded tiers identified by label', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: 0.04 }),
        makeInterval({ tier_label: '2K', per_request_price: 0.06 }),
        makeInterval({ tier_label: '4K', per_request_price: 0.08 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toBeNull()
      expect(validateIntervals(intervals, 'per_request', t)).toBeNull()
      expect(validateIntervals(intervals, 'video', t)).toBeNull()
    })

    it('still rejects negative prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: -1 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('negativePrice')
    })

    it('still rejects max <= min on a single tier', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', min_tokens: 100, max_tokens: 50, per_request_price: 0.04 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('maxGreaterThanMin')
    })
  })

  describe('video_token mode', () => {
    it('accepts conditional output token prices', () => {
      const intervals = [
        makeInterval({ tier_label: 'default:text', output_price: 37 }),
        makeInterval({ tier_label: '720P:video', output_price: 28 }),
        makeInterval({ tier_label: '4k:text', output_price: 26 }),
      ]
      expect(validateIntervals(intervals, 'video_token', t)).toBeNull()
    })

    it('requires an output price and unique valid tier', () => {
      expect(validateIntervals([
        makeInterval({ tier_label: '720p:text', input_price: 46 }),
      ], 'video_token', t)).toContain('videoTokenOutputRequired')
      expect(validateIntervals([
        makeInterval({ tier_label: '720p:text', output_price: 46 }),
        makeInterval({ tier_label: '720P:text', output_price: 45 }),
      ], 'video_token', t)).toContain('duplicateVideoTokenTier')
      expect(validateIntervals([
        makeInterval({ tier_label: '2k:text', output_price: 1 }),
      ], 'video_token', t)).toContain('invalidVideoTokenTier')
    })
  })
})

describe('Credit price conversion', () => {
  it('converts Credit display values to backend USD values in both directions', () => {
    expect(mTokToPerToken(200)).toBe(2e-6)
    expect(perTokenToMTok(2e-6)).toBe(200)
    expect(creditsToUSD(5)).toBe(0.05)
    expect(usdToCreditsValue(0.05)).toBe(5)
  })

  it('converts interval token and request prices', () => {
    const form = [makeInterval({ input_price: 200, output_price: 800, per_request_price: 5 })]
    const api = formIntervalsToAPI(form)
    expect(api[0].input_price).toBe(2e-6)
    expect(api[0].output_price).toBe(8e-6)
    expect(api[0].per_request_price).toBe(0.05)
    expect(apiIntervalsToForm(api)).toEqual(form)
  })
})
