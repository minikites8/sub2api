import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'common.giftExpiresAt') return `Next expiry: ${params?.date || ''}`
      return key
    }
  })
}))

describe('UserDashboardStats', () => {
  it('shows recharge and gift balances with source details and expiry', () => {
    const wrapper = mount(UserDashboardStats, {
      props: {
        stats: {
          total_api_keys: 0,
          active_api_keys: 0,
          today_requests: 0,
          total_requests: 0,
          total_input_tokens: 0,
          total_output_tokens: 0,
          total_cache_creation_tokens: 0,
          total_cache_read_tokens: 0,
          total_tokens: 0,
          total_cost: 0,
          today_cost: 0,
          today_actual_cost: 0,
          today_tokens: 0,
          today_input_tokens: 0,
          today_output_tokens: 0,
          today_cache_creation_tokens: 0,
          today_cache_read_tokens: 0,
          total_actual_cost: 0,
          average_duration_ms: 0,
          rpm: 0,
          tpm: 0
        },
        balance: 15,
        rechargeBalance: 10,
        giftBalance: 5,
        registrationGiftBalance: 3,
        dailyCheckinBalance: 2,
        giftBalanceExpiresAt: '2026-09-30T23:59:59Z',
        isSimple: false
      }
    })

    const text = wrapper.text()
    expect(text).toContain('10.00')
    expect(text).toContain('5.00')
    expect(text).toContain('3.00')
    expect(text).toContain('2.00')
    expect(text).toContain('Next expiry:')
  })
})
