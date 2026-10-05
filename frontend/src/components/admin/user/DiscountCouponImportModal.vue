<template>
  <BaseDialog :show="show" :title="t('admin.users.couponImport.title')" width="extra-wide" :z-index="60"
    :show-close-button="!busy" :close-on-escape="!busy" @close="close">
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.users.couponImport.instructions') }}</p>
      <div class="flex flex-wrap gap-3">
        <button v-for="type in couponTypes" :key="type" type="button" class="btn btn-secondary" :disabled="busy"
          :data-test="`template-${type}`" @click="downloadTemplate(type)">
          <Icon name="download" size="sm" />
          {{ t(`admin.users.couponImport.templates.${type}`) }}
        </button>
      </div>
      <div>
        <label class="input-label" for="coupon-import-file">{{ t('admin.users.couponImport.chooseFile') }}</label>
        <input id="coupon-import-file" ref="fileInput" type="file" accept=".xlsx,.xls" class="input"
          :disabled="busy" @change="selectFile" />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.users.couponImport.fileHint') }}</p>
      </div>
      <p v-if="busy" role="status" class="text-sm text-gray-600 dark:text-gray-300">
        {{ t(`admin.users.couponImport.phases.${phase}`) }}
      </p>
      <p v-if="fileError" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ fileError }}</p>
      <template v-if="rows.length">
        <p v-if="issuedCount" role="status" class="text-sm text-emerald-700 dark:text-emerald-300">
          {{ t('admin.users.couponImport.success', { count: issuedCount }) }}
        </p>
        <p v-else class="text-sm" :class="errorCount ? 'text-red-700 dark:text-red-300' : 'text-gray-600 dark:text-gray-300'">
          {{ t('admin.users.couponImport.previewSummary', { count: rows.length, errors: errorCount }) }}
        </p>
        <div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="w-full text-left text-sm">
            <thead class="sticky top-0 bg-gray-50 text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              <tr>
                <th v-for="column in previewColumns" :key="column" scope="col" class="whitespace-nowrap px-3 py-2">{{ t(`admin.users.couponImport.previewColumns.${column}`) }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
              <tr v-for="row in rows" :key="row.data.row_number" data-test="import-row">
                <td class="px-3 py-2">{{ row.data.row_number }}</td>
                <td class="px-3 py-2">{{ resultFor(row)?.user_id || row.data.user_id || '—' }}</td>
                <td class="px-3 py-2">{{ resultFor(row)?.email || row.data.email || '—' }}</td>
                <td class="whitespace-nowrap px-3 py-2">{{ row.data.coupon_type ? t(`admin.users.rechargeCoupon.types.${row.data.coupon_type}`) : '—' }}</td>
                <td class="whitespace-nowrap px-3 py-2">{{ row.data.coupon_type === 'subscription' ? '$' : '¥' }}{{ displayNumber(row.data.min_amount) }}</td>
                <td class="px-3 py-2">{{ displayNumber(row.data.discount_rate) }}</td>
                <td class="px-3 py-2">{{ displayNumber(row.data.total_uses) }}</td>
                <td class="max-w-48 break-words px-3 py-2">{{ row.data.notes || '—' }}</td>
                <td class="min-w-40 px-3 py-2" :class="rowError(row) ? 'text-red-700 dark:text-red-300' : 'text-emerald-700 dark:text-emerald-300'">
                  {{ rowError(row) || t(resultFor(row)?.coupon_id ? 'admin.users.couponImport.issued' : validated ? 'admin.users.couponImport.ready' : 'admin.users.couponImport.pending') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">{{ t('common.close') }}</button>
        <button type="button" data-test="issue-import" class="btn btn-primary" :disabled="!canIssue" @click="issue">
          {{ t('admin.users.couponImport.issue', { count: rows.length }) }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { DiscountCouponImportRowResult, DiscountCouponType } from '@/api/admin/users'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { CouponExcelError, downloadCouponTemplate, readCouponExcel, type ParsedCouponRow } from './discountCouponExcel'

const props = defineProps<{ show: boolean; user?: { id: number; email: string } | null }>()
const emit = defineEmits<{ close: []; success: [] }>()
const { t, te } = useI18n()
const appStore = useAppStore()
const couponTypes: DiscountCouponType[] = ['recharge', 'subscription']
const previewColumns = ['row', 'user_id', 'email', 'coupon_type', 'min_amount', 'discount_rate', 'total_uses', 'notes', 'status']
const phase = ref<'idle' | 'parsing' | 'validating' | 'issuing'>('idle')
const busy = computed(() => phase.value !== 'idle')
const rows = ref<ParsedCouponRow[]>([])
const results = ref<DiscountCouponImportRowResult[]>([])
const resultMap = computed(() => new Map(results.value.map(row => [row.row_number, row])))
const fileInput = ref<HTMLInputElement>()
const fileError = ref('')
const validated = ref(false)
const issuedCount = ref(0)
let requestKey = ''

watch(() => props.show, show => { if (show) reset() })

function reset() {
  rows.value = []
  results.value = []
  fileError.value = ''
  validated.value = false
  issuedCount.value = 0
  if (fileInput.value) fileInput.value.value = ''
  requestKey = `coupon-import-${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
}

function close() { if (!busy.value) emit('close') }
function displayNumber(value: number) { return Number.isFinite(value) ? value : '—' }
function resultFor(row: ParsedCouponRow) { return resultMap.value.get(row.data.row_number) }
function rowError(row: ParsedCouponRow) {
  const result = resultFor(row)
  const code = row.errorCode || result?.error_code
  const key = `admin.users.couponImport.errors.${code}`
  return code && te(key) ? t(key) : result?.error || (code ? t('admin.users.couponImport.invalidRow') : '')
}
const errorCount = computed(() => rows.value.filter(row => rowError(row)).length)
const canIssue = computed(() => !busy.value && validated.value && rows.value.length > 0 && !errorCount.value && !issuedCount.value)

async function downloadTemplate(type: DiscountCouponType) {
  try { await downloadCouponTemplate(type, t, props.user ?? undefined) }
  catch { appStore.showError(t('admin.users.couponImport.downloadFailed')) }
}

async function selectFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file || busy.value) return
  reset()
  phase.value = 'parsing'
  try {
    rows.value = await readCouponExcel(file)
    if (rows.value.some(row => row.errorCode)) return
    phase.value = 'validating'
    const result = await adminAPI.users.importDiscountCoupons(rows.value.map(row => row.data), true, requestKey)
    results.value = result.rows
    validated.value = result.valid
  } catch (error: unknown) {
    fileError.value = error instanceof CouponExcelError
      ? t(`admin.users.couponImport.errors.${error.code}`)
      : t('admin.users.couponImport.validateFailed')
  } finally { phase.value = 'idle' }
}

async function issue() {
  if (!canIssue.value) return
  phase.value = 'issuing'
  fileError.value = ''
  try {
    const batch = rows.value.map(row => ({
      ...row.data,
      user_id: resultFor(row)?.user_id ?? row.data.user_id,
      email: resultFor(row)?.email ?? row.data.email,
    }))
    const result = await adminAPI.users.importDiscountCoupons(batch, false, requestKey)
    results.value = result.rows.map(row => ({ ...resultMap.value.get(row.row_number), ...row }))
    validated.value = result.valid
    issuedCount.value = result.issued_count
    if (issuedCount.value > 0) {
      appStore.showSuccess(t('admin.users.couponImport.success', { count: issuedCount.value }))
      emit('success')
    }
  } catch {
    fileError.value = t('admin.users.couponImport.issueFailed')
  } finally { phase.value = 'idle' }
}
</script>
