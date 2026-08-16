<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.adminApiKey.multiTitle') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.adminApiKey.multiDescription') }}
      </p>
    </div>

    <div class="space-y-5 p-6">
      <div v-if="loading" class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('common.loading') }}
      </div>

      <div v-else-if="keys.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
        {{ t('admin.settings.adminApiKey.multiEmpty') }}
      </div>

      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="text-xs text-gray-500 dark:text-dark-400">
            <tr>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.settings.adminApiKey.keyName') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.settings.adminApiKey.keyValue') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.settings.adminApiKey.permission') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.settings.adminApiKey.defaults') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.settings.adminApiKey.createdAt') }}</th>
              <th class="pb-2 text-right font-medium">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="key in keys" :key="key.id" class="align-middle">
              <td class="py-3 pr-4 font-medium text-gray-900 dark:text-white">{{ key.name }}</td>
              <td class="py-3 pr-4 font-mono text-xs text-gray-600 dark:text-dark-300">{{ key.masked_key }}</td>
              <td class="py-3 pr-4">
                <span
                  class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="key.permission === 'full'
                    ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                    : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'"
                >
                  {{ permissionLabel(key.permission) }}
                </span>
              </td>
              <td class="max-w-xs py-3 pr-4 text-xs text-gray-500 dark:text-dark-400">
                {{ defaultsSummary(key) }}
              </td>
              <td class="py-3 pr-4 text-xs text-gray-500 dark:text-dark-400">{{ createdAt(key.created_at) }}</td>
              <td class="py-3 text-right">
                <button
                  v-if="key.id !== 'legacy'"
                  type="button"
                  class="btn btn-secondary btn-sm mr-2 inline-flex items-center gap-1.5"
                  data-testid="edit-key"
                  :aria-label="t('admin.settings.adminApiKey.edit')"
                  @click="startEdit(key)"
                >
                  <Icon name="edit" size="sm" />
                  {{ t('common.edit') }}
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                  data-testid="delete-key"
                  :disabled="deletingId === key.id"
                  @click="deleteTarget = key"
                >
                  <Icon name="trash" size="sm" />
                  {{ deletingId === key.id ? t('admin.settings.adminApiKey.deleting') : t('common.delete') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <form class="space-y-4 border-t border-gray-100 pt-5 dark:border-dark-700" @submit.prevent="saveKey">
        <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,18rem)_auto] md:items-end">
        <div>
          <label class="input-label" for="admin-api-key-name">{{ t('admin.settings.adminApiKey.keyName') }}</label>
          <input id="admin-api-key-name" v-model.trim="form.name" class="input" :placeholder="t('admin.settings.adminApiKey.keyNamePlaceholder')" maxlength="100" />
        </div>
        <div>
          <label class="input-label" for="admin-api-key-permission">{{ t('admin.settings.adminApiKey.permission') }}</label>
          <Select id="admin-api-key-permission" v-model="form.permission" :options="permissionOptions" />
        </div>
        <div class="flex gap-2">
          <button type="submit" class="btn btn-primary inline-flex items-center justify-center gap-1.5" :disabled="saving">
            <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
            {{ saving ? (editingId ? t('admin.settings.adminApiKey.saving') : t('admin.settings.adminApiKey.creating')) : (editingId ? t('admin.settings.adminApiKey.save') : t('admin.settings.adminApiKey.create')) }}
          </button>
          <button v-if="editingId" type="button" class="btn btn-secondary" @click="cancelEdit">
            {{ t('common.cancel') }}
          </button>
        </div>
        </div>

        <div v-if="form.permission === 'auto_pool'" class="grid gap-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600 md:grid-cols-2">
          <div>
            <label class="input-label" for="admin-api-key-proxy-mode">{{ t('admin.settings.adminApiKey.proxyMode') }}</label>
            <Select id="admin-api-key-proxy-mode" v-model="form.account_defaults.proxy_mode" :options="proxyModeOptions" />
          </div>
          <div v-if="form.account_defaults.proxy_mode === 'fixed'">
            <label class="input-label" for="admin-api-key-proxy-id">{{ t('admin.settings.adminApiKey.proxy') }}</label>
            <Select
              id="admin-api-key-proxy-id"
              v-model="form.account_defaults.proxy_id"
              :options="proxyOptions"
              :placeholder="t('admin.settings.adminApiKey.proxySelect')"
              searchable="auto"
            />
          </div>
          <div>
            <label class="input-label" for="admin-api-key-fingerprint">{{ t('admin.settings.adminApiKey.codexFingerprint') }}</label>
            <Select id="admin-api-key-fingerprint" v-model="form.account_defaults.codex_fingerprint_mode" :options="fingerprintOptions" />
          </div>
          <label class="flex items-center gap-2 self-end pb-2 text-sm text-gray-700 dark:text-gray-300">
            <input id="admin-api-key-revoke-sessions" v-model="form.account_defaults.revoke_other_sessions" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            {{ t('admin.settings.adminApiKey.revokeOtherSessions') }}
          </label>
        </div>
      </form>

      <div v-if="error" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
        {{ error }}
      </div>

      <div v-if="newKey" class="space-y-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
        <p class="text-sm font-medium text-green-700 dark:text-green-300">{{ t('admin.settings.adminApiKey.keyWarning') }}</p>
        <div class="flex items-center gap-2">
          <code class="flex-1 select-all break-all rounded border border-green-300 bg-white px-3 py-2 font-mono text-sm dark:border-green-700 dark:bg-dark-800">{{ newKey }}</code>
          <button type="button" class="btn btn-primary btn-sm inline-flex shrink-0 items-center gap-1.5" @click="copyKey">
            <Icon name="copy" size="sm" />
            {{ t('admin.settings.adminApiKey.copyKey') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="Boolean(deleteTarget)"
      :title="t('admin.settings.adminApiKey.deleteConfirm')"
      :message="t('admin.settings.adminApiKey.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import {
  createAdminApiKey,
  deleteAdminApiKeyById,
  listAdminApiKeys,
  updateAdminApiKey,
  type AdminApiKey,
  type AdminApiKeyAccountDefaults,
  type AdminApiKeyPermission
} from '@/api/admin/settings'
import { getAll as getAllProxies } from '@/api/admin/proxies'
import type { Proxy } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const emit = defineEmits<{ changed: [] }>()
const loading = ref(true)
const saving = ref(false)
const deletingId = ref('')
const error = ref('')
const keys = ref<AdminApiKey[]>([])
const proxies = ref<Proxy[]>([])
const newKey = ref('')
const deleteTarget = ref<AdminApiKey | null>(null)
const editingId = ref('')
const permissionOptions = computed<SelectOption[]>(() => [
  { value: 'full', label: t('admin.settings.adminApiKey.permissionFull') },
  { value: 'auto_pool', label: t('admin.settings.adminApiKey.permissionAutoPool') }
])
const proxyModeOptions = computed<SelectOption[]>(() => [
  { value: 'none', label: t('admin.settings.adminApiKey.proxyNone') },
  { value: 'fixed', label: t('admin.settings.adminApiKey.proxyFixed') },
  { value: 'random', label: t('admin.settings.adminApiKey.proxyRandom') }
])
const fingerprintOptions = computed<SelectOption[]>(() => [
  { value: 'off', label: t('admin.settings.adminApiKey.fingerprintOff') },
  { value: 'device', label: t('admin.settings.adminApiKey.fingerprintDevice') },
  { value: 'session', label: t('admin.settings.adminApiKey.fingerprintSession') },
  { value: 'full', label: t('admin.settings.adminApiKey.fingerprintFull') }
])
const proxyOptions = ref<SelectOption[]>([])
const form = reactive<{ name: string; permission: AdminApiKeyPermission; account_defaults: AdminApiKeyAccountDefaults }>({
  name: '',
  permission: 'auto_pool',
  account_defaults: defaultAccountDefaults()
})

onMounted(() => {
  void Promise.all([loadKeys(), loadProxies()])
})

async function loadKeys() {
  loading.value = true
  try {
    keys.value = await listAdminApiKeys()
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.settings.adminApiKey.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadProxies() {
  try {
    proxies.value = (await getAllProxies()).filter(proxy => proxy.status === 'active')
    proxyOptions.value = proxies.value.map(proxy => ({
      value: proxy.id,
      label: `${proxy.name} (${proxy.host}:${proxy.port})`
    }))
  } catch {
    proxies.value = []
    proxyOptions.value = []
  }
}

async function saveKey() {
  if (!form.name.trim()) {
    error.value = t('admin.settings.adminApiKey.keyNameRequired')
    return
  }
  if (form.permission === 'auto_pool' && form.account_defaults.proxy_mode === 'fixed' && !form.account_defaults.proxy_id) {
    error.value = t('admin.settings.adminApiKey.proxyRequired')
    return
  }
  saving.value = true
  error.value = ''
  try {
    const payload = {
      name: form.name,
      permission: form.permission,
      account_defaults: { ...form.account_defaults }
    }
    if (editingId.value) {
      const updated = await updateAdminApiKey(editingId.value, payload)
      keys.value = keys.value.map(key => key.id === updated.id ? updated : key)
      cancelEdit()
    } else {
      const result = await createAdminApiKey(payload)
      keys.value = [result.api_key, ...keys.value]
      newKey.value = result.key
      resetForm()
    }
    emit('changed')
  } catch (err) {
    error.value = extractApiErrorMessage(err, editingId.value ? t('admin.settings.adminApiKey.updateFailed') : t('admin.settings.adminApiKey.createFailed'))
  } finally {
    saving.value = false
  }
}

function defaultAccountDefaults(): AdminApiKeyAccountDefaults {
  return { proxy_mode: 'none', codex_fingerprint_mode: 'off', revoke_other_sessions: false }
}

function resetForm() {
  form.name = ''
  form.permission = 'auto_pool'
  form.account_defaults = defaultAccountDefaults()
  editingId.value = ''
}

function startEdit(key: AdminApiKey) {
  editingId.value = key.id
  form.name = key.name
  form.permission = key.permission
  form.account_defaults = { ...defaultAccountDefaults(), ...(key.account_defaults || {}) }
  error.value = ''
}

function cancelEdit() {
  resetForm()
}

async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleteTarget.value = null
  deletingId.value = target.id
  error.value = ''
  try {
    await deleteAdminApiKeyById(target.id)
    keys.value = keys.value.filter(key => key.id !== target.id)
    if (newKey.value && target.id === 'legacy') newKey.value = ''
    emit('changed')
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.settings.adminApiKey.deleteFailed'))
  } finally {
    deletingId.value = ''
  }
}

function permissionLabel(permission: AdminApiKeyPermission) {
  return permission === 'full'
    ? t('admin.settings.adminApiKey.permissionFull')
    : t('admin.settings.adminApiKey.permissionAutoPool')
}

function defaultsSummary(key: AdminApiKey) {
  if (key.permission === 'full') return t('admin.settings.adminApiKey.defaultsNone')
  const defaults = { ...defaultAccountDefaults(), ...(key.account_defaults || {}) }
  const proxy = defaults.proxy_mode === 'fixed'
    ? `${t('admin.settings.adminApiKey.proxyFixed')}: ${proxies.value.find(item => item.id === defaults.proxy_id)?.name || defaults.proxy_id || '?'}`
    : t(`admin.settings.adminApiKey.proxy${defaults.proxy_mode === 'random' ? 'Random' : 'None'}`)
  const fingerprint = t(`admin.settings.adminApiKey.fingerprint${defaults.codex_fingerprint_mode.charAt(0).toUpperCase()}${defaults.codex_fingerprint_mode.slice(1)}`)
  return `${proxy}; ${fingerprint}${defaults.revoke_other_sessions ? `; ${t('admin.settings.adminApiKey.revokeOtherSessions')}` : ''}`
}

function createdAt(value: string | null) {
  return value ? formatDateTime(new Date(value)) : t('admin.settings.adminApiKey.legacyCreatedAt')
}

async function copyKey() {
  if (!newKey.value) return
  try {
    await navigator.clipboard.writeText(newKey.value)
  } catch {
    error.value = t('common.copyFailed')
  }
}
</script>
