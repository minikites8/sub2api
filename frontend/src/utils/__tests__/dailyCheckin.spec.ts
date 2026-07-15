import { describe, expect, it } from 'vitest'
import { isDailyCheckinRechargeEligible } from '@/utils/dailyCheckin'

const status = (overrides: Partial<Parameters<typeof isDailyCheckinRechargeEligible>[0]> = {}) => ({
  recharge_eligible: false,
  ...overrides
})

describe('daily check-in recharge eligibility', () => {
  it('uses the API eligibility flag', () => {
    expect(isDailyCheckinRechargeEligible(status())).toBe(false)
    expect(isDailyCheckinRechargeEligible(status({ recharge_eligible: true }))).toBe(true)
  })
})
