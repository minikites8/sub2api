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
      <div v-if="result" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-700">
        <div>{{ t('admin.accounts.yeTeamResult', result) }}</div>
        <div v-if="result.import_errors?.length" class="mt-2 max-h-40 overflow-auto text-xs text-red-600 dark:text-red-400">
          <div v-for="(item, index) in result.import_errors" :key="index">{{ item.name || item.kind }}: {{ item.message }}</div>
        </div>
      </div>
    </form>
    <template #footer>
      <button class="btn btn-secondary" type="button" :disabled="loading" @click="handleClose">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" type="submit" form="ye-team-redeem-form" :disabled="loading || !cardCode.trim()">
        {{ loading ? t('admin.accounts.yeTeamSubmitting') : t('admin.accounts.yeTeamSubmit') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { YeTeamRedeemResult } from '@/api/admin/accounts'

const emit = defineEmits<{ (e: 'close'): void; (e: 'redeemed'): void }>()
const props = defineProps<{ show: boolean }>()
const { t } = useI18n()
const appStore = useAppStore()
const cardCode = ref('')
const loading = ref(false)
const result = ref<YeTeamRedeemResult | null>(null)

watch(() => props.show, (open) => {
  if (open) {
    cardCode.value = ''
    result.value = null
  }
})

const handleClose = () => {
  if (!loading.value) emit('close')
}

const submit = async () => {
  const code = cardCode.value.trim()
  if (!code || loading.value) return
  loading.value = true
  try {
    result.value = await adminAPI.accounts.redeemYeTeam({ card_code: code, skip_default_group_bind: true })
    appStore.showSuccess(t('admin.accounts.yeTeamSuccess', result.value))
    emit('redeemed')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.yeTeamFailed'))
  } finally {
    loading.value = false
  }
}
</script>
