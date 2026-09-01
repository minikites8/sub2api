<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.workspaceInfo.title')"
    width="normal"
    @close="emit('close')"
  >
    <div v-if="info" class="space-y-5">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ props.account?.name }}</p>
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="rounded-lg border border-gray-200 bg-gray-50/70 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/60">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.workspaceInfo.name') }}</dt>
          <dd class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white">{{ info.name || '-' }}</dd>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50/70 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/60">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.workspaceInfo.createdTime') }}</dt>
          <dd class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white">{{ formatDate(info.created_time) }}</dd>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50/70 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/60">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.workspaceInfo.organizationId') }}</dt>
          <dd class="mt-1 break-all font-mono text-xs text-gray-900 dark:text-white">{{ info.organization_id || '-' }}</dd>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50/70 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-800/60">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.workspaceInfo.accountId') }}</dt>
          <dd class="mt-1 break-all font-mono text-xs text-gray-900 dark:text-white">{{ info.account_id || '-' }}</dd>
        </div>
      </div>

      <div>
        <div class="mb-2 flex items-center justify-between gap-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.workspaceInfo.seats') }}</h4>
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.accounts.workspaceInfo.maximumSeats', { count: info.maximum_seats }) }}
          </span>
        </div>
        <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <div
            v-for="seat in seatEntries"
            :key="seat.key"
            class="flex items-center justify-between border-b border-gray-100 px-3 py-2.5 text-sm last:border-b-0 dark:border-dark-700"
          >
            <span class="text-gray-600 dark:text-dark-300">{{ t(`admin.accounts.workspaceInfo.seatTypes.${seat.key}`) }}</span>
            <span class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ seat.count }}</span>
          </div>
        </div>
      </div>

      <p class="text-xs text-gray-400 dark:text-dark-500">
        {{ t('admin.accounts.workspaceInfo.checkedAt', { time: formatDate(info.fetched_at) }) }}
      </p>

      <form class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700" @submit.prevent="submitInvite">
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.workspaceInfo.inviteTitle') }}</h4>
          <select v-model="seatType" class="input w-auto min-w-36 text-sm" :disabled="inviting">
            <option value="default">{{ t('admin.accounts.workspaceInfo.seatTypes.default') }}</option>
            <option value="prolite">{{ t('admin.accounts.workspaceInfo.seatTypes.prolite') }}</option>
            <option value="usage_based">{{ t('admin.accounts.workspaceInfo.seatTypes.usage_based') }}</option>
            <option value="automation">{{ t('admin.accounts.workspaceInfo.seatTypes.automation') }}</option>
          </select>
        </div>
        <textarea
          v-model="emailInput"
          class="input min-h-20 w-full resize-y text-sm"
          :placeholder="t('admin.accounts.workspaceInfo.emailPlaceholder')"
          :disabled="inviting"
        />
        <div class="flex items-center justify-between gap-3">
          <p v-if="inviteError" class="text-xs text-red-600 dark:text-red-400">{{ inviteError }}</p>
          <span v-else></span>
          <button type="submit" class="btn btn-primary btn-sm inline-flex shrink-0 items-center gap-1.5" :disabled="inviting || !emailInput.trim()">
            <Icon name="userPlus" size="sm" :class="inviting ? 'animate-pulse' : ''" />
            {{ inviting ? t('admin.accounts.workspaceInfo.inviting') : t('admin.accounts.workspaceInfo.invite') }}
          </button>
        </div>
        <div v-if="lastInvite" class="space-y-1 text-xs">
          <p v-if="lastInvite.account_invites.length" class="text-emerald-600 dark:text-emerald-400">
            {{ t('admin.accounts.workspaceInfo.inviteSuccess', { count: lastInvite.account_invites.length }) }}
          </p>
          <p v-if="lastInvite.errored_emails.length" class="break-words text-amber-600 dark:text-amber-400">
            {{ t('admin.accounts.workspaceInfo.inviteFailed', { emails: lastInvite.errored_emails.join(', ') }) }}
          </p>
        </div>
      </form>

      <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700">
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.workspaceInfo.inviteListTitle') }}</h4>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5" :disabled="inviteListLoading" @click="loadInvites">
            <Icon name="refresh" size="sm" :class="inviteListLoading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>
        <div class="flex gap-2">
          <input
            v-model="inviteQuery"
            class="input min-w-0 flex-1 text-sm"
            :placeholder="t('admin.accounts.workspaceInfo.inviteQueryPlaceholder')"
            :disabled="inviteListLoading"
            @keyup.enter="refreshInvites"
          />
          <button type="button" class="btn btn-secondary btn-sm" :disabled="inviteListLoading" @click="refreshInvites">
            {{ t('admin.accounts.workspaceInfo.search') }}
          </button>
        </div>
        <p v-if="inviteListError" class="text-xs text-red-600 dark:text-red-400">{{ inviteListError }}</p>
        <div v-if="inviteListLoading" class="flex items-center justify-center py-6 text-sm text-gray-500 dark:text-dark-400">
          <Icon name="refresh" size="md" class="mr-2 animate-spin" />
          {{ t('admin.accounts.workspaceInfo.inviteListLoading') }}
        </div>
        <div v-else-if="inviteItems.length === 0" class="rounded-lg border border-dashed border-gray-300 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
          {{ t('admin.accounts.workspaceInfo.inviteListEmpty') }}
        </div>
        <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <div
            v-for="invite in inviteItems"
            :key="invite.id"
            class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-3 py-2.5 last:border-b-0 dark:border-dark-700"
          >
            <div class="min-w-0">
              <p class="break-all text-sm text-gray-800 dark:text-dark-100">{{ invite.email_address }}</p>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-dark-500">
                {{ invite.seat_type }} · {{ formatDate(invite.created_time) }}
              </p>
            </div>
            <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">
              {{ t('admin.accounts.workspaceInfo.inviteStatus', { status: invite.status }) }}
            </span>
          </div>
        </div>
        <div v-if="inviteTotal > inviteLimit" class="flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-dark-400">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="inviteListLoading || inviteOffset === 0" @click="changeInvitePage(-1)">
            {{ t('common.previous') }}
          </button>
          <span>{{ inviteOffset + 1 }}-{{ Math.min(inviteOffset + inviteLimit, inviteTotal) }} / {{ inviteTotal }}</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="inviteListLoading || inviteOffset + inviteLimit >= inviteTotal" @click="changeInvitePage(1)">
            {{ t('common.next') }}
          </button>
        </div>
      </section>

      <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700">
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.workspaceInfo.userListTitle') }}</h4>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5" :disabled="userListLoading" @click="loadUsers">
            <Icon name="refresh" size="sm" :class="userListLoading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>
        <div class="flex gap-2">
          <input
            v-model="userQuery"
            class="input min-w-0 flex-1 text-sm"
            :placeholder="t('admin.accounts.workspaceInfo.userQueryPlaceholder')"
            :disabled="userListLoading"
            @keyup.enter="refreshUsers"
          />
          <button type="button" class="btn btn-secondary btn-sm" :disabled="userListLoading" @click="refreshUsers">
            {{ t('admin.accounts.workspaceInfo.search') }}
          </button>
        </div>
        <p v-if="userListError" class="text-xs text-red-600 dark:text-red-400">{{ userListError }}</p>
        <div v-if="userListLoading" class="flex items-center justify-center py-6 text-sm text-gray-500 dark:text-dark-400">
          <Icon name="refresh" size="md" class="mr-2 animate-spin" />
          {{ t('admin.accounts.workspaceInfo.userListLoading') }}
        </div>
        <div v-else-if="userItems.length === 0" class="rounded-lg border border-dashed border-gray-300 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
          {{ t('admin.accounts.workspaceInfo.userListEmpty') }}
        </div>
        <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <div
            v-for="user in userItems"
            :key="user.id"
            class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-3 py-2.5 last:border-b-0 dark:border-dark-700"
          >
            <div class="min-w-0">
              <p class="break-all text-sm text-gray-800 dark:text-dark-100">{{ user.name || user.email }}</p>
              <p class="break-all text-xs text-gray-500 dark:text-dark-400">{{ user.email }}</p>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-dark-500">
                {{ user.role }} / {{ user.seat_type }} / {{ formatDate(user.created_time) }}
              </p>
            </div>
            <span v-if="user.deactivated_time" class="rounded-full bg-red-100 px-2 py-0.5 text-xs text-red-700 dark:bg-red-900/40 dark:text-red-300">
              {{ t('admin.accounts.workspaceInfo.deactivated') }}
            </span>
          </div>
        </div>
        <div v-if="userTotal > userLimit" class="flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-dark-400">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="userListLoading || userOffset === 0" @click="changeUserPage(-1)">
            {{ t('common.previous') }}
          </button>
          <span>{{ userOffset + 1 }}-{{ Math.min(userOffset + userLimit, userTotal) }} / {{ userTotal }}</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="userListLoading || userOffset + userLimit >= userTotal" @click="changeUserPage(1)">
            {{ t('common.next') }}
          </button>
        </div>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getOpenAIWorkspaceInfo, inviteOpenAIWorkspaceMembers, listOpenAIWorkspaceInvites, listOpenAIWorkspaceUsers, type OpenAIWorkspaceInfo, type OpenAIWorkspaceInviteListResult, type OpenAIWorkspaceInviteResult, type OpenAIWorkspaceUserListResult } from '@/api/admin/accounts'
import Icon from '@/components/icons/Icon.vue'
import type { Account } from '@/types'

const emit = defineEmits<{ (event: 'close'): void; (event: 'invited'): void; (event: 'info-updated', info: OpenAIWorkspaceInfo): void }>()
const { t } = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
  info: OpenAIWorkspaceInfo | null
}>()

const emailInput = ref('')
const seatType = ref('default')
const inviting = ref(false)
const inviteError = ref('')
const lastInvite = ref<OpenAIWorkspaceInviteResult | null>(null)
const inviteItems = ref<OpenAIWorkspaceInviteListResult['items']>([])
const inviteTotal = ref(0)
const inviteOffset = ref(0)
const inviteLimit = 25
const inviteQuery = ref('')
const inviteListLoading = ref(false)
const inviteListError = ref('')
const userItems = ref<OpenAIWorkspaceUserListResult['items']>([])
const userTotal = ref(0)
const userOffset = ref(0)
const userLimit = 25
const userQuery = ref('')
const userListLoading = ref(false)
const userListError = ref('')

watch(() => props.show, (show) => {
  if (show) {
    emailInput.value = ''
    seatType.value = 'default'
    inviteError.value = ''
    lastInvite.value = null
    inviteItems.value = []
    inviteTotal.value = 0
    inviteOffset.value = 0
    inviteQuery.value = ''
    inviteListError.value = ''
    userItems.value = []
    userTotal.value = 0
    userOffset.value = 0
    userQuery.value = ''
    userListError.value = ''
    void loadInvites()
    void loadUsers()
  }
})

const seatEntries = computed(() => {
  const counts = props.info?.seat_type_counts || {}
  return [
    { key: 'default', count: counts.default ?? 0 },
    { key: 'usage_based', count: counts.usage_based ?? 0 },
    { key: 'automation', count: counts.automation ?? 0 },
    { key: 'prolite', count: counts.prolite ?? 0 }
  ]
})

function formatDate(value: string | number | undefined) {
  if (!value) return '-'
  if (typeof value === 'number') return formatDateTime(new Date(value * 1000))
  return formatDateTime(value)
}

async function submitInvite() {
  if (!props.account || !props.info?.account_id || inviting.value) return
  const emails = emailInput.value
    .split(/[\n,;]+/)
    .map(email => email.trim())
    .filter(Boolean)
  if (emails.length === 0) return

  inviting.value = true
  inviteError.value = ''
  lastInvite.value = null
  try {
    lastInvite.value = await inviteOpenAIWorkspaceMembers(
      props.account.id,
      props.info.account_id,
      emails,
      seatType.value
    )
    emailInput.value = ''
    try {
      const refreshedInfo = await getOpenAIWorkspaceInfo(props.account.id)
      emit('info-updated', refreshedInfo)
    } catch {
      // The invitation has already succeeded; the next manual refresh can
      // update the seat counts when the metadata request is transiently slow.
    }
    await loadInvites()
    emit('invited')
  } catch (error) {
    inviteError.value = extractApiErrorMessage(error, t('admin.accounts.workspaceInfo.inviteFailedGeneric'))
  } finally {
    inviting.value = false
  }
}

async function loadInvites() {
  if (!props.account || !props.info?.account_id) return
  inviteListLoading.value = true
  inviteListError.value = ''
  try {
    const result = await listOpenAIWorkspaceInvites(props.account.id, props.info.account_id, {
      offset: inviteOffset.value,
      limit: inviteLimit,
      query: inviteQuery.value
    })
    inviteItems.value = result.items
    inviteTotal.value = result.total
    inviteOffset.value = result.offset
  } catch (error) {
    inviteListError.value = extractApiErrorMessage(error, t('admin.accounts.workspaceInfo.inviteListFailed'))
  } finally {
    inviteListLoading.value = false
  }
}

function refreshInvites() {
  inviteOffset.value = 0
  void loadInvites()
}

function changeInvitePage(direction: number) {
  const nextOffset = inviteOffset.value + direction * inviteLimit
  if (nextOffset < 0 || nextOffset >= inviteTotal.value) return
  inviteOffset.value = nextOffset
  void loadInvites()
}

async function loadUsers() {
  if (!props.account || !props.info?.account_id) return
  userListLoading.value = true
  userListError.value = ''
  try {
    const result = await listOpenAIWorkspaceUsers(props.account.id, props.info.account_id, {
      offset: userOffset.value,
      limit: userLimit,
      query: userQuery.value
    })
    userItems.value = result.items
    userTotal.value = result.total
    userOffset.value = result.offset
  } catch (error) {
    userListError.value = extractApiErrorMessage(error, t('admin.accounts.workspaceInfo.userListFailed'))
  } finally {
    userListLoading.value = false
  }
}

function refreshUsers() {
  userOffset.value = 0
  void loadUsers()
}

function changeUserPage(direction: number) {
  const nextOffset = userOffset.value + direction * userLimit
  if (nextOffset < 0 || nextOffset >= userTotal.value) return
  userOffset.value = nextOffset
  void loadUsers()
}
</script>
