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
  it('renders video prices and toggles channel provider labels', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh',
      messages: { zh },
    })
    const wrapper = mount(ChannelStatusView, {
      props: { previewSnapshot: createModelMarketplacePreviewSnapshot() },
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

    const seedanceRow = wrapper
      .findAll('tr')
      .find((row) => row.text().includes('doubao-seedance-2-0-260128'))!
    expect(seedanceRow.text()).toContain('ByteDance')
    expect(seedanceRow.text()).not.toContain('Baidu VOD')

    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(seedanceRow.text()).toContain('Baidu VOD')
  })
})
