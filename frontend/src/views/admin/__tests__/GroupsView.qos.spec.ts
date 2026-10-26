import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'
import GroupQoSEditor from '@/components/group/GroupQoSEditor.vue'
import { defaultQoSGroup, type QoSConfig } from '@/api/admin/gptQos'

const mocks = vi.hoisted(() => ({
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  getQoS: vi.fn(),
  saveQoS: vi.fn()
}))

vi.mock('@/api/admin/gptQos', async importOriginal => ({
  ...(await importOriginal<typeof import('@/api/admin/gptQos')>()),
  getQoSConfig: mocks.getQoS,
  saveQoSConfig: mocks.saveQoS
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: mocks.listGroups,
      create: mocks.createGroup,
      update: mocks.updateGroup,
      getModelAllowlistCandidates: mocks.getModelAllowlistCandidates,
      getUsageSummary: mocks.getUsageSummary,
      getCapacitySummary: mocks.getCapacitySummary,
      getLiveCapability: mocks.getLiveCapability,
      getAll: vi.fn().mockResolvedValue([]),
      duplicate: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: { list: vi.fn(), getById: vi.fn() }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: vi.fn(() => false), nextStep: vi.fn() })
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const baseGroup = {
  id: 42, name: 'Claude Code', description: null, platform: 'openai', rate_multiplier: 1, rpm_limit: 0,
  is_exclusive: false, status: 'active', subscription_type: 'standard',
  daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
  allow_image_generation: false, allow_batch_image_generation: false,
  peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1,
  claude_code_only: false, fallback_group_id: null, fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false, require_oauth_only: false, require_privacy_set: false,
  force_openai_fast: false, free_openai_fast: false, stream_only: true, enable_bps: true, scheduling_strategy: 'priority_weekly',
  model_routing: null, model_routing_enabled: false, mcp_xml_inject: true, supported_model_scopes: [],
  account_count: 1, active_account_count: 1, rate_limited_account_count: 0, sort_order: 10,
  created_at: '2026-09-24T00:00:00Z', updated_at: '2026-09-24T00:00:00Z'
} as unknown as AdminGroup

// 同时渲染名称列和操作列，才能看到「仅流式」标记
const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: `<div><div v-for="row in data" :key="row.id" :data-testid="'group-row-' + row.id">
    <slot name="cell-name" :value="row.name" :row="row" /><slot name="cell-actions" :row="row" />
  </div></div>`
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: defineComponent({ template: '<main><slot /></main>' }),
        TablePageLayout: defineComponent({ template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' }),
        DataTable: DataTableStub,
        BaseDialog: defineComponent({ props: { show: Boolean }, template: '<div v-if="show"><slot /><slot name="footer" /></div>' }),
        Pagination: true, ConfirmDialog: true, EmptyState: true, Select: true, PlatformIcon: true, Icon: true, RouterLink: true,
        GroupCapacityBadge: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true
      }
    }
  })
}

const buttonByText = (wrapper: ReturnType<typeof mountView>, text: string) =>
  wrapper.findAll('button').find((button) => button.text() === text)

let stored: QoSConfig

async function openGroup(wrapper: ReturnType<typeof mountView>) {
  await wrapper.get('[data-testid="group-row-42"]').findAll('button').find(b => b.text() === 'common.edit')!.trigger('click')
  await flushPromises()
}

describe('GroupsView QoS persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.clearAllMocks()
    mocks.listGroups.mockResolvedValue({ items: [baseGroup], total: 1, page: 1, page_size: 20, pages: 1 })
    mocks.updateGroup.mockResolvedValue(baseGroup)
    mocks.getModelAllowlistCandidates.mockResolvedValue([])
    mocks.getUsageSummary.mockResolvedValue([])
    mocks.getCapacitySummary.mockResolvedValue([])
    mocks.getLiveCapability.mockResolvedValue({ supported: false })
    stored = {
      version: 1, quality_routing_enabled: false, enabled: false, capacity_enabled: false,
      premium_weight: 6, standard_weight: 3, economy_weight: 1, max_waiting: 1000,
      request_budget_ms: 300000, high_effort_budget_ms: 900000, max_attempts: 4, log_sample_rate: 0.1,
      peak: { enabled: false, strict_reserve: true, enter_utilization: 0.8, exit_utilization: 0.65, protection_ratio: 0.3, enter_hold_ms: 5000, exit_hold_ms: 15000 },
      groups: { '99': defaultQoSGroup('economy') }, resource_pools: {}
    }
    mocks.getQoS.mockImplementation(async () => structuredClone(stored))
    mocks.saveQoS.mockImplementation(async (next: QoSConfig) => {
      stored = structuredClone(next)
      return structuredClone(stored)
    })
  })
  afterEach(() => vi.restoreAllMocks())

  it.each(['premium', 'economy'] as const)('persists %s with the main Update button and restores it after reopening', async tier => {
    const wrapper = mountView()
    await flushPromises()
    await openGroup(wrapper)
    await wrapper.getComponent(GroupQoSEditor).get('select').setValue(tier)
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.saveQoS).toHaveBeenCalledTimes(1)
    expect(stored.groups['42']).toEqual(defaultQoSGroup(tier))
    expect(stored.groups['99']).toEqual(defaultQoSGroup('economy'))
    expect(mocks.updateGroup).toHaveBeenCalledWith(42, expect.objectContaining({ rate_multiplier: baseGroup.rate_multiplier }))
    expect(mocks.updateGroup.mock.calls[0][1]).not.toHaveProperty('qos_tier')
    expect(wrapper.find('#edit-group-form').exists()).toBe(false)

    await openGroup(wrapper)
    const editor = wrapper.getComponent(GroupQoSEditor)
    expect((editor.get('select').element as HTMLSelectElement).value).toBe(tier)
    expect((editor.findAll('input[type="number"]')[0].element as HTMLInputElement).value).toBe(String(defaultQoSGroup(tier).weight))
    wrapper.unmount()
  })

  it('keeps the dialog open and reports a partial save when QoS persistence fails', async () => {
    mocks.saveQoS.mockRejectedValueOnce(new Error('settings unavailable'))
    const wrapper = mountView()
    await flushPromises()
    await openGroup(wrapper)
    await wrapper.getComponent(GroupQoSEditor).get('select').setValue('premium')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.updateGroup).toHaveBeenCalledTimes(1)
    expect(mocks.saveQoS).toHaveBeenCalledTimes(1)
    expect(mocks.showSuccess).not.toHaveBeenCalled()
    expect(mocks.showError).toHaveBeenCalledWith(expect.stringContaining('gptQos.groupPartialSave'))
    expect(wrapper.find('#edit-group-form').exists()).toBe(true)
    expect(wrapper.getComponent(GroupQoSEditor).text()).toContain('settings unavailable')
    wrapper.unmount()
  })

  it('does not rewrite QoS settings when only unrelated group fields are updated', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openGroup(wrapper)
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.updateGroup).toHaveBeenCalledTimes(1)
    expect(mocks.saveQoS).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('finishes saving the original group when another group is opened during the request', async () => {
    mocks.listGroups.mockResolvedValue({ items: [baseGroup, { ...baseGroup, id: 43, name: 'Second group' }], total: 2, page: 1, page_size: 20, pages: 1 })
    let resolveUpdate!: (value: AdminGroup) => void
    mocks.updateGroup.mockReturnValueOnce(new Promise<AdminGroup>(resolve => { resolveUpdate = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await openGroup(wrapper)
    await wrapper.getComponent(GroupQoSEditor).get('select').setValue('premium')
    await wrapper.get('#edit-group-form').trigger('submit')
    await wrapper.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    await wrapper.get('[data-testid="group-row-43"]').findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()
    resolveUpdate(baseGroup)
    await flushPromises()
    expect(stored.groups['42']).toEqual(defaultQoSGroup('premium'))
    expect(stored.groups['43']).toBeUndefined()
    expect(wrapper.find('#edit-group-form').exists()).toBe(true)
    expect((wrapper.getComponent(GroupQoSEditor).get('select').element as HTMLSelectElement).value).toBe('standard')
    wrapper.unmount()
  })
})
