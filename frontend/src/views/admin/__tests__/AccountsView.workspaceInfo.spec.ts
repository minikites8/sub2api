import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import OpenAIWorkspaceInfoModal from '@/components/admin/account/OpenAIWorkspaceInfoModal.vue'

const { getById, getWorkspaceInfo, showError } = vi.hoisted(() => ({
  getById: vi.fn(),
  getWorkspaceInfo: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: vi.fn().mockImplementation(async () => ({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })),
      listWithEtag: vi.fn().mockResolvedValue({ notModified: true, etag: null, data: null }),
      getById,
      getOpenAIWorkspaceInfo: getWorkspaceInfo,
      getBatchTodayStats: vi.fn().mockResolvedValue({ stats: {} }),
      getManagementCapabilities: vi.fn().mockResolvedValue({ concurrency_upgrade_enabled: false }),
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: false })
    },
    proxies: { getAll: vi.fn().mockResolvedValue([]) },
    groups: { getAll: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isAdmin: true, isSimpleMode: false })
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const listRow = {
  id: 42, name: 'Team account', platform: 'openai', type: 'oauth', status: 'active',
  schedulable: true, concurrency: 2, priority: 1, group_ids: [],
  credentials: { team_plan_type: 'team' }
}
const fullAccount = {
  ...listRow, groups: [], account_groups: [],
  credentials: { ...listRow.credentials, access_token: 'fixture-token' }
}
const workspaceInfo = {
  account_id: 'workspace-fixture', name: 'Fresh workspace', created_time: '2026-10-01T00:00:00Z',
  organization_id: 'org-fixture', plan_type: 'team', account_user_role: 'account-owner',
  seat_type_counts: { default: 3 }, maximum_seats: 10, fetched_at: 1791763200
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: {
          props: ['data'],
          template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>'
        },
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
        AccountTableFilters: true, AccountBulkActionsBar: true, Pagination: true, ConfirmDialog: true,
        AccountActionMenu: false, OpenAIWorkspaceInfoModal: true, OpenAISessionsModal: true,
        IQTestModal: true, ImportDataModal: true, ReAuthAccountModal: true,
        AccountTestModal: true, AccountStatsModal: true, ScheduledTestsPanel: true,
        SyncFromCrsModal: true, TempUnschedStatusModal: true, ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true, CreateAccountModal: true, EditAccountModal: true,
        BulkEditAccountModal: true, YeTeamRedeemModal: true, TotpStepUpDialog: true,
        PlatformTypeBadge: true, HelpTooltip: true, Icon: true, Teleport: true
      }
    }
  })
}

let wrapper: ReturnType<typeof mountView>

async function clickWorkspaceMenu() {
  const more = wrapper.findAll('button').find(button => button.text() === 'common.more')!
  await more.trigger('click')
  const query = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.workspaceInfo.menu')!
  await query.trigger('click')
  await flushPromises()
}

describe('AccountsView workspace-info menu wiring', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    getById.mockReset().mockImplementation(async () => structuredClone(fullAccount))
    getWorkspaceInfo.mockReset().mockResolvedValue(workspaceInfo)
  })
  afterEach(() => {
    wrapper?.unmount()
  })

  it('queries the selected account and opens the modal from the real menu button', async () => {
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()

    expect(getById).toHaveBeenCalledTimes(1)
    expect(getById).toHaveBeenCalledWith(42)
    expect(getWorkspaceInfo).toHaveBeenCalledTimes(1)
    expect(getWorkspaceInfo).toHaveBeenCalledWith(42)
    const modal = wrapper.findComponent(OpenAIWorkspaceInfoModal)
    expect(modal.props('show')).toBe(true)
    expect(modal.props('info')).toEqual(workspaceInfo)
    expect(modal.props('account').credentials).toMatchObject({
      access_token: 'fixture-token', team_account_id: 'workspace-fixture', team_name: 'Fresh workspace'
    })
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(false)
  })

  it('shows query errors and permits a successful retry', async () => {
    getWorkspaceInfo.mockRejectedValueOnce({ message: 'workspace lookup failed' })
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()
    expect(showError).toHaveBeenCalledWith('workspace lookup failed')
    expect(wrapper.findComponent(OpenAIWorkspaceInfoModal).props('show')).toBe(false)

    await clickWorkspaceMenu()
    expect(getWorkspaceInfo).toHaveBeenCalledTimes(2)
    expect(wrapper.findComponent(OpenAIWorkspaceInfoModal).props('show')).toBe(true)
  })

  it('uses the localized fallback for an empty upstream error', async () => {
    getWorkspaceInfo.mockRejectedValueOnce({})
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()
    expect(showError).toHaveBeenCalledWith('admin.accounts.workspaceInfo.loadFailed')
  })

  it('coalesces repeated clicks while the workspace query is pending', async () => {
    let resolve!: (info: typeof workspaceInfo) => void
    getWorkspaceInfo.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()
    await clickWorkspaceMenu()
    expect(getWorkspaceInfo).toHaveBeenCalledTimes(1)
    expect(getById).toHaveBeenCalledTimes(1)
    resolve(workspaceInfo)
    await flushPromises()
    expect(wrapper.findComponent(OpenAIWorkspaceInfoModal).props('show')).toBe(true)
  })

  it('reports detail-loading errors and releases the query guard for retry', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getById.mockRejectedValueOnce({ message: 'account detail failed' })
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()
    expect(showError).toHaveBeenCalledWith('account detail failed')
    expect(getWorkspaceInfo).toHaveBeenCalledTimes(0)
    expect(wrapper.findComponent(OpenAIWorkspaceInfoModal).props('show')).toBe(false)

    await clickWorkspaceMenu()
    expect(getWorkspaceInfo).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent(OpenAIWorkspaceInfoModal).props('show')).toBe(true)
    consoleError.mockRestore()
  })

  it('clears workspace state when the modal closes', async () => {
    wrapper = mountView()
    await flushPromises()
    await clickWorkspaceMenu()
    const modal = wrapper.findComponent(OpenAIWorkspaceInfoModal)
    expect(modal.props('show')).toBe(true)
    modal.vm.$emit('close')
    await flushPromises()
    expect(modal.props()).toMatchObject({ show: false, account: null, info: null })
  })
})
