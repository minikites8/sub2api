import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import UsageLatencyHeatmap from '../UsageLatencyHeatmap.vue'
import { latencyPointsFromLogs, parseBucketDate } from '../latencyPoints'

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

const point = (date: string, avg: number, requests = 1) => ({
  date,
  avg_duration_ms: avg,
  requests,
})

describe('UsageLatencyHeatmap', () => {
  it('lays out daily buckets as a 30-day calendar', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        points: [point('2026-07-24', 480)],
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
        points: [point('2026-07-24 08:00', 480)],
        endDate: '2026-07-24',
        periodDays: 7,
      },
    })

    expect(wrapper.text()).toContain('Two-hour averages')
    expect(wrapper.findAll('.usage-heatmap__hour')).toHaveLength(12)
    expect(wrapper.findAll('.usage-heatmap__cell')).toHaveLength(84)
  })

  // 回归：热力图一度直接吃请求列表的当前分页，于是只有最新那一天有颜色。
  // 现在喂的是覆盖整个区间的服务端聚合，早于最后一天的桶必须照样着色。
  it('colours buckets across the whole range, not only the newest day', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        points: [
          point('2026-07-19 02:00', 2000),
          point('2026-07-22 14:00', 60_000),
          point('2026-07-24 08:00', 480),
        ],
        endDate: '2026-07-24',
        periodDays: 7,
      },
    })

    const painted = wrapper.findAll('.usage-heatmap__cell').filter((cell) => (
      !cell.classes('usage-heatmap__cell--empty')
    ))
    expect(painted).toHaveLength(3)
    expect(wrapper.html()).toContain('2026-07-19 02:00 · 2.0s')
    expect(wrapper.html()).toContain('2026-07-22 14:00 · 60.0s')
  })

  it('merges odd hours into the two-hour bucket weighted by request count', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        // 08:00 有 3 次 1s，09:00 有 1 次 5s → 加权平均 2s，而非算术平均 3s。
        points: [point('2026-07-24 08:00', 1000, 3), point('2026-07-24 09:00', 5000, 1)],
        endDate: '2026-07-24',
        periodDays: 7,
      },
    })

    expect(wrapper.html()).toContain('2026-07-24 08:00 · 2.0s')
  })

  // 后端在整桶都没有耗时样本时返回 0，那是「没有数据」而不是「0 毫秒」，
  // 否则空闲时段会被涂成最快的绿色。
  it('treats a zero average as no data', () => {
    const wrapper = mount(UsageLatencyHeatmap, {
      props: {
        points: [point('2026-07-24', 0, 12)],
        endDate: '2026-07-24',
        periodDays: 30,
      },
    })

    expect(wrapper.find('[data-date="2026-07-24"]').classes()).toContain('usage-heatmap__cell--empty')
    expect(wrapper.find('[data-date="2026-07-24"]').text()).toContain('--')
  })
})

describe('latency point helpers', () => {
  it('reads the server bucket key literally, without a timezone shift', () => {
    expect(parseBucketDate('2026-07-24 08:00')).toEqual({ day: '2026-07-24', hour: 8 })
    expect(parseBucketDate('2026-07-24')).toEqual({ day: '2026-07-24', hour: null })
    expect(parseBucketDate('2026-27')).toBeNull()
  })

  it('aggregates preview logs into the same shape', () => {
    const logs = [
      { created_at: '2026-07-24T08:30:00', duration_ms: 1000 },
      { created_at: '2026-07-24T08:45:00', duration_ms: 3000 },
      { created_at: '2026-07-24T09:10:00', duration_ms: 5000 },
    ] as any

    expect(latencyPointsFromLogs(logs, true)).toEqual([
      { date: '2026-07-24 08:00', avg_duration_ms: 2000, requests: 2 },
      { date: '2026-07-24 09:00', avg_duration_ms: 5000, requests: 1 },
    ])
    expect(latencyPointsFromLogs(logs, false)).toEqual([
      { date: '2026-07-24', avg_duration_ms: 3000, requests: 3 },
    ])
  })
})
