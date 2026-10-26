import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import View from '../GPTQoSSchedulingView.vue'
import GroupQoSEditor from '@/components/group/GroupQoSEditor.vue'
import { defaultQoSGroup, type QoSConfig, type QoSTierMetrics } from '@/api/admin/gptQos'
const mocks = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), snapshot: vi.fn() }))
vi.mock('@/api/admin/gptQos', async importOriginal => ({ ...(await importOriginal<typeof import('@/api/admin/gptQos')>()), getQoSConfig: mocks.get, saveQoSConfig: mocks.save, getQoSSnapshot: mocks.snapshot }))
vi.mock('vue-i18n', async importOriginal => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
function config(): QoSConfig { return { version: 1, quality_routing_enabled: false, enabled: false, capacity_enabled: false, premium_weight: 6, standard_weight: 3, economy_weight: 1, max_waiting: 1000, request_budget_ms: 300000, high_effort_budget_ms: 900000, max_attempts: 4, log_sample_rate: 0.1, peak: { enabled: false, strict_reserve: true, enter_utilization: 0.8, exit_utilization: 0.65, protection_ratio: 0.3, enter_hold_ms: 5000, exit_hold_ms: 15000 }, groups: {}, resource_pools: {} } }
function metrics(overrides: Partial<QoSTierMetrics> = {}): QoSTierMetrics {
  return { Requests: 0, Attempts: 0, Successes: 0, FirstSuccesses: 0, Canceled: 0, Downgrades: 0, StrongRequests: 0, APIRequests: 0, OAuthRequests: 0, strong_hit_rate: 0, first_success_rate: 0, average_queue_ms: 0, p50_queue_ms: 0, p95_queue_ms: 0, p99_queue_ms: 0, ...overrides }
}
beforeEach(() => { vi.clearAllMocks(); mocks.get.mockResolvedValue(config()); mocks.save.mockImplementation(async c => c); mocks.snapshot.mockResolvedValue({ available: true, tiers: {}, capacity: { active: 0, queued: 0 } }) })
describe('GPT QoS controls', () => {
  it('uses actual API/OAuth request counts instead of manual upstream source counts', async () => {
    mocks.snapshot.mockResolvedValue({ available: true, tiers: { premium: { ...metrics({ Requests: 10, APIRequests: 4, OAuthRequests: 6 }), VM2APIRequests: 999, DirectRequests: 777 } }, account_type_requests: { apikey: 4, oauth: 6, unknown: 2 } })
    const wrapper = mount(View)
    await flushPromises()
    const table = wrapper.get('[data-testid="qos-metrics"]')
    const headers = table.findAll('thead th').map(cell => cell.text())
    expect(headers).toContain('gptQos.apiRequests')
    expect(headers).toContain('gptQos.oauthRequests')
    expect(table.text()).not.toMatch(/vmShare|directShare|999|777/)
    const cells = table.get('[data-qos-tier="premium"]').findAll('td')
    expect(cells[headers.indexOf('gptQos.apiRequests') - 1]!.text()).toBe('4')
    expect(cells[headers.indexOf('gptQos.oauthRequests') - 1]!.text()).toBe('6')
    expect(wrapper.text()).toContain('gptQos.apiRequests: 4 (33.3%)')
    expect(wrapper.text()).toContain('gptQos.oauthRequests: 6 (50%)')
    expect(wrapper.text()).toContain('gptQos.unclassifiedRequests: 2 (16.7%)')
    wrapper.unmount()
  })

  it('expands group metric subrows independently for each QoS and collapses them again', async () => {
    mocks.snapshot.mockResolvedValue({ available: true, tiers: { premium: metrics({ Requests: 10 }), standard: metrics({ Requests: 3 }), economy: metrics() }, groups: {
      premium: [{ group_id: 7, group_name: '月卡分组', ...metrics({ Requests: 8, Attempts: 9, APIRequests: 6, OAuthRequests: 2, p95_queue_ms: 150 }) }, { group_id: 8, group_name: '高级分组', ...metrics({ Requests: 2 }) }],
      standard: [{ group_id: 7, group_name: '月卡分组', ...metrics({ Requests: 3 }) }]
    } })
    const wrapper = mount(View)
    await flushPromises()
    const premium = wrapper.get('[data-qos-tier="premium"] button')
    const standard = wrapper.get('[data-qos-tier="standard"] button')
    expect(premium.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-group-id]').exists()).toBe(false)
    await premium.trigger('click')
    expect(premium.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('#qos-groups-premium [data-group-id]')).toHaveLength(2)
    const group = wrapper.get('#qos-groups-premium [data-group-id="7"]')
    expect(group.get('th').text()).toContain('月卡分组')
    expect(group.get('th').text()).toContain('#7')
    expect(group.findAll('td')[0]!.text()).toBe('8')
    expect(group.findAll('td')[1]!.text()).toBe('9')
    expect(group.findAll('td')[13]!.text()).toBe('150')
    expect(wrapper.find('#qos-groups-standard').exists()).toBe(false)
    await standard.trigger('click')
    expect(wrapper.get('#qos-groups-standard [data-group-id="7"]').findAll('td')[0]!.text()).toBe('3')
    await premium.trigger('click')
    expect(wrapper.find('#qos-groups-premium').exists()).toBe(false)
    expect(wrapper.find('#qos-groups-standard').exists()).toBe(true)
    await wrapper.get('[data-qos-tier="economy"] button').trigger('click')
    expect(wrapper.get('#qos-groups-economy').text()).toBe('gptQos.noGroupMetrics')
    wrapper.unmount()
  })

  it('keeps group IDs as fallback names and escapes group names', async () => {
    mocks.snapshot.mockResolvedValue({ available: true, tiers: { standard: metrics() }, groups: { standard: [{ group_id: 41, ...metrics() }, { group_id: 42, group_name: '<img src=x onerror=alert(1)>', ...metrics() }] } })
    const wrapper = mount(View)
    await flushPromises()
    await wrapper.get('[data-qos-tier="standard"] button').trigger('click')
    expect(wrapper.get('[data-group-id="41"]').text()).toContain('gptQos.unnamedGroup #41')
    expect(wrapper.get('[data-group-id="42"]').text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.get('[data-group-id="42"]').find('img').exists()).toBe(false)
    wrapper.unmount()
  })

  it('refreshes expanded group details with the same group/model/effort filters as totals', async () => {
    mocks.snapshot.mockResolvedValueOnce({ available: true, tiers: { premium: metrics({ Requests: 1 }) }, groups: { premium: [{ group_id: 1, group_name: 'Old group', ...metrics({ Requests: 1 }) }] } })
    const wrapper = mount(View)
    await flushPromises()
    await wrapper.get('[data-qos-tier="premium"] button').trigger('click')
    const inputs = wrapper.findAll('section label input')
    await inputs[0]!.setValue('7')
    await inputs[1]!.setValue('gpt-test')
    await inputs[2]!.setValue('high')
    mocks.snapshot.mockResolvedValue({ available: true, tiers: { premium: metrics({ Requests: 4 }) }, groups: { premium: [{ group_id: 7, group_name: 'Filtered group', ...metrics({ Requests: 4 }) }] } })
    await wrapper.findAll('button').find(b => b.text() === 'gptQos.refresh')!.trigger('click')
    await flushPromises()
    expect(mocks.snapshot).toHaveBeenLastCalledWith({ group_id: '7', model: 'gpt-test', effort: 'high' })
    expect(wrapper.find('[data-group-id="1"]').exists()).toBe(false)
    expect(wrapper.get('[data-group-id="7"]').findAll('td')[0]!.text()).toBe('4')
    expect(wrapper.get('[data-qos-tier="premium"] button').attributes('aria-expanded')).toBe('true')
    wrapper.unmount()
  })

  it('handles an older snapshot without protocol or group fields without relabeling legacy counts', async () => {
    const { APIRequests: _api, OAuthRequests: _oauth, ...legacy } = metrics()
    mocks.snapshot.mockResolvedValue({ available: true, tiers: { premium: { ...legacy, VM2APIRequests: 9, DirectRequests: 8 } } })
    const wrapper = mount(View)
    await flushPromises()
    const cells = wrapper.get('[data-qos-tier="premium"]').findAll('td')
    expect(cells[9]!.text()).toBe('—')
    expect(cells[10]!.text()).toBe('—')
    await wrapper.get('[data-qos-tier="premium"] button').trigger('click')
    expect(wrapper.get('#qos-groups-premium').text()).toBe('gptQos.noGroupMetrics')
    wrapper.unmount()
  })

  it('shows quality and account names while preserving shared pools as one row', async () => {
    mocks.snapshot.mockResolvedValue({ available: true, tiers: {}, capacity: { active: 2, queued: 0, pools: [
      { id: 'account:1', display_name: 'Strong:正常 OAuth', active: 0, capacity: 3 },
      { id: 'shared:fixture', display_name: 'Strong:账号 A / Basic:账号 B', active: 0, capacity: 10 }
    ] } })
    const wrapper = mount(View)
    await flushPromises()
    const single = wrapper.get('tr[data-pool-id="account:1"]')
    const shared = wrapper.get('tr[data-pool-id="shared:fixture"]')
    expect(single.get('td').text()).toBe('Strong:正常 OAuth')
    expect(single.get('td').text()).not.toContain('account:1')
    expect(shared.get('td').text()).toBe('Strong:账号 A / Basic:账号 B')
    expect(wrapper.findAll('tr[data-pool-id]')).toHaveLength(2)
    expect(shared.findAll('td')[1]!.text()).toBe('0 / 10')
    wrapper.unmount()
  })

  it('shows enabled but untriggered peak protection as waiting, not disabled', async () => {
    const applied = config()
    applied.enabled = applied.quality_routing_enabled = applied.capacity_enabled = applied.peak.enabled = true
    mocks.get.mockResolvedValue(structuredClone(applied))
    mocks.snapshot.mockResolvedValue({ available: true, config: applied, tiers: {}, capacity: { active: 0, queued: 0, strong_pool: { peak: false, capacity: 26 }, pools: [{ id: 'account:1', strong_capacity: 10, peak: false, reason: 'below_threshold_or_hysteresis_hold' }] } })
    const wrapper = mount(View)
    await flushPromises()
    const row = wrapper.findAll('tr').find(r => r.text().includes('account:1'))!
    expect(row.text()).toContain('gptQos.peakWaiting')
    expect(row.text()).toContain('gptQos.peakBelowThreshold')
    expect(row.text()).not.toContain('common.disabled')
    expect(wrapper.text()).toContain('gptQos.peakStatusHint')
    wrapper.unmount()
  })

  it('does not treat unsaved checkbox changes as applied peak configuration', async () => {
    mocks.snapshot.mockResolvedValue({ available: true, config: config(), tiers: {}, capacity: { active: 0, queued: 0, pools: [{ id: 'account:1', strong_capacity: 10, peak: false }] } })
    const wrapper = mount(View)
    await flushPromises()
    await wrapper.findAll('input[type="checkbox"]')[3]!.setValue(true)
    const row = wrapper.findAll('tr').find(r => r.text().includes('account:1'))!
    expect(row.text()).toContain('gptQos.peakOff')
    expect(row.text()).not.toContain('gptQos.peakWaiting')
    wrapper.unmount()
  })

  it('shows aggregate protection for eligible strong pools even below the individual threshold', async () => {
    const applied = config()
    applied.enabled = applied.quality_routing_enabled = applied.capacity_enabled = applied.peak.enabled = true
    mocks.get.mockResolvedValue(structuredClone(applied))
    mocks.snapshot.mockResolvedValue({ available: true, config: applied, tiers: {}, capacity: { active: 9, queued: 0, strong_pool: { peak: true, capacity: 10 }, pools: [{ id: 'strong', strong_capacity: 3, peak: false }, { id: 'basic', strong_capacity: 0, peak: false }] } })
    const wrapper = mount(View)
    await flushPromises()
    expect(wrapper.findAll('tbody tr').find(r => r.get('td').text() === 'strong')!.text()).toContain('gptQos.peakActive')
    expect(wrapper.findAll('tbody tr').find(r => r.get('td').text() === 'strong')!.text()).toContain('gptQos.peakAggregateTriggered')
    expect(wrapper.findAll('tbody tr').find(r => r.get('td').text() === 'basic')!.text()).toContain('gptQos.peakNotApplicable')
    wrapper.unmount()
  })

  it('refreshes runtime observations after saving peak configuration', async () => {
    const wrapper = mount(View)
    await flushPromises()
    const checks = wrapper.findAll('input[type="checkbox"]')
    for (const check of checks.slice(0, 4)) await check.setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.snapshot).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it.each([
    { name: 'stale instance policy', saved: true, version: 2, observed: false, observedVersion: 1, available: true, expected: 'peakConfigMismatch' },
    { name: 'newer disabled policy', saved: true, version: 2, observed: false, observedVersion: 3, available: true, expected: 'peakOff' },
    { name: 'disabled policy with old peak sample', saved: false, version: 2, observed: false, observedVersion: 2, available: true, expected: 'peakOff' },
    { name: 'unavailable Redis', saved: true, version: 2, observed: true, observedVersion: 2, available: false, expected: 'peakUnknown' }
  ])('reports $name without claiming active protection', async tc => {
    const stored = config()
    stored.version = tc.version
    stored.peak.enabled = tc.saved
    const observed = config()
    observed.version = tc.observedVersion
    observed.peak.enabled = tc.observed
    mocks.get.mockResolvedValue(stored)
    mocks.snapshot.mockResolvedValue({ available: tc.available, config: observed, tiers: {}, capacity: { active: 0, queued: 0, pools: [{ id: 'account:1', strong_capacity: 10, peak: true }] } })
    const wrapper = mount(View)
    await flushPromises()
    const row = wrapper.findAll('tr').find(r => r.text().includes('account:1'))!
    expect(row.text()).toContain(`gptQos.${tc.expected}`)
    expect(row.text()).not.toContain('gptQos.peakActive')
    wrapper.unmount()
  })

  it('ignores a slow pre-save observation after the post-save refresh finishes', async () => {
    let resolveOld!: (value: unknown) => void
    mocks.snapshot.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mount(View)
    await flushPromises()
    const applied = config()
    applied.enabled = applied.quality_routing_enabled = applied.capacity_enabled = applied.peak.enabled = true
    const capacity = { active: 9, queued: 0, pools: [{ id: 'account:1', strong_capacity: 10, peak: true }] }
    mocks.snapshot.mockResolvedValue({ available: true, config: applied, capacity, tiers: {} })
    for (const check of wrapper.findAll('input[type="checkbox"]').slice(0, 4)) await check.setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    resolveOld({ available: true, config: config(), capacity, tiers: {} })
    await flushPromises()
    expect(wrapper.findAll('tr').find(r => r.text().includes('account:1'))!.text()).toContain('gptQos.peakActive')
    wrapper.unmount()
  })

  it('starts with all new behavior off and saves explicit changes', async () => {
    const wrapper = mount(View, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises()
    const checks = wrapper.findAll('input[type="checkbox"]'); expect((checks[0].element as HTMLInputElement).checked).toBe(false); expect((checks[1].element as HTMLInputElement).checked).toBe(false)
    await checks[0].setValue(true); await checks[1].setValue(true); await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({ quality_routing_enabled: true, enabled: true }))
  })
  it('rollback disables every feature without modifying groups', async () => {
    const c = config(); c.enabled = c.quality_routing_enabled = c.capacity_enabled = c.peak.enabled = true; c.groups['9'] = defaultQoSGroup('premium'); mocks.get.mockResolvedValue(c)
    const wrapper = mount(View, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'gptQos.rollback')!.trigger('click'); await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, quality_routing_enabled: false, capacity_enabled: false, groups: { '9': defaultQoSGroup('premium') }, peak: expect.objectContaining({ enabled: false }) }))
  })
  it('Premium group defaults prohibit downgrade and save only scheduling fields', async () => {
    const wrapper = mount(GroupQoSEditor, { props: { groupId: 7 } }); await flushPromises(); await wrapper.find('select').setValue('premium')
    const checks = wrapper.findAll('input[type="checkbox"]'); expect((checks[0].element as HTMLInputElement).checked).toBe(false); expect((checks[1].element as HTMLInputElement).checked).toBe(true)
    await wrapper.find('button').trigger('click'); await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({ groups: { '7': defaultQoSGroup('premium') } }))
    expect(JSON.stringify(mocks.save.mock.calls[0][0])).not.toContain('rate_multiplier')
  })
  it('preserves group QoS saved after the global settings page was opened', async () => {
    const wrapper = mount(View)
    await flushPromises()
    const latest = config()
    latest.groups['7'] = defaultQoSGroup('premium')
    mocks.get.mockResolvedValue(latest)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({ groups: { '7': defaultQoSGroup('premium') } }))
    wrapper.unmount()
  })

  it('ignores a stale response for the previously edited group', async () => {
    let resolveOld!: (value: QoSConfig) => void
    mocks.get.mockReturnValueOnce(new Promise<QoSConfig>(resolve => { resolveOld = resolve }))
    const wrapper = mount(GroupQoSEditor, { props: { groupId: 7 } })
    const next = config()
    next.groups['9'] = defaultQoSGroup('premium')
    mocks.get.mockResolvedValue(next)
    await wrapper.setProps({ groupId: 9 })
    await flushPromises()
    const old = config()
    old.groups['7'] = defaultQoSGroup('economy')
    resolveOld(old)
    await flushPromises()
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('premium')
    await wrapper.get('select').setValue('economy')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({ groups: { '9': defaultQoSGroup('economy') } }))
    wrapper.unmount()
  })

  it('does not report success when the server does not return the saved group QoS', async () => {
    mocks.save.mockResolvedValue(config())
    const wrapper = mount(GroupQoSEditor, { props: { groupId: 7 } })
    await flushPromises()
    await wrapper.get('select').setValue('premium')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toBe('gptQos.saveNotConfirmed')
    wrapper.unmount()
  })
})
