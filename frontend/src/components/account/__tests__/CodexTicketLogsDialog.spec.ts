import { flushPromises, mount, enableAutoUnmount } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import CodexTicketLogsDialog from '../CodexTicketLogsDialog.vue'
import type { Account } from '@/types'
import type { CodexTicketLogsResponse } from '@/types/codexTicket'

enableAutoUnmount(afterEach)
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'zh-CN' }, t: (key: string, params: Record<string, unknown> = {}) => {
    const message = key.split('.').reduce<unknown>((value, segment) => value && typeof value === 'object' ? (value as Record<string, unknown>)[segment] : undefined, { admin: zhAccounts })
    return String(message ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? name))
  } }) }
})

const { getLogs } = vi.hoisted(() => ({ getLogs: vi.fn() }))
vi.mock('@/api/admin/codexTickets', () => ({ getCodexTicketLogs: getLogs }))
const account = { id: 71, name: 'ticket@example.test' } as Account
const snapshot = (model = 'gpt-6-astra'): CodexTicketLogsResponse => ({
  model, limit: 200, fetched_at: '2026-09-20T14:00:00Z',
  status: { model, state: 'ready', ready: true, attempts: 3, remaining_seconds: 3540, target_length: 332, harvesting: false, harvest_enabled: true },
  entries: [
    { id: 1, time: '2026-09-20T13:59:00Z', attempt: 3, event: 'started', reason: 'request_started', target_length: 332 },
    { id: 2, time: '2026-09-20T14:00:00Z', attempt: 3, event: 'acquired', reason: 'target_length_matched', target_length: 332, ticket_length: 332, http_status: 200, duration_ms: 6719, egress_ip: '103.131.213.7', egress_country_code: 'PK' }
  ]
})
function mountDialog() {
  return mount(CodexTicketLogsDialog, { props: { show: true, account, model: 'gpt-6-astra' }, global: { plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { admin: zhAccounts } } })], stubs: { Teleport: true, Transition: false } } })
}
beforeEach(() => { vi.useFakeTimers(); getLogs.mockReset(); getLogs.mockResolvedValue(snapshot()); vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible') })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })
describe('CodexTicketLogsDialog', () => {
  it('renders latest-first events with real diagnostics and polls every two seconds', async () => {
    const wrapper = mountDialog(); await flushPromises()
    expect(getLogs).toHaveBeenCalledWith(71, 'gpt-6-astra', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('打票 3 次'); expect(wrapper.text()).toContain('每 2 秒自动刷新')
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('103.131.213.7')
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('332 / 332')
    expect(wrapper.text()).toContain('6719 ms')
    await vi.advanceTimersByTimeAsync(2000); await flushPromises(); expect(getLogs).toHaveBeenCalledTimes(2)
    wrapper.unmount(); await vi.advanceTimersByTimeAsync(6000); expect(getLogs).toHaveBeenCalledTimes(2)
  })
  it('preserves the last successful snapshot when a refresh fails', async () => {
    const wrapper = mountDialog(); await flushPromises(); getLogs.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('刷新失败')
    expect(wrapper.text()).toContain('103.131.213.7'); wrapper.unmount()
  })
  it('aborts an in-flight request on close and ignores its late response', async () => {
    let resolve!: (value: CodexTicketLogsResponse) => void
    getLogs.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = mountDialog(); await flushPromises()
    const signal = getLogs.mock.calls[0][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(6000); expect(getLogs).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ show: false }); expect(signal.aborted).toBe(true)
    resolve(snapshot()); await flushPromises(); expect(wrapper.emitted('status')).toBeUndefined(); wrapper.unmount()
  })
  it('switches account/model without showing the previous request result', async () => {
    let resolve!: (value: CodexTicketLogsResponse) => void
    getLogs.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = mountDialog(); await flushPromises()
    getLogs.mockResolvedValueOnce(snapshot('gpt-5.6-sol'))
    await wrapper.setProps({ account: { ...account, id: 72 }, model: 'gpt-5.6-sol' }); await flushPromises()
    resolve(snapshot()); await flushPromises()
    expect(wrapper.emitted('status')).toHaveLength(1)
    expect(wrapper.emitted('status')?.[0]?.[0]).toMatchObject({ model: 'gpt-5.6-sol' }); wrapper.unmount()
  })
  it('pauses polling while the page is hidden and resumes immediately', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const wrapper = mountDialog(); await flushPromises()
    visibility.mockReturnValue('hidden'); document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(6000); expect(getLogs).toHaveBeenCalledTimes(1)
    visibility.mockReturnValue('visible'); document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(getLogs).toHaveBeenCalledTimes(2); wrapper.unmount()
  })
})
