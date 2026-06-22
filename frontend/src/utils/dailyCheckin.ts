import type { DailyCheckinStatus } from '@/types'

const DAILY_CHECKIN_AMOUNT_SCALE = 1e8

export function roundDailyCheckinAmount(value: number): number {
  const amount = Number(value)
  if (!Number.isFinite(amount) || amount <= 0) return 0
  return Math.round(amount * DAILY_CHECKIN_AMOUNT_SCALE) / DAILY_CHECKIN_AMOUNT_SCALE
}

export function isDailyCheckinRechargeEligible(
  status: Pick<DailyCheckinStatus, 'recharge_eligible' | 'min_recharge_amount' | 'total_recharged'>
): boolean {
  const minRechargeAmount = roundDailyCheckinAmount(status.min_recharge_amount)
  if (minRechargeAmount <= 0) return true
  if (status.recharge_eligible) return true
  return roundDailyCheckinAmount(status.total_recharged) >= minRechargeAmount
}
