import { describe, expect, it } from 'vitest'
import { getReverseAvailableQuota, getSubscriptionAvailableQuota } from '@/utils/groupQuota'

describe('group available quota reverse calculation', () => {
  it('calculates available quota from balance and rate multiplier', () => {
    expect(getReverseAvailableQuota(10, 0.5)).toBe(20)
    expect(getReverseAvailableQuota(10, 2)).toBe(5)
  })

  it('treats zero multiplier as unlimited', () => {
    expect(getReverseAvailableQuota(10, 0)).toBe(Number.POSITIVE_INFINITY)
  })
})

describe('subscription group available quota', () => {
  const subscription = {
    status: 'active' as const,
    expires_at: '2099-01-01T00:00:00Z',
    daily_usage_usd: 4,
    weekly_usage_usd: 17,
    monthly_usage_usd: 91
  }
  const limits = { daily_limit_usd: 10, weekly_limit_usd: 20, monthly_limit_usd: 100 }

  it('uses the most restrictive remaining spending window before applying the effective rate', () => {
    expect(getSubscriptionAvailableQuota(subscription, limits, 0.5)).toBe(6)
    expect(getSubscriptionAvailableQuota(subscription, limits, 2)).toBe(1.5)
    expect(getSubscriptionAvailableQuota(subscription, { ...limits, daily_limit_usd: 5 }, 0.5)).toBe(2)
    expect(getSubscriptionAvailableQuota(subscription, { ...limits, monthly_limit_usd: 92 }, 0.5)).toBe(2)
  })

  it('ignores unlimited windows and supports unused windows normalized by the active-subscriptions API', () => {
    expect(getSubscriptionAvailableQuota(subscription, { ...limits, weekly_limit_usd: null }, 1)).toBe(6)
    expect(getSubscriptionAvailableQuota(subscription, {
      daily_limit_usd: null, weekly_limit_usd: 0, monthly_limit_usd: null
    }, 1)).toBe(Number.POSITIVE_INFINITY)
    expect(getSubscriptionAvailableQuota({
      ...subscription, daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0
    }, limits, 2)).toBe(5)
  })

  it('clamps exhausted windows to zero, including zero-rate groups', () => {
    for (const rate of [0, 0.5, 2]) {
      expect(getSubscriptionAvailableQuota({ ...subscription, weekly_usage_usd: 20 }, limits, rate)).toBe(0)
      expect(getSubscriptionAvailableQuota({ ...subscription, daily_usage_usd: 11 }, limits, rate)).toBe(0)
    }
    expect(getSubscriptionAvailableQuota(subscription, limits, 0)).toBe(Number.POSITIVE_INFINITY)
  })

  it('does not report spendable quota for missing, suspended or expired subscriptions', () => {
    expect(getSubscriptionAvailableQuota(undefined, limits, 1)).toBe(0)
    expect(getSubscriptionAvailableQuota({ ...subscription, status: 'suspended' }, limits, 1)).toBe(0)
    expect(getSubscriptionAvailableQuota(subscription, limits, 1, Date.parse(subscription.expires_at))).toBe(0)
    expect(getSubscriptionAvailableQuota(subscription, limits, null)).toBeNull()
  })
})
