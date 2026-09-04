import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
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
    },
    global: {
      stubs: {
        BaseDialog: defineComponent({
          props: { show: Boolean },
          template: '<div v-if="show"><slot /></div>'
        })
      }
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

  it('opens the detailed fallback timeline', async () => {
    const wrapper = mountBadge({
      ye_team_card_code: 'TEAM-TEST-401',
      ye_team_last_refresh_status: 'success',
      ye_team_last_refresh_at: '2026-08-28T01:54:09Z',
      ye_team_last_refresh_flow: {
        status: 'success',
        started_at: '2026-08-28T01:53:59Z',
        finished_at: '2026-08-28T01:54:09Z',
        fallback_used: true,
        order_no: 'ord-refresh',
        batch: {
          ok: true,
          total: 1,
          queued: 0,
          already_running: 0,
          done: 0,
          failed: 0,
          unreclaimable: 1,
          not_owned: 0,
          skipped: 0,
          cards: 1,
          tasks: 1
        },
        task: {
          status: 'unreclaimable',
          order_no: 'ord-dead',
          resource_uid: 'acct-dead',
          error_code: 'account_deactivated',
          failure_class: 'account_dead',
          permanent: true,
          provider_status: 403,
          message: 'account deactivated'
        },
        tasks: [
          {
            status: 'unreclaimable',
            order_no: 'ord-dead',
            resource_uid: 'acct-dead',
            error_code: 'account_deactivated',
            failure_class: 'account_dead',
            permanent: true,
            provider_status: 403,
            message: 'account deactivated'
          },
          {
            status: 'done',
            order_no: 'ord-healthy',
            resource_uid: 'acct-healthy',
            message: 'credential healthy'
          }
        ],
        stages: [
          { name: 'batch_reclaim', status: 'success', message: 'unreclaimable=1', at: '2026-08-28T01:54:00Z' },
          { name: 'refresh_bound_order', status: 'success', message: 'order created', at: '2026-08-28T01:54:03Z' },
          { name: 'complete', status: 'success', message: 'replacement credential ready for retry', at: '2026-08-28T01:54:09Z' }
        ]
      }
    })

    await wrapper.get('[data-testid="ye-team-refresh-badge"]').trigger('click')

    expect(wrapper.get('[data-testid="ye-team-refresh-details"]').text()).toContain('ord-refresh')
    expect(wrapper.get('[data-testid="ye-team-refresh-details"]').text()).toContain('account_deactivated')
    expect(wrapper.get('[data-testid="ye-team-refresh-details"]').text()).toContain('403')
    expect(wrapper.get('[data-testid="ye-team-refresh-details"]').text()).toContain('ord-healthy')
    expect(wrapper.get('[data-testid="ye-team-refresh-timeline"]').text()).toContain('refresh_bound_order')
  })

  it('shows the running state from the flow', () => {
    const wrapper = mountBadge({
      ye_team_card_code: 'TEAM-TEST-401',
      ye_team_last_refresh_status: 'running',
      ye_team_last_refresh_flow: {
        status: 'running',
        started_at: '2026-08-28T01:54:09Z',
        stages: []
      }
    })

    expect(wrapper.get('[data-testid="ye-team-refresh-badge"]').text()).toContain('running')
  })

  it('stays hidden for accounts without a ye.team binding', () => {
    expect(mountBadge({}).find('[data-testid="ye-team-refresh-state"]').exists()).toBe(false)
  })
})
