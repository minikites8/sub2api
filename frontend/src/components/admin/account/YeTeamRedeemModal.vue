<template>
  <BaseDialog :show="show" :title="t('admin.accounts.yeTeamTitle')" width="normal" @close="handleClose">
    <form id="ye-team-redeem-form" class="space-y-4" @submit.prevent="submit">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.accounts.yeTeamHint') }}</p>
      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('admin.accounts.yeTeamWarning') }}
      </div>
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.yeTeamCardCode') }}</span>
        <input
          v-model="cardCode"
          class="input w-full font-mono uppercase"
          autocomplete="off"
          :placeholder="t('admin.accounts.yeTeamCardCodePlaceholder')"
          :disabled="loading"
        />
      </label>
      <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
        <div class="mb-3 text-sm font-medium text-gray-800 dark:text-gray-200">
          {{ t('admin.accounts.yeTeamAccountSettings') }}
        </div>
        <div class="space-y-3">
          <GroupSelector
            v-model="options.group_ids"
            :groups="groups"
            :label="t('admin.accounts.yeTeamGroups')"
            searchable="auto"
          />
          <p class="input-hint">{{ t('admin.accounts.yeTeamGroupsHint') }}</p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamConcurrency') }}</span>
              <input v-model.number="options.concurrency" type="number" min="0" class="input mt-1 w-full" />
              <span class="input-hint mt-1 block">{{ t('admin.accounts.yeTeamConcurrencyHint') }}</span>
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamPriority') }}</span>
              <input v-model.number="options.priority" type="number" min="0" class="input mt-1 w-full" />
              <span class="input-hint mt-1 block">{{ t('admin.accounts.yeTeamPriorityHint') }}</span>
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamProxyMode') }}</span>
              <Select v-model="options.proxy_mode" class="mt-1" :options="proxyModeOptions" />
            </label>
            <label v-if="options.proxy_mode === 'specified'" class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamProxy') }}</span>
              <Select
                v-model="options.proxy_id"
                class="mt-1"
                :options="proxyOptions"
                :placeholder="t('admin.accounts.yeTeamProxyPlaceholder')"
                searchable="auto"
              />
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamCodexFingerprint') }}</span>
              <Select v-model="options.codex_fingerprint_mode" class="mt-1" :options="fingerprintOptions" />
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.accounts.yeTeamOpenAIWSMode') }}</span>
              <Select v-model="options.openai_ws_mode" class="mt-1" :options="openAIWSModeOptions" />
            </label>
          </div>
          <div class="flex items-center gap-2">
            <input v-model="options.enable_account_guard" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.yeTeamEnableAccountGuard') }}</span>
          </div>
          <label v-if="options.enable_account_guard" class="block sm:max-w-xs">
            <span class="input-label">{{ t('admin.accounts.yeTeamAccountGuardInterval') }}</span>
            <input v-model.number="options.account_guard_interval_minutes" type="number" min="5" max="1440" class="input mt-1 w-full" />
          </label>
        </div>
      </div>
      <div v-if="result" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-700">
        <div>{{ t('admin.accounts.yeTeamResult', result) }}</div>
        <div v-if="result.import_errors?.length" class="mt-2 max-h-40 overflow-auto text-xs text-red-600 dark:text-red-400">
          <div v-for="(item, index) in result.import_errors" :key="index">{{ item.name || item.kind }}: {{ item.message }}</div>
        </div>
      </div>
    </form>
    <template #footer>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="handleClose">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" type="submit" form="ye-team-redeem-form" :disabled="loading || optionsLoading || !cardCode.trim()">
        {{ loading ? t('admin.accounts.yeTeamSubmitting') : t('admin.accounts.yeTeamSubmit') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { YeTeamAccountOptions, YeTeamRedeemResult } from '@/api/admin/accounts'
import type { AdminGroup, Proxy } from '@/types'
import Select from '@/components/common/Select.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'

const emit = defineEmits<{ (e: 'close'): void; (e: 'redeemed'): void }>()
const props = defineProps<{ show: boolean }>()
const { t } = useI18n()
const appStore = useAppStore()
const cardCode = ref('')
const loading = ref(false)
const optionsLoading = ref(false)
const result = ref<YeTeamRedeemResult | null>(null)
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const options = reactive<Required<Omit<YeTeamAccountOptions, 'proxy_id'>> & { proxy_id?: number }>({
  group_ids: [],
  concurrency: 0,
  priority: 0,
  proxy_mode: 'none',
  proxy_id: undefined,
  codex_fingerprint_mode: 'off',
  enable_account_guard: false,
  account_guard_interval_minutes: 30,
  openai_ws_mode: 'off'
})

const proxyModeOptions = computed(() => [
  { value: 'none', label: t('admin.accounts.yeTeamProxyNone') },
  { value: 'specified', label: t('admin.accounts.yeTeamProxySpecified') },
  { value: 'random', label: t('admin.accounts.yeTeamProxyRandom') }
])
const fingerprintOptions = computed(() => [
  { value: 'off', label: t('admin.accounts.yeTeamFingerprintOff') },
  { value: 'device', label: t('admin.accounts.yeTeamFingerprintDevice') },
  { value: 'session', label: t('admin.accounts.yeTeamFingerprintSession') },
  { value: 'full', label: t('admin.accounts.yeTeamFingerprintFull') }
])
const openAIWSModeOptions = computed(() => [
  { value: 'off', label: t('admin.accounts.yeTeamOpenAIWSOff') },
  { value: 'ctx_pool', label: t('admin.accounts.yeTeamOpenAIWSCtxPool') },
  { value: 'passthrough', label: t('admin.accounts.yeTeamOpenAIWSPassthrough') },
  { value: 'http_bridge', label: t('admin.accounts.yeTeamOpenAIWSHTTPBridge') }
])
const proxyOptions = computed(() => proxies.value.map(proxy => ({
  value: proxy.id,
  label: `${proxy.name} (${proxy.host}:${proxy.port})`
})))

watch(() => props.show, (open) => {
  if (open) {
    cardCode.value = ''
    result.value = null
    resetOptions()
    void loadOptions()
  }
})

const resetOptions = () => {
  options.group_ids = []
  options.concurrency = 0
  options.priority = 0
  options.proxy_mode = 'none'
  options.proxy_id = undefined
  options.codex_fingerprint_mode = 'off'
  options.enable_account_guard = false
  options.account_guard_interval_minutes = 30
  options.openai_ws_mode = 'off'
}

const loadOptions = async () => {
  optionsLoading.value = true
  try {
    const [allGroups, allProxies] = await Promise.all([
      adminAPI.groups.getAll(),
      adminAPI.proxies.getAll()
    ])
    groups.value = allGroups.filter(group => group.status === 'active')
    proxies.value = allProxies.filter(proxy => proxy.status === 'active')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.yeTeamOptionsLoadFailed'))
  } finally {
    optionsLoading.value = false
  }
}

onMounted(() => {
  if (props.show) void loadOptions()
})

const handleClose = () => {
  if (!loading.value) emit('close')
}

const submit = async () => {
  const code = cardCode.value.trim()
  if (!code || loading.value) return
  if (options.proxy_mode === 'specified' && !options.proxy_id) {
    appStore.showError(t('admin.accounts.yeTeamProxyRequired'))
    return
  }
  loading.value = true
  try {
    const accountOptions: YeTeamAccountOptions = {
      group_ids: [...options.group_ids],
      concurrency: options.concurrency > 0 ? options.concurrency : undefined,
      priority: options.priority > 0 ? options.priority : undefined,
      proxy_mode: options.proxy_mode,
      proxy_id: options.proxy_mode === 'specified' ? options.proxy_id : undefined,
      codex_fingerprint_mode: options.codex_fingerprint_mode,
      enable_account_guard: options.enable_account_guard,
      account_guard_interval_minutes: options.enable_account_guard ? options.account_guard_interval_minutes : undefined,
      openai_ws_mode: options.openai_ws_mode
    }
    result.value = await adminAPI.accounts.redeemYeTeam({ card_code: code, skip_default_group_bind: true, account_options: accountOptions })
    appStore.showSuccess(t('admin.accounts.yeTeamSuccess', result.value))
    emit('redeemed')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.yeTeamFailed'))
  } finally {
    loading.value = false
  }
}
</script>
