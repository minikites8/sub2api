import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it, vi } from 'vitest'
import zh from '@/i18n/locales/zh'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import ChannelStatusView from '@/views/user/ChannelStatusView.vue'

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

describe('ChannelStatusView video pricing', () => {
  it('renders provider labels according to channel configuration', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh',
      messages: { zh },
    })
    const snapshot = createModelMarketplacePreviewSnapshot()
    const videoGroup = snapshot.groups.find((group) => group.name === '百度 VOD 视频')!
    videoGroup.provider_visible = true
    const happyHorse = videoGroup.models.find((model) => model.standard_model === 'happyhorse-1.1-t2v')!
    const happyHorseAliases = [
      'happyhorse-1.1-t2v',
      'happyhorse-1.1-i2v',
      'happyhorse-1.1-r2v',
      'happyhorse-1.1-video-edit',
    ]
    Object.assign(happyHorse, { pricing_models: happyHorseAliases })
    for (const alias of happyHorseAliases.slice(1)) {
      videoGroup.models.push({
        ...happyHorse,
        standard_model: alias,
        raw_model: alias,
        pricing_models: happyHorseAliases,
      })
    }

    const wrapper = mount(ChannelStatusView, {
      props: { previewSnapshot: snapshot },
      global: {
        plugins: [i18n],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PublicSiteFooter: true,
          Icon: true,
          PlatformIcon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('doubao-seedance-2-0-260128')
    expect(text).toContain('happyhorse-1.1-t2v')
    expect(text).toContain('4K')
    expect(text).toContain('252.5')
    expect(text).toContain('Credits/s')
    expect(text).toContain('Credits/1M')

    const happyHorseRows = wrapper
      .findAll('tr')
      .filter((row) => happyHorseAliases.some((alias) => row.text().includes(alias)))
    expect(happyHorseRows).toHaveLength(1)

    const seedanceRow = wrapper
      .findAll('tr')
      .find((row) => row.text().includes('doubao-seedance-2-0-260128'))!
    expect(seedanceRow.text()).toContain('ByteDance')
    expect(seedanceRow.text()).toContain('Baidu VOD')

    const gptRow = wrapper
      .findAll('tr')
      .find((row) => row.text().includes('gpt-4.1'))!
    expect(gptRow.find('.border-l').exists()).toBe(false)
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
  })
})
