import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AffiliateView from '@/views/user/AffiliateView.vue'

const {
  getAffiliateDetailMock,
  transferAffiliateQuotaMock,
  copyToClipboardMock,
  showSuccessMock,
  showErrorMock,
  showWarningMock,
  refreshUserMock
} = vi.hoisted(() => ({
  getAffiliateDetailMock: vi.fn(),
  transferAffiliateQuotaMock: vi.fn(),
  copyToClipboardMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  showWarningMock: vi.fn(),
  refreshUserMock: vi.fn()
}))

vi.mock('@/api/user', () => ({
  default: {
    getAffiliateDetail: getAffiliateDetailMock,
    transferAffiliateQuota: transferAffiliateQuotaMock
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showError: showErrorMock,
    showWarning: showWarningMock
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    refreshUser: refreshUserMock
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: copyToClipboardMock
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const STUBS = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true
}

function createDetail(overrides: Record<string, unknown> = {}) {
  return {
    user_id: 1,
    aff_code: 'AFF123',
    inviter_id: null,
    aff_count: 3,
    aff_quota: 12.5,
    aff_frozen_quota: 0,
    aff_history_quota: 40,
    effective_rebate_rate_percent: 15,
    invitees: [
      {
        user_id: 2,
        email: 'bob@example.com',
        username: 'bob',
        total_rebate: 5,
        created_at: '2026-06-17T08:00:00Z'
      }
    ],
    ...overrides
  }
}

describe('AffiliateView', () => {
  beforeEach(() => {
    getAffiliateDetailMock.mockReset()
    transferAffiliateQuotaMock.mockReset()
    copyToClipboardMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    showWarningMock.mockReset()
    refreshUserMock.mockReset()
    refreshUserMock.mockResolvedValue(undefined)
    getAffiliateDetailMock.mockResolvedValue(createDetail())
  })

  it('renders the stat grid, referral code and invitee history', async () => {
    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    expect(wrapper.findAll('.affiliate-stat-card')).toHaveLength(4)
    expect(wrapper.get('.affiliate-title').exists()).toBe(true)
    expect(wrapper.text()).toContain('AFF123')
    expect(wrapper.text()).toContain('bob@example.com')
    expect(wrapper.findAll('.affiliate-table tbody tr')).toHaveLength(1)
  })

  it('copies the referral code and the invite link separately', async () => {
    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    const copyButtons = wrapper.findAll('.affiliate-copy-button')
    expect(copyButtons).toHaveLength(2)

    await copyButtons[0].trigger('click')
    expect(copyToClipboardMock).toHaveBeenCalledWith('AFF123', 'affiliate.codeCopied')

    await copyButtons[1].trigger('click')
    expect(copyToClipboardMock).toHaveBeenLastCalledWith(
      expect.stringContaining('/register?aff=AFF123'),
      'affiliate.linkCopied'
    )
  })

  it('transfers the available rebate quota and refreshes the detail', async () => {
    transferAffiliateQuotaMock.mockResolvedValue({ transferred_quota: 12.5, balance: 30 })
    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    await wrapper.get('.affiliate-claim-button').trigger('click')
    await flushPromises()

    expect(transferAffiliateQuotaMock).toHaveBeenCalled()
    expect(showSuccessMock).toHaveBeenCalled()
    expect(getAffiliateDetailMock).toHaveBeenCalledTimes(2)
  })

  it('disables the claim button when there is no transferable quota', async () => {
    getAffiliateDetailMock.mockResolvedValue(createDetail({ aff_quota: 0 }))
    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    expect(wrapper.get('.affiliate-claim-button').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.affiliate-claim-hint').exists()).toBe(true)
  })

  it('shows the empty state and disables export when there are no invitees', async () => {
    getAffiliateDetailMock.mockResolvedValue(createDetail({ invitees: [] }))
    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    expect(wrapper.get('.affiliate-empty').exists()).toBe(true)
    expect(wrapper.find('.affiliate-table').exists()).toBe(false)
    expect(wrapper.get('.affiliate-export-button').attributes('disabled')).toBeDefined()
  })

  it('exports invitee rows as a CSV download', async () => {
    const createObjectURL = vi.fn(() => 'blob:affiliate')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...window.URL, createObjectURL, revokeObjectURL })
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const wrapper = mount(AffiliateView, { global: { stubs: STUBS } })
    await flushPromises()

    await wrapper.get('.affiliate-export-button').trigger('click')

    expect(createObjectURL).toHaveBeenCalled()
    expect(clickSpy).toHaveBeenCalled()
    expect(showSuccessMock).toHaveBeenCalledWith('affiliate.invitees.exportSuccess')

    clickSpy.mockRestore()
    vi.unstubAllGlobals()
  })
})
