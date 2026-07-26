import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function createStats(overrides: Partial<UserStatsType> = {}): UserStatsType {
  return {
    total_requests: 100,
    today_requests: 10,
    average_duration_ms: 500,
    rpm: 2,
    total_tokens: 1000,
    total_actual_cost: 0,
    active_api_keys: 1,
    total_api_keys: 2,
    ...overrides
  } as UserStatsType
}

function mountStats(props: { stats?: Partial<UserStatsType>; balance?: number; isSimple?: boolean } = {}) {
  return mount(UserDashboardStats, {
    props: {
      stats: createStats(props.stats),
      balance: props.balance ?? 0,
      isSimple: props.isSimple ?? false
    },
    global: { stubs: { Icon: true } }
  })
}

describe('UserDashboardStats', () => {
  // 1 Credit = 0.01 USD, so USD amounts are multiplied by 100 for display.
  it('shows the balance in Credits rather than dollars', () => {
    const wrapper = mountStats({ balance: 12.34 })

    const balanceCard = wrapper.findAll('.telemetry-stat-card').at(-1)!
    expect(balanceCard.text()).toContain('1,234')
    expect(balanceCard.text()).toContain('Credits')
    expect(wrapper.text()).not.toContain('$')
  })

  it('shows the accumulated actual cost in Credits', () => {
    const wrapper = mountStats({ stats: { total_actual_cost: 0.5 } })

    expect(wrapper.text()).toContain('50 Credits')
    expect(wrapper.text()).not.toContain('$')
  })

  it('shows the active key count instead of a balance in simple mode', () => {
    const wrapper = mountStats({ balance: 12.34, isSimple: true })

    const lastCard = wrapper.findAll('.telemetry-stat-card').at(-1)!
    expect(lastCard.text()).not.toContain('Credits')
    expect(lastCard.text()).not.toContain('1,234')
  })
})
