import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountCapacityCell from '../AccountCapacityCell.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

function accountFixture(overrides: Partial<Account> = {}): Account {
  return {
    id: 7,
    name: 'multi-proxy-account',
    platform: 'openai',
    type: 'oauth',
    proxy_id: 13,
    concurrency: 12,
    current_concurrency: 3,
    priority: 1,
    is_fallback: false,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '',
    updated_at: '',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: '',
    session_window_start: null,
    session_window_end: null,
    session_window_status: '',
    proxy_pool: [
      { proxy_id: 13, concurrency: 6, current_concurrency: 2, proxy: { id: 13, name: 'IP13' } },
      { proxy_id: 12, concurrency: 6, current_concurrency: 1, proxy: { id: 12, name: 'IP12' } }
    ],
    ...overrides
  } as Account
}

describe('AccountCapacityCell', () => {
  it('renders aggregate and per-proxy concurrency badges', () => {
    const wrapper = mount(AccountCapacityCell, {
      props: { account: accountFixture() }
    })

    const badges = wrapper.findAll('[data-testid="proxy-capacity-badge"]')
    expect(badges).toHaveLength(2)
    expect(badges[0].text().replace(/\s+/g, '')).toContain('IP132/6')
    expect(badges[1].text().replace(/\s+/g, '')).toContain('IP121/6')
    expect(wrapper.findAll('[data-testid="proxy-capacity-icon"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="proxy-capacity-icon"] path').attributes('d')).toContain('M15 10.5a3 3')
    expect(badges[0].attributes('title')).toBe('IP13: 2/6')
    expect(badges[1].attributes('title')).toBe('IP12: 1/6')

    const aggregate = wrapper.findAllComponents({ name: 'CapacityBadge' })[0]
    expect(aggregate.props('current')).toBe(3)
    expect(aggregate.props('max')).toBe(12)
  })

  it('keeps the single-account badge when no proxy pool is configured', () => {
    const wrapper = mount(AccountCapacityCell, {
      props: { account: accountFixture({ proxy_pool: [] }) }
    })

    expect(wrapper.findAll('[data-testid="proxy-capacity-badge"]')).toHaveLength(0)
    expect(wrapper.text().replace(/\s+/g, '')).toContain('3/12')
  })
})
