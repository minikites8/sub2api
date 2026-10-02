import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { locale: { value: 'en' }, setLocaleMessage: vi.fn() } }),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('@/api/publicTransit', () => ({ getPublicTransitSnapshot: vi.fn(async () => null) }))

import ChannelStatusView from '../ChannelStatusView.vue'

const mountView = () => mount(ChannelStatusView, {
  global: {
    stubs: {
      AppLayout: defineComponent({
        name: 'AppLayoutStub',
        setup: (_, { slots }) => () => h('div', slots.default?.())
      }),
      PublicSiteFooter: true,
      Icon: true,
      ModelIcon: true
    }
  }
})

describe('ChannelStatusView marketplace shell', () => {
  it('renders the public channel marketplace page', () => {
    const wrapper = mountView()
    expect(wrapper.find('.model-marketplace').exists()).toBe(true)
    expect(wrapper.find('h1').text()).toBe('modelMarketplace.title')
  })
})