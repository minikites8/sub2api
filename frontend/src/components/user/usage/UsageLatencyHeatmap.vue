<template>
  <section class="usage-panel usage-heatmap">
    <div class="usage-panel__header usage-heatmap__header">
      <div>
        <p class="usage-panel__eyebrow">RESPONSE LATENCY</p>
        <h2>{{ t('usage.analytics.latencyHeatmap') }}</h2>
        <p>{{ t(heatmapDescriptionKey, { days: periodDays }) }}</p>
      </div>
      <div class="usage-heatmap__legend" aria-hidden="true">
        <span v-for="item in legend" :key="item.label"><i :class="item.className" />{{ item.label }}</span>
      </div>
    </div>

    <div class="usage-heatmap__scroll">
      <div v-if="isDailyView" class="usage-heatmap__calendar" data-testid="latency-calendar">
        <div v-for="weekday in weekdayLabels" :key="weekday" class="usage-heatmap__weekday">
          {{ weekday }}
        </div>
        <div
          v-for="index in calendarLeadingDays"
          :key="`calendar-leading-${index}`"
          class="usage-heatmap__calendar-blank"
          aria-hidden="true"
        />
        <div
          v-for="day in days"
          :key="day.key"
          class="usage-heatmap__cell usage-heatmap__calendar-day"
          :class="latencyClass(day.key, null)"
          :title="cellTitle(day.key, null)"
          :data-date="day.key"
        >
          <span>{{ day.label }}</span>
          <strong>{{ dailyLatencyLabel(day.key) }}</strong>
        </div>
      </div>

      <div v-else class="usage-heatmap__grid" :style="{ '--heatmap-columns': String(timeBuckets.length) }">
        <div class="usage-heatmap__corner">DATE</div>
        <div v-for="bucket in timeBuckets" :key="`header-${bucket.key}`" class="usage-heatmap__hour">
          {{ bucket.label }}
        </div>
        <template v-for="day in days" :key="day.key">
          <div class="usage-heatmap__date">{{ day.label }}</div>
          <div
            v-for="bucket in timeBuckets"
            :key="`${day.key}-${bucket.key}`"
            class="usage-heatmap__cell"
            :class="latencyClass(day.key, bucket.hour)"
            :title="cellTitle(day.key, bucket.hour)"
          />
        </template>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { type LatencyPoint, parseBucketDate } from './latencyPoints'

// points 必须覆盖整个查询区间。这里曾经直接吃请求列表的 items，而那只是当前
// 分页的一页——于是除了最新那一天/两小时以外的格子全是空的。
const props = defineProps<{
  points: LatencyPoint[]
  endDate: string
  periodDays: number
}>()

const { t } = useI18n()
const hours = Array.from({ length: 12 }, (_, index) => index * 2)
const isDailyView = computed(() => props.periodDays >= 30)
const heatmapDescriptionKey = computed(() => isDailyView.value
  ? 'usage.analytics.latencyHeatmapDailyDescription'
  : 'usage.analytics.latencyHeatmapDescription')
const timeBuckets = computed(() => hours.map((hour) => ({ key: String(hour), label: `${hour}:00`, hour })))
const weekdayLabels = computed(() => {
  const monday = new Date(2024, 0, 1, 12)
  return Array.from({ length: 7 }, (_, index) => {
    const date = new Date(monday)
    date.setDate(monday.getDate() + index)
    return new Intl.DateTimeFormat(undefined, { weekday: 'short' }).format(date)
  })
})

const localDateKey = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const days = computed(() => {
  const end = new Date(`${props.endDate}T12:00:00`)
  const result: Array<{ key: string; label: string }> = []
  const cursor = new Date(end.getTime() - Math.max(0, props.periodDays - 1) * 86_400_000)
  while (cursor <= end && result.length < props.periodDays) {
    const key = localDateKey(cursor)
    result.push({
      key,
      label: new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric' }).format(cursor),
    })
    cursor.setDate(cursor.getDate() + 1)
  }
  return result
})

const calendarLeadingDays = computed(() => {
  const firstDay = days.value[0]
  if (!firstDay) return 0
  const weekday = new Date(`${firstDay.key}T12:00:00`).getDay()
  return (weekday + 6) % 7
})

const buckets = computed(() => {
  const result = new Map<string, { total: number; count: number }>()
  props.points.forEach((point) => {
    const parsed = parseBucketDate(point.date)
    if (!parsed) return
    // 后端在整桶都没有耗时样本时返回 0，这是"没有数据"而不是"0 毫秒"。
    const average = Number(point.avg_duration_ms) || 0
    if (average <= 0) return
    let key: string
    if (isDailyView.value) {
      key = parsed.day
    } else {
      // 按小时的点合并成 2 小时一格；只有按天的点时这一格无从落位，跳过。
      if (parsed.hour == null) return
      key = `${parsed.day}-${Math.floor(parsed.hour / 2) * 2}`
    }
    // 按请求数加权，否则一个只有 1 次请求的小时会和 500 次请求的小时等权。
    const weight = Math.max(1, Number(point.requests) || 0)
    const bucket = result.get(key) || { total: 0, count: 0 }
    bucket.total += average * weight
    bucket.count += weight
    result.set(key, bucket)
  })
  return result
})

const averageFor = (day: string, hour: number | null) => {
  const key = hour == null ? day : `${day}-${hour}`
  const bucket = buckets.value.get(key)
  return bucket ? bucket.total / bucket.count : null
}

// 这里统计的是总时间（duration_ms），所以用总时间那把尺子：
// <45s 正常、45~115s 偏慢、>115s 过慢，与请求列表的总时间列一致。
// 正常区间再拆出一档 <15s，否则健康请求会全部落进同一格。
const latencyClass = (day: string, hour: number | null) => {
  const value = averageFor(day, hour)
  if (value == null) return 'usage-heatmap__cell--empty'
  if (value < 15000) return 'usage-heatmap__cell--fast'
  if (value < 45000) return 'usage-heatmap__cell--normal'
  if (value <= 115000) return 'usage-heatmap__cell--slow'
  return 'usage-heatmap__cell--critical'
}

// 秒级延迟写成 "47.8s" 比 "47823ms" 好读，与请求列表的写法一致。
const formatLatency = (value: number) => value >= 1000
  ? `${(value / 1000).toFixed(1)}s`
  : `${Math.round(value)}ms`

const cellTitle = (day: string, hour: number | null) => {
  const value = averageFor(day, hour)
  const timeLabel = hour == null ? t('usage.analytics.dailyAverage') : `${String(hour).padStart(2, '0')}:00`
  return value == null
    ? `${day} ${timeLabel} · ${t('usage.analytics.noRequests')}`
    : `${day} ${timeLabel} · ${formatLatency(value)}`
}

const dailyLatencyLabel = (day: string) => {
  const value = averageFor(day, null)
  return value == null ? '--' : formatLatency(value)
}

const legend = computed(() => [
  { label: '< 15s', className: 'usage-heatmap__key usage-heatmap__key--fast' },
  { label: '15–45s', className: 'usage-heatmap__key usage-heatmap__key--normal' },
  { label: '45–115s', className: 'usage-heatmap__key usage-heatmap__key--slow' },
  { label: '> 115s', className: 'usage-heatmap__key usage-heatmap__key--critical' },
])
</script>

<style scoped>
.usage-heatmap__header { align-items: flex-end; }
.usage-heatmap__header p:last-child { margin-top: 5px; color: #7d8f89; font-size: 12px; }
.usage-heatmap__legend { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 12px; color: #91a39d; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-heatmap__legend span { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
.usage-heatmap__key { width: 9px; height: 9px; border: 1px solid #263934; background: #00f5a8; }
.usage-heatmap__key--normal { background: #0e4e3d; }
.usage-heatmap__key--slow { background: #ffbd59; }
.usage-heatmap__key--critical { background: #ff9f97; }
.usage-heatmap__scroll { overflow-x: auto; padding: 5px 18px 22px; scrollbar-width: thin; scrollbar-color: #30443f transparent; }
.usage-heatmap__grid { display: grid; grid-template-columns: 54px repeat(var(--heatmap-columns), minmax(45px, 1fr)); min-width: 720px; gap: 5px; }
.usage-heatmap__corner, .usage-heatmap__hour, .usage-heatmap__date { display: flex; align-items: center; color: #879991; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.usage-heatmap__corner, .usage-heatmap__hour { height: 24px; }
.usage-heatmap__hour { justify-content: center; }
.usage-heatmap__date { min-height: 25px; }
.usage-heatmap__cell { min-height: 25px; border: 1px solid #20322e; background: #0a1316; transition: transform .12s ease, border-color .12s ease; }
.usage-heatmap__cell:hover { z-index: 1; transform: scale(1.08); border-color: #9cb0a9; }
.usage-heatmap__cell--fast { background: #00f5a8; }
.usage-heatmap__cell--normal { background: #0e4e3d; }
.usage-heatmap__cell--slow { background: #ffbd59; }
.usage-heatmap__cell--critical { border-color: #ffb8b2; background: #ff9f97; }
.usage-heatmap__calendar { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); min-width: 630px; gap: 7px; }
.usage-heatmap__weekday { padding: 4px 8px 7px; color: #879991; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; text-align: center; text-transform: uppercase; }
.usage-heatmap__calendar-blank { min-height: 64px; border: 1px dashed rgba(48, 68, 63, .28); background: rgba(5, 11, 14, .25); }
.usage-heatmap__calendar-day { display: flex; min-height: 64px; flex-direction: column; justify-content: space-between; padding: 8px 10px; }
.usage-heatmap__calendar-day span { color: #b8c7c2; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-heatmap__calendar-day strong { align-self: flex-end; color: #eff7f4; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-heatmap__calendar-day.usage-heatmap__cell--critical span,
.usage-heatmap__calendar-day.usage-heatmap__cell--critical strong { color: #2c1717; }
@media (max-width: 720px) {
  .usage-heatmap__header { align-items: flex-start; }
  .usage-heatmap__legend { justify-content: flex-start; }
}
@media (max-width: 640px) {
  .usage-heatmap { display: none; }
}
</style>
