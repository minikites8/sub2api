import type { DailyCheckinStatus } from '@/types'

export function isDailyCheckinRechargeEligible(status: Pick<DailyCheckinStatus, 'recharge_eligible'>): boolean {
  return status.recharge_eligible
}
