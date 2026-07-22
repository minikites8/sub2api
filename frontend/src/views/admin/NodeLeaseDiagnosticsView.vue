<template>
  <AppLayout>
    <div class="lease-diagnostics-page">
      <section class="diagnostics-panel">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">租约状态诊断</h1>
              <span :class="['badge', healthBadgeClass(diagnostics?.health)]">
                {{ healthLabel(diagnostics?.health) }}
              </span>
            </div>
            <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ diagnostics ? `更新时间 ${formatTime(diagnostics.generated_at)}` : '等待加载' }}
            </div>
            <div class="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
              <div class="stat-tile">
                <span class="stat-label">问题</span>
                <span class="stat-value">{{ stats.issue_count }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">异常</span>
                <span class="stat-value">{{ stats.critical_count }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">透支租约</span>
                <span class="stat-value">{{ stats.overdraft_leases }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">待上传</span>
                <span class="stat-value">{{ pendingTotal }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">活跃额度</span>
                <span class="stat-value">{{ formatAmount(stats.remaining_total) }}</span>
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-end gap-2 xl:min-w-[520px]">
            <div class="w-full sm:w-44">
              <Select v-model="nodeFilter" :options="nodeFilterOptions" size="sm" />
            </div>
            <div class="w-full sm:w-36">
              <Select v-model="levelFilter" :options="levelFilterOptions" size="sm" />
            </div>
            <RouterLink to="/admin/node-leases" class="btn btn-secondary">
              <Icon name="server" size="sm" class="mr-2" />
              节点租约
            </RouterLink>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadDiagnostics">
              <Icon name="refresh" size="sm" class="mr-2" :class="loading ? 'animate-spin' : ''" />
              刷新
            </button>
          </div>
        </div>
      </section>

      <section class="diagnostics-panel">
        <div class="panel-heading">
          <h2>问题列表</h2>
          <span>{{ filteredIssues.length }}</span>
        </div>
        <DataTable
          :columns="issueColumns"
          :data="filteredIssues"
          :loading="loading"
          row-key="id"
          :sticky-actions-column="false"
        >
          <template #cell-level="{ value }">
            <span :class="['badge', healthBadgeClass(value)]">{{ healthLabel(value) }}</span>
          </template>
          <template #cell-scope="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ scopeLabel(value) }}</span>
          </template>
          <template #cell-message="{ row }">
            <div class="min-w-[220px]">
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ row.message }}</div>
              <div v-if="row.detail" class="mt-1 max-w-[360px] truncate text-xs text-gray-500 dark:text-gray-400" :title="row.detail">
                {{ row.detail }}
              </div>
            </div>
          </template>
          <template #cell-node_id="{ value }">
            <code v-if="value" class="code-cell max-w-[160px]" :title="value">{{ value }}</code>
            <span v-else>-</span>
          </template>
          <template #cell-user_id="{ row }">
            <RouterLink v-if="row.user_id" :to="userLink(row.user_id)" class="table-link">
              {{ userLabel(row.user_id) }}
            </RouterLink>
            <span v-else>-</span>
          </template>
          <template #cell-api_key_id="{ value }">
            <span v-if="value" class="font-mono text-xs">#{{ value }}</span>
            <span v-else>-</span>
          </template>
          <template #cell-lease_id="{ value }">
            <code v-if="value" class="code-cell max-w-[180px]" :title="value">{{ value }}</code>
            <span v-else>-</span>
          </template>
          <template #empty>
            <EmptyState title="暂无问题" description="当前租约状态正常" />
          </template>
        </DataTable>
      </section>

      <section class="diagnostics-panel">
        <div class="panel-heading">
          <h2>用户租约</h2>
          <span>{{ filteredUsers.length }}</span>
        </div>
        <DataTable
          :columns="userColumns"
          :data="filteredUsers"
          :loading="loading"
          row-key="user_id"
          :sticky-actions-column="false"
        >
          <template #cell-user="{ row }">
            <RouterLink :to="userLink(row.user_id)" class="table-link">
              {{ userDisplayName(row) }}
            </RouterLink>
            <div class="mt-1 font-mono text-xs text-gray-500 dark:text-gray-400">#{{ row.user_id }}</div>
          </template>
          <template #cell-health="{ value }">
            <span :class="['badge', healthBadgeClass(value)]">{{ healthLabel(value) }}</span>
          </template>
          <template #cell-balance="{ row }">
            <span class="font-mono text-xs">{{ nullableAmount(row.balance) }}</span>
          </template>
          <template #cell-active_remaining="{ value }">
            <span :class="['font-mono text-xs', Number(value || 0) < 0 ? 'text-red-600 dark:text-red-300' : '']">
              {{ formatAmount(value || 0) }}
            </span>
          </template>
          <template #cell-overdraft_amount="{ value }">
            <span :class="['font-mono text-xs', Number(value || 0) > 0 ? 'text-red-600 dark:text-red-300' : '']">
              {{ formatAmount(value || 0) }}
            </span>
          </template>
          <template #cell-lease_count="{ row }">
            <span class="font-mono text-xs">{{ row.active_lease_count }} / {{ row.lease_count }}</span>
          </template>
          <template #cell-api_key_ids="{ value }">
            <span class="block max-w-[180px] truncate font-mono text-xs" :title="apiKeyList(value)">
              {{ apiKeyList(value) || '-' }}
            </span>
          </template>
          <template #cell-issues="{ row }">
            <span class="block max-w-[260px] truncate text-xs text-gray-600 dark:text-gray-300" :title="issueList(row.issues)">
              {{ issueList(row.issues) || '-' }}
            </span>
          </template>
          <template #empty>
            <EmptyState title="暂无用户租约" description="产生租约后会显示在这里" />
          </template>
        </DataTable>
      </section>

      <section class="diagnostics-panel">
        <div class="panel-heading">
          <h2>节点链路</h2>
          <span>{{ filteredNodes.length }}</span>
        </div>
        <DataTable
          :columns="nodeColumns"
          :data="filteredNodes"
          :loading="loading"
          row-key="node_id"
          :sticky-actions-column="false"
        >
          <template #cell-node_id="{ value }">
            <code class="code-cell max-w-[180px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-health="{ value }">
            <span :class="['badge', healthBadgeClass(value)]">{{ healthLabel(value) }}</span>
          </template>
          <template #cell-status="{ value }">
            <span :class="['badge', statusBadgeClass(value)]">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-heartbeat_age_seconds="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ heartbeatLabel(row) }}</span>
          </template>
          <template #cell-sync="{ row }">
            <div class="sync-state-cell">
              <span :class="['badge', syncStatusBadgeClass(row)]">{{ syncStatusLabel(row) }}</span>
              <span class="sync-time">{{ syncStatusDetail(row) }}</span>
            </div>
          </template>
          <template #cell-pending="{ row }">
            <div class="sync-pill-group">
              <span :class="['sync-pill', pendingClass(row.pending_usage_events)]">扣费 {{ row.pending_usage_events }}</span>
              <span :class="['sync-pill', pendingClass(row.pending_usage_logs)]">记录 {{ row.pending_usage_logs }}</span>
              <span :class="['sync-pill', pendingClass(row.pending_ops_error_logs)]">错误 {{ row.pending_ops_error_logs }}</span>
            </div>
          </template>
          <template #cell-active_remaining="{ value }">
            <span class="font-mono text-xs">{{ formatAmount(value || 0) }}</span>
          </template>
          <template #cell-issues="{ row }">
            <span class="block max-w-[260px] truncate text-xs text-gray-600 dark:text-gray-300" :title="issueList(row.issues)">
              {{ issueList(row.issues) || '-' }}
            </span>
          </template>
          <template #empty>
            <EmptyState title="暂无节点" description="注册节点后会显示在这里" />
          </template>
        </DataTable>
      </section>

      <section class="diagnostics-panel">
        <div class="panel-heading">
          <h2>异常租约</h2>
          <span>{{ filteredProblemLeases.length }}</span>
        </div>
        <DataTable
          :columns="leaseColumns"
          :data="filteredProblemLeases"
          :loading="loading"
          row-key="id"
          :sticky-actions-column="false"
        >
          <template #cell-id="{ value }">
            <code class="code-cell max-w-[200px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-health="{ value }">
            <span :class="['badge', healthBadgeClass(value)]">{{ healthLabel(value) }}</span>
          </template>
          <template #cell-user_id="{ row }">
            <RouterLink :to="userLink(row.user_id)" class="table-link">
              {{ userLabel(row.user_id) }}
            </RouterLink>
          </template>
          <template #cell-amounts="{ row }">
            <span class="font-mono text-xs">{{ formatAmount(row.consumed) }} / {{ formatAmount(row.granted) }}</span>
          </template>
          <template #cell-remaining="{ row }">
            <span :class="['font-mono text-xs', row.remaining < 0 ? 'text-red-600 dark:text-red-300' : '']">
              {{ formatAmount(row.remaining) }}
            </span>
          </template>
          <template #cell-usage_event_total="{ row }">
            <span class="font-mono text-xs">{{ formatAmount(row.usage_event_total) }}</span>
          </template>
          <template #cell-updated_at="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>
          <template #cell-issues="{ row }">
            <span class="block max-w-[280px] truncate text-xs text-gray-600 dark:text-gray-300" :title="issueList(row.issues)">
              {{ issueList(row.issues) || '-' }}
            </span>
          </template>
          <template #empty>
            <EmptyState title="暂无异常租约" description="透支、低额度、流水异常会显示在这里" />
          </template>
        </DataTable>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type {
  QuotaLeaseDiagnostics,
  QuotaLeaseNodeDiagnostic,
  QuotaLeaseUserDiagnostic
} from '@/api/admin/nodeLeases'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const appStore = useAppStore()

const diagnostics = ref<QuotaLeaseDiagnostics | null>(null)
const loading = ref(false)
const nodeFilter = ref('')
const levelFilter = ref('')
let autoRefreshTimer: number | null = null

const emptyStats = {
  node_count: 0,
  online_nodes: 0,
  user_count: 0,
  lease_count: 0,
  active_leases: 0,
  expired_leases: 0,
  closed_leases: 0,
  reclaimed_leases: 0,
  overdraft_leases: 0,
  low_capacity_leases: 0,
  granted_total: 0,
  consumed_total: 0,
  reclaimed_total: 0,
  remaining_total: 0,
  overdraft_total: 0,
  event_count: 0,
  pending_usage_events: 0,
  pending_usage_logs: 0,
  pending_ops_error_logs: 0,
  issue_count: 0,
  warning_count: 0,
  critical_count: 0
}

const stats = computed(() => diagnostics.value?.stats || emptyStats)
const pendingTotal = computed(() => stats.value.pending_usage_events + stats.value.pending_usage_logs + stats.value.pending_ops_error_logs)

const nodeFilterOptions = computed(() => [
  { value: '', label: '全部节点' },
  ...(diagnostics.value?.nodes || []).map((node) => ({
    value: node.node_id,
    label: `${node.node_id}${node.region ? ` (${node.region})` : ''}`
  }))
])

const levelFilterOptions = [
  { value: '', label: '全部状态' },
  { value: 'critical', label: '异常' },
  { value: 'warning', label: '关注' },
  { value: 'ok', label: '正常' }
]

const issueColumns = computed<Column[]>(() => [
  { key: 'level', label: '状态', sortable: true },
  { key: 'scope', label: '范围', sortable: true },
  { key: 'message', label: '问题', sortable: false },
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'user_id', label: '用户', sortable: true },
  { key: 'api_key_id', label: 'Key', sortable: true },
  { key: 'lease_id', label: '租约', sortable: true }
])

const userColumns = computed<Column[]>(() => [
  { key: 'user', label: '用户', sortable: false },
  { key: 'health', label: '状态', sortable: true },
  { key: 'balance', label: '余额', sortable: true },
  { key: 'active_remaining', label: '活跃额度', sortable: true },
  { key: 'overdraft_amount', label: '透支', sortable: true },
  { key: 'lease_count', label: '活跃 / 全部租约', sortable: false },
  { key: 'api_key_ids', label: 'API Key', sortable: false },
  { key: 'issues', label: '问题', sortable: false }
])

const nodeColumns = computed<Column[]>(() => [
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'health', label: '状态', sortable: true },
  { key: 'status', label: '运行', sortable: true },
  { key: 'heartbeat_age_seconds', label: '心跳', sortable: true },
  { key: 'sync', label: '同步', sortable: false },
  { key: 'pending', label: '待上传', sortable: false },
  { key: 'active_lease_count', label: '活跃租约', sortable: true },
  { key: 'active_remaining', label: '活跃额度', sortable: true },
  { key: 'issues', label: '问题', sortable: false }
])

const leaseColumns = computed<Column[]>(() => [
  { key: 'id', label: '租约', sortable: true },
  { key: 'health', label: '状态', sortable: true },
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'user_id', label: '用户', sortable: true },
  { key: 'api_key_id', label: 'Key', sortable: true },
  { key: 'amounts', label: '消费 / 分配', sortable: false },
  { key: 'remaining', label: '剩余', sortable: true },
  { key: 'usage_event_total', label: '流水消费', sortable: true },
  { key: 'updated_at', label: '更新时间', sortable: true },
  { key: 'issues', label: '问题', sortable: false }
])

const usersByID = computed(() => {
  const out = new Map<number, QuotaLeaseUserDiagnostic>()
  for (const user of diagnostics.value?.users || []) {
    out.set(user.user_id, user)
  }
  return out
})

const filteredIssues = computed(() => {
  return (diagnostics.value?.issues || []).filter((issue) => {
    if (levelFilter.value && issue.level !== levelFilter.value) return false
    if (nodeFilter.value && issue.node_id !== nodeFilter.value) return false
    return true
  })
})

const filteredNodes = computed(() => {
  return (diagnostics.value?.nodes || []).filter((node) => {
    if (nodeFilter.value && node.node_id !== nodeFilter.value) return false
    if (levelFilter.value && node.health !== levelFilter.value) return false
    return true
  })
})

const filteredProblemLeases = computed(() => {
  return (diagnostics.value?.leases || []).filter((lease) => {
    if (lease.health === 'ok') return false
    if (nodeFilter.value && lease.node_id !== nodeFilter.value) return false
    if (levelFilter.value && lease.health !== levelFilter.value) return false
    return true
  })
})

const filteredUserIDsForNode = computed(() => {
  if (!nodeFilter.value) return null
  const ids = new Set<number>()
  for (const lease of diagnostics.value?.leases || []) {
    if (lease.node_id === nodeFilter.value) {
      ids.add(lease.user_id)
    }
  }
  return ids
})

const filteredUsers = computed(() => {
  const nodeUserIDs = filteredUserIDsForNode.value
  return (diagnostics.value?.users || []).filter((user) => {
    if (nodeUserIDs && !nodeUserIDs.has(user.user_id)) return false
    if (levelFilter.value && user.health !== levelFilter.value) return false
    return true
  })
})

async function loadDiagnostics() {
  loading.value = true
  try {
    diagnostics.value = await adminAPI.nodeLeases.getDiagnostics()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '租约诊断加载失败'))
  } finally {
    loading.value = false
  }
}

function healthLabel(value?: string | null) {
  if (value === 'critical') return '异常'
  if (value === 'warning') return '关注'
  if (value === 'ok') return '正常'
  return '待加载'
}

function healthBadgeClass(value?: string | null) {
  if (value === 'critical') return 'badge-danger'
  if (value === 'warning') return 'badge-warning'
  if (value === 'ok') return 'badge-success'
  return 'badge-gray'
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    online: '在线',
    offline: '离线',
    disabled: '禁用',
    active: '活跃',
    expired: '过期',
    reclaimed: '已回收',
    closed: '已关闭'
  }
  return labels[status] || status || '-'
}

function statusBadgeClass(status: string) {
  if (status === 'online' || status === 'active' || status === 'closed') return 'badge-success'
  if (status === 'expired') return 'badge-warning'
  if (status === 'offline' || status === 'disabled') return 'badge-danger'
  return 'badge-gray'
}

function scopeLabel(scope: string) {
  const labels: Record<string, string> = {
    system: '系统',
    node: '节点',
    user: '用户',
    lease: '租约'
  }
  return labels[scope] || scope || '-'
}

function userDisplayName(user: QuotaLeaseUserDiagnostic) {
  return user.username?.trim() || user.email?.trim() || `用户 #${user.user_id}`
}

function userLabel(userId?: number) {
  if (!userId) return '-'
  const user = usersByID.value.get(userId)
  return user ? userDisplayName(user) : `用户 #${userId}`
}

function userLink(userId: number) {
  const label = userLabel(userId)
  return { path: '/admin/users', query: { search: label === `用户 #${userId}` ? String(userId) : label } }
}

function apiKeyList(value?: number[]) {
  return (value || []).map((id) => `#${id}`).join(', ')
}

function issueList(value?: string[]) {
  return (value || []).join('，')
}

function nullableAmount(value?: number | null) {
  return typeof value === 'number' ? formatAmount(value) : '-'
}

function formatAmount(value: number) {
  const amount = Number(value || 0)
  return amount.toFixed(6).replace(/\.?0+$/, '')
}

function formatTime(value?: string | null) {
  return value ? formatDateTime(value) : '-'
}

function formatAge(seconds?: number | null) {
  if (seconds === null || seconds === undefined) return '-'
  const value = Math.max(0, Number(seconds || 0))
  if (value < 60) return `${Math.round(value)}秒`
  const minutes = Math.floor(value / 60)
  if (minutes < 60) return `${minutes}分${Math.round(value % 60)}秒`
  const hours = Math.floor(minutes / 60)
  return `${hours}时${minutes % 60}分`
}

function heartbeatLabel(node: QuotaLeaseNodeDiagnostic) {
  const age = formatAge(node.heartbeat_age_seconds)
  return node.last_heartbeat_at ? `${age}前` : '-'
}

function syncStatusLabel(node: QuotaLeaseNodeDiagnostic) {
  const status = node.sync_status
  if (!status) return '待上报'
  if (status.last_sync_error) return '同步异常'
  if (status.last_sync_success_at || status.mirror_synced_at) return '已同步'
  if (status.last_sync_started_at) return '同步中'
  return '待同步'
}

function syncStatusBadgeClass(node: QuotaLeaseNodeDiagnostic) {
  const status = node.sync_status
  if (!status) return 'badge-gray'
  if (status.last_sync_error) return 'badge-danger'
  if (status.last_sync_success_at || status.mirror_synced_at) return 'badge-success'
  if (status.last_sync_started_at) return 'badge-warning'
  return 'badge-gray'
}

function syncStatusDetail(node: QuotaLeaseNodeDiagnostic) {
  const status = node.sync_status
  if (!status) return '-'
  const parts: string[] = []
  if (Number(status.mirror_version || 0) > 0) {
    parts.push(`v${status.mirror_version}`)
  }
  if (status.last_sync_mode === 'delta') {
    parts.push('增量')
  } else if (status.last_sync_mode === 'full') {
    parts.push('全量')
  }
  const time = status.last_sync_success_at || status.mirror_synced_at || status.last_sync_started_at
  if (time) {
    parts.push(formatTime(time))
  }
  return parts.length > 0 ? parts.join(' · ') : '-'
}

function pendingClass(count: number) {
  return count > 0 ? 'sync-pill-warning' : ''
}

onMounted(() => {
  void loadDiagnostics()
  autoRefreshTimer = window.setInterval(loadDiagnostics, 30_000)
})

onUnmounted(() => {
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>

<style scoped>
.lease-diagnostics-page {
  @apply space-y-5;
}

.diagnostics-panel {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
  @apply p-4;
}

.panel-heading {
  @apply mb-3 flex items-center justify-between gap-3;
}

.panel-heading h2 {
  @apply text-base font-semibold text-gray-900 dark:text-white;
}

.panel-heading span {
  @apply rounded-full px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-gray-300;
  background: var(--md-surface-container-low);
}

.stat-tile {
  @apply flex min-h-[64px] flex-col justify-center rounded-lg px-3 py-2;
  border: 1px solid var(--md-outline-variant);
  background: var(--md-surface-container-low);
}

.stat-label {
  @apply text-xs font-medium text-gray-500 dark:text-gray-400;
}

.stat-value {
  @apply mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white;
}

.code-cell {
  @apply block truncate rounded-md px-2 py-1 font-mono text-xs text-gray-700 dark:text-gray-200;
  background: var(--md-surface-container-low);
}

.table-link {
  @apply inline-flex max-w-[180px] truncate text-sm font-medium text-primary-600 hover:underline dark:text-primary-400;
}

.sync-state-cell {
  @apply flex min-w-[120px] flex-col gap-1;
}

.sync-time {
  @apply text-xs text-gray-500 dark:text-gray-400;
}

.sync-pill-group {
  @apply flex max-w-[240px] flex-wrap gap-1;
}

.sync-pill {
  @apply rounded-md px-2 py-0.5 font-mono text-xs text-gray-600 dark:text-gray-300;
  background: var(--md-surface-container-low);
}

.sync-pill-warning {
  @apply text-amber-700 dark:text-amber-300;
  background: rgb(254 243 199 / 0.8);
}

:global(.dark) .sync-pill-warning {
  background: rgb(120 53 15 / 0.35);
}

.diagnostics-panel :deep(.table-wrapper) {
  max-height: 440px;
  overflow: auto;
}

.diagnostics-panel :deep(.data-table-surface) {
  min-width: 1180px;
}
</style>
