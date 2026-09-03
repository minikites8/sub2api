<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.ban.title')"
    width="normal"
    @close="emit('close')"
  >
    <div v-if="user" class="space-y-5">
      <div class="border-b border-gray-200 pb-4 dark:border-dark-700">
        <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.users.ban.userId', { id: user.id }) }}
        </p>
      </div>

      <div
        class="grid grid-cols-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-800"
        role="tablist"
        :aria-label="t('admin.users.ban.scope')"
      >
        <button
          type="button"
          class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
          :class="scope === 'user' ? activeSegmentClass : inactiveSegmentClass"
          data-test="ban-scope-user"
          @click="scope = 'user'"
        >
          {{ t('admin.users.ban.userScope') }}
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
          :class="scope === 'group' ? activeSegmentClass : inactiveSegmentClass"
          data-test="ban-scope-group"
          @click="scope = 'group'"
        >
          {{ t('admin.users.ban.groupScope') }}
        </button>
      </div>

      <div v-if="scope === 'user'" class="space-y-4">
        <div
          v-if="userBanActive"
          class="flex items-start justify-between gap-4 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900/60 dark:bg-red-950/30"
        >
          <div class="min-w-0">
            <p class="text-sm font-medium text-red-800 dark:text-red-300">
              {{ user.disabled_until ? t('admin.users.ban.temporaryActive') : t('admin.users.ban.permanentActive') }}
            </p>
            <p v-if="user.disabled_until" class="mt-1 text-xs text-red-700 dark:text-red-400">
              {{ t('admin.users.ban.until', { time: formatDateTime(user.disabled_until) }) }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm shrink-0"
            :disabled="submitting"
            data-test="unban-user"
            @click="handleUnbanUser"
          >
            {{ t('admin.users.ban.unbanUser') }}
          </button>
        </div>

      </div>

      <div v-else class="space-y-4">
        <div v-if="activeGroupBans.length" class="space-y-2">
          <p class="input-label">{{ t('admin.users.ban.activeGroupBans') }}</p>
          <div class="max-h-36 divide-y divide-gray-200 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
            <div
              v-for="ban in activeGroupBans"
              :key="ban.groupId"
              class="flex items-center justify-between gap-3 px-3 py-2"
            >
              <div class="min-w-0">
                <p class="truncate text-sm font-medium text-gray-800 dark:text-gray-200">{{ ban.groupName }}</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ ban.expiresAt
                    ? t('admin.users.ban.until', { time: formatDateTime(ban.expiresAt) })
                    : t('admin.users.ban.permanent') }}
                </p>
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0"
                :disabled="submitting"
                :data-test="`unban-group-${ban.groupId}`"
                @click="handleUnbanGroup(ban.groupId)"
              >
                {{ t('admin.users.ban.unbanGroup') }}
              </button>
            </div>
          </div>
        </div>

        <div>
          <label class="input-label" for="ban-group-select">{{ t('admin.users.ban.selectGroup') }}</label>
          <Select
            id="ban-group-select"
            v-model="selectedGroupId"
            :options="groupOptions"
            searchable
            :search-placeholder="t('admin.users.searchGroups')"
            :aria-label="t('admin.users.ban.selectGroup')"
            data-test="ban-group-select"
          />
        </div>
      </div>

      <div class="space-y-3">
        <p class="input-label">{{ t('admin.users.ban.duration') }}</p>
        <div class="grid grid-cols-2 gap-2">
          <button
            type="button"
            class="btn"
            :class="durationMode === 'permanent' ? 'btn-primary' : 'btn-secondary'"
            data-test="ban-duration-permanent"
            @click="durationMode = 'permanent'"
          >
            {{ t('admin.users.ban.permanent') }}
          </button>
          <button
            type="button"
            class="btn"
            :class="durationMode === 'temporary' ? 'btn-primary' : 'btn-secondary'"
            data-test="ban-duration-temporary"
            @click="durationMode = 'temporary'"
          >
            {{ t('admin.users.ban.temporary') }}
          </button>
        </div>
        <div v-if="durationMode === 'temporary'" class="grid grid-cols-[minmax(0,1fr)_9rem] gap-2">
          <input
            v-model="durationValue"
            type="number"
            min="1"
            step="1"
            class="input"
            data-test="ban-duration-value"
          />
          <select v-model="durationUnit" class="input" data-test="ban-duration-unit">
            <option value="hours">{{ t('admin.users.ban.hours') }}</option>
            <option value="days">{{ t('admin.users.ban.days') }}</option>
          </select>
        </div>
      </div>

      <p v-if="durationError" class="text-sm text-red-600 dark:text-red-400" data-test="ban-duration-error">
        {{ durationError }}
      </p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-danger"
          :disabled="!canSubmit"
          data-test="submit-ban"
          @click="handleBan"
        >
          <Icon name="ban" size="sm" />
          {{ submitting ? t('admin.users.ban.submitting') : submitLabel }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminGroup, AdminUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

type BanScope = 'user' | 'group'
type DurationMode = 'permanent' | 'temporary'
type DurationUnit = 'hours' | 'days'

const props = defineProps<{
  show: boolean
  user: AdminUser | null
  groups: AdminGroup[]
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()
const scope = ref<BanScope>('user')
const durationMode = ref<DurationMode>('permanent')
const durationValue = ref<string | number>(24)
const durationUnit = ref<DurationUnit>('hours')
const selectedGroupId = ref<number | null>(null)
const submitting = ref(false)

const activeSegmentClass = 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
const inactiveSegmentClass = 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'

const isFuture = (value?: string | null) => Boolean(value && new Date(value).getTime() > Date.now())
const userBanActive = computed(() => Boolean(
  props.user?.status === 'disabled'
  && (!props.user.disabled_until || isFuture(props.user.disabled_until))
))

const activeBannedGroupIds = computed(() => (props.user?.banned_group_ids || []).filter((groupId) => {
  const expiresAt = props.user?.banned_group_expirations?.[groupId]
  return !expiresAt || isFuture(expiresAt)
}))

const activeGroupBans = computed(() => activeBannedGroupIds.value.map((groupId) => ({
  groupId,
  groupName: props.groups.find(group => group.id === groupId)?.name || `#${groupId}`,
  expiresAt: props.user?.banned_group_expirations?.[groupId] || null
})))

const groupOptions = computed(() => props.groups
  .filter(group => !activeBannedGroupIds.value.includes(group.id))
  .map(group => ({ value: group.id, label: group.name })))

const durationHours = computed(() => {
  if (durationMode.value === 'permanent') return 0
  const value = Number(durationValue.value)
  if (!Number.isInteger(value) || value <= 0) return null
  return value * (durationUnit.value === 'days' ? 24 : 1)
})

const durationError = computed(() => {
  if (durationHours.value === null) return t('admin.users.ban.invalidDuration')
  if (durationHours.value > 8760) return t('admin.users.ban.durationTooLong')
  return ''
})

const canSubmit = computed(() => Boolean(
  props.user
  && props.user.role !== 'admin'
  && !submitting.value
  && durationHours.value !== null
  && durationHours.value <= 8760
  && (scope.value === 'user' || selectedGroupId.value)
))

const submitLabel = computed(() => scope.value === 'user'
  ? t('admin.users.ban.banUser')
  : t('admin.users.ban.banGroup'))

const reset = () => {
  scope.value = 'user'
  durationMode.value = 'permanent'
  durationValue.value = 24
  durationUnit.value = 'hours'
  selectedGroupId.value = null
  submitting.value = false
}

watch(() => props.show, (show) => {
  if (show) reset()
})

const errorMessage = (error: any) => error?.message
  || error?.response?.data?.message
  || error?.response?.data?.detail
  || t('admin.users.ban.failed')

const finishSuccess = (message: string) => {
  appStore.showSuccess(message)
  emit('success')
  emit('close')
}

const handleBan = async () => {
  if (!canSubmit.value || !props.user || durationHours.value === null) return
  submitting.value = true
  try {
    if (scope.value === 'user') {
      await adminAPI.users.banUser(props.user.id, { duration_hours: durationHours.value })
      finishSuccess(t('admin.users.ban.userBanned'))
    } else if (selectedGroupId.value) {
      await adminAPI.users.banGroup(props.user.id, selectedGroupId.value, { duration_hours: durationHours.value })
      finishSuccess(t('admin.users.ban.groupBanned'))
    }
  } catch (error: any) {
    appStore.showError(errorMessage(error))
  } finally {
    submitting.value = false
  }
}

const handleUnbanUser = async () => {
  if (!props.user || submitting.value) return
  submitting.value = true
  try {
    await adminAPI.users.unbanUser(props.user.id)
    finishSuccess(t('admin.users.ban.userUnbanned'))
  } catch (error: any) {
    appStore.showError(errorMessage(error))
  } finally {
    submitting.value = false
  }
}

const handleUnbanGroup = async (groupId: number) => {
  if (!props.user || submitting.value) return
  submitting.value = true
  try {
    await adminAPI.users.unbanGroup(props.user.id, groupId)
    finishSuccess(t('admin.users.ban.groupUnbanned'))
  } catch (error: any) {
    appStore.showError(errorMessage(error))
  } finally {
    submitting.value = false
  }
}

</script>
