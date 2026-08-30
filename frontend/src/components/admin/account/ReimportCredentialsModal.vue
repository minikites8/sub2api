<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.reimportCredentialsTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="reimport-credentials-form" class="space-y-4" @submit.prevent="handleImport">
      <div v-if="account" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="text-sm font-medium text-gray-900 dark:text-white">{{ account.name }}</div>
        <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ account.platform }} / {{ account.type }}
        </div>
      </div>
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.reimportCredentialsHint') }}
      </div>
      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('admin.accounts.reimportCredentialsWarning') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.reimportCredentialsFile') }}</label>
        <div class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800">
          <div class="min-w-0 truncate text-sm text-gray-700 dark:text-dark-200" :title="selectedFile?.name">
            {{ selectedFile?.name || t('admin.accounts.reimportCredentialsSelectFile') }}
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          accept="application/json,.json"
          @change="handleFileChange"
        />
      </div>

      <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMessage }}
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="reimport-credentials-form"
          :disabled="importing || !selectedFile"
        >
          {{ importing ? t('admin.accounts.reimportCredentialsImporting') : t('admin.accounts.reimportCredentialsButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { Account, AdminDataPayload } from '@/types'

defineOptions({ name: 'ReimportCredentialsModal' })

const props = defineProps<{ show: boolean; account: Account | null }>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'reimported', account: Account): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const importing = ref(false)
const errorMessage = ref('')

watch(
  () => props.show,
  (open) => {
    if (open) {
      selectedFile.value = null
      importing.value = false
      errorMessage.value = ''
      if (fileInput.value) fileInput.value.value = ''
    }
  }
)

const openFilePicker = () => fileInput.value?.click()

const handleFileChange = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] || null
  input.value = ''
  if (!file) return
  if (!file.name.toLowerCase().endsWith('.json') && file.type !== 'application/json') {
    errorMessage.value = t('admin.accounts.reimportCredentialsInvalidFile')
    selectedFile.value = null
    return
  }
  selectedFile.value = file
  errorMessage.value = ''
}

const readFileAsText = async (file: File): Promise<string> => {
  if (typeof file.text === 'function') return file.text()
  const buffer = await file.arrayBuffer()
  return new TextDecoder().decode(buffer)
}

const isDataPayload = (value: unknown): value is AdminDataPayload => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const candidate = value as Record<string, unknown>
  if (candidate.type !== undefined && candidate.type !== '' && !['sub2api-data', 'sub2api-bundle'].includes(String(candidate.type))) return false
  if (candidate.version !== undefined && candidate.version !== 0 && candidate.version !== 1) return false
  return Array.isArray(candidate.proxies) && Array.isArray(candidate.accounts)
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const handleImport = async () => {
  if (!props.account || !selectedFile.value) {
    errorMessage.value = t('admin.accounts.reimportCredentialsSelectFile')
    return
  }

  importing.value = true
  errorMessage.value = ''
  try {
    let parsed: unknown
    try {
      parsed = JSON.parse(await readFileAsText(selectedFile.value))
    } catch {
      errorMessage.value = t('admin.accounts.reimportCredentialsParseFailed')
      return
    }
    if (!isDataPayload(parsed)) {
      errorMessage.value = t('admin.accounts.reimportCredentialsInvalidFile')
      return
    }
    if (parsed.accounts.length !== 1) {
      errorMessage.value = t('admin.accounts.reimportCredentialsSingleAccount')
      return
    }
    const imported = parsed.accounts[0]
    if (imported.platform !== props.account.platform || imported.type !== props.account.type) {
      errorMessage.value = t('admin.accounts.reimportCredentialsIdentityMismatch')
      return
    }

    const updated = await adminAPI.accounts.reimportCredentials(props.account.id, parsed)
    appStore.showSuccess(t('admin.accounts.reimportCredentialsSuccess'))
    emit('reimported', updated)
    emit('close')
  } catch (error: any) {
    errorMessage.value = error?.response?.data?.message || error?.message || t('admin.accounts.reimportCredentialsFailed')
  } finally {
    importing.value = false
  }
}
</script>
