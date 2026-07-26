<template>
  <section class="telemetry-keys-panel">
    <header class="telemetry-panel-header">
      <div>
        <span class="telemetry-panel-kicker">ACCESS CONTROL</span>
        <h2>{{ t('dashboard.activeKeys') }}</h2>
      </div>
      <router-link
        to="/keys"
        class="telemetry-icon-link"
        :title="t('dashboard.manageKeys')"
        :aria-label="t('dashboard.manageKeys')"
      >
        <Icon name="plus" size="sm" :stroke-width="2" />
      </router-link>
    </header>

    <div v-if="loading" class="telemetry-keys-loading">
      <LoadingSpinner size="md" />
    </div>

    <div v-else-if="keys.length" class="telemetry-key-list">
      <article v-for="apiKey in keys" :key="apiKey.id" class="telemetry-key-item">
        <div class="telemetry-key-heading">
          <div class="telemetry-key-name">
            <span class="telemetry-key-status" :class="`telemetry-key-status--${apiKey.status}`"></span>
            <strong>{{ apiKey.name }}</strong>
          </div>
          <span>{{ formatExpiration(apiKey.expires_at) }}</span>
        </div>

        <code>{{ maskApiKey(apiKey.key) }}</code>

        <div class="telemetry-key-usage">
          <div class="telemetry-key-track" aria-hidden="true">
            <span :style="{ width: `${quotaPercent(apiKey)}%` }"></span>
          </div>
          <span>{{ quotaLabel(apiKey) }}</span>
        </div>
      </article>
    </div>

    <div v-else class="telemetry-keys-empty">
      <Icon name="key" size="lg" />
      <p>{{ t('dashboard.noActiveKeys') }}</p>
      <router-link to="/keys">{{ t('dashboard.manageKeys') }}</router-link>
    </div>

    <router-link v-if="keys.length" to="/keys" class="telemetry-keys-footer-link">
      <span>{{ t('dashboard.manageKeys') }}</span>
      <Icon name="arrowRight" size="sm" />
    </router-link>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { maskApiKey } from '@/utils/maskApiKey'
import type { ApiKey } from '@/types'

defineProps<{
  keys: ApiKey[]
  loading: boolean
}>()

const { t, locale } = useI18n()

function quotaPercent(apiKey: ApiKey): number {
  if (apiKey.quota <= 0) return 0
  return Math.min(100, Math.max(0, (apiKey.quota_used / apiKey.quota) * 100))
}

function quotaLabel(apiKey: ApiKey): string {
  if (apiKey.quota <= 0) return t('dashboard.platformQuota.noLimit')
  return `${quotaPercent(apiKey).toFixed(0)}% ${t('keys.quotaUsed')}`
}

function formatExpiration(value: string | null): string {
  if (!value) return t('keys.noExpiration')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('keys.noExpiration')
  return new Intl.DateTimeFormat(locale.value, {
    month: 'short',
    day: 'numeric'
  }).format(date)
}
</script>

<style scoped>
.telemetry-keys-panel {
  display: flex;
  min-width: 0;
  min-height: 398px;
  flex-direction: column;
  border: 1px solid var(--md-outline-variant);
  border-radius: 6px;
  background: var(--md-surface-container-low);
  padding: 22px;
  transition: border-color 180ms ease;
}

.telemetry-keys-panel:hover {
  border-color: var(--md-outline);
}

.telemetry-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.telemetry-panel-kicker {
  display: block;
  margin-bottom: 7px;
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.64rem;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.telemetry-panel-header h2 {
  color: var(--md-on-surface);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.9rem;
  font-weight: 700;
}

.telemetry-icon-link {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  place-items: center;
  border: 1px solid var(--md-outline-variant);
  border-radius: 4px;
  color: #00e38b;
  transition: border-color 160ms ease, background-color 160ms ease;
}

.telemetry-icon-link:hover {
  border-color: #00e38b;
  background: rgb(0 227 139 / 7%);
}

.telemetry-keys-loading,
.telemetry-keys-empty {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}

.telemetry-keys-empty {
  flex-direction: column;
  gap: 10px;
  color: var(--md-on-surface-variant);
  text-align: center;
}

.telemetry-keys-empty p {
  font-size: 0.82rem;
}

.telemetry-keys-empty a {
  color: #00e38b;
  font-size: 0.78rem;
  font-weight: 700;
}

.telemetry-key-list {
  display: grid;
  gap: 12px;
}

.telemetry-key-item {
  min-width: 0;
  border: 1px solid var(--md-outline-variant);
  border-radius: 4px;
  background: var(--md-surface-container-low);
  padding: 13px;
  transition: border-color 160ms ease;
}

.telemetry-key-item:hover {
  border-color: #00e38b;
}

.telemetry-key-heading,
.telemetry-key-name,
.telemetry-key-usage {
  display: flex;
  align-items: center;
}

.telemetry-key-heading {
  justify-content: space-between;
  gap: 12px;
}

.telemetry-key-name {
  min-width: 0;
  gap: 8px;
}

.telemetry-key-name strong {
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.78rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.telemetry-key-heading > span {
  flex: 0 0 auto;
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.62rem;
}

.telemetry-key-status {
  width: 7px;
  height: 7px;
  flex: 0 0 7px;
  border-radius: 50%;
  background: var(--md-on-surface-variant);
}

.telemetry-key-status--active {
  background: #00e38b;
  box-shadow: 0 0 8px rgb(0 227 139 / 45%);
}

.telemetry-key-status--quota_exhausted,
.telemetry-key-status--expired {
  background: #ff8d84;
}

.telemetry-key-item code {
  display: block;
  overflow: hidden;
  margin-top: 10px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 3px;
  background: var(--md-app-bg);
  padding: 7px 9px;
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.68rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.telemetry-key-usage {
  gap: 10px;
  margin-top: 10px;
}

.telemetry-key-track {
  height: 3px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
  background: var(--md-app-bg);
}

.telemetry-key-track span {
  display: block;
  height: 100%;
  background: #00e38b;
}

.telemetry-key-usage > span {
  flex: 0 0 auto;
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.6rem;
}

.telemetry-keys-footer-link {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: auto;
  padding-top: 18px;
  color: var(--md-on-surface-variant);
  font-size: 0.74rem;
  font-weight: 700;
}

.telemetry-keys-footer-link:hover {
  color: #00e38b;
}
</style>
