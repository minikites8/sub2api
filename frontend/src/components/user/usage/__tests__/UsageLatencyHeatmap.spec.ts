import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import UsageLatencyHeatmap from '../UsageLatencyHeatmap.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'usage.analytics.latencyHeatmap': 'Latency heatmap',
      'usage.analytics.latencyHeatmapDescription': 'Two-hour averages',
      'usage.analytics.latencyHeatmapDailyDescription': 'Daily averages',
      'usage.analytics.dailyAverage': 'Daily avg',
      'usage.analytics.noRequests': 'No requests',
    })[key] ?? key,
  }),
}))

const log = {
  id: 1,
  created_at: '2026-07-24T08:30:00',
  duration_ms: 480,
}

describe('UsageLatencyHeatmap', () => {
  it('lays out daily buckets as a 30-day calendar', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        logs: [log] as any,
        endDate: '2026-07-24',
        periodDays: 30,
      },
    })

    expect(wrapper.text()).toContain('Daily averages')
    expect(wrapper.find('[data-testid="latency-calendar"]').exists()).toBe(true)
    expect(wrapper.findAll('.usage-heatmap__weekday')).toHaveLength(7)
    expect(wrapper.findAll('.usage-heatmap__calendar-blank')).toHaveLength(3)
    expect(wrapper.findAll('.usage-heatmap__calendar-day')).toHaveLength(30)
    expect(wrapper.findAll('.usage-heatmap__cell')).toHaveLength(30)
    expect(wrapper.find('[data-date="2026-07-24"]').text()).toContain('480ms')
    expect(wrapper.find('[data-date="2026-07-24"]').attributes('title')).toContain('Daily avg · 480ms')
  })

  it('keeps two-hour buckets in the 7-day view', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        logs: [log] as any,
        endDate: '2026-07-24',
        periodDays: 7,
      },
    })

    expect(wrapper.text()).toContain('Two-hour averages')
    expect(wrapper.findAll('.usage-heatmap__hour')).toHaveLength(12)
    expect(wrapper.findAll('.usage-heatmap__cell')).toHaveLength(84)
  })
})
