<template>
  <AppLayout>
    <div data-testid="admin-invoices-shell" class="space-y-5">
      <header>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t('admin.invoices.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.invoices.description') }}
        </p>
      </header>

      <div class="card p-4">
        <div class="flex flex-wrap gap-3">
          <input
            v-model="search"
            type="search"
            class="input flex-1"
            :placeholder="t('admin.invoices.searchPlaceholder')"
            @keyup.enter="reload"
          />
          <Select
            v-model="statusFilter"
            :options="statusOptions"
            class="w-48"
            @change="reload"
          />
        </div>
      </div>

      <div class="card">
        <div v-if="loading" class="flex justify-center py-12">
          <div class="admin-invoice-spinner"></div>
        </div>
        <EmptyState v-else-if="invoices.length === 0" :description="t('admin.invoices.empty')" />
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[1080px] text-left text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-dark-700 dark:text-dark-400">
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnCreatedAt') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnInvoiceNo') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnTitle') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnTaxId') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnEmail') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.invoices.columnAmount') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnOrders') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.invoices.columnStatus') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.invoices.columnAction') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="invoice in invoices"
                :key="invoice.id"
                class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
              >
                <td class="px-3 py-3 text-gray-700 dark:text-gray-300">
                  {{ formatDateTime(invoice.created_at) || '-' }}
                </td>
                <td class="px-3 py-3">
                  <span class="font-mono text-gray-900 dark:text-white">{{ invoice.invoice_no }}</span>
                  <span v-if="invoice.issued_invoice_no" class="block font-mono text-xs text-emerald-600 dark:text-emerald-400">
                    {{ invoice.issued_invoice_no }}
                  </span>
                </td>
                <td class="px-3 py-3 text-gray-900 dark:text-white">
                  {{ invoice.title }}
                  <span class="block text-xs text-gray-500 dark:text-gray-400">
                    {{ invoice.entity_type === 'company'
                      ? t('invoice.apply.entityCompany')
                      : t('invoice.apply.entityIndividual') }}
                  </span>
                </td>
                <td class="px-3 py-3 font-mono text-xs text-gray-700 dark:text-gray-300">
                  {{ invoice.tax_id || '-' }}
                </td>
                <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ invoice.delivery_email }}</td>
                <td class="px-3 py-3 text-right font-medium text-gray-900 dark:text-white">
                  {{ formatCreditValue(invoice.amount) }}
                </td>
                <td class="px-3 py-3 text-gray-700 dark:text-gray-300">
                  {{ t('admin.invoices.orderCount', { count: invoice.items?.length ?? 0 }) }}
                </td>
                <td class="px-3 py-3">
                  <span :class="['badge', statusBadgeClass(invoice.status)]">
                    {{ t(`invoice.status.${invoice.status}`) }}
                  </span>
                  <span v-if="invoice.reject_reason" class="mt-1 block text-xs text-red-600 dark:text-red-400">
                    {{ invoice.reject_reason }}
                  </span>
                </td>
                <td class="px-3 py-3 text-right">
                  <div v-if="invoice.status === 'pending'" class="flex justify-end gap-2">
                    <button type="button" class="btn btn-primary btn-sm" @click="openIssue(invoice)">
                      {{ t('admin.invoices.issue') }}
                    </button>
                    <button type="button" class="btn btn-secondary btn-sm" @click="openReject(invoice)">
                      {{ t('admin.invoices.reject') }}
                    </button>
                  </div>
                  <span v-else class="text-gray-400 dark:text-gray-500">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <BaseDialog
      :show="issueTarget !== null"
      :title="t('admin.invoices.issueTitle')"
      width="narrow"
      @close="closeIssue"
    >
      <div class="space-y-4">
        <div>
          <label for="admin-issued-no" class="input-label">{{ t('admin.invoices.issuedNo') }}</label>
          <input
            id="admin-issued-no"
            v-model="issueForm.issued_invoice_no"
            type="text"
            class="input"
            :placeholder="t('admin.invoices.issuedNoPlaceholder')"
          />
        </div>
        <div>
          <label for="admin-issued-url" class="input-label">{{ t('admin.invoices.issuedFileUrl') }}</label>
          <input
            id="admin-issued-url"
            v-model="issueForm.issued_file_url"
            type="url"
            class="input"
            :placeholder="t('admin.invoices.issuedFileUrlPlaceholder')"
          />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeIssue">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving || !issueForm.issued_invoice_no.trim()"
            @click="submitIssue"
          >
            {{ t('admin.invoices.issueSubmit') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="rejectTarget !== null"
      :title="t('admin.invoices.rejectTitle')"
      width="narrow"
      @close="closeReject"
    >
      <div class="space-y-4">
        <div>
          <label for="admin-reject-reason" class="input-label">{{ t('admin.invoices.rejectReason') }}</label>
          <textarea
            id="admin-reject-reason"
            v-model="rejectReason"
            rows="3"
            class="input"
            :placeholder="t('admin.invoices.rejectReasonPlaceholder')"
          ></textarea>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.invoices.rejectHint') }}</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeReject">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-danger"
            :disabled="saving || !rejectReason.trim()"
            @click="submitReject"
          >
            {{ t('admin.invoices.rejectSubmit') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import { adminAPI } from '@/api/admin'
import type { Invoice } from '@/api/invoice'
import { useAppStore } from '@/stores/app'
import { formatCredits, usdToCredits } from '@/utils/credit'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const invoices = ref<Invoice[]>([])
const loading = ref(false)
const saving = ref(false)
const search = ref('')
const statusFilter = ref('')

const issueTarget = ref<Invoice | null>(null)
const rejectTarget = ref<Invoice | null>(null)
const rejectReason = ref('')
const issueForm = reactive({ issued_invoice_no: '', issued_file_url: '' })

const statusOptions = computed(() => [
  { value: '', label: t('admin.invoices.allStatuses') },
  { value: 'pending', label: t('invoice.status.pending') },
  { value: 'issued', label: t('invoice.status.issued') },
  { value: 'rejected', label: t('invoice.status.rejected') },
  { value: 'cancelled', label: t('invoice.status.cancelled') },
])

// 金额来自接口的 USD，界面统一按 Credits 展示。
const formatCreditValue = (usd: number) => `${formatCredits(usdToCredits(usd))} Credits`

function statusBadgeClass(status: string): string {
  if (status === 'issued') return 'badge-success'
  if (status === 'pending') return 'badge-primary'
  if (status === 'rejected') return 'badge-danger'
  return 'badge-gray'
}

async function reload() {
  loading.value = true
  try {
    const result = await adminAPI.invoices.listInvoices({
      page: 1,
      page_size: 50,
      status: statusFilter.value || undefined,
      search: search.value.trim() || undefined,
    })
    invoices.value = result.items ?? []
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.invoices.loadFailed')))
  } finally {
    loading.value = false
  }
}

function openIssue(invoice: Invoice) {
  issueTarget.value = invoice
  issueForm.issued_invoice_no = ''
  issueForm.issued_file_url = ''
}

function closeIssue() {
  if (saving.value) return
  issueTarget.value = null
}

function openReject(invoice: Invoice) {
  rejectTarget.value = invoice
  rejectReason.value = ''
}

function closeReject() {
  if (saving.value) return
  rejectTarget.value = null
}

async function submitIssue() {
  const target = issueTarget.value
  if (!target || saving.value) return

  saving.value = true
  try {
    await adminAPI.invoices.issueInvoice(target.id, {
      issued_invoice_no: issueForm.issued_invoice_no.trim(),
      issued_file_url: issueForm.issued_file_url.trim() || undefined,
    })
    appStore.showSuccess(t('admin.invoices.issueSuccess'))
    issueTarget.value = null
    await reload()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.invoices.issueFailed')))
  } finally {
    saving.value = false
  }
}

async function submitReject() {
  const target = rejectTarget.value
  if (!target || saving.value) return

  saving.value = true
  try {
    await adminAPI.invoices.rejectInvoice(target.id, rejectReason.value.trim())
    appStore.showSuccess(t('admin.invoices.rejectSuccess'))
    rejectTarget.value = null
    await reload()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.invoices.rejectFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void reload()
})
</script>

<style scoped>
.admin-invoice-spinner {
  height: 32px;
  width: 32px;
  border: 2px solid var(--md-outline-variant);
  border-top-color: var(--md-primary);
  border-radius: 999px;
  animation: admin-invoice-spin 0.8s linear infinite;
}

@keyframes admin-invoice-spin {
  to { transform: rotate(360deg); }
}
</style>
