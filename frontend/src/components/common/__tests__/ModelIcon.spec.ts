import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ModelIcon from '@/components/common/ModelIcon.vue'

describe('ModelIcon developer logos', () => {
  it('uses the developer logo before model-name matching', () => {
    const wrapper = mount(ModelIcon, {
      props: {
        model: 'claude-compatible-alias',
        developer: 'OpenAI',
        size: '24px',
      },
    })

    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.find('path').attributes('d')).toContain('M21.55 10.004')
  })

  it('renders the Kling logo for Kuaishou models', () => {
    const wrapper = mount(ModelIcon, {
      props: {
        model: 'custom-video-alias',
        developer: 'Kuaishou',
      },
    })

    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.find('.model-icon-fallback').exists()).toBe(false)
  })

  it('uses a developer initial when no logo is available', () => {
    const wrapper = mount(ModelIcon, {
      props: {
        model: 'custom-video-alias',
        developer: 'HappyHorse',
      },
    })

    expect(wrapper.find('.model-icon-fallback').text()).toBe('H')
  })
})
