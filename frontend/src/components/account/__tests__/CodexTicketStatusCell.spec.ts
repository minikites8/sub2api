import { flushPromises, mount, enableAutoUnmount } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import type { Account } from '@/types'
import type { CodexTicketStatus } from '@/types/codexTicket'
import CodexTicketStatusCell from '../CodexTicketStatusCell.vue'

enableAutoUnmount(afterEach)
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'zh-CN' }, t: (key: string, params: Record<string, unknown> = {}) => {
    const message = key.split('.').reduce<unknown>((value, segment) => value && typeof value === 'object' ? (value as Record<string, unknown>)[segment] : undefined, { admin: zhAccounts })
    return String(message ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? name))
  } }) }
})

const { subscribe, dispose } = vi.hoisted(() => ({ subscribe: vi.fn(), dispose: vi.fn() }))
vi.mock('@/composables/useCodexTicketStatuses', () => ({ subscribeCodexTicketStatuses: subscribe }))
vi.mock('../CodexTicketLogsDialog.vue', () => ({ default: { props: ['account', 'model'], template: '<div data-testid="dialog">{{ account.id }} {{ model }}</div>' } }))
const ticket = (model: string): CodexTicketStatus => ({ model, state: 'ready', ready: true, remaining_seconds: 3540, attempts: 3, target_length: 332, harvesting: false, harvest_enabled: true })
const account = (): Account => ({ id: 71, name: 'ticket', platform: 'openai', type: 'oauth', codex_tickets: [ticket('gpt-6-astra'), ticket('gpt-5.6-sol')] } as Account)
function render(value = account()) { return mount(CodexTicketStatusCell, { props: { account: value }, global: { plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { admin: zhAccounts } } })] } }) }
beforeEach(() => { vi.useFakeTimers(); subscribe.mockReset().mockReturnValue(dispose); dispose.mockClear(); vi.stubGlobal('IntersectionObserver', undefined); vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible') })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
describe('CodexTicketStatusCell', () => {
  it('shows per-model remaining time and attempts, opens scoped logs and cleans up', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.findAll('button')).toHaveLength(2); expect(wrapper.text()).toContain('astra'); expect(wrapper.text()).toContain('59m00s'); expect(wrapper.text()).toContain('打票 3 次')
    await vi.advanceTimersByTimeAsync(2000); expect(wrapper.text()).toContain('58m58s')
    await wrapper.findAll('button')[1].trigger('click'); expect(wrapper.get('[data-testid="dialog"]').text()).toContain('71 gpt-5.6-sol')
    wrapper.unmount(); expect(dispose).toHaveBeenCalled()
  })
  it('uses fresh batch snapshots and displays expiry instead of a negative duration', async () => {
    const value = account(); value.codex_tickets = [{ ...ticket('gpt-6-astra'), remaining_seconds: 1 }]
    const wrapper = render(value); await flushPromises(); await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('门票已过期')
    const accept = subscribe.mock.calls[0][1] as (items: CodexTicketStatus[]) => void
    accept([{ ...ticket('gpt-6-astra'), attempts: 5 }]); await flushPromises()
    expect(wrapper.text()).toContain('打票 5 次'); wrapper.unmount()
  })
  it('keeps API-key and shadow accounts out of ticket polling', async () => {
    for (const overrides of [{ type: 'apikey' }, { parent_account_id: 70 }]) {
      const wrapper = render({ ...account(), ...overrides } as Account); await flushPromises(); expect(wrapper.find('button').exists()).toBe(false); wrapper.unmount()
    }
    expect(subscribe).not.toHaveBeenCalled()
  })
})
