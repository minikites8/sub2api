import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OrderedGroupSelect from '../OrderedGroupSelect.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('OrderedGroupSelect', () => {
  it('adds groups in order, prevents duplicates, reorders and removes', async () => {
    const wrapper = mount(OrderedGroupSelect, {
      props: {
        modelValue: [2],
        options: [{ value: 1, label: 'First' }, { value: 2, label: 'Second' }],
        'onUpdate:modelValue': (value: number[]) => wrapper.setProps({ modelValue: value })
      },
      global: { stubs: { Select: { name: 'Select', props: ['options'], emits: ['update:modelValue'], template: '<div />' } } }
    })
    const select = wrapper.findComponent({ name: 'Select' })
    expect(select.props('options')).toEqual([{ value: 1, label: 'First' }])
    await select.vm.$emit('update:modelValue', 1)
    expect(wrapper.props('modelValue')).toEqual([2, 1])
    await select.vm.$emit('update:modelValue', 1)
    expect(wrapper.props('modelValue')).toEqual([2, 1])
    await wrapper.findAll('[aria-label="keys.moveGroupUp"]')[1].trigger('click')
    expect(wrapper.props('modelValue')).toEqual([1, 2])
    await wrapper.findAll('[aria-label="keys.removeGroup"]')[0].trigger('click')
    expect(wrapper.props('modelValue')).toEqual([2])
  })
})
