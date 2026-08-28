import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it, vi } from 'vitest'
import zh from '@/i18n/locales/zh'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import ChannelStatusView from '@/views/user/ChannelStatusView.vue'

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

describe('ChannelStatusView model marketplace', () => {
  it('renders pricing, model developers, and monitoring status from the public snapshot', () => {
    const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })
    const wrapper = mount(ChannelStatusView, {
      props: { previewSnapshot: createModelMarketplacePreviewSnapshot() },
      global: {
        plugins: [i18n],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PublicSiteFooter: true,
          Icon: true,
          ModelIcon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('gpt-4.1')
    expect(text).toContain('OpenAI')
    expect(text).toContain('$2')
    expect(text).toMatch(/不可用|modelMarketplace\.status\.unavailable/)
    expect(wrapper.findAll('button').map((button) => button.text())).toEqual(expect.arrayContaining(['90min', '12h', '1d', '15d']))
  })

  it('renders no-request passive windows as grey unknown cells', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.windows = {
      '90m': {
        status: 'unmonitored',
        availability: 0,
        coverage_complete: true,
        buckets: [],
      },
    }
    const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })
    const wrapper = mount(ChannelStatusView, {
      props: { previewSnapshot: snapshot },
      global: {
        plugins: [i18n],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PublicSiteFooter: true,
          Icon: true,
          ModelIcon: true,
        },
      },
    })

    expect(wrapper.findAll('.health-unknown').length).toBeGreaterThanOrEqual(24)
    expect(wrapper.text()).not.toContain('0.00%')
  })

  it('uses the V2 green-to-red score bands for timeline success rates', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.buckets[0].success_rate = 50
    const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })
    const wrapper = mount(ChannelStatusView, {
      props: { previewSnapshot: snapshot },
      global: {
        plugins: [i18n],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PublicSiteFooter: true,
          Icon: true,
          ModelIcon: true,
        },
      },
    })

    expect(wrapper.findAll('.health-score5').length).toBeGreaterThan(0)
    expect(wrapper.findAll('.health-score10').length).toBeGreaterThan(0)
  })
})
