<template>
  <BaseDialog
    :show="Boolean(log)"
    :title="t('usage.requestDetails.title')"
    width="normal"
    @close="emit('close')"
  >
    <div v-if="log" class="usage-request-details" data-testid="request-details-dialog">
      <div class="usage-request-details__summary">
        <span>{{ t('usage.requestDetails.request') }}</span>
        <strong>{{ log.model || emptyValue }}</strong>
        <time :datetime="log.created_at">{{ formatDateTime(log.created_at) }}</time>
      </div>

      <section v-for="section in sections" :key="section.title" class="usage-request-details__section">
        <h4><Icon :name="section.icon" size="sm" />{{ section.title }}</h4>
        <dl>
          <div v-for="item in section.items" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd :class="{ 'usage-request-details__mono': item.monospace }">
              <a v-if="item.href" :href="item.href" target="_blank" rel="noopener noreferrer">{{ item.value }}</a>
              <span v-else>{{ item.value }}</span>
            </dd>
          </div>
        </dl>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatReasoningEffort } from '@/utils/format'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import type { UsageLog } from '@/types'

type DetailIcon = 'clipboard' | 'server' | 'globe' | 'sparkles'

interface DetailItem {
  label: string
  value: string
  href?: string
  monospace?: boolean
}

interface DetailSection {
  title: string
  icon: DetailIcon
  items: DetailItem[]
}

const props = defineProps<{
  log: UsageLog | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const emptyValue = '--'

const present = (value: string | number | null | undefined) => {
  if (value == null || String(value).trim() === '') return emptyValue
  return String(value)
}

const formatDateTime = (value: string) => new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
}).format(new Date(value))

const requestTypeLabel = (log: UsageLog) => {
  const type = resolveUsageRequestType(log)
  if (type === 'async') return t('usage.async')
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

const groupLabel = (log: UsageLog) => {
  const group = log.group?.name || t('usage.analytics.unknownGroup')
  const multiplier = Number(log.rate_multiplier || 1)
    .toFixed(2)
    .replace(/\.00$/, '')
    .replace(/(\.\d)0$/, '$1')
  return `${group} (${multiplier}x)`
}

const reasoningEffortLabel = (effort: string | null | undefined) => {
  const formatted = formatReasoningEffort(effort)
  return formatted === '-' ? emptyValue : formatted
}

const imageSizeSourceLabel = (source: UsageLog['image_size_source']) => {
  if (source === 'output') return t('usage.imageSizeSourceOutput')
  if (source === 'input') return t('usage.imageSizeSourceInput')
  if (source === 'default') return t('usage.imageSizeSourceDefault')
  if (source === 'legacy') return t('usage.imageSizeSourceLegacy')
  return emptyValue
}

const safeHref = (value: string | null | undefined): string | undefined => {
  const candidate = value?.trim()
  if (!candidate) return undefined
  if (candidate.startsWith('/')) return candidate
  try {
    const parsed = new URL(candidate)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? candidate : undefined
  } catch {
    return undefined
  }
}

const asyncTaskKindLabel = (kind: string) => {
  if (kind === 'video') return t('usage.requestDetails.taskKindVideo')
  if (kind === 'grok_video') return t('usage.requestDetails.taskKindGrokVideo')
  if (kind === 'batch_image') return t('usage.requestDetails.taskKindBatchImage')
  return kind
}

// 这个弹窗只放辅助元数据——token 和 Credit 明细在表格行上各有自己的悬浮层。
// 所以这里的取舍一律是减法：拿不到值的字段直接不出现，整段都拿不到就整段不出现。
// 之前每个字段无论有没有值都占一个带边框的格子，十几个 "--" 把真正有用的几行淹了。
const sections = computed<DetailSection[]>(() => {
  const log = props.log
  if (!log) return []

  const draft: DetailSection[] = [
    {
      title: t('usage.requestDetails.requestContext'),
      icon: 'clipboard',
      items: [
        { label: t('usage.requestDetails.requestId'), value: present(log.request_id), monospace: true },
        { label: t('usage.apiKeyFilter'), value: present(log.api_key?.name) },
        { label: t('admin.usage.group'), value: groupLabel(log) },
      ],
    },
    {
      title: t('usage.requestDetails.routing'),
      icon: 'server',
      items: [
        { label: t('usage.inboundEndpoint'), value: present(log.inbound_endpoint), monospace: true },
        // 不再单列"传输"：它只会说流式/同步/WebSocket，而"类型"已经涵盖同一件事。
        { label: t('usage.type'), value: requestTypeLabel(log) },
        { label: t('usage.reasoningEffort'), value: reasoningEffortLabel(log.reasoning_effort) },
        { label: t('usage.serviceTier'), value: getUsageServiceTierLabel(log.service_tier, t) },
        { label: t('usage.requestDetails.nodeId'), value: present(log.node_id), monospace: true },
      ],
    },
    {
      title: t('usage.requestDetails.client'),
      icon: 'globe',
      items: [
        { label: t('usage.requestDetails.ipAddress'), value: present(log.ip_address), monospace: true },
        { label: t('usage.userAgent'), value: present(log.user_agent), monospace: true },
      ],
    },
  ]

  if (log.image_count > 0) {
    draft.push({
      title: t('usage.requestDetails.imageMetadata'),
      icon: 'sparkles',
      items: [
        { label: t('usage.requestDetails.mediaType'), value: present(log.media_type) },
        { label: t('usage.imageCount'), value: present(log.image_count) },
        { label: t('usage.imageBillingSize'), value: present(log.image_size) },
        { label: t('usage.imageInputSize'), value: present(log.image_input_size) },
        { label: t('usage.imageOutputSize'), value: present(log.image_output_size) },
        { label: t('usage.imageSizeSource'), value: imageSizeSourceLabel(log.image_size_source) },
      ],
    })
  }

  if (log.video_count > 0) {
    draft.push({
      title: t('usage.requestDetails.videoMetadata'),
      icon: 'sparkles',
      items: [
        { label: t('usage.requestDetails.videoCount'), value: present(log.video_count) },
        { label: t('usage.requestDetails.videoResolution'), value: present(log.video_resolution) },
        { label: t('usage.requestDetails.videoDuration'), value: present(log.video_duration_seconds) },
      ],
    })
  }

  const asyncTask = log.async_task
  if (asyncTask) {
    draft.push({
      title: t('usage.requestDetails.asyncTask'),
      icon: 'server',
      items: [
        { label: t('usage.requestDetails.taskKind'), value: asyncTaskKindLabel(asyncTask.kind) },
        { label: t('usage.requestDetails.taskId'), value: present(asyncTask.task_id), monospace: true },
        { label: t('usage.requestDetails.taskStatus'), value: present(asyncTask.status) },
        { label: t('usage.requestDetails.statusUrl'), value: present(asyncTask.status_url), href: safeHref(asyncTask.status_url), monospace: true },
        ...(asyncTask.result_urls ?? []).map((url, index) => ({
          label: t('usage.requestDetails.resultUrl', { index: index + 1 }),
          value: url,
          href: safeHref(url),
          monospace: true,
        })),
        { label: t('usage.requestDetails.expiresAt'), value: asyncTask.expires_at ? formatDateTime(asyncTask.expires_at) : emptyValue },
      ],
    })
  }

  return draft
    .map((section) => ({ ...section, items: section.items.filter((item) => item.value !== emptyValue) }))
    .filter((section) => section.items.length > 0)
})
</script>

<style scoped>
/* 等宽字体只留给数值——请求 ID、端点、UA 这类需要逐字符看的东西。
   标签跟着界面字体走，否则整个弹窗都像一片数据。 */
.usage-request-details {
  display: grid;
  gap: 20px;
  color: #d9e4e0;
}

.usage-request-details__summary {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px 12px;
}

.usage-request-details__summary span { flex: 0 0 100%; color: #71877f; font-size: 11px; letter-spacing: .04em; text-transform: uppercase; }
.usage-request-details__summary strong { min-width: 0; overflow: hidden; color: #f2f7f5; font-size: 17px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.usage-request-details__summary time { color: #8b9e97; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }

.usage-request-details__section h4 { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; color: #9db0a9; font-size: 11px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; }
.usage-request-details__section h4 :deep(svg) { color: #00f5a8; }

/* 规格表式的两列：标签定宽在左，取值靠右占满剩余宽度，行间只用一条细线分隔。
   原先每个字段都套一层边框加底色，十几个方块的视觉噪音盖过了内容本身。 */
.usage-request-details__section dl { display: grid; }
.usage-request-details__section dl > div {
  display: grid;
  min-width: 0;
  align-items: baseline;
  grid-template-columns: minmax(88px, 132px) minmax(0, 1fr);
  gap: 16px;
  border-bottom: 1px solid rgba(72, 96, 89, .28);
  padding: 9px 0;
}
.usage-request-details__section dl > div:last-child { border-bottom: 0; }
.usage-request-details__section dt { color: #748981; font-size: 12px; line-height: 1.45; }
.usage-request-details__section dd { min-width: 0; overflow-wrap: anywhere; color: #eaf2ef; font-size: 12px; line-height: 1.45; }
.usage-request-details__mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.usage-request-details__section a { color: #5ee6b2; text-decoration: underline; text-decoration-color: rgba(94, 230, 178, .4); text-underline-offset: 2px; }
.usage-request-details__section a:hover { color: #9af3d1; text-decoration-color: currentColor; }

@media (max-width: 520px) {
  .usage-request-details__section dl > div { grid-template-columns: 1fr; gap: 3px; }
}
</style>
