<template>
  <AppLayout>
    <div class="node-leases-page">
      <section class="node-panel">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div class="min-w-0">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">节点租约 Demo</h1>
            <div class="mt-2 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
              <div class="stat-tile">
                <span class="stat-label">节点</span>
                <span class="stat-value">{{ stats.node_count }} / {{ stats.online_nodes }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">活跃租约</span>
                <span class="stat-value">{{ stats.active_leases }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">剩余额度</span>
                <span class="stat-value">{{ formatAmount(stats.remaining_total) }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-label">流水</span>
                <span class="stat-value">{{ stats.event_count }}</span>
              </div>
            </div>
          </div>

          <div class="flex flex-col gap-3 xl:min-w-[420px]">
            <div class="flex flex-wrap items-center justify-end gap-2">
              <div class="w-full sm:w-40">
                <Select v-model="nodeFilter" :options="nodeFilterOptions" size="sm" />
              </div>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAll">
                <Icon name="refresh" size="sm" class="mr-2" :class="loading ? 'animate-spin' : ''" />
                刷新
              </button>
              <button type="button" class="btn btn-secondary" @click="openLeaseSettingsDialog">
                <Icon name="cog" size="sm" class="mr-2" />
                租约设置
              </button>
              <button type="button" class="btn btn-secondary" :disabled="reclaiming" @click="reclaimExpired">
                <Icon name="sync" size="sm" class="mr-2" :class="reclaiming ? 'animate-spin' : ''" />
                回收
              </button>
              <button type="button" class="btn btn-secondary" @click="openRegisterNodeDialog">
                <Icon name="server" size="sm" class="mr-2" />
                注册节点
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="node-panel">
        <div class="panel-heading">
          <h2>节点</h2>
          <span>{{ nodes.length }}</span>
        </div>
        <DataTable
          :columns="nodeColumns"
          :data="nodes"
          :loading="loading"
          row-key="node_id"
          :sticky-actions-column="false"
        >
          <template #cell-node_id="{ value }">
            <code class="code-cell">{{ value }}</code>
          </template>
          <template #cell-region="{ value }">
            <span>{{ value || '-' }}</span>
          </template>
          <template #cell-status="{ value }">
            <span :class="['badge', statusBadgeClass(value)]">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-lease_remaining="{ value }">
            <span class="font-mono">{{ formatAmount(value || 0) }}</span>
          </template>
          <template #cell-last_heartbeat_at="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>
          <template #cell-base_url="{ value }">
            <span class="block max-w-[280px] truncate font-mono text-xs" :title="value || ''">
              {{ value || '-' }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <button
              type="button"
              class="table-action-button"
              title="编辑节点"
              @click.stop="openEditNodeDialog(row)"
            >
              <Icon name="edit" size="sm" />
              编辑
            </button>
          </template>
          <template #empty>
            <EmptyState title="暂无节点" description="注册节点后会显示在这里" />
          </template>
        </DataTable>
      </section>

      <section class="node-panel">
        <div class="panel-heading">
          <h2>最近租约</h2>
          <span>{{ recentLeases.length }}</span>
        </div>
        <DataTable
          :columns="leaseColumns"
          :data="recentLeases"
          :loading="loading"
          row-key="id"
          :sticky-actions-column="false"
        >
          <template #cell-id="{ value }">
            <code class="code-cell max-w-[220px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-node_id="{ value }">
            <code class="code-cell max-w-[180px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-user_key="{ row }">
            <RouterLink
              :to="leaseUserLink(row.user_id)"
              class="inline-flex max-w-[180px] truncate text-sm font-medium text-primary-600 hover:underline dark:text-primary-400"
              :title="leaseUserLabel(row.user_id)"
            >
              {{ leaseUserLabel(row.user_id) || `用户 #${row.user_id}` }}
            </RouterLink>
          </template>
          <template #cell-amounts="{ row }">
            <span class="font-mono text-xs">
              {{ formatAmount(row.consumed) }} / {{ formatAmount(row.granted) }}
            </span>
          </template>
          <template #cell-remaining="{ row }">
            <span class="font-mono text-xs">{{ formatAmount(leaseRemaining(row)) }}</span>
          </template>
          <template #cell-status="{ value }">
            <span :class="['badge', statusBadgeClass(value)]">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-expires_at="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>
          <template #empty>
            <EmptyState title="暂无租约" description="节点申请额度后会显示最近租约" />
          </template>
        </DataTable>
      </section>
    </div>

    <BaseDialog
      :show="showRegisterNode"
      title="注册节点"
      width="wide"
      @close="closeRegisterNodeDialog"
    >
      <form id="register-node-form" class="space-y-4" @submit.prevent="createNodeRegistrationURL">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">节点名称 / ID</label>
            <input v-model.trim="nodeForm.node_id" class="input" placeholder="foreign-1" />
          </div>
          <div>
            <label class="input-label">区域</label>
            <input v-model.trim="nodeForm.region" class="input" placeholder="us-west" />
          </div>
        </div>

        <div v-if="lastRegistration" class="rounded-lg border border-emerald-200 bg-emerald-50 p-3 dark:border-emerald-800/50 dark:bg-emerald-900/20">
          <div class="mb-2 text-sm font-medium text-emerald-800 dark:text-emerald-200">
            节点注册链接
          </div>
          <div class="flex items-center gap-2">
            <code class="code-cell flex-1" :title="lastRegistration.registration_url">
              {{ lastRegistration.registration_url }}
            </code>
            <button type="button" class="icon-button" title="复制注册链接" @click="copyRegistrationURL">
              <Icon name="copy" size="sm" />
            </button>
          </div>
          <div class="mt-2 text-xs text-emerald-700 dark:text-emerald-200">
            有效期至 {{ formatTime(lastRegistration.expires_at) }}
          </div>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeRegisterNodeDialog">关闭</button>
        <button type="submit" form="register-node-form" class="btn btn-primary" :disabled="submittingNode">
          {{ submittingNode ? '生成中...' : '生成注册链接' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showEditNode"
      title="编辑节点"
      width="normal"
      @close="closeEditNodeDialog"
    >
      <form id="edit-node-form" class="space-y-4" @submit.prevent="saveNode">
        <div>
          <label class="input-label">节点 ID</label>
          <input v-model="editNodeForm.node_id" class="input font-mono text-xs" disabled />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">区域</label>
            <input v-model="editNodeForm.region" class="input" placeholder="us-west" />
          </div>
          <div>
            <label class="input-label">运行状态</label>
            <Select v-model="editNodeForm.status" :options="nodeStatusOptions" size="sm" />
          </div>
        </div>
        <div>
          <label class="input-label">节点访问地址</label>
          <input v-model="editNodeForm.base_url" class="input font-mono text-xs" placeholder="https://node.example" />
        </div>
        <div>
          <label class="input-label">Public Key</label>
          <textarea
            v-model="editNodeForm.public_key"
            rows="3"
            class="input font-mono text-xs"
            placeholder="可留空"
          ></textarea>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeEditNodeDialog">关闭</button>
        <button type="submit" form="edit-node-form" class="btn btn-primary" :disabled="savingNode">
          <Icon name="check" size="sm" class="mr-2" />
          {{ savingNode ? '保存中...' : '保存' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showLeaseSettings"
      title="租约设置"
      width="normal"
      @close="closeLeaseSettingsDialog"
    >
      <form id="lease-settings-form" class="space-y-4" @submit.prevent="saveSettings">
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">剩多少额度开始补</label>
            <input
              v-model.number="settingsForm.prefetch_low_watermark_amount"
              type="number"
              min="0"
              step="0.000001"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">看最近几次请求</label>
            <input
              v-model.number="settingsForm.prefetch_average_window"
              type="number"
              min="0"
              step="1"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">提前准备几次请求的量</label>
            <input
              v-model.number="settingsForm.prefetch_average_multiplier"
              type="number"
              min="0"
              step="0.1"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">两次补额度最少间隔</label>
            <input
              v-model.number="settingsForm.prefetch_debounce_seconds"
              type="number"
              min="0"
              step="1"
              class="input"
            />
          </div>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeLeaseSettingsDialog">关闭</button>
        <button type="submit" form="lease-settings-form" class="btn btn-primary" :disabled="savingSettings">
          <Icon name="check" size="sm" class="mr-2" />
          {{ savingSettings ? '保存中...' : '保存' }}
        </button>
      </template>
    </BaseDialog>

  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  NodeRegistrationURLResult,
  QuotaLeaseDemoLease,
  QuotaLeaseDemoNode,
  QuotaLeaseDemoSettings,
  QuotaLeaseDemoSnapshot,
  UpdateNodeRequest
} from '@/api/admin/nodeLeases'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(false)
const reclaiming = ref(false)
const savingSettings = ref(false)
const savingNode = ref(false)
let autoRefreshTimer: number | null = null
const snapshot = ref<QuotaLeaseDemoSnapshot | null>(null)
const nodes = ref<QuotaLeaseDemoNode[]>([])
const leaseUserLabels = reactive<Record<number, string>>({})
const nodeFilter = ref('')

const showLeaseSettings = ref(false)
const showEditNode = ref(false)
const showRegisterNode = ref(false)
const submittingNode = ref(false)
const lastRegistration = ref<NodeRegistrationURLResult | null>(null)

const emptyStats = {
  active_leases: 0,
  expired_leases: 0,
  closed_leases: 0,
  reclaimed_leases: 0,
  granted_total: 0,
  consumed_total: 0,
  reclaimed_total: 0,
  remaining_total: 0,
  event_count: 0,
  node_count: 0,
  online_nodes: 0
}

const nodeForm = reactive({
  node_id: '',
  region: ''
})

const editNodeForm = reactive({
  node_id: '',
  region: '',
  base_url: '',
  public_key: '',
  status: 'online'
})

const settingsForm = reactive<QuotaLeaseDemoSettings>({
  prefetch_low_watermark_amount: 0.2,
  prefetch_average_window: 5,
  prefetch_average_multiplier: 3,
  prefetch_debounce_seconds: 10
})

const stats = computed(() => snapshot.value?.stats || emptyStats)

const nodeFilterOptions = computed(() => [
  { value: '', label: '全部节点' },
  ...nodes.value.map((node) => ({
    value: node.node_id,
    label: `${node.node_id}${node.region ? ` (${node.region})` : ''}`
  }))
])

const nodeStatusOptions = [
  { value: 'online', label: '在线' },
  { value: 'offline', label: '离线' },
  { value: 'disabled', label: '禁用' }
]

const nodeColumns = computed<Column[]>(() => [
  { key: 'node_id', label: '节点 ID', sortable: true },
  { key: 'region', label: '区域', sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'lease_remaining', label: '剩余额度', sortable: true },
  { key: 'last_heartbeat_at', label: '心跳', sortable: true },
  { key: 'base_url', label: 'Base URL', sortable: false },
  { key: 'actions', label: '操作', sortable: false }
])

const leaseColumns = computed<Column[]>(() => [
  { key: 'id', label: '租约 ID', sortable: true },
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'user_key', label: '用户', sortable: false },
  { key: 'amounts', label: '消费 / 分配', sortable: false },
  { key: 'remaining', label: '剩余', sortable: false },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'expires_at', label: '过期时间', sortable: true }
])

const recentLeases = computed(() => {
  const leases = snapshot.value?.leases || []
  return [...leases]
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    .slice(0, 30)
})

function leaseUserLabel(userId: number) {
  return leaseUserLabels[userId] || ''
}

function leaseUserLink(userId: number) {
  const search = leaseUserLabel(userId)
  return search ? { path: '/admin/users', query: { search } } : { path: '/admin/users' }
}

async function hydrateRecentLeaseUsers() {
  const userIds = [...new Set(recentLeases.value.map((lease) => lease.user_id).filter((userId) => userId > 0 && !leaseUserLabels[userId]))]
  if (userIds.length === 0) {
    return
  }
  const results = await Promise.all(
    userIds.map(async (userId) => {
      try {
        const user = await adminAPI.users.getById(userId, true)
        const label = user.username?.trim() || user.email?.trim() || `用户 #${userId}`
        return [userId, label] as const
      } catch {
        return [userId, `用户 #${userId}`] as const
      }
    })
  )
  for (const [userId, label] of results) {
    leaseUserLabels[userId] = label
  }
}

watch(nodeFilter, () => {
  loadAll()
})

async function loadAll() {
  loading.value = true
  try {
    const [settingsResult] = await Promise.all([
      adminAPI.nodeLeases.getSettings()
    ])
    applySettingsForm(settingsResult)
    await loadRuntimeState()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '节点租约状态加载失败'))
  } finally {
    loading.value = false
  }
}

async function loadRuntimeState() {
  const [statusResult, nodeResult] = await Promise.all([
    adminAPI.nodeLeases.getStatus(),
    adminAPI.nodeLeases.listNodes()
  ])
  snapshot.value = statusResult
  nodes.value = nodeResult
  await hydrateRecentLeaseUsers()
}

async function refreshRuntimeState() {
  if (loading.value || reclaiming.value || submittingNode.value || savingNode.value) return
  try {
    await loadRuntimeState()
  } catch (error) {
    console.error('Failed to refresh node lease runtime state:', error)
  }
}

async function reclaimExpired() {
  reclaiming.value = true
  try {
    const result = await adminAPI.nodeLeases.reclaimExpired()
    appStore.showSuccess(
      `已回收 ${result.reclaimed_count} 个租约，额度 ${formatAmount(result.reclaimed_total)}`
    )
    await loadAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '租约回收失败'))
  } finally {
    reclaiming.value = false
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    const result = await adminAPI.nodeLeases.updateSettings(
      {
        prefetch_low_watermark_amount: Number(settingsForm.prefetch_low_watermark_amount || 0),
        prefetch_average_window: Number(settingsForm.prefetch_average_window || 0),
        prefetch_average_multiplier: Number(settingsForm.prefetch_average_multiplier || 0),
        prefetch_debounce_seconds: Number(settingsForm.prefetch_debounce_seconds || 0)
      }
    )
    applySettingsForm(result)
    appStore.showSuccess('租约设置已保存')
    closeLeaseSettingsDialog()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '租约设置保存失败'))
  } finally {
    savingSettings.value = false
  }
}

function applySettingsForm(settings?: QuotaLeaseDemoSettings | null) {
  if (!settings) return
  settingsForm.prefetch_low_watermark_amount = Number(settings.prefetch_low_watermark_amount || 0)
  settingsForm.prefetch_average_window = Number(settings.prefetch_average_window || 0)
  settingsForm.prefetch_average_multiplier = Number(settings.prefetch_average_multiplier || 0)
  settingsForm.prefetch_debounce_seconds = Number(settings.prefetch_debounce_seconds || 0)
}

function openLeaseSettingsDialog() {
  showLeaseSettings.value = true
}

function closeLeaseSettingsDialog() {
  showLeaseSettings.value = false
}

function openRegisterNodeDialog() {
  lastRegistration.value = null
  showRegisterNode.value = true
}

function closeRegisterNodeDialog() {
  showRegisterNode.value = false
}

function openEditNodeDialog(node: QuotaLeaseDemoNode) {
  editNodeForm.node_id = node.node_id
  editNodeForm.region = node.region || ''
  editNodeForm.base_url = node.base_url || ''
  editNodeForm.public_key = node.public_key || ''
  editNodeForm.status = node.status || 'online'
  showEditNode.value = true
}

function closeEditNodeDialog() {
  showEditNode.value = false
}

async function saveNode() {
  const nodeId = editNodeForm.node_id.trim()
  if (!nodeId) return

  savingNode.value = true
  try {
    const payload: UpdateNodeRequest = {
      region: editNodeForm.region.trim(),
      base_url: editNodeForm.base_url.trim(),
      public_key: editNodeForm.public_key.trim(),
      status: editNodeForm.status.trim() || 'online'
    }
    await adminAPI.nodeLeases.updateNode(nodeId, payload)
    appStore.showSuccess('节点信息已保存')
    closeEditNodeDialog()
    await loadRuntimeState()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '节点信息保存失败'))
  } finally {
    savingNode.value = false
  }
}

async function createNodeRegistrationURL() {
  submittingNode.value = true
  try {
    const result = await adminAPI.nodeLeases.createNodeRegistrationURL(
      {
        node_id: nodeForm.node_id.trim() || undefined,
        region: nodeForm.region.trim() || undefined
      }
    )
    lastRegistration.value = result
    appStore.showSuccess('注册链接已生成')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '注册链接生成失败'))
  } finally {
    submittingNode.value = false
  }
}

function copyRegistrationURL() {
  if (lastRegistration.value?.registration_url) {
    void copyToClipboard(lastRegistration.value.registration_url, '节点注册链接已复制')
  }
}

function leaseRemaining(row: QuotaLeaseDemoLease) {
  return Math.max(0, (row.granted || 0) - (row.consumed || 0) - (row.reclaimed || 0))
}

function formatAmount(value: number) {
  const amount = Number(value || 0)
  return amount.toFixed(6).replace(/\.?0+$/, '')
}

function formatTime(value?: string | null) {
  return value ? formatDateTime(value) : '-'
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    online: '在线',
    offline: '离线',
    disabled: '禁用',
    active: '活跃',
    expired: '已过期',
    reclaimed: '已回收',
    closed: '已关闭',
    pending: '待处理',
    waiting_callback: '等待 Callback',
    callback_ready: 'Callback 已提交',
    completed: '已完成',
    failed: '失败',
    error: '异常'
  }
  return labels[status] || status || '-'
}

function statusBadgeClass(status: string) {
  if (['online', 'active', 'completed', 'closed'].includes(status)) return 'badge-success'
  if (['pending', 'waiting_callback', 'callback_ready', 'expired'].includes(status)) return 'badge-warning'
  if (['failed', 'error', 'offline', 'disabled'].includes(status)) return 'badge-danger'
  return 'badge-gray'
}

onMounted(() => {
  loadAll()
  autoRefreshTimer = window.setInterval(refreshRuntimeState, 10_000)
})

onUnmounted(() => {
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>

<style scoped>
.node-leases-page {
  @apply space-y-5;
}

.node-panel {
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

.icon-button {
  @apply inline-flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-primary-300;
}

.table-action-button {
  @apply inline-flex h-8 items-center gap-1 rounded-md px-2 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-primary-300;
}

.node-panel :deep(.table-wrapper) {
  max-height: 440px;
  overflow: auto;
}

.node-panel :deep(.data-table-surface) {
  min-width: 1040px;
}
</style>
