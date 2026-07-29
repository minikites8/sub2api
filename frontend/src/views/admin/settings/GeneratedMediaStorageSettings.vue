<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.generatedMediaStorage.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.generatedMediaStorage.description') }}
      </p>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-12">
      <div class="h-7 w-7 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <div v-else class="space-y-6 p-6">
      <div class="flex items-start justify-between gap-6">
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.generatedMediaStorage.enabled') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.generatedMediaStorage.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.enabled" />
      </div>

      <div class="grid gap-5 md:grid-cols-2">
        <div class="md:col-span-2">
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.endpoint') }}</label>
          <input
            v-model.trim="form.endpoint"
            type="url"
            class="input"
            placeholder="https://cos.ap-guangzhou.myqcloud.com"
            autocomplete="off"
          />
          <p class="input-hint">{{ t('admin.settings.generatedMediaStorage.endpointHint') }}</p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.region') }}</label>
          <input v-model.trim="form.region" type="text" class="input" placeholder="ap-guangzhou" autocomplete="off" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.bucket') }}</label>
          <input v-model.trim="form.bucket" type="text" class="input" placeholder="generated-media-1250000000" autocomplete="off" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.accessKeyId') }}</label>
          <input v-model.trim="form.access_key_id" type="text" class="input" autocomplete="off" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.secretAccessKey') }}</label>
          <input
            v-model="form.secret_access_key"
            type="password"
            class="input"
            :placeholder="secretConfigured ? t('admin.settings.generatedMediaStorage.secretConfigured') : ''"
            autocomplete="new-password"
          />
          <p v-if="secretConfigured" class="input-hint">
            {{ t('admin.settings.generatedMediaStorage.secretKeepHint') }}
          </p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.prefix') }}</label>
          <input v-model.trim="form.prefix" type="text" class="input" placeholder="generated-videos" autocomplete="off" />
          <p class="input-hint">{{ t('admin.settings.generatedMediaStorage.prefixHint') }}</p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.generatedMediaStorage.publicBaseUrl') }}</label>
          <input
            v-model.trim="form.public_base_url"
            type="url"
            class="input"
            placeholder="https://media.example.com"
            autocomplete="off"
          />
          <p class="input-hint">{{ t('admin.settings.generatedMediaStorage.publicBaseUrlHint') }}</p>
        </div>
      </div>

      <div class="flex items-start justify-between gap-6 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.generatedMediaStorage.forcePathStyle') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.generatedMediaStorage.forcePathStyleHint') }}
          </p>
        </div>
        <Toggle v-model="form.force_path_style" />
      </div>

      <div class="flex flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="testing || saving" @click="testStorage">
          {{ testing ? t('admin.settings.generatedMediaStorage.testing') : t('admin.settings.generatedMediaStorage.test') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving || testing" @click="saveStorage">
          {{ saving ? t('admin.settings.saving') : t('admin.settings.generatedMediaStorage.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import generatedMediaStorageAPI, {
  type GeneratedMediaStorageConfig,
} from '@/api/admin/generatedMediaStorage'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const emptyConfig = (): GeneratedMediaStorageConfig => ({
  enabled: false,
  endpoint: '',
  region: '',
  bucket: '',
  access_key_id: '',
  secret_access_key: '',
  secret_configured: false,
  prefix: 'generated-videos',
  public_base_url: '',
  force_path_style: false,
})

const form = ref<GeneratedMediaStorageConfig>(emptyConfig())
const secretConfigured = ref(false)
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)

async function loadConfig() {
  loading.value = true
  try {
    const config = await generatedMediaStorageAPI.getConfig()
    form.value = {
      ...emptyConfig(),
      ...config,
      secret_access_key: '',
      prefix: config.prefix || 'generated-videos',
    }
    secretConfigured.value = Boolean(config.secret_configured)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    loading.value = false
  }
}

async function saveStorage() {
  saving.value = true
  try {
    await generatedMediaStorageAPI.updateConfig(form.value)
    appStore.showSuccess(t('admin.settings.generatedMediaStorage.saved'))
    await loadConfig()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    saving.value = false
  }
}

async function testStorage() {
  testing.value = true
  try {
    const result = await generatedMediaStorageAPI.testConnection(form.value)
    if (result.ok) {
      appStore.showSuccess(t('admin.settings.generatedMediaStorage.testSuccess'))
    } else {
      appStore.showError(result.message || t('admin.settings.generatedMediaStorage.testFailed'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testing.value = false
  }
}

onMounted(loadConfig)
</script>
