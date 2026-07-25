<template>
  <BaseDialog
    :show="Boolean(log)"
    :title="t('usage.requestDetails.title')"
    width="normal"
    @close="emit('close')"
  >
    <div v-if="log" class="usage-request-details" data-testid="request-details-dialog">
      <div class="usage-request-details__summary">
        <div>
          <span>{{ t('usage.requestDetails.request') }}</span>
          <strong>{{ log.model || emptyValue }}</strong>
        </div>
        <span class="usage-request-details__status"><i />200 OK</span>
      </div>

      <section v-for="section in sections" :key="section.title" class="usage-request-details__section">
        <h4><Icon :name="section.icon" size="sm" />{{ section.title }}</h4>
        <dl>
          <div
            v-for="item in section.items"
            :key="item.label"
            :class="{ 'usage-request-details__item--wide': item.wide }"
          >
            <dt>{{ item.label }}</dt>
            <dd :class="{ 'usage-request-details__mono': item.monospace }">{{ item.value }}</dd>
          </div>
        </dl>
      </section>

      <section v-if="imageItems.length" class="usage-request-details__section">
        <h4><Icon name="sparkles" size="sm" />{{ t('usage.requestDetails.imageMetadata') }}</h4>
        <dl>
          <div v-for="item in imageItems" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
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

type DetailIcon = 'clipboard' | 'server' | 'globe'

interface DetailItem {
  label: string
  value: string
  monospace?: boolean
  wide?: boolean
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
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

const transportLabel = (log: UsageLog) => {
  if (log.openai_ws_mode || resolveUsageRequestType(log) === 'ws_v2') {
    return t('usage.requestDetails.websocket')
  }
  return log.stream
    ? t('usage.requestDetails.streaming')
    : t('usage.requestDetails.synchronous')
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

const sections = computed<DetailSection[]>(() => {
  const log = props.log
  if (!log) return []

  return [
    {
      title: t('usage.requestDetails.requestContext'),
      icon: 'clipboard',
      items: [
        { label: t('usage.requestDetails.requestId'), value: present(log.request_id), monospace: true, wide: true },
        { label: t('usage.time'), value: formatDateTime(log.created_at) },
        { label: t('usage.apiKeyFilter'), value: present(log.api_key?.name) },
        { label: t('admin.usage.group'), value: groupLabel(log) },
      ],
    },
    {
      title: t('usage.requestDetails.routing'),
      icon: 'server',
      items: [
        { label: t('usage.inboundEndpoint'), value: present(log.inbound_endpoint), monospace: true, wide: true },
        { label: t('usage.type'), value: requestTypeLabel(log) },
        { label: t('usage.requestDetails.transport'), value: transportLabel(log) },
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
        { label: t('usage.userAgent'), value: present(log.user_agent), monospace: true, wide: true },
      ],
    },
  ]
})

const imageItems = computed<DetailItem[]>(() => {
  const log = props.log
  if (!log || log.image_count <= 0) return []
  return [
    { label: t('usage.requestDetails.mediaType'), value: present(log.media_type) },
    { label: t('usage.imageCount'), value: present(log.image_count) },
    { label: t('usage.imageBillingSize'), value: present(log.image_size) },
    { label: t('usage.imageInputSize'), value: present(log.image_input_size) },
    { label: t('usage.imageOutputSize'), value: present(log.image_output_size) },
    { label: t('usage.imageSizeSource'), value: imageSizeSourceLabel(log.image_size_source) },
  ]
})
</script>

<style scoped>
.usage-request-details {
  display: grid;
  gap: 18px;
  color: #d9e4e0;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.usage-request-details__summary {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid #30443f;
  background: #0b1519;
  padding: 13px 14px;
}

.usage-request-details__summary > div { min-width: 0; }
.usage-request-details__summary span { display: block; color: #71877f; font-size: 9px; text-transform: uppercase; }
.usage-request-details__summary strong { display: block; margin-top: 5px; overflow: hidden; color: #f2f7f5; font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.usage-request-details__status { display: inline-flex !important; flex: 0 0 auto; align-items: center; gap: 7px; color: #00f5a8 !important; font-size: 10px !important; white-space: nowrap; }
.usage-request-details__status i { width: 6px; height: 6px; border-radius: 50%; background: #00f5a8; }

.usage-request-details__section { border-top: 1px solid #2b3e38; padding-top: 14px; }
.usage-request-details__section h4 { display: flex; align-items: center; gap: 8px; margin-bottom: 11px; color: #9db0a9; font-size: 10px; font-weight: 600; text-transform: uppercase; }
.usage-request-details__section h4 :deep(svg) { color: #00f5a8; }
.usage-request-details__section dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.usage-request-details__section dl > div { min-width: 0; border: 1px solid rgba(72, 96, 89, .46); background: rgba(8, 17, 21, .68); padding: 10px 11px; }
.usage-request-details__item--wide { grid-column: 1 / -1; }
.usage-request-details__section dt { color: #748981; font-size: 9px; }
.usage-request-details__section dd { margin-top: 5px; overflow-wrap: anywhere; color: #d9e4e0; font-size: 11px; line-height: 1.45; }
.usage-request-details__mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

@media (max-width: 520px) {
  .usage-request-details__section dl { grid-template-columns: 1fr; }
  .usage-request-details__item--wide { grid-column: auto; }
}
</style>
