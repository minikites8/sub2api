import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '../AccountActionMenu.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const position = { top: 100, left: 100 }
const account = (extra?: Record<string, unknown>) => ({
  id: 1,
  name: 'team-account',
  platform: 'openai',
  type: 'oauth',
  parent_account_id: null,
  proxy_id: null,
  status: 'active',
  schedulable: true,
  extra,
} as any)

describe('AccountActionMenu ye.team reset', () => {
  it('shows and emits reset for a bound account', async () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: account({ ye_team_card_code: 'TEAM-TEST-401' }), position },
      attachTo: document.body,
    })
    const button = Array.from(document.body.querySelectorAll('button')).find(node =>
      node.textContent?.includes('admin.accounts.yeTeamReset'))
    expect(button).toBeDefined()
    button!.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('ye-team-reset')?.[0][0]).toMatchObject({ id: 1 })
    wrapper.unmount()
  })

  it('hides reset for an unbound account', () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: account(), position },
      attachTo: document.body,
    })
    expect(document.body.textContent).not.toContain('admin.accounts.yeTeamReset')
    wrapper.unmount()
  })
})
