<template>
  <AppLayout>
    <div data-testid="invoice-shell" class="console-page invoice-page">
      <header class="invoice-header">
        <p class="invoice-kicker">BILLING / INVOICES</p>
        <h1 class="invoice-title">{{ t('invoice.title') }}</h1>
        <p class="invoice-description">{{ t('invoice.description') }}</p>
      </header>

      <nav class="invoice-tabs" aria-label="Invoice sections">
        <button
          type="button"
          :class="{ 'invoice-tab-active': activeTab === 'apply' }"
          :aria-current="activeTab === 'apply' ? 'page' : undefined"
          @click="selectTab('apply')"
        >{{ t('invoice.tabs.apply') }}</button>
        <button
          type="button"
          :class="{ 'invoice-tab-active': activeTab === 'history' }"
          :aria-current="activeTab === 'history' ? 'page' : undefined"
          @click="selectTab('history')"
        >{{ t('invoice.tabs.history') }}</button>
      </nav>

      <!-- 申请开票 -->
      <section v-if="activeTab === 'apply'" class="invoice-apply">
        <div class="invoice-apply-main">
          <div class="invoice-card">
            <h2 class="invoice-card-title">{{ t('invoice.apply.billingTitle') }}</h2>

            <fieldset class="invoice-field">
              <legend class="invoice-label">{{ t('invoice.apply.entityType') }}</legend>
              <div class="invoice-radio-row">
                <label class="invoice-radio">
                  <input v-model="form.entity_type" type="radio" value="company" />
                  <span>{{ t('invoice.apply.entityCompany') }}</span>
                </label>
                <label class="invoice-radio">
                  <input v-model="form.entity_type" type="radio" value="individual" />
                  <span>{{ t('invoice.apply.entityIndividual') }}</span>
                </label>
              </div>
            </fieldset>

            <div class="invoice-field-grid">
              <div class="invoice-field">
                <label for="invoice-title-input" class="invoice-label">
                  {{ t('invoice.apply.invoiceTitle') }}
                </label>
                <input
                  id="invoice-title-input"
                  v-model="form.title"
                  type="text"
                  class="invoice-input"
                  :placeholder="isCompany
                    ? t('invoice.apply.invoiceTitlePlaceholderCompany')
                    : t('invoice.apply.invoiceTitlePlaceholderIndividual')"
                />
              </div>

              <div class="invoice-field">
                <label for="invoice-tax-input" class="invoice-label">
                  {{ t('invoice.apply.taxId') }}
                </label>
                <input
                  id="invoice-tax-input"
                  v-model="form.tax_id"
                  type="text"
                  class="invoice-input"
                  :disabled="!isCompany"
                  :placeholder="t('invoice.apply.taxIdPlaceholder')"
                />
                <p v-if="!isCompany" class="invoice-hint">
                  {{ t('invoice.apply.taxIdIndividualHint') }}
                </p>
              </div>
            </div>

            <div class="invoice-field">
              <label for="invoice-email-input" class="invoice-label">
                {{ t('invoice.apply.deliveryEmail') }}
              </label>
              <input
                id="invoice-email-input"
                v-model="form.delivery_email"
                type="email"
                class="invoice-input"
                :placeholder="t('invoice.apply.deliveryEmailPlaceholder')"
              />
              <p class="invoice-hint">{{ t('invoice.apply.deliveryEmailHint') }}</p>
            </div>

            <div class="invoice-field">
              <label for="invoice-notes-input" class="invoice-label">
                {{ t('invoice.apply.notes') }}
              </label>
              <textarea
                id="invoice-notes-input"
                v-model="form.notes"
                rows="3"
                class="invoice-input invoice-textarea"
                :placeholder="t('invoice.apply.notesPlaceholder')"
              ></textarea>
            </div>
          </div>

          <div class="invoice-card">
            <div class="invoice-card-header">
              <h2 class="invoice-card-title">{{ t('invoice.apply.ordersTitle') }}</h2>
              <span class="invoice-card-note">{{ t('invoice.apply.ordersHint') }}</span>
            </div>

            <div v-if="loadingOrders" class="invoice-loading">
              <span class="invoice-spinner"></span>
            </div>
            <p v-else-if="orders.length === 0" class="invoice-empty">
              {{ t('invoice.apply.ordersEmpty') }}
            </p>
            <div v-else class="invoice-table-scroll">
              <table class="invoice-table">
                <thead>
                  <tr>
                    <th class="invoice-table-check">
                      <input
                        type="checkbox"
                        :checked="allSelected"
                        :aria-label="t('invoice.apply.selectAll')"
                        @change="toggleAll"
                      />
                    </th>
                    <th>{{ t('invoice.apply.columnDescription') }}</th>
                    <th>{{ t('invoice.apply.columnDate') }}</th>
                    <th class="invoice-align-right">{{ t('invoice.apply.columnAmount') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="order in orders"
                    :key="order.order_id"
                    :class="{ 'invoice-row-selected': selectedIds.includes(order.order_id) }"
                  >
                    <td class="invoice-table-check">
                      <input
                        type="checkbox"
                        :value="order.order_id"
                        :checked="selectedIds.includes(order.order_id)"
                        :aria-label="order.description"
                        @change="toggleOrder(order.order_id)"
                      />
                    </td>
                    <td>
                      <span class="invoice-order-desc">{{ order.description }}</span>
                      <span class="invoice-order-no">{{ order.out_trade_no }}</span>
                    </td>
                    <td>{{ formatDateTime(order.created_at) || '-' }}</td>
                    <td class="invoice-align-right">{{ formatCreditValue(order.amount) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <aside class="invoice-summary">
          <h2 class="invoice-card-title">{{ t('invoice.apply.summaryTitle') }}</h2>

          <div class="invoice-summary-row">
            <span>{{ t('invoice.apply.summarySelected', { count: selectedIds.length }) }}</span>
            <strong>{{ formatCreditValue(selectedAmount) }}</strong>
          </div>

          <div class="invoice-summary-total">
            <span>{{ t('invoice.apply.summaryTotal') }}</span>
            <strong>{{ formatCreditValue(selectedAmount) }}</strong>
          </div>

          <button
            type="button"
            class="invoice-submit"
            :disabled="!canSubmit || submitting"
            @click="submitRequest"
          >
            {{ submitting ? t('invoice.apply.submitting') : t('invoice.apply.submit') }}
          </button>

          <p class="invoice-hint">{{ t('invoice.apply.manualHint') }}</p>
        </aside>
      </section>

      <!-- 历史发票 -->
      <section v-else class="invoice-card invoice-history">
        <div class="invoice-history-toolbar">
          <input
            v-model="search"
            type="search"
            class="invoice-input invoice-search"
            :placeholder="t('invoice.history.searchPlaceholder')"
            @keyup.enter="reloadHistory"
          />
          <Select
            v-model="statusFilter"
            :options="statusOptions"
            class="invoice-status-select"
            @change="reloadHistory"
          />
        </div>

        <div v-if="loadingHistory" class="invoice-loading">
          <span class="invoice-spinner"></span>
        </div>
        <p v-else-if="invoices.length === 0" class="invoice-empty">
          {{ t('invoice.history.empty') }}
        </p>
        <div v-else class="invoice-table-scroll">
          <table class="invoice-table">
            <thead>
              <tr>
                <th>{{ t('invoice.history.columnDate') }}</th>
                <th>{{ t('invoice.history.columnInvoiceNo') }}</th>
                <th class="invoice-align-right">{{ t('invoice.history.columnAmount') }}</th>
                <th>{{ t('invoice.history.columnStatus') }}</th>
                <th class="invoice-align-right">{{ t('invoice.history.columnAction') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="invoice in invoices" :key="invoice.id">
                <td>{{ formatDateTime(invoice.created_at) || '-' }}</td>
                <td>
                  <span class="invoice-no">{{ invoice.invoice_no }}</span>
                  <span v-if="invoice.issued_invoice_no" class="invoice-order-no">
                    {{ t('invoice.history.issuedNo') }}: {{ invoice.issued_invoice_no }}
                  </span>
                  <span v-else-if="invoice.reject_reason" class="invoice-reject">
                    {{ t('invoice.history.rejectReason') }}: {{ invoice.reject_reason }}
                  </span>
                </td>
                <td class="invoice-align-right">{{ formatCreditValue(invoice.amount) }}</td>
                <td>
                  <span :class="['invoice-status', `invoice-status--${invoice.status}`]">
                    {{ t(`invoice.status.${invoice.status}`) }}
                  </span>
                </td>
                <td class="invoice-align-right">
                  <a
                    v-if="invoice.status === 'issued' && downloadUrl(invoice)"
                    :href="downloadUrl(invoice)"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="invoice-action"
                  >{{ t('invoice.history.download') }}</a>
                  <button
                    v-else-if="invoice.status === 'pending'"
                    type="button"
                    class="invoice-action invoice-action-danger"
                    @click="cancelRequest(invoice)"
                  >{{ t('invoice.history.cancel') }}</button>
                  <span v-else class="invoice-action-empty">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-if="invoices.length" class="invoice-history-footer">
          {{ t('invoice.history.showing', { total }) }}
        </p>
      </section>
    </div>

    <BaseDialog
      :show="cancelTarget !== null"
      :title="t('invoice.history.cancel')"
      width="narrow"
      @close="closeCancel"
    >
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('invoice.history.cancelConfirm') }}</p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="cancelling" @click="closeCancel">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-danger" :disabled="cancelling" @click="confirmCancel">
            {{ t('invoice.history.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import invoiceAPI, { type Invoice, type InvoiceEntityType, type InvoiceableOrder } from '@/api/invoice'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { formatCredits, usdToCredits } from '@/utils/credit'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import { sanitizeUrl } from '@/utils/url'

type InvoiceTab = 'apply' | 'history'

function normalizeTab(value: unknown): InvoiceTab {
  return value === 'history' ? 'history' : 'apply'
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const activeTab = ref<InvoiceTab>(normalizeTab(route.query.tab))

const orders = ref<InvoiceableOrder[]>([])
const selectedIds = ref<number[]>([])
const loadingOrders = ref(false)
const submitting = ref(false)

const invoices = ref<Invoice[]>([])
const total = ref(0)
const loadingHistory = ref(false)
const search = ref('')
const statusFilter = ref('')

const form = reactive({
  entity_type: 'company' as InvoiceEntityType,
  title: '',
  tax_id: '',
  delivery_email: '',
  notes: '',
})

const cancelTarget = ref<Invoice | null>(null)
const cancelling = ref(false)

const isCompany = computed(() => form.entity_type === 'company')

const statusOptions = computed(() => [
  { value: '', label: t('invoice.history.allStatuses') },
  { value: 'pending', label: t('invoice.status.pending') },
  { value: 'issued', label: t('invoice.status.issued') },
  { value: 'rejected', label: t('invoice.status.rejected') },
  { value: 'cancelled', label: t('invoice.status.cancelled') },
])

const selectedAmount = computed(() =>
  orders.value
    .filter((o) => selectedIds.value.includes(o.order_id))
    .reduce((sum, o) => sum + Number(o.amount || 0), 0)
)

const allSelected = computed(
  () => orders.value.length > 0 && selectedIds.value.length === orders.value.length
)

const canSubmit = computed(() => {
  if (selectedIds.value.length === 0) return false
  if (!form.title.trim() || !form.delivery_email.trim()) return false
  // 企业发票必须有纳税人识别号，和后端校验保持一致，避免提交后才报错。
  if (isCompany.value && !form.tax_id.trim()) return false
  return true
})

// 金额来自接口的 USD，界面统一用 Credits 展示。
const formatCreditValue = (usd: number) => `${formatCredits(usdToCredits(usd))} Credits`

// 发票文件地址由管理员填写，属于外部输入，渲染成链接前必须过滤协议。
function downloadUrl(invoice: Invoice): string {
  return sanitizeUrl(invoice.issued_file_url || '')
}

function selectTab(tab: InvoiceTab) {
  activeTab.value = tab
  void router.replace({ path: route.path, query: { ...route.query, tab } })
}

watch(() => route.query.tab, (value) => {
  activeTab.value = normalizeTab(value)
})

// 切到历史页时按需加载，避免一进页面就发两个请求。
watch(activeTab, (tab) => {
  if (tab === 'history' && invoices.value.length === 0 && !loadingHistory.value) {
    void reloadHistory()
  }
})

function toggleOrder(orderID: number) {
  const index = selectedIds.value.indexOf(orderID)
  if (index >= 0) {
    selectedIds.value.splice(index, 1)
  } else {
    selectedIds.value.push(orderID)
  }
}

function toggleAll() {
  selectedIds.value = allSelected.value ? [] : orders.value.map((o) => o.order_id)
}

async function loadOrders() {
  loadingOrders.value = true
  try {
    orders.value = await invoiceAPI.getInvoiceableOrders()
    // 重新加载后丢弃已不可开票的选择，否则提交会被后端整体拒绝。
    const available = new Set(orders.value.map((o) => o.order_id))
    selectedIds.value = selectedIds.value.filter((id) => available.has(id))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('invoice.apply.loadOrdersFailed')))
  } finally {
    loadingOrders.value = false
  }
}

async function reloadHistory() {
  loadingHistory.value = true
  try {
    const result = await invoiceAPI.listInvoices({
      page: 1,
      page_size: 20,
      status: statusFilter.value || undefined,
      search: search.value.trim() || undefined,
    })
    invoices.value = result.items ?? []
    total.value = result.total ?? 0
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('invoice.history.loadFailed')))
  } finally {
    loadingHistory.value = false
  }
}

async function submitRequest() {
  if (!canSubmit.value || submitting.value) return

  submitting.value = true
  try {
    const invoice = await invoiceAPI.createInvoice({
      entity_type: form.entity_type,
      title: form.title.trim(),
      tax_id: isCompany.value ? form.tax_id.trim() : undefined,
      delivery_email: form.delivery_email.trim(),
      notes: form.notes.trim() || undefined,
      order_ids: [...selectedIds.value],
    })
    appStore.showSuccess(t('invoice.apply.submitSuccess', { invoiceNo: invoice.invoice_no }))
    selectedIds.value = []
    // 提交后订单已被占用，两个列表都要刷新才不会显示过期数据。
    await Promise.all([loadOrders(), reloadHistory()])
    selectTab('history')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('invoice.apply.submitFailed')))
  } finally {
    submitting.value = false
  }
}

function cancelRequest(invoice: Invoice) {
  cancelTarget.value = invoice
}

function closeCancel() {
  if (cancelling.value) return
  cancelTarget.value = null
}

async function confirmCancel() {
  const target = cancelTarget.value
  if (!target || cancelling.value) return

  cancelling.value = true
  try {
    await invoiceAPI.cancelInvoice(target.id)
    appStore.showSuccess(t('invoice.history.cancelSuccess'))
    cancelTarget.value = null
    // 撤回释放了订单，可开票列表也要跟着变。
    await Promise.all([reloadHistory(), loadOrders()])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('invoice.history.cancelFailed')))
  } finally {
    cancelling.value = false
  }
}

onMounted(() => {
  // 接收邮箱默认填账号邮箱，多数情况下用户不需要改。
  form.delivery_email = authStore.user?.email?.trim() || ''
  void loadOrders()
  if (activeTab.value === 'history') {
    void reloadHistory()
  }
})
</script>

<style scoped>
.invoice-page {
  --invoice-surface: var(--md-surface-container-low);
  --invoice-surface-deep: rgb(0 0 0 / 30%);
  --invoice-border: var(--md-outline-variant);
  --invoice-text: var(--md-on-surface);
  --invoice-muted: var(--md-on-surface-variant);
  --invoice-accent: #00e38b;
  color: var(--invoice-text);
}

.invoice-header {
  max-width: 1280px;
  margin: 0 auto 24px;
}

.invoice-kicker {
  margin-bottom: 10px;
  color: var(--invoice-accent);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.invoice-title {
  color: var(--md-on-surface);
  font-family: 'Geist', ui-sans-serif, system-ui, sans-serif;
  font-size: 2.5rem;
  font-weight: 760;
  line-height: 1.1;
  letter-spacing: -0.01em;
}

.invoice-description {
  max-width: 640px;
  margin-top: 10px;
  color: var(--invoice-muted);
  font-size: 0.95rem;
  line-height: 1.6;
}

.invoice-tabs {
  display: flex;
  gap: 8px;
  max-width: 1280px;
  margin: 0 auto 24px;
  border-bottom: 1px solid var(--invoice-border);
}

.invoice-tabs button {
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  padding: 10px 4px;
  margin-right: 20px;
  color: var(--invoice-muted);
  font-size: 0.95rem;
  cursor: pointer;
  transition: color 150ms ease, border-color 150ms ease;
}

.invoice-tabs button:hover {
  color: var(--invoice-text);
}

.invoice-tabs button.invoice-tab-active {
  border-bottom-color: var(--invoice-accent);
  color: var(--md-on-surface);
  font-weight: 700;
}

.invoice-apply {
  display: grid;
  max-width: 1280px;
  margin: 0 auto;
  gap: 24px;
  align-items: start;
}

@media (min-width: 1024px) {
  .invoice-apply {
    grid-template-columns: minmax(0, 1fr) minmax(300px, 340px);
  }
}

.invoice-apply-main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 24px;
}

.invoice-card {
  border: 1px solid var(--invoice-border);
  border-radius: 8px;
  background: var(--invoice-surface);
  padding: 24px;
  transition: border-color 180ms ease;
}

.invoice-card:hover {
  border-color: var(--invoice-accent);
}

.invoice-card-header {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.invoice-card-title {
  color: var(--md-on-surface);
  font-size: 1.1rem;
  font-weight: 700;
}

.invoice-card-note {
  color: var(--invoice-muted);
  font-size: 0.78rem;
}

.invoice-field {
  margin-top: 18px;
  border: none;
  padding: 0;
}

.invoice-field-grid {
  display: grid;
  gap: 18px;
}

@media (min-width: 768px) {
  .invoice-field-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.invoice-label {
  display: block;
  margin-bottom: 8px;
  padding: 0;
  color: var(--invoice-muted);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.invoice-radio-row {
  display: flex;
  gap: 20px;
}

.invoice-radio {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--invoice-text);
  font-size: 0.9rem;
  cursor: pointer;
}

.invoice-radio input {
  accent-color: var(--invoice-accent);
}

.invoice-input {
  width: 100%;
  border: 1px solid var(--invoice-border);
  border-radius: 4px;
  background: var(--invoice-surface-deep);
  padding: 9px 12px;
  color: var(--invoice-text);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.85rem;
  outline: none;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.invoice-input::placeholder {
  color: #6c7f77;
}

.invoice-input:focus {
  border-color: var(--invoice-accent);
  box-shadow: 0 0 0 1px rgb(0 227 139 / 30%);
}

.invoice-input:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.invoice-textarea {
  resize: vertical;
}

.invoice-hint {
  margin-top: 8px;
  color: var(--invoice-muted);
  font-size: 0.75rem;
  line-height: 1.5;
}

.invoice-loading {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
}

.invoice-spinner {
  height: 28px;
  width: 28px;
  border: 2px solid var(--invoice-border);
  border-top-color: var(--invoice-accent);
  border-radius: 999px;
  animation: invoice-spin 0.8s linear infinite;
}

@keyframes invoice-spin {
  to { transform: rotate(360deg); }
}

.invoice-empty {
  margin-top: 18px;
  border: 1px dashed var(--invoice-border);
  border-radius: 6px;
  padding: 28px;
  text-align: center;
  color: var(--invoice-muted);
  font-size: 0.88rem;
}

.invoice-table-scroll {
  margin-top: 18px;
  overflow-x: auto;
}

.invoice-table {
  width: 100%;
  min-width: 520px;
  border-collapse: collapse;
  text-align: left;
}

.invoice-table thead tr {
  border-bottom: 1px solid var(--invoice-border);
}

.invoice-table th {
  padding: 0 14px 10px 0;
  color: var(--invoice-muted);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.64rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.invoice-table tbody tr {
  border-bottom: 1px solid rgb(59 74 63 / 50%);
}

.invoice-table tbody tr:last-child {
  border-bottom: none;
}

.invoice-table tbody tr.invoice-row-selected {
  background: rgb(0 227 139 / 6%);
}

.invoice-table td {
  padding: 12px 14px 12px 0;
  color: var(--invoice-text);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.8rem;
  vertical-align: top;
}

.invoice-table-check {
  width: 36px;
  padding-right: 8px;
}

.invoice-table-check input {
  accent-color: var(--invoice-accent);
}

.invoice-align-right {
  text-align: right;
}

.invoice-order-desc,
.invoice-no {
  display: block;
  color: var(--invoice-text);
}

.invoice-order-no {
  display: block;
  margin-top: 3px;
  color: var(--invoice-muted);
  font-size: 0.7rem;
}

.invoice-reject {
  display: block;
  margin-top: 3px;
  color: #ffb4ab;
  font-size: 0.7rem;
}

.invoice-summary {
  position: sticky;
  top: 24px;
  border: 1px solid rgb(0 227 139 / 40%);
  border-radius: 8px;
  background: var(--invoice-surface);
  padding: 24px;
  box-shadow: 0 0 20px rgb(0 227 139 / 6%);
}

.invoice-summary-row,
.invoice-summary-total {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  font-size: 0.85rem;
}

.invoice-summary-row span {
  color: var(--invoice-muted);
}

.invoice-summary-total {
  border-top: 1px solid var(--invoice-border);
  padding-top: 16px;
}

.invoice-summary-total strong {
  color: var(--md-on-surface);
  font-size: 1.5rem;
  font-weight: 760;
}

.invoice-submit {
  width: 100%;
  margin-top: 20px;
  border: none;
  border-radius: 6px;
  background: var(--invoice-accent);
  padding: 11px 16px;
  color: #00230f;
  font-size: 0.88rem;
  font-weight: 700;
  cursor: pointer;
  transition: opacity 150ms ease;
}

.invoice-submit:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.invoice-submit:not(:disabled):hover {
  opacity: 0.9;
}

.invoice-history {
  max-width: 1280px;
  margin: 0 auto;
}

.invoice-history-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.invoice-search {
  flex: 1;
  min-width: 200px;
}

.invoice-status-select {
  min-width: 160px;
}

.invoice-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.78rem;
}

.invoice-status::before {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentcolor;
  content: '';
}

.invoice-status--issued { color: var(--invoice-accent); }
.invoice-status--pending { color: #8eb5ff; }
.invoice-status--rejected { color: #ffb4ab; }
.invoice-status--cancelled { color: var(--invoice-muted); }

.invoice-action {
  border: 1px solid var(--invoice-border);
  border-radius: 4px;
  background: none;
  padding: 5px 10px;
  color: var(--invoice-text);
  font-size: 0.75rem;
  text-decoration: none;
  cursor: pointer;
  transition: border-color 150ms ease, color 150ms ease;
}

.invoice-action:hover {
  border-color: var(--invoice-accent);
  color: var(--invoice-accent);
}

.invoice-action-danger:hover {
  border-color: #ffb4ab;
  color: #ffb4ab;
}

.invoice-action-empty {
  color: var(--invoice-muted);
}

.invoice-history-footer {
  margin-top: 18px;
  color: var(--invoice-muted);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.75rem;
}
</style>
