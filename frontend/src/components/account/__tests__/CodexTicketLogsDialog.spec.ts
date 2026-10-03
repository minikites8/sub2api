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

const { getLogs, addBlacklist } = vi.hoisted(() => ({ getLogs: vi.fn(), addBlacklist: vi.fn() }))
vi.mock('@/api/admin/codexTickets', () => ({ getCodexTicketLogs: getLogs, addCodexTicketGatewayToBlacklist: addBlacklist }))
const account = { id: 71, name: 'ticket@example.test' } as Account
const snapshot = (model = 'gpt-6-astra'): CodexTicketLogsResponse => ({
  model, limit: 200, fetched_at: '2026-09-20T14:00:00Z',
  status: { model, state: 'ready', ready: true, attempts: 3, remaining_seconds: 3540, target_length: 332, harvesting: false, harvest_enabled: true },
  entries: [
    { id: 1, time: '2026-09-20T13:59:00Z', attempt: 3, event: 'started', reason: 'request_started', target_length: 332 },
    { id: 2, time: '2026-09-20T14:00:00Z', attempt: 3, event: 'acquired', reason: 'target_length_matched', target_length: 332, ticket_length: 332, http_status: 200, duration_ms: 6719, gateway: 'unified-88', edge_ip: '203.0.113.10', egress_ip: '103.131.213.7', egress_country_code: 'PK' }
  ]
})
function mountDialog() {
  return mount(CodexTicketLogsDialog, { props: { show: true, account, model: 'gpt-6-astra' }, global: { plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { admin: zhAccounts } } })], stubs: { Teleport: true, Transition: false } } })
}
beforeEach(() => { vi.useFakeTimers(); getLogs.mockReset(); addBlacklist.mockReset(); getLogs.mockResolvedValue(snapshot()); vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible') })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })
describe('CodexTicketLogsDialog', () => {
  it('adds the current gateway, updates the account and prevents repeat clicks', async () => {
    const initial = { ...snapshot(), gateway_blacklist: ['unified-12'] }
    const updated = { ...account, extra: { openai_codex_ticket_gateway_blacklist: ['unified-12', 'unified-88'] } }
    getLogs.mockResolvedValueOnce(initial).mockResolvedValue({ ...initial, gateway_blacklist: ['unified-12', 'unified-88'] })
    let resolve!: (account: Account) => void
    addBlacklist.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = mountDialog(); await flushPromises()
    const button = wrapper.get('tbody tr button')
    await button.trigger('click'); await button.trigger('click')
    expect(addBlacklist).toHaveBeenCalledTimes(1)
    expect(addBlacklist).toHaveBeenCalledWith(71, 'unified-88')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.text()).toBe('保存中…')
    resolve(updated); await flushPromises()
    expect(button.text()).toBe('已加入黑名单')
    expect(wrapper.emitted('account-updated')).toEqual([[updated]])
    expect(getLogs).toHaveBeenCalledTimes(2)
  })
  it('disables gateways already blacklisted and hides actions for missing gateways', async () => {
    getLogs.mockResolvedValue({ ...snapshot(), gateway_blacklist: ['unified-88'] })
    const wrapper = mountDialog(); await flushPromises()
    expect(wrapper.findAll('tbody tr button')).toHaveLength(1)
    expect(wrapper.get('tbody tr button').text()).toBe('已加入黑名单')
    expect(wrapper.get('tbody tr button').attributes('disabled')).toBeDefined()
    expect(addBlacklist).not.toHaveBeenCalled()
  })
  it('retains the action and reports a failed save for retry', async () => {
    addBlacklist.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountDialog(); await flushPromises()
    await wrapper.get('tbody tr button').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('加入黑名单失败')
    expect(wrapper.get('tbody tr button').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('account-updated')).toBeUndefined()
  })
  it('ignores a completed save after switching accounts', async () => {
    let resolve!: (account: Account) => void
    addBlacklist.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = mountDialog(); await flushPromises()
    await wrapper.get('tbody tr button').trigger('click')
    await wrapper.setProps({ account: { ...account, id: 72 } }); await flushPromises()
    resolve({ ...account, extra: { openai_codex_ticket_gateway_blacklist: ['unified-88'] } }); await flushPromises()
    expect(wrapper.emitted('account-updated')).toBeUndefined()
    expect(wrapper.get('tbody tr button').text()).toBe('加入黑名单')
  })
  it('renders legacy status summaries with a translated state and default attempts', async () => {
    const response = snapshot()
    response.status = { model: response.model, ready: false, remaining_seconds: 0 }
    getLogs.mockResolvedValue(response)
    const wrapper = mountDialog(); await flushPromises()
    expect(wrapper.text()).toContain('等待打票')
    expect(wrapper.text()).toContain('打票 0 次')
    expect(wrapper.text()).not.toContain('admin.accounts.codexTickets.states.')
  })
  it('renders latest-first events with real diagnostics and polls every two seconds', async () => {
    const wrapper = mountDialog(); await flushPromises()
    expect(getLogs).toHaveBeenCalledWith(71, 'gpt-6-astra', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('打票 3 次'); expect(wrapper.text()).toContain('每 2 秒自动刷新')
    expect(wrapper.findAll('thead th').map(header => header.text())).toEqual(['时间（最新在前）', '轮内次数', '结果', '原因', 'HTTP', 'Gateway', 'Edge IP', '长度（实际 / 目标）', '耗时'])
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('unified-88')
    expect(wrapper.findAll('tbody tr')[0].findAll('td')[6].text()).toBe('203.0.113.10')
    expect(wrapper.text()).not.toContain('103.131.213.7')
    expect(wrapper.text()).not.toContain('出口')
    expect(wrapper.findAll('tbody tr')[1].findAll('td')[5].text()).toBe('—')
    expect(wrapper.findAll('tbody tr')[1].findAll('td')[6].text()).toBe('—')
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('332 / 332')
    expect(wrapper.text()).toContain('6719 ms')
    const refreshed = snapshot()
    refreshed.entries[1].edge_ip = '2001:db8::20'
    getLogs.mockResolvedValueOnce(refreshed)
    await vi.advanceTimersByTimeAsync(2000); await flushPromises(); expect(getLogs).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('tbody tr')[0].findAll('td')[6].text()).toBe('2001:db8::20')
    wrapper.unmount(); await vi.advanceTimersByTimeAsync(6000); expect(getLogs).toHaveBeenCalledTimes(2)
  })
  it('preserves the last successful snapshot when a refresh fails', async () => {
    const wrapper = mountDialog(); await flushPromises(); getLogs.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('刷新失败')
    expect(wrapper.text()).toContain('unified-88'); wrapper.unmount()
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
