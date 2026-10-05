import type { SubscriptionPlan } from '@/types/payment'

export const MAX_SUBSCRIPTION_QUANTITY = 1000
const MAX_SUBSCRIPTION_DAYS = 36500

export function subscriptionPeriodDays(plan: Pick<SubscriptionPlan, 'validity_days' | 'validity_unit'>): number {
  switch (plan.validity_unit) {
    case 'week':
    case 'weeks':
      return plan.validity_days * 7
    case 'month':
    case 'months':
      return plan.validity_days * 30
    default:
      return plan.validity_days
  }
}

export function maxSubscriptionQuantity(plan: Pick<SubscriptionPlan, 'validity_days' | 'validity_unit'>): number {
  const days = subscriptionPeriodDays(plan)
  return days > 0 ? Math.max(1, Math.min(MAX_SUBSCRIPTION_QUANTITY, Math.floor(MAX_SUBSCRIPTION_DAYS / days))) : 1
}
