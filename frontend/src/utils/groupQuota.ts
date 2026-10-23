import type { Group, UserSubscription } from '@/types'

export function getReverseAvailableQuota(balance: number, rate: number | null): number | null {
  if (rate === null || Number.isNaN(rate)) return null
  if (rate === 0) return Number.POSITIVE_INFINITY
  return balance / rate
}

// Subscription usage is billed after rate multipliers, just like wallet usage.
// All configured spending windows must allow the next request.
export function getSubscriptionAvailableQuota(
  subscription: Pick<UserSubscription, 'status' | 'expires_at' | 'daily_usage_usd' | 'weekly_usage_usd' | 'monthly_usage_usd'> | null | undefined,
  limits: Pick<Group, 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd'>,
  rate: number | null,
  now = Date.now()
): number | null {
  if (!subscription || subscription.status !== 'active') return 0
  if (subscription.expires_at && !(Date.parse(subscription.expires_at) > now)) return 0

  let remaining = Number.POSITIVE_INFINITY
  for (const [limit, used] of [
    [limits.daily_limit_usd, subscription.daily_usage_usd],
    [limits.weekly_limit_usd, subscription.weekly_usage_usd],
    [limits.monthly_limit_usd, subscription.monthly_usage_usd]
  ]) {
    if (limit != null && limit > 0) remaining = Math.min(remaining, Math.max(0, limit - (used ?? 0)))
  }
  // An exhausted window still blocks admission when the current rate is zero.
  if (remaining === 0) return 0
  return getReverseAvailableQuota(remaining, rate)
}
