import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import DashboardView from '@/views/user/DashboardView.vue'
import type { DailyCheckinStatus } from '@/types'

const routerPush = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const getDashboardStats = vi.hoisted(() => vi.fn())
const getDashboardTrend = vi.hoisted(() => vi.fn())
const getDashboardModels = vi.hoisted(() => vi.fn())
const getByDateRange = vi.hoisted(() => vi.fn())
const getMyPlatformQuotas = vi.hoisted(() => vi.fn())
const getDailyCheckinStatus = vi.hoisted(() => vi.fn())
const claimDailyCheckin = vi.hoisted(() => vi.fn())
const fetchPublicSettings = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      push: routerPush
    })
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'dashboard.dailyCheckin.title': '每日签到',
    'dashboard.dailyCheckin.action': '每日签到',
    'dashboard.dailyCheckin.checking': '签到中...',
    'dashboard.dailyCheckin.checked': '已签到',
    'dashboard.dailyCheckin.exhausted': '今日已发完',
    'dashboard.dailyCheckin.ready': '可签到',
    'dashboard.dailyCheckin.hint': '试试看今天的手气吧',
    'dashboard.dailyCheckin.checkedHint': '今日已获得 {amount}',
    'dashboard.dailyCheckin.exhaustedHint': '今日签到额度已发完',
    'dashboard.dailyCheckin.rechargeRequired': '需要充值',
    'dashboard.dailyCheckin.rechargeRequiredHint': '达到累计充值要求后即可签到',
    'dashboard.dailyCheckin.goRecharge': '去充值'
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        let message = messages[key] ?? key
        for (const [name, value] of Object.entries(params ?? {})) {
          message = message.replace(`{${name}}`, value)
        }
        return message
      }
    })
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { id: 1, username: 'alice', balance: 0 },
    isSimpleMode: false,
    refreshUser
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: {
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    },
    fetchPublicSettings,
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats,
    getDashboardTrend,
    getDashboardModels,
    getByDateRange
  }
}))

vi.mock('@/api/user', () => ({
  getMyPlatformQuotas,
  getDailyCheckinStatus,
  claimDailyCheckin
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: () => '签到失败'
}))

const IconStub = defineComponent({
  props: {
    name: { type: String, required: true }
  },
  template: '<span class="icon-stub" :data-icon="name" />'
})

const BaseDialogStub = defineComponent({
  props: {
    show: { type: Boolean, required: true },
    title: { type: String, required: true }
  },
  template: `
    <section v-if="show" class="dialog-stub">
      <h3 class="dialog-title">
        <slot name="title">{{ title }}</slot>
      </h3>
      <slot />
    </section>
  `
})

const GoogleAdSenseAdStub = defineComponent({
  template: '<div data-testid="adsense-ad" />'
})

function statusFixture(overrides: Partial<DailyCheckinStatus> = {}): DailyCheckinStatus {
  return {
    enabled: true,
    ads_enabled: true,
    checked_in_today: false,
    today_reward: 0,
    recharge_eligible: false,
    checkin_date: '2026-06-22',
    last_checkin_at: null,
    next_available_at: '2026-06-23T00:00:00Z',
    exhausted_today: false,
    ...overrides
  }
}

async function mountDashboard(status: DailyCheckinStatus) {
  getDailyCheckinStatus.mockResolvedValue(status)

  const wrapper = mount(DashboardView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: true,
        UserDashboardStats: true,
        UserDashboardCharts: true,
        UserDashboardRecentUsage: true,
        UserDashboardQuickActions: true,
        TurnstileWidget: true,
        GoogleAdSenseAd: GoogleAdSenseAdStub,
        BaseDialog: BaseDialogStub,
        Icon: IconStub
      }
    }
  })

  await flushPromises()
  return wrapper
}

describe('DashboardView daily check-in UI', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    refreshUser.mockResolvedValue(undefined)
    getDashboardStats.mockResolvedValue({
      total_api_keys: 1,
      active_api_keys: 1,
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_creation_tokens: 0,
      total_cache_read_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      today_requests: 0,
      today_input_tokens: 0,
      today_output_tokens: 0,
      today_cache_creation_tokens: 0,
      today_cache_read_tokens: 0,
      today_tokens: 0,
      today_cost: 0,
      today_actual_cost: 0,
      average_duration_ms: 0,
      rpm: 0,
      tpm: 0,
      by_platform: []
    })
    getDashboardTrend.mockResolvedValue({ trend: [] })
    getDashboardModels.mockResolvedValue({ models: [] })
    getByDateRange.mockResolvedValue({ items: [] })
    getMyPlatformQuotas.mockResolvedValue({ platform_quotas: [] })
    fetchPublicSettings.mockResolvedValue(undefined)
  })

  it('keeps the entry button as daily check-in when recharge is required', async () => {
    const wrapper = await mountDashboard(statusFixture())
    const entryButton = wrapper.get('[data-testid="daily-checkin-entry"]')

    expect(entryButton.text()).toContain('每日签到')
    expect(entryButton.text()).not.toContain('需要充值')
    expect(entryButton.find('[data-icon="gift"]').exists()).toBe(true)
    expect(entryButton.find('[data-icon="creditCard"]').exists()).toBe(false)
  })

  it('renders the gift icon beside the dialog title and shows the recharge hint once', async () => {
    const wrapper = await mountDashboard(statusFixture())

    await wrapper.get('[data-testid="daily-checkin-entry"]').trigger('click')
    await flushPromises()

    const title = wrapper.get('.dialog-title')
    expect(title.text()).toContain('每日签到')
    expect(title.find('[data-icon="gift"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('需要充值')
    expect(wrapper.text()).toContain('达到累计充值要求后即可签到')
    expect(wrapper.findAll('[data-icon="creditCard"]')).toHaveLength(2)
    expect(wrapper.text().match(/达到累计充值要求后即可签到/g)).toHaveLength(1)
  })

  it('shows the ad only when the daily check-in ad switch is enabled', async () => {
    const readyStatus = statusFixture({
      recharge_eligible: true
    })
    const enabledWrapper = await mountDashboard(readyStatus)

    await enabledWrapper.get('[data-testid="daily-checkin-entry"]').trigger('click')
    await flushPromises()

    expect(enabledWrapper.find('[data-testid="adsense-ad"]').exists()).toBe(true)

    const disabledWrapper = await mountDashboard({
      ...readyStatus,
      ads_enabled: false
    })

    await disabledWrapper.get('[data-testid="daily-checkin-entry"]').trigger('click')
    await flushPromises()

    expect(disabledWrapper.find('[data-testid="adsense-ad"]').exists()).toBe(false)
  })
})
