// 首字时间和总时间用两把不同的尺子：
//   首字：<5s 绿 / 5~10s 黄 / >10s 红
//   总时间：<45s 绿 / 45~115s 黄 / >115s 红
// 平均延迟 KPI 和延迟热力图统计的都是总时间，必须跟总时间那把尺子，
// 否则同一条请求在页面不同位置会显示成不同的严重程度。边界值单独覆盖。
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const usageViewSource = readFileSync(resolve(dir, '../../../../views/user/UsageView.vue'), 'utf8')
const heatmapSource = readFileSync(resolve(dir, '../UsageLatencyHeatmap.vue'), 'utf8')

function tone(ms: number, slowMs: number, criticalMs: number): 'fast' | 'slow' | 'critical' {
  if (ms < slowMs) return 'fast'
  if (ms <= criticalMs) return 'slow'
  return 'critical'
}

const firstToken = (ms: number) => tone(ms, 5000, 10000)
const totalLatency = (ms: number) => tone(ms, 45000, 115000)

describe('latency bands', () => {
  it('bands first-token latency at 5s and 10s', () => {
    expect(firstToken(0)).toBe('fast')
    expect(firstToken(4999)).toBe('fast')
    expect(firstToken(5000)).toBe('slow')
    expect(firstToken(10000)).toBe('slow')
    expect(firstToken(10001)).toBe('critical')
  })

  it('bands total latency at 45s and 115s', () => {
    expect(totalLatency(0)).toBe('fast')
    expect(totalLatency(44999)).toBe('fast')
    expect(totalLatency(45000)).toBe('slow')
    expect(totalLatency(115000)).toBe('slow')
    expect(totalLatency(115001)).toBe('critical')
  })

  // 两把尺子不能混用：10s 对首字已是红色，对总时间仍属正常。
  it('keeps the two scales independent', () => {
    expect(firstToken(10001)).toBe('critical')
    expect(totalLatency(10001)).toBe('fast')
  })

  it('drives UsageView from both threshold pairs', () => {
    expect(usageViewSource).toContain('const FIRST_TOKEN_SLOW_MS = 5000')
    expect(usageViewSource).toContain('const FIRST_TOKEN_CRITICAL_MS = 10000')
    expect(usageViewSource).toContain('const TOTAL_LATENCY_SLOW_MS = 45000')
    expect(usageViewSource).toContain('const TOTAL_LATENCY_CRITICAL_MS = 115000')
    // 首字那一列必须真的着色，否则阈值定了也看不出来。
    expect(usageViewSource).toContain('firstTokenTone(log.first_token_ms)')
    expect(usageViewSource).toContain('latencyTone(log.duration_ms)')
    // 平均延迟 KPI 统计的是 average_duration_ms，要用总时间阈值。
    expect(usageViewSource).toContain("'usage-kpi-card--warning': averageLatency >= TOTAL_LATENCY_SLOW_MS")
    expect(usageViewSource).toContain('usage-latency--fast')
  })

  it('keeps the heatmap on the total-latency boundaries', () => {
    // 热力图聚合的是 duration_ms，所以边界必须是 45s / 115s。
    expect(heatmapSource).toContain('value < 45000')
    expect(heatmapSource).toContain('value <= 115000')
    expect(heatmapSource).toContain("label: '45–115s'")
    expect(heatmapSource).toContain("label: '> 115s'")
  })
})
