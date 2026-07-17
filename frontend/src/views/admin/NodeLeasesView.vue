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
              <div class="w-full sm:w-44">
                <Select v-model="taskStatusFilter" :options="taskStatusOptions" size="sm" />
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
              <button type="button" class="btn btn-primary" @click="openCreateTaskDialog">
                <Icon name="plus" size="sm" class="mr-2" />
                创建登录任务
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
          <h2>OAuth 登录任务</h2>
          <span>{{ loginTasks.length }}</span>
        </div>
        <DataTable
          :columns="taskColumns"
          :data="loginTasks"
          :loading="loading"
          row-key="id"
          :sticky-actions-column="false"
          :estimate-row-height="92"
        >
          <template #cell-id="{ value }">
            <code class="code-cell max-w-[220px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-account_id="{ value }">
            <span class="font-mono">#{{ value }}</span>
          </template>
          <template #cell-platform="{ row }">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="badge badge-gray">{{ row.platform }}</span>
              <span class="badge badge-gray">{{ row.type }}</span>
            </div>
          </template>
          <template #cell-assigned_node_id="{ value }">
            <code class="code-cell max-w-[180px]" :title="value">{{ value }}</code>
          </template>
          <template #cell-status="{ value, row }">
            <div class="flex flex-col gap-1">
              <span :class="['badge w-fit', statusBadgeClass(value)]">{{ statusLabel(value) }}</span>
              <span v-if="row.error" class="max-w-[240px] truncate text-xs text-red-600 dark:text-red-300" :title="row.error">
                {{ row.error }}
              </span>
            </div>
          </template>
          <template #cell-auth_url="{ row }">
            <div v-if="authUrl(row)" class="flex min-w-[260px] items-center gap-2">
              <code class="code-cell max-w-[220px]" :title="authUrl(row)">{{ authUrl(row) }}</code>
              <button type="button" class="icon-button" title="复制授权 URL" @click="copyAuthUrl(row)">
                <Icon name="copy" size="sm" />
              </button>
              <button type="button" class="icon-button" title="打开授权 URL" @click="openAuthUrl(row)">
                <Icon name="externalLink" size="sm" />
              </button>
            </div>
            <span v-else class="text-sm text-gray-400">-</span>
          </template>
          <template #cell-updated_at="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="row.status === 'completed' || row.status === 'failed'"
                @click="openCallbackDialog(row)"
              >
                <Icon name="login" size="sm" class="mr-1.5" />
                Callback
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState title="暂无任务" description="创建登录任务后会显示授权状态" />
          </template>
        </DataTable>
      </section>

      <section class="node-panel">
        <div class="panel-heading">
          <h2>已分配账号</h2>
          <span>{{ assignedAccounts.length }}</span>
        </div>
        <DataTable
          :columns="accountColumns"
          :data="assignedAccounts"
          :loading="loading"
          :row-key="assignedAccountKey"
          :sticky-actions-column="false"
        >
          <template #cell-account_id="{ row }">
            <span class="font-mono">#{{ row.account.id }}</span>
          </template>
          <template #cell-name="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.account.name || '-' }}</span>
              <span v-if="row.task_id" class="max-w-[240px] truncate font-mono text-xs text-gray-500" :title="row.task_id">
                {{ row.task_id }}
              </span>
            </div>
          </template>
          <template #cell-platform="{ row }">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="badge badge-gray">{{ row.account.platform }}</span>
              <span class="badge badge-gray">{{ row.account.type }}</span>
            </div>
          </template>
          <template #cell-node_id="{ row }">
            <code class="code-cell max-w-[180px]" :title="row.node_id">{{ row.node_id }}</code>
          </template>
          <template #cell-status="{ row }">
            <div class="flex flex-wrap items-center gap-1.5">
              <span :class="['badge', statusBadgeClass(row.account.status)]">{{ statusLabel(row.account.status) }}</span>
              <span :class="['badge', row.account.schedulable ? 'badge-success' : 'badge-danger']">
                {{ row.account.schedulable ? '可调度' : '暂停调度' }}
              </span>
            </div>
          </template>
          <template #cell-groups="{ row }">
            <span class="font-mono text-xs">{{ row.account.group_ids?.join(', ') || '-' }}</span>
          </template>
          <template #cell-updated_at="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatTime(row.updated_at) }}</span>
          </template>
          <template #empty>
            <EmptyState title="暂无账号" description="节点完成登录任务后会回传账号快照" />
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

    <BaseDialog
      :show="showCreateTask"
      title="创建账号登录任务"
      width="wide"
      @close="closeCreateTaskDialog"
    >
      <form id="create-login-task-form" class="space-y-4" @submit.prevent="createLoginTask">
        <datalist id="node-lease-node-options">
          <option v-for="node in nodes" :key="node.node_id" :value="node.node_id" />
        </datalist>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">账号 ID</label>
            <input v-model.trim="taskForm.account_id" type="number" min="1" class="input" required />
          </div>
          <div>
            <label class="input-label">名称</label>
            <input v-model.trim="taskForm.name" class="input" placeholder="gpt-oauth-1" />
          </div>
          <div>
            <label class="input-label">平台</label>
            <Select v-model="taskForm.platform" :options="platformOptions" />
          </div>
          <div>
            <label class="input-label">类型</label>
            <input v-model.trim="taskForm.type" class="input" placeholder="oauth" required />
          </div>
          <div>
            <label class="input-label">分配节点</label>
            <input
              v-model.trim="taskForm.assigned_node_id"
              list="node-lease-node-options"
              class="input"
              placeholder="foreign-1"
              required
            />
          </div>
          <div>
            <label class="input-label">分组 IDs</label>
            <input v-model.trim="taskForm.group_ids" class="input" placeholder="1,2,3" />
          </div>
          <div>
            <label class="input-label">并发</label>
            <input v-model.trim="taskForm.concurrency" type="number" min="1" class="input" />
          </div>
          <div>
            <label class="input-label">优先级</label>
            <input v-model.trim="taskForm.priority" type="number" class="input" />
          </div>
          <div>
            <label class="input-label">Redirect URI</label>
            <input v-model.trim="taskForm.redirect_uri" class="input" placeholder="http://localhost:1455/auth/callback" />
          </div>
          <div>
            <label class="input-label">代理 ID</label>
            <input v-model.trim="taskForm.proxy_id" type="number" min="1" class="input" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">Login Payload JSON</label>
            <textarea
              v-model="taskForm.login_payload_json"
              class="input min-h-[88px] font-mono text-xs"
              placeholder='{"prompt":"login"}'
            />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">Metadata JSON</label>
            <textarea
              v-model="taskForm.metadata_json"
              class="input min-h-[72px] font-mono text-xs"
              placeholder='{"owner":"ops"}'
            />
          </div>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeCreateTaskDialog">取消</button>
        <button type="submit" form="create-login-task-form" class="btn btn-primary" :disabled="submittingTask">
          {{ submittingTask ? '创建中...' : '创建任务' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="!!callbackTask"
      title="提交 OAuth Callback"
      width="wide"
      @close="closeCallbackDialog"
    >
      <form id="callback-form" class="space-y-4" @submit.prevent="submitCallback">
        <div v-if="callbackTask" class="grid gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="grid gap-2 md:grid-cols-2">
            <div>
              <span class="field-label">任务</span>
              <code class="code-cell">{{ callbackTask.id }}</code>
            </div>
            <div>
              <span class="field-label">节点</span>
              <code class="code-cell">{{ callbackTask.assigned_node_id }}</code>
            </div>
          </div>
          <div v-if="authUrl(callbackTask)" class="flex items-center gap-2">
            <code class="code-cell flex-1" :title="authUrl(callbackTask)">{{ authUrl(callbackTask) }}</code>
            <button type="button" class="icon-button" title="复制授权 URL" @click="copyAuthUrl(callbackTask)">
              <Icon name="copy" size="sm" />
            </button>
            <button type="button" class="icon-button" title="打开授权 URL" @click="openAuthUrl(callbackTask)">
              <Icon name="externalLink" size="sm" />
            </button>
          </div>
        </div>

        <div>
          <label class="input-label">Callback URL</label>
          <textarea
            v-model="callbackForm.callback_url"
            class="input min-h-[96px] font-mono text-xs"
            placeholder="http://localhost:1455/auth/callback?code=...&state=..."
          />
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">Code</label>
            <input v-model.trim="callbackForm.code" class="input font-mono" />
          </div>
          <div>
            <label class="input-label">State</label>
            <input v-model.trim="callbackForm.state" class="input font-mono" />
          </div>
          <div>
            <label class="input-label">Session ID</label>
            <input v-model.trim="callbackForm.session_id" class="input font-mono" />
          </div>
          <div>
            <label class="input-label">Redirect URI</label>
            <input v-model.trim="callbackForm.redirect_uri" class="input" />
          </div>
          <div>
            <label class="input-label">代理 ID</label>
            <input v-model.trim="callbackForm.proxy_id" type="number" min="1" class="input" />
          </div>
        </div>
        <div>
          <label class="input-label">Payload JSON</label>
          <textarea
            v-model="callbackForm.payload_json"
            class="input min-h-[72px] font-mono text-xs"
            placeholder='{"custom":"value"}'
          />
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeCallbackDialog">取消</button>
        <button type="submit" form="callback-form" class="btn btn-primary" :disabled="submittingCallback">
          {{ submittingCallback ? '提交中...' : '提交 Callback' }}
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
  QuotaLeaseDemoAccountLoginTask,
  QuotaLeaseDemoAssignedAccount,
  QuotaLeaseDemoLease,
  QuotaLeaseDemoNode,
  QuotaLeaseDemoSnapshot,
  RegisterNodeResult,
  SubmitAccountLoginTaskCallbackRequest
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
const loginTasks = ref<QuotaLeaseDemoAccountLoginTask[]>([])
const assignedAccounts = ref<QuotaLeaseDemoAssignedAccount[]>([])
const nodeFilter = ref('')
const taskStatusFilter = ref('')

const showRegisterNode = ref(false)
const showCreateTask = ref(false)
const submittingNode = ref(false)
const submittingTask = ref(false)
const submittingCallback = ref(false)
const lastRegistration = ref<RegisterNodeResult | null>(null)
const callbackTask = ref<QuotaLeaseDemoAccountLoginTask | null>(null)

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

const taskForm = reactive({
  account_id: '',
  name: '',
  platform: 'openai',
  type: 'oauth',
  assigned_node_id: '',
  group_ids: '',
  concurrency: '1',
  priority: '0',
  redirect_uri: '',
  proxy_id: '',
  login_payload_json: '',
  metadata_json: ''
})

const callbackForm = reactive({
  callback_url: '',
  code: '',
  state: '',
  session_id: '',
  redirect_uri: '',
  proxy_id: '',
  payload_json: ''
})

const stats = computed(() => snapshot.value?.stats || emptyStats)

const nodeFilterOptions = computed(() => [
  { value: '', label: '全部节点' },
  ...nodes.value.map((node) => ({
    value: node.node_id,
    label: `${node.node_id}${node.region ? ` (${node.region})` : ''}`
  }))
])

const taskStatusOptions = [
  { value: '', label: '全部任务状态' },
  { value: 'pending', label: '待处理' },
  { value: 'waiting_callback', label: '等待 Callback' },
  { value: 'callback_ready', label: 'Callback 已提交' },
  { value: 'completed', label: '已完成' },
  { value: 'failed', label: '失败' }
]

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'grok', label: 'Grok' }
]

const nodeColumns = computed<Column[]>(() => [
  { key: 'node_id', label: '节点 ID', sortable: true },
  { key: 'region', label: '区域', sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'inflight_requests', label: '请求中', sortable: true },
  { key: 'lease_remaining', label: '剩余额度', sortable: true },
  { key: 'last_heartbeat_at', label: '心跳', sortable: true },
  { key: 'base_url', label: 'Base URL', sortable: false }
])

const taskColumns = computed<Column[]>(() => [
  { key: 'id', label: '任务 ID', sortable: true },
  { key: 'account_id', label: '账号', sortable: true },
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'platform', label: '平台', sortable: true },
  { key: 'assigned_node_id', label: '节点', sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'auth_url', label: 'Auth URL', sortable: false },
  { key: 'updated_at', label: '更新时间', sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false }
])

const accountColumns = computed<Column[]>(() => [
  { key: 'account_id', label: '账号', sortable: true },
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'platform', label: '平台', sortable: true },
  { key: 'node_id', label: '节点', sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'groups', label: '分组', sortable: false },
  { key: 'updated_at', label: '更新时间', sortable: true }
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

watch([nodeFilter, taskStatusFilter], () => {
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
    const [statusResult, nodeResult, taskResult, accountResult] = await Promise.all([
      adminAPI.nodeLeases.getStatus(options),
      adminAPI.nodeLeases.listNodes(options),
      adminAPI.nodeLeases.listLoginTasks(
        {
          status: taskStatusFilter.value || undefined,
          node_id: nodeFilter.value || undefined
        },
        options
      ),
      adminAPI.nodeLeases.listAssignedAccounts(
        { node_id: nodeFilter.value || undefined },
        options
      )
    ])
    snapshot.value = statusResult
    nodes.value = nodeResult
    loginTasks.value = taskResult
    assignedAccounts.value = accountResult
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

function openCreateTaskDialog() {
  taskForm.assigned_node_id = nodeFilter.value || taskForm.assigned_node_id
  showCreateTask.value = true
}

function closeCreateTaskDialog() {
  showCreateTask.value = false
}

async function createLoginTask() {
  const accountID = toPositiveInteger(taskForm.account_id)
  if (!accountID) {
    appStore.showError('请输入有效账号 ID')
    return
  }
  if (!taskForm.assigned_node_id.trim()) {
    appStore.showError('请输入分配节点')
    return
  }

  submittingTask.value = true
  try {
    const loginPayload = parseJsonObject(taskForm.login_payload_json, 'Login Payload JSON') || {}
    if (taskForm.redirect_uri.trim()) {
      loginPayload.redirect_uri = taskForm.redirect_uri.trim()
    }
    const proxyID = toPositiveInteger(taskForm.proxy_id)
    if (proxyID) {
      loginPayload.proxy_id = proxyID
    }

    const task = await adminAPI.nodeLeases.createLoginTask(
      {
        account_id: accountID,
        name: taskForm.name.trim() || `account-${accountID}`,
        platform: taskForm.platform,
        type: taskForm.type.trim() || 'oauth',
        assigned_node_id: taskForm.assigned_node_id.trim(),
        login_payload: loginPayload,
        metadata: parseJsonObject(taskForm.metadata_json, 'Metadata JSON') as Record<string, string> | undefined,
        group_ids: parseIDList(taskForm.group_ids),
        concurrency: toPositiveInteger(taskForm.concurrency) || 1,
        priority: toInteger(taskForm.priority) || 0
      },
      controlOptions()
    )
    upsertTask(task)
    appStore.showSuccess('登录任务已创建')
    showCreateTask.value = false
    await loadAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '创建登录任务失败'))
  } finally {
    submittingTask.value = false
  }
}

function openCallbackDialog(task: QuotaLeaseDemoAccountLoginTask) {
  callbackTask.value = task
  const payload = task.login_payload || {}
  callbackForm.callback_url = ''
  callbackForm.code = stringValue(payload.code)
  callbackForm.state = stringValue(payload.state)
  callbackForm.session_id = stringValue(payload.session_id)
  callbackForm.redirect_uri = stringValue(payload.redirect_uri)
  callbackForm.proxy_id = stringValue(payload.proxy_id)
  callbackForm.payload_json = ''
}

function closeCallbackDialog() {
  callbackTask.value = null
}

async function submitCallback() {
  if (!callbackTask.value) return

  submittingCallback.value = true
  try {
    const payloadJson = parseJsonObject(callbackForm.payload_json, 'Payload JSON')
    const payload: SubmitAccountLoginTaskCallbackRequest = {}
    if (callbackForm.callback_url.trim()) {
      payload.callback_url = callbackForm.callback_url.trim()
    } else {
      if (callbackForm.code.trim()) payload.code = callbackForm.code.trim()
      if (callbackForm.state.trim()) payload.state = callbackForm.state.trim()
      if (callbackForm.session_id.trim()) payload.session_id = callbackForm.session_id.trim()
      if (callbackForm.redirect_uri.trim()) payload.redirect_uri = callbackForm.redirect_uri.trim()
    }
    const proxyID = toPositiveInteger(callbackForm.proxy_id)
    if (proxyID) payload.proxy_id = proxyID
    if (payloadJson && Object.keys(payloadJson).length > 0) {
      payload.payload = payloadJson
    }
    if (Object.keys(payload).length === 0) {
      appStore.showError('请输入 Callback URL 或 code/state')
      return
    }

    const updated = await adminAPI.nodeLeases.submitLoginTaskCallback(
      callbackTask.value.id,
      payload,
      controlOptions()
    )
    upsertTask(updated)
    callbackTask.value = updated
    appStore.showSuccess('Callback 已提交')
    await loadAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '提交 Callback 失败'))
  } finally {
    submittingCallback.value = false
  }
}

function upsertTask(task: QuotaLeaseDemoAccountLoginTask) {
  const index = loginTasks.value.findIndex((item) => item.id === task.id)
  if (index >= 0) {
    loginTasks.value.splice(index, 1, task)
  } else {
    loginTasks.value.unshift(task)
  }
}

function authUrl(task: QuotaLeaseDemoAccountLoginTask): string {
  return stringValue(task.login_payload?.auth_url)
}

function openAuthUrl(task: QuotaLeaseDemoAccountLoginTask) {
  const url = authUrl(task)
  if (!url) return
  window.open(url, '_blank', 'noopener,noreferrer')
}

function copyAuthUrl(task: QuotaLeaseDemoAccountLoginTask) {
  const url = authUrl(task)
  if (url) {
    void copyToClipboard(url, '授权 URL 已复制')
  }
}

function copyNodeSecret() {
  if (lastRegistration.value?.node_secret) {
    void copyToClipboard(lastRegistration.value.node_secret, '节点 Secret 已复制')
  }
}

function assignedAccountKey(row: QuotaLeaseDemoAssignedAccount) {
  return `${row.node_id}:${row.account.id}`
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

function stringValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  return String(value)
}

function toPositiveInteger(value: string): number | null {
  const n = Number(value)
  return Number.isInteger(n) && n > 0 ? n : null
}

function toInteger(value: string): number | null {
  const n = Number(value)
  return Number.isInteger(n) ? n : null
}

function parseIDList(raw: string): number[] | undefined {
  const values = raw
    .split(/[\s,，]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .map(Number)
    .filter((item) => Number.isInteger(item) && item > 0)
  return values.length > 0 ? Array.from(new Set(values)) : undefined
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
