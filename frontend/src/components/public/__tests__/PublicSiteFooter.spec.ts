import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import zh from '@/i18n/locales/zh'
import PublicSiteFooter from '@/components/public/PublicSiteFooter.vue'

const appStore = vi.hoisted(() => ({
  cachedPublicSettings: null as Record<string, unknown> | null,
  siteName: 'Sub2API',
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
}))

function mountFooter(theme: 'home' | 'docs' | 'pricing' | 'models' = 'home') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh',
    messages: { zh },
  })

  return mount(PublicSiteFooter, {
    props: {
      description: '高性能 AI 网关',
      theme,
    },
    global: {
      plugins: [i18n],
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="to"><slot /></a>',
        },
      },
    },
  })
}

describe('PublicSiteFooter', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = null
    appStore.siteName = 'Sub2API'
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
  })

  it('renders legal documents and filing links from public settings', () => {
    appStore.cachedPublicSettings = {
      site_name: 'Example API',
      site_icp_filing_number: ' 京ICP备12345678号-1 ',
      site_public_security_filing_number: '京公网安备11000002000001号',
      login_agreement_documents: [
        { id: 'terms', title: 'Terms' },
        { id: 'service-rules', title: '服务规则' },
      ],
    }

    const wrapper = mountFooter('docs')

    expect(wrapper.classes()).toContain('public-site-footer--docs')
    expect(wrapper.text()).toContain('Example API - 高性能 AI 网关')
    expect(wrapper.text()).toContain('服务条款')
    expect(wrapper.text()).toContain('服务规则')
    expect(wrapper.text()).toContain('京ICP备12345678号-1')
    expect(wrapper.text()).toContain('京公网安备11000002000001号')
    expect(wrapper.get('a[data-to="/legal/terms"]').exists()).toBe(true)
    expect(wrapper.get('a[href="https://beian.miit.gov.cn/"]').attributes('rel')).toBe('noopener noreferrer')
    expect(wrapper.get('a[href="https://beian.mps.gov.cn/#/query/webSearch"]').attributes('target')).toBe('_blank')
  })

  it('hides the link navigation for an empty public configuration', () => {
    const wrapper = mountFooter()

    expect(wrapper.find('nav').exists()).toBe(false)
  })

  it('loads public settings when the shared cache is pending', () => {
    appStore.publicSettingsLoaded = false

    mountFooter('models')

    expect(appStore.fetchPublicSettings).toHaveBeenCalledOnce()
  })
})
