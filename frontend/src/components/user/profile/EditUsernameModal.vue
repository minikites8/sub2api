<template>
  <BaseDialog
    :show="show"
    :title="t('profile.editUsername.title')"
    width="narrow"
    @close="handleClose"
  >
    <form class="space-y-4" @submit.prevent="handleSave">
      <div>
        <label for="edit-username-input" class="input-label">
          {{ t('profile.username') }}
        </label>
        <input
          id="edit-username-input"
          v-model="username"
          type="text"
          class="input"
          :placeholder="t('profile.enterUsername')"
        />
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="handleSave">
          {{ saving ? t('profile.updating') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { userAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{
  show: boolean
  currentUsername: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.currentUsername)
const saving = ref(false)

watch(() => props.show, (isOpen) => {
  if (isOpen) {
    username.value = props.currentUsername
  }
})

function handleClose() {
  if (saving.value) {
    return
  }
  emit('close')
}

async function handleSave() {
  const trimmed = username.value.trim()
  if (!trimmed) {
    appStore.showError(t('profile.usernameRequired'))
    return
  }

  saving.value = true
  try {
    const updatedUser = await userAPI.updateProfile({ username: trimmed })
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.updateSuccess'))
    emit('close')
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('profile.updateFailed')))
  } finally {
    saving.value = false
  }
}
</script>
