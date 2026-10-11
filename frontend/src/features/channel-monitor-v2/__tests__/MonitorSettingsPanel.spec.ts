import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), groups: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => false }) }))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorV2Mode: () => true, getChannelMonitorMode: () => 'v2' }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: mocks.groups } } }))
vi.mock('@/api/channelMonitorV2', () => ({ getConfig: mocks.get, updateConfig: mocks.update, MONITOR_ERROR_CATEGORIES: [] }))

it('shows the TypeSafe / Jev platform and persists the Jev monitored group', async () => {
  mocks.get.mockResolvedValue({
    version: 3, enabled: true, refresh_interval_seconds: 60, group_ids: [9],
    platforms: [{ platform: 'typesafe', enabled: true, models: [] }],
    candy_probes: [], ignored_error_categories: [],
  })
  mocks.groups.mockResolvedValue([{ id: 7, name: 'Jev', platform: 'typesafe' }])
  mocks.update.mockImplementation(async payload => ({ ...payload, version: 4 }))
  const wrapper = mount(MonitorSettingsPanel, {
    global: { stubs: { Toggle: true, Icon: true, RouterLink: true, MonitorCandySettings: true } },
  })
  await flushPromises()
  expect(wrapper.text()).toContain('TypeSafe / Jev')
  const group = wrapper.findAll('label').find(label => label.text().includes('#7'))!
  expect(group.text()).toContain('Jev')
  await group.get('input[type="checkbox"]').setValue(true)
  await wrapper.get('header button.btn-primary').trigger('click')
  await flushPromises()
  expect(mocks.update).toHaveBeenCalledWith(expect.objectContaining({
    group_ids: [7, 9], platforms: [{ platform: 'typesafe', enabled: true, models: [] }],
  }))
  wrapper.unmount()
})
