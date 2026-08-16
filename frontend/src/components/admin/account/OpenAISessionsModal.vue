<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.sessions.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ account?.name }}
          <span v-if="sessions" class="text-gray-400 dark:text-dark-500">
            · {{ t('admin.accounts.sessions.checkedAt', { time: formatFetchedAt(sessions.fetched_at) }) }}
          </span>
        </p>
        <button
          type="button"
          class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
          :disabled="loading"
          @click="loadSessions"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div v-if="error" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
        {{ error }}
      </div>

      <div v-if="loading" class="flex items-center justify-center py-12 text-sm text-gray-500 dark:text-dark-400">
        <Icon name="refresh" size="md" class="mr-2 animate-spin" />
        {{ t('admin.accounts.sessions.loading') }}
      </div>

      <div v-else-if="sessions && sessions.devices.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
        {{ t('admin.accounts.sessions.empty') }}
      </div>

      <div v-else class="grid gap-3 md:grid-cols-2">
        <article
          v-for="(device, index) in sessions?.devices ?? []"
          :key="deviceKey(device, index)"
          class="rounded-lg border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/60"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-start gap-2">
              <Icon name="server" size="md" class="mt-0.5 shrink-0 text-primary-500" />
              <div class="min-w-0">
                <h4 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                  {{ deviceTitle(device) }}
                </h4>
                <p v-if="device.human_readable_description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ device.human_readable_description }}
                </p>
              </div>
            </div>
            <span
              v-if="device.is_current_device"
              class="shrink-0 rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
            >
              {{ t('admin.accounts.sessions.current') }}
            </span>
          </div>

          <dl class="mt-3 space-y-1.5 text-xs text-gray-600 dark:text-dark-300">
            <div v-if="device.platform || device.os_version || device.device_model" class="flex gap-2">
              <dt class="shrink-0 text-gray-400 dark:text-dark-500">{{ t('admin.accounts.sessions.device') }}</dt>
              <dd class="min-w-0 break-words">{{ deviceDetails(device) }}</dd>
            </div>
            <div v-if="location(device)" class="flex gap-2">
              <dt class="shrink-0 text-gray-400 dark:text-dark-500">{{ t('admin.accounts.sessions.location') }}</dt>
              <dd class="min-w-0 break-words">{{ location(device) }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-gray-400 dark:text-dark-500">{{ t('admin.accounts.sessions.lastSignedIn') }}</dt>
              <dd>{{ signedInAt(device.last_signed_in_timestamp_second) }}</dd>
            </div>
            <div v-if="appNames(device).length" class="flex gap-2">
              <dt class="shrink-0 text-gray-400 dark:text-dark-500">{{ t('admin.accounts.sessions.apps') }}</dt>
              <dd class="min-w-0 break-words">{{ appNames(device).join(', ') }}</dd>
            </div>
          </dl>

          <div class="mt-4 flex items-center justify-between gap-2">
            <span v-if="device.is_trusted_device" class="inline-flex items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
              <Icon name="shield" size="xs" />
              {{ t('admin.accounts.sessions.trusted') }}
            </span>
            <span v-else></span>
            <button
              v-if="device.session_id && !device.is_current_device"
              type="button"
              class="btn btn-danger btn-sm inline-flex items-center gap-1.5"
              :disabled="revoking.has(device.session_id)"
              @click="requestRevoke(device)"
            >
              <Icon name="ban" size="sm" />
              {{ revoking.has(device.session_id) ? t('admin.accounts.sessions.revoking') : t('admin.accounts.sessions.revoke') }}
            </button>
          </div>
        </article>
      </div>
    </div>

    <ConfirmDialog
      :show="Boolean(revokeTarget)"
      :title="t('admin.accounts.sessions.revokeTitle')"
      :message="t('admin.accounts.sessions.revokeConfirm', { name: revokeTarget ? deviceTitle(revokeTarget) : '' })"
      :confirm-text="t('admin.accounts.sessions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="revokeTarget = null"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { listOpenAISessions, revokeOpenAISession, type OpenAISessionDevice, type OpenAISessionsResponse } from '@/api/admin/accounts'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type { Account } from '@/types'

const props = defineProps<{
  show: boolean
  account: Account | null
}>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()

const loading = ref(false)
const error = ref('')
const sessions = ref<OpenAISessionsResponse | null>(null)
const revokeTarget = ref<OpenAISessionDevice | null>(null)
const revoking = ref(new Set<string>())

watch(
  () => [props.show, props.account?.id] as const,
  ([show]) => {
    if (show) void loadSessions()
  },
  { immediate: true }
)

async function loadSessions() {
  if (!props.account) return
  loading.value = true
  error.value = ''
  try {
    sessions.value = await listOpenAISessions(props.account.id)
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.accounts.sessions.loadFailed'))
  } finally {
    loading.value = false
  }
}

function requestRevoke(device: OpenAISessionDevice) {
  if (!device.session_id || device.is_current_device) return
  revokeTarget.value = device
}

async function confirmRevoke() {
  const device = revokeTarget.value
  const account = props.account
  if (!device?.session_id || !account) return
  const sessionId = device.session_id
  revokeTarget.value = null
  const next = new Set(revoking.value)
  next.add(sessionId)
  revoking.value = next
  error.value = ''
  try {
    await revokeOpenAISession(account.id, sessionId)
    await loadSessions()
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.accounts.sessions.revokeFailed'))
  } finally {
    const remaining = new Set(revoking.value)
    remaining.delete(sessionId)
    revoking.value = remaining
  }
}

function deviceKey(device: OpenAISessionDevice, index: number) {
  return device.session_id || device.render_id || `device-${index}`
}

function deviceTitle(device: OpenAISessionDevice) {
  return device.display_name || device.device_model || device.platform || t('admin.accounts.sessions.unknownDevice')
}

function deviceDetails(device: OpenAISessionDevice) {
  return [device.platform, device.os_version, device.device_model].filter(Boolean).join(' · ')
}

function location(device: OpenAISessionDevice) {
  return [device.last_signed_in_city, device.last_signed_in_region_code, device.last_signed_in_country].filter(Boolean).join(', ')
}

function appNames(device: OpenAISessionDevice) {
  return (device.app_sessions ?? []).map(item => item.client_name).filter((name): name is string => Boolean(name))
}

function signedInAt(timestamp: number) {
  return timestamp > 0 ? formatDateTime(new Date(timestamp * 1000)) : t('admin.accounts.sessions.unknownTime')
}

function formatFetchedAt(timestamp: number) {
  return timestamp > 0 ? formatDateTime(new Date(timestamp * 1000)) : t('admin.accounts.sessions.unknownTime')
}
</script>
