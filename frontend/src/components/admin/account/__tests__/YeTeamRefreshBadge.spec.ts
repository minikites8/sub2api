import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import YeTeamRefreshBadge from '../YeTeamRefreshBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/utils/format', () => ({
  formatDateTimeToMinute: () => '2026/08/28 01:54'
}))

function mountBadge(extra: Record<string, unknown>) {
  return mount(YeTeamRefreshBadge, {
    props: {
      account: {
        id: 1270,
        name: 'OpenAI account',
        platform: 'openai',
        type: 'oauth',
        extra
      } as any
    }
  })
}

describe('YeTeamRefreshBadge', () => {
  it('shows an unrefreshed tag for a bound account without a refresh result', () => {
    const wrapper = mountBadge({ ye_team_card_code: 'TEAM-TEST-401' })

    expect(wrapper.get('[data-testid="ye-team-refresh-badge"]').text()).toContain('notRefreshed')
  })

  it('shows the persisted success time', () => {
    const wrapper = mountBadge({
      ye_team_card_code: 'TEAM-TEST-401',
      ye_team_last_refresh_status: 'success',
      ye_team_last_refresh_at: '2026-08-28T01:54:09Z'
    })

    expect(wrapper.get('[data-testid="ye-team-refresh-badge"]').text()).toContain('refreshed')
    expect(wrapper.get('[data-testid="ye-team-refresh-time"]').text()).toBe('2026/08/28 01:54')
  })

  it('shows the persisted failure message', () => {
    const wrapper = mountBadge({
      ye_team_card_code: 'TEAM-TEST-401',
      ye_team_last_refresh_status: 'failed',
      ye_team_last_refresh_at: '2026-08-28T01:54:09Z',
      ye_team_last_refresh_error: 'context deadline exceeded'
    })

    expect(wrapper.get('[data-testid="ye-team-refresh-badge"]').text()).toContain('failed')
    expect(wrapper.get('[data-testid="ye-team-refresh-error"]').text()).toBe('context deadline exceeded')
    expect(wrapper.get('[data-testid="ye-team-refresh-state"]').attributes('title')).toContain('context deadline exceeded')
  })

  it('stays hidden for accounts without a ye.team binding', () => {
    expect(mountBadge({}).find('[data-testid="ye-team-refresh-state"]').exists()).toBe(false)
  })
})
