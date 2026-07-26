import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileView from '@/views/user/ProfileView.vue'

const {
  fetchPublicSettingsMock,
  refreshUserMock,
  authState,
  routeState,
  routerReplace,
  routerPush
} = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn(),
  refreshUserMock: vi.fn(),
  authState: {
    user: null as Record<string, unknown> | null,
    refreshUser: vi.fn()
  },
  routeState: {
    path: '/profile',
    query: {} as Record<string, unknown>
  },
  routerReplace: vi.fn(),
  routerPush: vi.fn()
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    fetchPublicSettings: fetchPublicSettingsMock
  })
}))

vi.mock('@/utils/format', () => ({
  formatDate: () => 'April 2026'
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush
    })
  }
})

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
  StatCard: { template: '<div class="stat-card" />' },
  ProfileInfoCard: { template: '<div data-testid="profile-info-card" />' },
  ProfileReferralCodesCard: { template: '<div data-testid="profile-referral-codes-card" />' },
  ProfileBalanceNotifyCard: { template: '<div data-testid="profile-balance-notify-card" />' },
  ProfilePasswordForm: { template: '<div data-testid="profile-password-form" />' },
  ProfileTotpCard: { template: '<div data-testid="profile-totp-card" />' },
  Icon: true
}

describe('ProfileView', () => {
  beforeEach(() => {
    refreshUserMock.mockReset()
    fetchPublicSettingsMock.mockReset()
    routerReplace.mockReset()
    routerPush.mockReset()
    routeState.path = '/profile'
    routeState.query = {}
    refreshUserMock.mockResolvedValue(undefined)
    authState.refreshUser = refreshUserMock
    authState.user = {
      id: 1,
      username: 'alice',
      email: 'alice@example.com',
      role: 'user',
      balance: 10,
      concurrency: 2,
      status: 'active',
      allowed_groups: null,
      balance_notify_enabled: true,
      balance_notify_threshold: null,
      balance_notify_extra_emails: [],
      created_at: '2026-04-20T00:00:00Z',
      updated_at: '2026-04-20T00:00:00Z'
    }
    fetchPublicSettingsMock.mockResolvedValue({
      contact_info: '',
      balance_low_notify_enabled: false,
      balance_low_notify_threshold: 0,
      linuxdo_oauth_enabled: true,
      wechat_oauth_enabled: true,
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: true,
      oidc_oauth_provider_name: 'OIDC'
    })
  })

  it('renders only the profile tab by default, as its own standalone section', async () => {
    const wrapper = mount(ProfileView, { global: { stubs: STUBS } })
    await flushPromises()

    expect(wrapper.findAll('.stat-card')).toHaveLength(0)
    const shell = wrapper.get('[data-testid="profile-shell"]')
    expect(shell.exists()).toBe(true)
    expect(shell.html()).toContain('profile-info-card')
    expect(shell.html()).not.toContain('profile-password-form')
    expect(shell.html()).not.toContain('profile-totp-card')
  })

  it('switches to the security tab as a separate page, hiding the profile tab', async () => {
    const wrapper = mount(ProfileView, { global: { stubs: STUBS } })
    await flushPromises()

    const securityLink = wrapper.findAll('button.profile-settings-nav-link')[2]
    await securityLink.trigger('click')

    const shell = wrapper.get('[data-testid="profile-shell"]')
    expect(shell.html()).toContain('profile-password-form')
    expect(shell.html()).toContain('profile-totp-card')
    expect(shell.html()).not.toContain('profile-info-card')
    expect(routerReplace).toHaveBeenCalledWith({ path: '/profile', query: { tab: 'security' } })
  })

  it('switches to the referrals tab as a separate page, hiding the profile tab', async () => {
    const wrapper = mount(ProfileView, { global: { stubs: STUBS } })
    await flushPromises()

    const referralsLink = wrapper.findAll('button.profile-settings-nav-link')[1]
    await referralsLink.trigger('click')

    const shell = wrapper.get('[data-testid="profile-shell"]')
    expect(shell.html()).toContain('profile-referral-codes-card')
    expect(shell.html()).not.toContain('profile-info-card')
    expect(routerReplace).toHaveBeenCalledWith({ path: '/profile', query: { tab: 'referrals' } })
  })

  it('deep-links directly into the security tab via the tab query param', async () => {
    routeState.query = { tab: 'security' }
    const wrapper = mount(ProfileView, { global: { stubs: STUBS } })
    await flushPromises()

    const shell = wrapper.get('[data-testid="profile-shell"]')
    expect(shell.html()).toContain('profile-password-form')
    expect(shell.html()).not.toContain('profile-info-card')
  })
})
