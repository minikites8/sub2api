import { describe, expect, it } from 'vitest'
import { isDailyCheckinRechargeEligible, roundDailyCheckinAmount } from '@/utils/dailyCheckin'

const status = (overrides: Partial<Parameters<typeof isDailyCheckinRechargeEligible>[0]> = {}) => ({
  recharge_eligible: false,
  min_recharge_amount: 0,
  total_recharged: 0,
  ...overrides
})

describe('daily check-in recharge eligibility', () => {
  it('allows check-in when minimum recharge is zero even if the API flag is false', () => {
    expect(isDailyCheckinRechargeEligible(status())).toBe(true)
  })

  it('allows check-in when total recharge reaches the rounded minimum', () => {
    expect(isDailyCheckinRechargeEligible(status({
      min_recharge_amount: 0.000000004,
      total_recharged: 0
    }))).toBe(true)
    expect(isDailyCheckinRechargeEligible(status({
      min_recharge_amount: 1,
      total_recharged: 1
    }))).toBe(true)
  })

  it('requires recharge only when the positive minimum is not reached', () => {
    expect(isDailyCheckinRechargeEligible(status({
      min_recharge_amount: 1,
      total_recharged: 0.99
    }))).toBe(false)
  })

  it('keeps the backend eligibility flag as a successful signal', () => {
    expect(isDailyCheckinRechargeEligible(status({
      recharge_eligible: true,
      min_recharge_amount: 5,
      total_recharged: 0
    }))).toBe(true)
  })
})

describe('daily check-in amount rounding', () => {
  it('matches the backend precision for non-negative finite amounts', () => {
    expect(roundDailyCheckinAmount(1.234567895)).toBe(1.2345679)
    expect(roundDailyCheckinAmount(Number.NaN)).toBe(0)
    expect(roundDailyCheckinAmount(-1)).toBe(0)
  })
})
