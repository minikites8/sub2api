// 延迟分档：<5s 绿、5~10s 黄、>10s 红。请求列表和延迟热力图必须用同一把尺子，
// 否则同一条请求在两处会显示成不同的严重程度。边界值（正好 5s / 10s）单独覆盖。
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const usageViewSource = readFileSync(resolve(dir, '../../../../views/user/UsageView.vue'), 'utf8')
const heatmapSource = readFileSync(resolve(dir, '../UsageLatencyHeatmap.vue'), 'utf8')

// 复刻 UsageView 的 latencyTone，用来断言边界行为。
const LATENCY_SLOW_MS = 5000
const LATENCY_CRITICAL_MS = 10000
function tone(ms: number): 'fast' | 'slow' | 'critical' {
  if (ms < LATENCY_SLOW_MS) return 'fast'
  if (ms <= LATENCY_CRITICAL_MS) return 'slow'
  return 'critical'
}

describe('latency bands', () => {
  it('classifies durations into green / amber / red', () => {
    expect(tone(0)).toBe('fast')
    expect(tone(4999)).toBe('fast')
    expect(tone(5000)).toBe('slow')
    expect(tone(7500)).toBe('slow')
    expect(tone(10000)).toBe('slow')
    expect(tone(10001)).toBe('critical')
  })

  it('keeps UsageView driven by the 5s and 10s thresholds', () => {
    expect(usageViewSource).toContain('const LATENCY_SLOW_MS = 5000')
    expect(usageViewSource).toContain('const LATENCY_CRITICAL_MS = 10000')
    // 平均延迟 KPI 也要用同一阈值，否则它会在行还是绿色时就报警。
    expect(usageViewSource).toContain("'usage-kpi-card--warning': averageLatency >= LATENCY_SLOW_MS")
    // 快档必须有独立配色，否则 <5s 只是默认色而非绿色。
    expect(usageViewSource).toContain('usage-latency--fast')
  })

  it('keeps the heatmap on the same 5s and 10s boundaries', () => {
    expect(heatmapSource).toContain('value < 5000')
    expect(heatmapSource).toContain('value <= 10000')
    // 图例文案必须和分档一致，否则读者会按错误的刻度理解颜色。
    expect(heatmapSource).toContain("label: '5–10s'")
    expect(heatmapSource).toContain("label: '> 10s'")
  })
})
