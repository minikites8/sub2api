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
  })
})
