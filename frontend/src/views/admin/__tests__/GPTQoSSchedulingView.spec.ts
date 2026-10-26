import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import View from '../GPTQoSSchedulingView.vue'
import GroupQoSEditor from '@/components/group/GroupQoSEditor.vue'
import { defaultQoSGroup, type QoSConfig } from '@/api/admin/gptQos'
const mocks = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), snapshot: vi.fn() }))
vi.mock('@/api/admin/gptQos', async importOriginal => ({ ...(await importOriginal<typeof import('@/api/admin/gptQos')>()), getQoSConfig: mocks.get, saveQoSConfig: mocks.save, getQoSSnapshot: mocks.snapshot }))
vi.mock('vue-i18n', async importOriginal => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
function config(): QoSConfig { return { version: 1, quality_routing_enabled: false, enabled: false, capacity_enabled: false, premium_weight: 6, standard_weight: 3, economy_weight: 1, max_waiting: 1000, request_budget_ms: 300000, high_effort_budget_ms: 900000, max_attempts: 4, log_sample_rate: 0.1, peak: { enabled: false, strict_reserve: true, enter_utilization: 0.8, exit_utilization: 0.65, protection_ratio: 0.3, enter_hold_ms: 5000, exit_hold_ms: 15000 }, groups: {}, resource_pools: {} } }
beforeEach(() => { vi.clearAllMocks(); mocks.get.mockResolvedValue(config()); mocks.save.mockImplementation(async c => c); mocks.snapshot.mockResolvedValue({ available: true, tiers: {}, capacity: { active: 0, queued: 0 } }) })
describe('GPT QoS controls', () => {
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
