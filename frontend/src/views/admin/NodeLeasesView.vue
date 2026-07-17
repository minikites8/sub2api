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

          <div class="flex flex-col gap-3 xl:min-w-[520px]">
            <div class="grid gap-2 md:grid-cols-[minmax(0,1fr)_auto]">
              <div class="relative">
                <label class="input-label">控制 Key</label>
                <input
                  v-model="controlKey"
                  :type="controlKeyVisible ? 'text' : 'password'"
                  class="input pr-10"
                  placeholder="X-Node-Secret"
                  autocomplete="off"
                />
                <button
                  type="button"
                  class="absolute bottom-2.5 right-3 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                  :title="controlKeyVisible ? '隐藏' : '显示'"
                  @click="controlKeyVisible = !controlKeyVisible"
                >
                  <Icon :name="controlKeyVisible ? 'eyeOff' : 'eye'" size="sm" />
                </button>
              </div>
              <div class="flex items-end gap-2">
                <button type="button" class="btn btn-secondary" @click="saveControlKey">
                  <Icon name="check" size="sm" class="mr-2" />
                  保存
                </button>
                <button type="button" class="btn btn-secondary" @click="clearControlKey">
                  <Icon name="x" size="sm" class="mr-2" />
                  清空
                </button>
              </div>
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <div class="w-full sm:w-40">
                <Select v-model="nodeFilter" :options="nodeFilterOptions" size="sm" />
              </div>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAll">
                <Icon name="refresh" size="sm" class="mr-2" :class="loading ? 'animate-spin' : ''" />
                刷新
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
            <span class="font-mono text-xs">U{{ row.user_id }} / K{{ row.api_key_id }}</span>
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
      <form id="register-node-form" class="space-y-4" @submit.prevent="registerNode">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">节点 ID</label>
            <input v-model.trim="nodeForm.node_id" class="input" placeholder="foreign-1" />
          </div>
          <div>
            <label class="input-label">区域</label>
            <input v-model.trim="nodeForm.region" class="input" placeholder="us-west" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">节点 Base URL</label>
            <input v-model.trim="nodeForm.base_url" class="input" placeholder="https://node.example.com" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">Public Key</label>
            <textarea v-model="nodeForm.public_key" class="input min-h-[80px] font-mono text-xs" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">Metadata JSON</label>
            <textarea
              v-model="nodeForm.metadata_json"
              class="input min-h-[88px] font-mono text-xs"
              placeholder='{"provider":"aws"}'
            />
          </div>
        </div>

        <div v-if="lastRegistration" class="rounded-lg border border-emerald-200 bg-emerald-50 p-3 dark:border-emerald-800/50 dark:bg-emerald-900/20">
          <div class="mb-2 text-sm font-medium text-emerald-800 dark:text-emerald-200">
            节点 Secret
          </div>
          <div class="flex items-center gap-2">
            <code class="code-cell flex-1" :title="lastRegistration.node_secret">
              {{ lastRegistration.node_secret }}
            </code>
            <button type="button" class="icon-button" title="复制 Secret" @click="copyNodeSecret">
              <Icon name="copy" size="sm" />
            </button>
          </div>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeRegisterNodeDialog">关闭</button>
        <button type="submit" form="register-node-form" class="btn btn-primary" :disabled="submittingNode">
          {{ submittingNode ? '注册中...' : '注册节点' }}
        </button>
      </template>
    </BaseDialog>

  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  QuotaLeaseDemoLease,
  QuotaLeaseDemoNode,
  QuotaLeaseDemoSnapshot,
  RegisterNodeResult
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

const CONTROL_KEY_STORAGE = 'sub2api_node_leases_demo_control_key'

const controlKey = ref(sessionStorage.getItem(CONTROL_KEY_STORAGE) || '')
const controlKeyVisible = ref(false)
const loading = ref(false)
const reclaiming = ref(false)
const snapshot = ref<QuotaLeaseDemoSnapshot | null>(null)
const nodes = ref<QuotaLeaseDemoNode[]>([])
const nodeFilter = ref('')

const showRegisterNode = ref(false)
const submittingNode = ref(false)
const lastRegistration = ref<RegisterNodeResult | null>(null)

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
  region: '',
  base_url: '',
  public_key: '',
  metadata_json: ''
})

const stats = computed(() => snapshot.value?.stats || emptyStats)

const nodeFilterOptions = computed(() => [
  { value: '', label: '全部节点' },
  ...nodes.value.map((node) => ({
    value: node.node_id,
    label: `${node.node_id}${node.region ? ` (${node.region})` : ''}`
  }))
])

const nodeColumns = computed<Column[]>(() => [
  { key: 'node_id', label: '节点 ID', sortable: true },
  { key: 'region', label: '区域', sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'inflight_requests', label: '请求中', sortable: true },
  { key: 'lease_remaining', label: '剩余额度', sortable: true },
  { key: 'last_heartbeat_at', label: '心跳', sortable: true },
  { key: 'base_url', label: 'Base URL', sortable: false }
])

const leaseColumns = computed<Column[]>(() => [
  { key: 'id', label: '租约 ID', sortable: true },
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'user_key', label: '用户 / Key', sortable: false },
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

watch(controlKey, (value) => {
  sessionStorage.setItem(CONTROL_KEY_STORAGE, value)
})

watch(nodeFilter, () => {
  loadAll()
})

function controlOptions() {
  return { controlKey: controlKey.value.trim() }
}

function saveControlKey() {
  sessionStorage.setItem(CONTROL_KEY_STORAGE, controlKey.value.trim())
  appStore.showSuccess('控制 Key 已保存')
}

function clearControlKey() {
  controlKey.value = ''
  sessionStorage.removeItem(CONTROL_KEY_STORAGE)
  appStore.showSuccess('控制 Key 已清空')
}

async function loadAll() {
  loading.value = true
  try {
    const options = controlOptions()
    const [statusResult, nodeResult] = await Promise.all([
      adminAPI.nodeLeases.getStatus(options),
      adminAPI.nodeLeases.listNodes(options)
    ])
    snapshot.value = statusResult
    nodes.value = nodeResult
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '节点租约状态加载失败'))
  } finally {
    loading.value = false
  }
}

async function reclaimExpired() {
  reclaiming.value = true
  try {
    const result = await adminAPI.nodeLeases.reclaimExpired(controlOptions())
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

function openRegisterNodeDialog() {
  lastRegistration.value = null
  showRegisterNode.value = true
}

function closeRegisterNodeDialog() {
  showRegisterNode.value = false
}

async function registerNode() {
  submittingNode.value = true
  try {
    const metadata = parseJsonObject(nodeForm.metadata_json, 'Metadata JSON')
    const result = await adminAPI.nodeLeases.registerNode(
      {
        node_id: nodeForm.node_id.trim() || undefined,
        region: nodeForm.region.trim() || undefined,
        base_url: nodeForm.base_url.trim() || undefined,
        public_key: nodeForm.public_key.trim() || undefined,
        metadata: metadata as Record<string, string> | undefined
      },
      controlOptions()
    )
    lastRegistration.value = result
    appStore.showSuccess('节点已注册')
    await loadAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '节点注册失败'))
  } finally {
    submittingNode.value = false
  }
}

function copyNodeSecret() {
  if (lastRegistration.value?.node_secret) {
    void copyToClipboard(lastRegistration.value.node_secret, '节点 Secret 已复制')
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

function parseJsonObject(raw: string, label: string): Record<string, unknown> | undefined {
  const trimmed = raw.trim()
  if (!trimmed) return undefined
  const parsed = JSON.parse(trimmed)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(`${label} 必须是 JSON 对象`)
  }
  return parsed as Record<string, unknown>
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

.field-label {
  @apply mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400;
}

.node-panel :deep(.table-wrapper) {
  max-height: 440px;
  overflow: auto;
}

.node-panel :deep(.data-table-surface) {
  min-width: 920px;
}
</style>
