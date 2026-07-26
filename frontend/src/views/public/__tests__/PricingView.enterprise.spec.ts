// 定价页的档位特性和对比表都是按 i18n key 逐条组装的，key 拼错不会报错，
// 只会在页面上渲染出 "pricingPage.xxx" 这样的原文。这里用真实文案挂载，
// 确认企业版条目和对比表都真的解析到了。
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import PricingView from '@/views/public/PricingView.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  }),
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false, checkAuth: vi.fn() }),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => ({ path: '/pricing', query: {} }) }
})

function mountWith(locale: string, messages: unknown) {
  const i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: locale,
    messages: { [locale]: messages as Record<string, unknown> },
  })
  return mount(PricingView, {
    global: {
      plugins: [i18n],
      stubs: { RouterLink: { template: '<a><slot /></a>' }, LocaleSwitcher: true, Icon: true },
    },
  })
}

describe('PricingView enterprise copy', () => {
  it('renders the enterprise tier with all eight selling points (zh)', () => {
    const wrapper = mountWith('zh', zh)
    const enterprise = wrapper.findAll('.pricing-tier').at(-1)!

    expect(enterprise.text()).toContain('企业版')
    expect(enterprise.text()).toContain('定制方案')
    expect(enterprise.text()).toContain('精细化成本管理')
    expect(enterprise.findAll('li')).toHaveLength(8)
    expect(enterprise.text()).toContain('阶梯折扣')
    expect(enterprise.text()).toContain('IP 白名单与操作审计')
  })

  it('renders the full comparison table (zh)', () => {
    const wrapper = mountWith('zh', zh)
    const rows = wrapper.findAll('.pricing-comparison tbody tr')

    expect(rows).toHaveLength(13)
    const text = wrapper.get('.pricing-comparison').text()
    expect(text).toContain('合同与发票')
    expect(text).toContain('团队成员管理')
    expect(text).toContain('团队及项目级预算')
  })

  it.each([
    ['zh', zh],
    ['en', en],
  ])('leaves no unresolved i18n keys on the page (%s)', (locale, messages) => {
    const wrapper = mountWith(locale, messages)
    expect(wrapper.text()).not.toContain('pricingPage.')
  })
})
