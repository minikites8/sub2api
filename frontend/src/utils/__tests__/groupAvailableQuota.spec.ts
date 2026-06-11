import { describe, expect, it } from 'vitest'
import { getReverseAvailableQuota } from '@/utils/groupQuota'

describe('group available quota reverse calculation', () => {
  it('calculates available quota from balance and rate multiplier', () => {
    expect(getReverseAvailableQuota(10, 0.5)).toBe(20)
    expect(getReverseAvailableQuota(10, 2)).toBe(5)
  })

  it('treats zero multiplier as unlimited', () => {
    expect(getReverseAvailableQuota(10, 0)).toBe(Number.POSITIVE_INFINITY)
  })
})
