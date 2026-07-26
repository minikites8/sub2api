<template>
  <AppLayout>
    <div class="affiliate-page">
      <div v-if="loading" class="affiliate-loading">
        <div class="affiliate-spinner"></div>
      </div>

      <template v-else-if="detail">
        <header class="affiliate-header">
          <p class="affiliate-kicker">AFFILIATE / REFERRALS</p>
          <h1 class="affiliate-title">{{ t('affiliate.title') }}</h1>
          <p class="affiliate-description">{{ t('affiliate.description') }}</p>
        </header>

        <div class="affiliate-stat-grid">
          <div class="affiliate-stat-card">
            <span class="affiliate-stat-label">{{ t('affiliate.stats.invitedUsers') }}</span>
            <strong class="affiliate-stat-value">{{ formatCount(detail.aff_count) }}</strong>
          </div>

          <div class="affiliate-stat-card">
            <span class="affiliate-stat-label">{{ t('affiliate.stats.totalQuota') }}</span>
            <strong class="affiliate-stat-value">{{ formatCurrency(detail.aff_history_quota) }}</strong>
          </div>

          <div class="affiliate-stat-card">
            <span class="affiliate-stat-label">{{ t('affiliate.stats.rebateRate') }}</span>
            <strong class="affiliate-stat-value">{{ formattedRebateRate }}<small>%</small></strong>
            <span class="affiliate-stat-meta">{{ t('affiliate.stats.rebateRateHint') }}</span>
          </div>

          <div class="affiliate-stat-card affiliate-stat-card-action">
            <span class="affiliate-stat-label">{{ t('affiliate.stats.availableQuota') }}</span>
            <strong class="affiliate-stat-value">{{ formatCurrency(detail.aff_quota) }}</strong>
            <span v-if="detail.aff_frozen_quota > 0" class="affiliate-stat-meta">
              {{ t('affiliate.stats.frozenQuota') }}: {{ formatCurrency(detail.aff_frozen_quota) }}
            </span>
            <button
              type="button"
              class="affiliate-claim-button"
              :disabled="transferring || detail.aff_quota <= 0"
              @click="transferQuota"
            >
              <Icon v-if="transferring" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="dollar" size="sm" />
              {{ transferring ? t('affiliate.transfer.transferring') : t('affiliate.transfer.button') }}
            </button>
            <p v-if="detail.aff_quota <= 0" class="affiliate-claim-hint">
              {{ t('affiliate.transfer.empty') }}
            </p>
          </div>
        </div>

        <div class="affiliate-card affiliate-invite-card">
          <h2 class="affiliate-card-title">
            <Icon name="link" size="sm" />
            {{ t('affiliate.inviteLink') }}
          </h2>

          <div class="affiliate-code-row">
            <code class="affiliate-code-value">{{ inviteLink }}</code>
            <button type="button" class="affiliate-copy-button" @click="copyInviteLink">
              <Icon name="copy" size="sm" />
              {{ t('affiliate.copyLink') }}
            </button>
          </div>

          <div class="affiliate-tips">
            <h3 class="affiliate-tips-title">
              <Icon name="lightbulb" size="sm" />
              {{ t('affiliate.tips.title') }}
            </h3>
            <ul class="affiliate-tips-list">
              <li>{{ t('affiliate.tips.line1') }}</li>
              <li>{{ t('affiliate.tips.line2', { rate: `${formattedRebateRate}%` }) }}</li>
              <li>{{ t('affiliate.tips.line3') }}</li>
              <li v-if="detail.aff_frozen_quota > 0">{{ t('affiliate.tips.line4') }}</li>
            </ul>
          </div>
        </div>

        <div class="affiliate-card affiliate-history-card">
          <div class="affiliate-history-header">
            <h2 class="affiliate-card-title">
              <Icon name="clock" size="sm" />
              {{ t('affiliate.invitees.title') }}
            </h2>
            <button
              type="button"
              class="affiliate-export-button"
              :disabled="exporting || detail.invitees.length === 0"
              @click="exportInviteesCSV"
            >
              <Icon name="download" size="sm" />
              {{ t('usage.exportCsv') }}
            </button>
          </div>

          <div v-if="detail.invitees.length === 0" class="affiliate-empty">
            {{ t('affiliate.invitees.empty') }}
          </div>
          <div v-else class="affiliate-table-scroll">
            <table class="affiliate-table">
              <thead>
                <tr>
                  <th>{{ t('affiliate.invitees.columns.email') }}</th>
                  <th>{{ t('affiliate.invitees.columns.username') }}</th>
                  <th>{{ t('affiliate.invitees.columns.joinedAt') }}</th>
                  <th class="affiliate-table-align-right">{{ t('affiliate.invitees.columns.rebate') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in detail.invitees" :key="item.user_id">
                  <td>{{ item.email || '-' }}</td>
                  <td>{{ item.username || '-' }}</td>
                  <td>{{ formatDateTime(item.created_at) || '-' }}</td>
                  <td class="affiliate-table-align-right affiliate-table-rebate">
                    +{{ formatCurrency(item.total_rebate) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import userAPI from '@/api/user'
import type { UserAffiliateDetail } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useClipboard } from '@/composables/useClipboard'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const transferring = ref(false)
const exporting = ref(false)
const detail = ref<UserAffiliateDetail | null>(null)

const inviteLink = computed(() => {
  if (!detail.value) return ''
  if (typeof window === 'undefined') return `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return `${window.location.origin}/register?aff=${encodeURIComponent(detail.value.aff_code)}`
})

// Rebate rate is a percentage in the range [0, 100]; backend already clamps it.
// We trim trailing zeros (e.g. 20.00 → "20", 12.50 → "12.5") for a cleaner UI.
const formattedRebateRate = computed(() => {
  const v = detail.value?.effective_rebate_rate_percent ?? 0
  const rounded = Math.round(v * 100) / 100
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
})

function formatCount(value: number): string {
  return value.toLocaleString()
}

async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
  }
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function copyInviteLink(): Promise<void> {
  if (!inviteLink.value) return
  await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))
}

async function transferQuota(): Promise<void> {
  if (!detail.value || detail.value.aff_quota <= 0 || transferring.value) return
  transferring.value = true
  try {
    const resp = await userAPI.transferAffiliateQuota()
    appStore.showSuccess(t('affiliate.transfer.success', { amount: formatCurrency(resp.transferred_quota) }))
    await Promise.all([
      loadAffiliateDetail(true),
      authStore.refreshUser().catch(() => undefined),
    ])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.transferFailed')))
  } finally {
    transferring.value = false
  }
}

const escapeCSVValue = (value: unknown): string => {
  if (value == null) return ''
  const str = String(value)
  const escaped = str.replace(/"/g, '""')
  if (/^[=+\-@\t\r]/.test(str)) return `"'${escaped}"`
  if (/[,"\n\r]/.test(str)) return `"${escaped}"`
  return str
}

function exportInviteesCSV(): void {
  if (!detail.value || detail.value.invitees.length === 0) {
    appStore.showWarning(t('affiliate.invitees.noDataToExport'))
    return
  }

  exporting.value = true
  try {
    const headers = [
      t('affiliate.invitees.columns.email'),
      t('affiliate.invitees.columns.username'),
      t('affiliate.invitees.columns.joinedAt'),
      t('affiliate.invitees.columns.rebate'),
    ]
    const rows = detail.value.invitees.map((item) => [
      item.email || '',
      item.username || '',
      item.created_at,
      formatCurrency(item.total_rebate),
    ].map(escapeCSVValue))
    const csvContent = [
      headers.map(escapeCSVValue).join(','),
      ...rows.map((row) => row.join(',')),
    ].join('\n')
    const blob = new Blob(['﻿' + csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `affiliate_invitees_${detail.value.aff_code}.csv`
    link.click()
    window.URL.revokeObjectURL(url)
    appStore.showSuccess(t('affiliate.invitees.exportSuccess'))
  } catch (error) {
    console.error('Affiliate CSV export failed:', error)
    appStore.showError(t('affiliate.invitees.exportFailed'))
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  void loadAffiliateDetail()
})
</script>

<style scoped>
.affiliate-page {
  --md-surface: #0b141c;
  --md-surface-container-low: #141c24;
  --md-surface-container: #182028;
  --md-surface-container-high: #222b33;
  --md-on-surface: #dae3ee;
  --md-on-surface-variant: #b9cbbc;
  --md-outline-variant: #3b4a3f;
  --md-primary: #00e38b;
  min-height: calc(100vh - 64px);
  margin: -24px -32px;
  padding: 42px 48px 80px;
  background-color: #0b141c;
  background-image:
    linear-gradient(rgb(59 74 63 / 13%) 1px, transparent 1px),
    linear-gradient(90deg, rgb(59 74 63 / 13%) 1px, transparent 1px);
  background-size: 32px 32px;
  color: #dae3ee;
}

.affiliate-loading {
  display: flex;
  min-height: 240px;
  align-items: center;
  justify-content: center;
}

.affiliate-spinner {
  height: 32px;
  width: 32px;
  border: 2px solid var(--md-outline-variant);
  border-top-color: var(--md-primary);
  border-radius: 999px;
  animation: affiliate-spin 0.8s linear infinite;
}

@keyframes affiliate-spin {
  to { transform: rotate(360deg); }
}

.affiliate-header {
  max-width: 1280px;
  margin: 0 auto 32px;
}

.affiliate-kicker {
  margin-bottom: 10px;
  color: #00e38b;
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.affiliate-title {
  color: #f4fff8;
  font-family: 'Geist', ui-sans-serif, system-ui, sans-serif;
  font-size: 2.5rem;
  font-weight: 760;
  line-height: 1.1;
  letter-spacing: -0.01em;
}

.affiliate-description {
  max-width: 640px;
  margin-top: 10px;
  color: #b9cbbc;
  font-size: 0.95rem;
  line-height: 1.6;
}

.affiliate-stat-grid {
  display: grid;
  max-width: 1280px;
  margin: 0 auto 24px;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 20px;
}

@media (min-width: 768px) {
  .affiliate-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .affiliate-stat-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

.affiliate-stat-card {
  position: relative;
  display: flex;
  min-height: 130px;
  flex-direction: column;
  gap: 8px;
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 20px;
  transition: border-color 180ms ease;
}

.affiliate-stat-card:hover {
  border-color: var(--md-primary);
}

.affiliate-stat-label {
  color: var(--md-on-surface-variant);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.66rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.affiliate-stat-value {
  color: #f4fff8;
  font-size: 2rem;
  font-weight: 700;
  line-height: 1;
}

.affiliate-stat-value small {
  margin-left: 2px;
  font-size: 1rem;
  font-weight: 600;
}

.affiliate-stat-meta {
  margin-top: auto;
  color: var(--md-on-surface-variant);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.72rem;
  line-height: 1.4;
}

.affiliate-stat-card-action {
  border-color: rgb(0 227 139 / 30%);
}

.affiliate-stat-card-action::after {
  position: absolute;
  top: -36px;
  right: -36px;
  width: 92px;
  height: 92px;
  border-radius: 50%;
  background: rgb(0 227 139 / 8%);
  content: '';
}

.affiliate-claim-button {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  gap: 6px;
  border: none;
  border-radius: 6px;
  background: var(--md-primary);
  padding: 6px 14px;
  color: #00230f;
  font-size: 0.8rem;
  font-weight: 700;
  cursor: pointer;
  transition: opacity 150ms ease;
}

.affiliate-claim-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.affiliate-claim-button:not(:disabled):hover {
  opacity: 0.9;
}

.affiliate-claim-hint {
  color: var(--md-on-surface-variant);
  font-size: 0.72rem;
}

.affiliate-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 24px;
  transition: border-color 180ms ease;
}

.affiliate-card:hover {
  border-color: var(--md-primary);
}

.affiliate-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #f4fff8;
  font-size: 1.1rem;
  font-weight: 700;
}

.affiliate-invite-card {
  max-width: 1280px;
  margin: 0 auto 24px;
}

.affiliate-code-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 16px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 6px;
  background: rgb(0 0 0 / 30%);
  padding: 10px 12px;
}

.affiliate-code-value {
  min-width: 0;
  flex: 1;
  overflow-x: auto;
  white-space: nowrap;
  color: #f4fff8;
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.85rem;
}

.affiliate-copy-button,
.affiliate-export-button {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 6px;
  background: none;
  padding: 6px 12px;
  color: var(--md-on-surface);
  font-size: 0.78rem;
  cursor: pointer;
  transition: border-color 150ms ease, color 150ms ease;
}

.affiliate-copy-button:hover,
.affiliate-export-button:not(:disabled):hover {
  border-color: var(--md-primary);
  color: var(--md-primary);
}

.affiliate-export-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.affiliate-tips {
  margin-top: 24px;
  border-top: 1px solid var(--md-outline-variant);
  padding-top: 20px;
}

.affiliate-tips-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  color: var(--md-on-surface);
  font-size: 0.9rem;
  font-weight: 700;
}

.affiliate-tips-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  list-style: none;
  counter-reset: affiliate-tip;
  color: var(--md-on-surface-variant);
  font-size: 0.85rem;
  line-height: 1.6;
}

.affiliate-tips-list li {
  position: relative;
  padding-left: 24px;
  counter-increment: affiliate-tip;
}

.affiliate-tips-list li::before {
  position: absolute;
  left: 0;
  color: var(--md-primary);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-weight: 700;
  content: counter(affiliate-tip) '.';
}

.affiliate-history-card {
  max-width: 1280px;
  margin: 0 auto;
}

.affiliate-history-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 20px;
}

.affiliate-empty {
  border: 1px dashed var(--md-outline-variant);
  border-radius: 8px;
  padding: 32px;
  text-align: center;
  color: var(--md-on-surface-variant);
  font-size: 0.9rem;
}

.affiliate-table-scroll {
  overflow-x: auto;
}

.affiliate-table {
  width: 100%;
  min-width: 560px;
  border-collapse: collapse;
  text-align: left;
}

.affiliate-table thead tr {
  border-bottom: 1px solid var(--md-outline-variant);
}

.affiliate-table th {
  padding: 0 16px 12px 0;
  color: var(--md-on-surface-variant);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.66rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.affiliate-table tbody tr {
  border-bottom: 1px solid rgb(59 74 63 / 50%);
  transition: background-color 150ms ease;
}

.affiliate-table tbody tr:last-child {
  border-bottom: none;
}

.affiliate-table tbody tr:hover {
  background: var(--md-surface-container-high);
}

.affiliate-table td {
  padding: 14px 16px 14px 0;
  color: var(--md-on-surface);
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.82rem;
}

.affiliate-table-align-right {
  text-align: right;
}

.affiliate-table-rebate {
  color: var(--md-primary);
  font-weight: 600;
}
</style>
