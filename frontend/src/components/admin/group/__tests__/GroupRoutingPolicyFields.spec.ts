import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupRoutingPolicyFields from '../GroupRoutingPolicyFields.vue'
import zh from '@/i18n/locales/zh/admin/overview'
import en from '@/i18n/locales/en/admin/overview'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => zh.groups.routingPolicy[key.split('.').pop() as keyof typeof zh.groups.routingPolicy] ?? key })
}))

describe('GroupRoutingPolicyFields', () => {
  it('renders labels and emits all selectable strategies and the BPS switch', async () => {
    const wrapper = mount(GroupRoutingPolicyFields, {
      props: { idPrefix: 'test', enableBps: false, schedulingStrategy: 'balanced' }
    })
    expect(wrapper.text()).toContain('启用 BPS')
    expect(wrapper.text()).toContain('优先5h')
    expect(wrapper.text()).toContain('优先周限额')
    await wrapper.get('[data-testid="test-enable-bps"]').trigger('click')
    expect(wrapper.emitted('update:enableBps')).toEqual([[true]])
    for (const strategy of ['balanced', 'priority_5h', 'priority_weekly']) {
      await wrapper.get('[data-testid="test-strategy-' + strategy + '"]').trigger('click')
    }
    expect(wrapper.emitted('update:schedulingStrategy')).toEqual([['balanced'], ['priority_5h'], ['priority_weekly']])
    for (const locale of [zh, en]) {
      expect(Object.keys(locale.groups.routingPolicy)).toEqual(Object.keys(zh.groups.routingPolicy))
      expect(Object.values(locale.groups.routingPolicy).every(label => label.length > 0)).toBe(true)
    }
    wrapper.unmount()
  })
})
