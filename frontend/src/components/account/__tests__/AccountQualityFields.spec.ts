import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountQualityFields from '../AccountQualityFields.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('AccountQualityFields visibility', () => {
  it('shows the shared pool and upstream source by default for Strong accounts', () => {
    const wrapper = mount(AccountQualityFields, { props: { tier: 'strong', pool: '', source: 'unknown' } })
    expect(wrapper.get('input[maxlength="64"]').exists()).toBe(true)
    expect(wrapper.findAll('select')).toHaveLength(2)
    expect(wrapper.text()).toContain('gptQos.sharedPool')
    expect(wrapper.text()).toContain('gptQos.upstreamKind')
  })

  it('only hides pool and source when explicitly disabled by the caller', () => {
    const wrapper = mount(AccountQualityFields, { props: { tier: 'strong', pool: '', showPool: false, showSource: false } })
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.findAll('select')).toHaveLength(1)
  })

  it('emits the pool and upstream source changes for the account form to save', async () => {
    const wrapper = mount(AccountQualityFields, { props: { tier: 'strong', pool: '', source: 'unknown' } })
    await wrapper.get('input[maxlength="64"]').setValue('vm2api-shared')
    await wrapper.findAll('select')[1]!.setValue('vm2api')
    expect(wrapper.emitted('update:pool')).toEqual([['vm2api-shared']])
    expect(wrapper.emitted('update:source')).toEqual([['vm2api']])
  })
})
