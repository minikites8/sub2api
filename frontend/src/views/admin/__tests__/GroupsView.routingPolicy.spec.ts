import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const mocks = vi.hoisted(() => ({
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
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
        Pagination: true, ConfirmDialog: true, EmptyState: true, Select: true, PlatformIcon: true, Icon: true,
        GroupCapacityBadge: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true
      }
    }
  })
}

const buttonByText = (wrapper: ReturnType<typeof mountView>, text: string) =>
  wrapper.findAll('button').find((button) => button.text() === text)

describe('GroupsView routing policy', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.clearAllMocks()
    mocks.listGroups.mockResolvedValue({ items: [baseGroup], total: 1, page: 1, page_size: 20, pages: 1 })
    mocks.createGroup.mockResolvedValue(baseGroup)
    mocks.updateGroup.mockResolvedValue(baseGroup)
    mocks.getModelAllowlistCandidates.mockResolvedValue([])
    mocks.getUsageSummary.mockResolvedValue([])
    mocks.getCapacitySummary.mockResolvedValue([])
    mocks.getLiveCapability.mockResolvedValue({ supported: false })
  })
  afterEach(() => vi.restoreAllMocks())

  it('defaults to platform upstream and balanced scheduling when creating', async () => {
    const wrapper = mountView()
    await flushPromises()
    await buttonByText(wrapper, 'admin.groups.createGroup')!.trigger('click')
    await flushPromises()
    const platform = wrapper.findAllComponents({ name: 'Select' }).find(c => c.props('modelValue') === 'anthropic')!
    platform.vm.$emit('update:modelValue', 'openai')
    await flushPromises()
    expect(wrapper.get('[data-testid="create-enable-bps"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[data-testid="create-strategy-balanced"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('#create-group-form input[type="text"]').setValue('New routing group')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.createGroup).toHaveBeenCalledWith(expect.objectContaining({ enable_bps: false, scheduling_strategy: 'balanced' }))
    wrapper.unmount()
  })

  it('creates a BPS-enabled group using the 5h priority strategy', async () => {
    const wrapper = mountView()
    await flushPromises()
    await buttonByText(wrapper, 'admin.groups.createGroup')!.trigger('click')
    await flushPromises()
    const platform = wrapper.findAllComponents({ name: 'Select' }).find(c => c.props('modelValue') === 'anthropic')!
    platform.vm.$emit('update:modelValue', 'openai')
    await flushPromises()
    await wrapper.get('[data-testid="create-enable-bps"]').trigger('click')
    await wrapper.get('[data-testid="create-strategy-priority_5h"]').trigger('click')
    await wrapper.get('#create-group-form input[type="text"]').setValue('5h group')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.createGroup).toHaveBeenCalledWith(expect.objectContaining({ enable_bps: true, scheduling_strategy: 'priority_5h' }))
    wrapper.unmount()
  })

  it('loads saved settings and submits explicit false and balanced values', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="group-row-42"]').findAll('button').find(b => b.text() === 'common.edit')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="edit-enable-bps"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.get('[data-testid="edit-strategy-priority_weekly"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[data-testid="edit-enable-bps"]').trigger('click')
    await wrapper.get('[data-testid="edit-strategy-balanced"]').trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.updateGroup).toHaveBeenCalledWith(42, expect.objectContaining({ enable_bps: false, scheduling_strategy: 'balanced' }))
    wrapper.unmount()
  })
})
