import type { UsageLog } from '@/types'

// 延迟热力图的数据点。后端趋势接口按 granularity 聚合后直接就是这个形状，
// date 形如 "2026-07-24"（按天）或 "2026-07-24 08:00"（按小时）。
export interface LatencyPoint {
  date: string
  avg_duration_ms: number
  requests: number
}

// 桶的时间键由后端 TO_CHAR 生成，直接按字面量拆，不要过 new Date()——
// 那会把服务端已经分好的桶再按浏览器时区平移一次。
export function parseBucketDate(value: string): { day: string; hour: number | null } | null {
  const match = /^(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}))?/.exec(String(value || ''))
  if (!match) return null
  return { day: match[1], hour: match[2] == null ? null : Number(match[2]) }
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

// 预览模式没有后端聚合，但它手里的日志本来就是完整集合（不是分页后的一页），
// 在本地聚合出同样形状的点即可，热力图不必为此保留两套取数路径。
export function latencyPointsFromLogs(logs: UsageLog[], hourly: boolean): LatencyPoint[] {
  const buckets = new Map<string, { total: number; count: number }>()
  logs.forEach((log) => {
    const duration = Number(log.duration_ms || 0)
    if (duration <= 0) return
    const date = new Date(log.created_at)
    if (Number.isNaN(date.getTime())) return
    const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
    const key = hourly ? `${day} ${pad(date.getHours())}:00` : day
    const bucket = buckets.get(key) || { total: 0, count: 0 }
    bucket.total += duration
    bucket.count += 1
    buckets.set(key, bucket)
  })
  return Array.from(buckets, ([date, bucket]) => ({
    date,
    avg_duration_ms: bucket.total / bucket.count,
    requests: bucket.count,
  }))
}
