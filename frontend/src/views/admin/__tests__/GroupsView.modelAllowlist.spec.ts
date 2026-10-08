import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup, UpdateGroupRequest } from '@/types'
import GroupsView from '../GroupsView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), create: vi.fn(), update: vi.fn(), getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(), getCapacitySummary: vi.fn(), getLiveCapability: vi.fn(),
  showSuccess: vi.fn(), showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { ...mocks, getAll: vi.fn().mockResolvedValue([]), duplicate: vi.fn(), delete: vi.fn(), updateSortOrder: vi.fn() },
    accounts: { list: vi.fn(), getById: vi.fn() },
  },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ isCurrentStep: vi.fn(() => false), nextStep: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const baseGroup = {
  id: 42, name: 'Allowed models', platform: 'anthropic', rate_multiplier: 1, rpm_limit: 0,
  is_exclusive: false, status: 'active', subscription_type: 'standard',
  daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
  allow_image_generation: false, allow_batch_image_generation: false,
  peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1,
  model_routing: null, model_routing_enabled: false, mcp_xml_inject: true,
  supported_model_scopes: [], account_count: 1, active_account_count: 1, sort_order: 10,
  model_allowlist: { enabled: true, models: ['claude-sonnet-5'] },
  created_at: '2026-10-08T00:00:00Z', updated_at: '2026-10-08T00:00:00Z',
} as unknown as AdminGroup
let groups: AdminGroup[]

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: defineComponent({ template: '<main><slot /></main>' }),
        TablePageLayout: defineComponent({ template: '<section><slot name="filters" /><slot name="table" /></section>' }),
        DataTable: defineComponent({ props: { data: Array }, template: '<div><div v-for="row in data" :key="row.id" :data-testid="`group-row-${row.id}`"><slot name="cell-actions" :row="row" /></div></div>' }),
        BaseDialog: defineComponent({ props: { show: Boolean }, template: '<div v-if="show"><slot /><slot name="footer" /></div>' }),
        Pagination: true, ConfirmDialog: true, EmptyState: true, Select: true, PlatformIcon: true, Icon: true,
        GroupCapacityBadge: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true,
      },
    },
  })
}

async function editGroup(wrapper: ReturnType<typeof mountView>, id: number) {
  await wrapper.get(`[data-testid="group-row-${id}"]`).findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
  await flushPromises()
}

describe('GroupsView model allowlist', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.clearAllMocks()
    groups = [structuredClone(baseGroup), { ...structuredClone(baseGroup), id: 43, name: 'Second group', model_allowlist: { enabled: false, models: ['claude-opus-5'] } }]
    mocks.list.mockImplementation(async () => ({ items: groups, total: groups.length, page: 1, page_size: 20, pages: 1 }))
    mocks.update.mockImplementation(async (id: number, payload: UpdateGroupRequest) => {
      const index = groups.findIndex(group => group.id === id)
      groups[index] = { ...groups[index], ...payload } as AdminGroup
      return groups[index]
    })
    mocks.create.mockResolvedValue(baseGroup)
    mocks.getModelAllowlistCandidates.mockResolvedValue(['claude-sonnet-5', 'claude-opus-5'])
    mocks.getUsageSummary.mockResolvedValue([])
    mocks.getCapacitySummary.mockResolvedValue([])
    mocks.getLiveCapability.mockResolvedValue({ supported: false })
  })
  afterEach(() => vi.restoreAllMocks())

  it('loads an enabled allowlist, saves it disabled and restores its selected models when enabled again', async () => {
    const wrapper = mountView()
    await flushPromises()
    await editGroup(wrapper, 42)
    const toggle = wrapper.get('[data-testid="edit-model-allowlist"]')
    expect(toggle.attributes('aria-checked')).toBe('true')
    await toggle.trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenLastCalledWith(42, expect.objectContaining({ model_allowlist: { enabled: false, models: ['claude-sonnet-5'] } }))

    await editGroup(wrapper, 42)
    expect(wrapper.get('[data-testid="edit-model-allowlist"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('[data-testid="edit-model-allowlist"]').trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenLastCalledWith(42, expect.objectContaining({ model_allowlist: { enabled: true, models: ['claude-sonnet-5'] } }))
    expect(mocks.showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('loads each group’s saved allowlist when editing groups on the same platform', async () => {
    const wrapper = mountView()
    await flushPromises()
    await editGroup(wrapper, 42)
    await wrapper.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    await editGroup(wrapper, 43)
    expect(wrapper.get('[data-testid="edit-model-allowlist"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenLastCalledWith(43, expect.objectContaining({ model_allowlist: { enabled: false, models: ['claude-opus-5'] } }))
    wrapper.unmount()
  })

  it('resets the allowlist when a create dialog is cancelled and reopened', async () => {
    const wrapper = mountView()
    await flushPromises()
    const openCreate = () => wrapper.findAll('button').find(button => button.text() === 'admin.groups.createGroup')!.trigger('click')
    await openCreate()
    await flushPromises()
    await wrapper.get('[data-testid="create-model-allowlist"]').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    await openCreate()
    await flushPromises()
    expect(wrapper.get('[data-testid="create-model-allowlist"]').attributes('aria-checked')).toBe('false')
    wrapper.unmount()
  })
})
