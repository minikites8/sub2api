import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/api/admin/channels', () => ({
  default: {
    getModelDefaultPricing: vi.fn(),
  },
}))

import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'

function baseEntry(): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    priority_multiplier: null,
    intervals: [
      {
        min_tokens: 0,
        max_tokens: null,
        tier_label: '',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: null,
        sort_order: 0,
      },
    ],
  }
}

describe('PricingEntryCard 布局', () => {
  it('展开区域为输入框和下拉框的 focus ring 保留安全区', () => {
    const wrapper = mount(PricingEntryCard, {
      props: {
        entry: baseEntry(),
        platform: 'openai',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const inner = wrapper.get('.collapsible-inner')
    expect(inner.classes()).toContain('px-0.5')
    expect(inner.classes()).toContain('pb-0.5')
  })

  it('视频模式使用 Credits 每秒价格并依次添加 720P 档位', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: {
        entry: { ...baseEntry(), billing_mode: 'video', intervals: [] },
        platform: 'baidu_vod',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('Credits / s')
    const addTier = wrapper.findAll('button').find(button =>
      button.text().includes('admin.channels.form.addTier'),
    )
    expect(addTier).toBeDefined()
    await addTier!.trigger('click')

    const updates = wrapper.emitted('update')
    expect(updates).toHaveLength(1)
    expect((updates![0][0] as PricingFormEntry).intervals[0].tier_label).toBe('720P')
  })

  it('视频 Token 模式按分辨率和输入类型写入价格档位', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: {
        entry: { ...baseEntry(), billing_mode: 'video_token', intervals: [] },
        platform: 'baidu_vod',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('Credits / MTok')
    expect(wrapper.text()).toContain('admin.channels.form.inputWithoutVideo')
    const priceInputs = wrapper.findAll('input[type="number"]')
    expect(priceInputs).toHaveLength(10)
    await priceInputs[0].setValue('37')

    const updates = wrapper.emitted('update')
    expect(updates).toHaveLength(1)
    const entry = updates![0][0] as PricingFormEntry
    expect(entry.intervals[0].tier_label).toBe('default:text')
    expect(entry.intervals[0].output_price).toBe('37')
  })
})
