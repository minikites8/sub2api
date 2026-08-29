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

  it('uses the complete 18-bucket V2 timeline for the 90-minute window', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    for (const monitor of snapshot.monitoring) {
      monitor.buckets = monitor.buckets.slice(-18)
      monitor.windows = {
        '90m': {
          status: monitor.status,
          availability: monitor.availability_7d,
          coverage_complete: true,
          metrics: {
            has_requests: true,
            success_rate: monitor.availability_7d / 100,
            error_rate: 1 - monitor.availability_7d / 100,
            cache_rate: 0,
            ttft: {},
            duration: {},
          },
          health: {
            overall: 'healthy',
            error_rate: 'healthy',
            ttft: 'healthy',
            cache: 'unknown',
            score: 100,
            minimum_sample: 50,
          },
          buckets: monitor.buckets,
        },
      }
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

    const firstTrack = wrapper.find('.market-row .monitor-track')
    expect(firstTrack.findAll(':scope > span')).toHaveLength(18)
    expect(firstTrack.findAll('.health-unknown')).toHaveLength(0)
  })

  it('renders no-request passive windows as grey unknown cells', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.windows = {
      '90m': {
        status: 'operational',
        availability: 0,
        coverage_complete: true,
        metrics: {
          has_requests: false,
          success_rate: 0,
          error_rate: 0,
          cache_rate: 0,
          ttft: {},
          duration: {},
        },
        health: {
          overall: 'unknown',
          error_rate: 'unknown',
          ttft: 'unknown',
          cache: 'unknown',
          minimum_sample: 50,
        },
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

  it('uses the V2 green-to-red score bands for timeline health scores', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.buckets[0].success_rate = 50
    monitor.buckets[0].metrics = {
      has_requests: true,
      success_rate: 0.5,
      error_rate: 0.5,
      cache_rate: 0,
      ttft: { p50_ms: 200 },
      duration: { p50_ms: 800 },
    }
    monitor.buckets[0].health = {
      overall: 'warning',
      error_rate: 'warning',
      ttft: 'healthy',
      cache: 'unknown',
      score: 50,
      success_rate_score: 50,
      minimum_sample: 10,
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

    expect(wrapper.findAll('.health-score5').length).toBeGreaterThan(0)
    expect(wrapper.findAll('.health-score10').length).toBeGreaterThan(0)
  })

  it('shows V2 success rate when the legacy availability field is zero', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.windows = {
      '90m': {
        status: 'operational',
        availability: 0,
        coverage_complete: true,
        metrics: {
          has_requests: true,
          success_rate: 0.95,
          error_rate: 0.05,
          cache_rate: 0,
          ttft: { p50_ms: 200 },
          duration: { p50_ms: 800, avg_ms: 900 },
        },
        health: {
          overall: 'healthy',
          error_rate: 'healthy',
          ttft: 'healthy',
          cache: 'unknown',
          score: 90,
          success_rate_score: 95,
          minimum_sample: 50,
        },
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

    expect(wrapper.text()).toContain('95.00%')
    expect(wrapper.findAll('.health-score9').length).toBeGreaterThan(0)
  })
})
