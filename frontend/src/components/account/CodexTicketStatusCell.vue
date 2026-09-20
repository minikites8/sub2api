<template>
  <div v-if="eligible && account.codex_tickets?.length" ref="root" class="mb-1 space-y-0.5" data-testid="codex-ticket-status">
    <button
      v-for="status in statuses" :key="status.model" type="button"
      class="grid w-full max-w-[250px] grid-cols-[minmax(38px,1fr)_auto_auto] items-center gap-x-3 rounded px-1 py-0.5 text-left text-[10px] leading-4 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700"
      :aria-label="t('admin.accounts.codexTickets.viewLogs', { model: status.model })"
      :title="status.model" @click.stop="selectedModel = status.model"
    >
      <span class="truncate font-mono text-gray-500 dark:text-gray-400">{{ codexTicketModelLabel(status.model) }}</span>
      <span class="whitespace-nowrap tabular-nums" :class="codexTicketStateClass(state(status))">
        {{ state(status) === 'ready' ? codexTicketDuration(codexTicketRemaining(status, receivedAt, now)) : t(`admin.accounts.codexTickets.states.${state(status)}`) }}
      </span>
      <span class="whitespace-nowrap tabular-nums text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.codexTickets.attempts', { count: status.attempts }) }}
      </span>
    </button>
    <CodexTicketLogsDialog v-if="selectedModel" :show="true" :account="account" :model="selectedModel" @close="selectedModel = ''" @status="updateModelStatus" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import type { CodexTicketStatus } from '@/types/codexTicket'
import { subscribeCodexTicketStatuses } from '@/composables/useCodexTicketStatuses'
import { codexTicketModelLabel, codexTicketRemaining, codexTicketDuration, codexTicketState, codexTicketStateClass } from '@/utils/codexTicketDisplay'
import CodexTicketLogsDialog from './CodexTicketLogsDialog.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const eligible = computed(() => props.account.platform === 'openai' && ['oauth', 'setup-token'].includes(props.account.type) && !props.account.parent_account_id)
const root = ref<HTMLElement | null>(null)
const statuses = ref<CodexTicketStatus[]>([])
const now = ref(Date.now())
const receivedAt = ref(Date.now())
const selectedModel = ref('')
let unsubscribe: (() => void) | undefined
let observer: IntersectionObserver | undefined
let clock: ReturnType<typeof setInterval> | undefined
let mounted = false
let generation = 0
const state = (status: CodexTicketStatus) => codexTicketState(status, receivedAt.value, now.value)
function accept(snapshot: CodexTicketStatus[]) {
  statuses.value = snapshot
  receivedAt.value = Date.now()
  now.value = Date.now()
}
function updateModelStatus(status: CodexTicketStatus) {
  const elapsed = Math.max(0, (Date.now() - receivedAt.value) / 1000)
  accept(statuses.value.map(item => item.model === status.model ? status : { ...item, remaining_seconds: Math.max(0, item.remaining_seconds - elapsed) }))
}
function stop() {
  unsubscribe?.(); unsubscribe = undefined
  observer?.disconnect(); observer = undefined
  if (clock) clearInterval(clock)
  clock = undefined
}
function activate() {
  if (unsubscribe) return
  unsubscribe = subscribeCodexTicketStatuses(props.account.id, accept)
  clock = setInterval(() => { if (document.visibilityState !== 'hidden') now.value = Date.now() }, 1000)
}
async function setup() {
  const run = ++generation
  stop()
  if (!mounted || !eligible.value || !props.account.codex_tickets?.length) return
  await nextTick()
  if (run !== generation || !mounted || !root.value) return
  if (typeof IntersectionObserver === 'undefined') { activate(); return }
  observer = new IntersectionObserver(([entry]) => {
    if (entry?.isIntersecting) { activate() } else {
      unsubscribe?.(); unsubscribe = undefined
      if (clock) clearInterval(clock)
      clock = undefined
    }
  }, { rootMargin: '100px' })
  observer.observe(root.value)
}
watch(() => [props.account.id, props.account.codex_tickets] as const, ([id, snapshot], previous) => {
  accept(snapshot ?? [])
  if (id !== previous?.[0]) selectedModel.value = ''
  void setup()
}, { immediate: true })
onMounted(() => { mounted = true; void setup() })
onBeforeUnmount(() => { mounted = false; generation++; stop() })
</script>
