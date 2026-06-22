<template>
  <section class="md3-actions-panel">
    <header class="md3-actions-header">
      <h2>{{ t('dashboard.quickActions') }}</h2>
    </header>

    <div class="md3-actions-list">
      <button
        v-for="action in actions"
        :key="action.to"
        type="button"
        class="md3-action-row"
        @click="router.push(action.to)"
      >
        <span class="md3-action-icon" :class="action.iconClass">
          <Icon :name="action.icon" size="lg" />
        </span>
        <span class="md3-action-copy">
          <strong>{{ action.title }}</strong>
          <span>{{ action.description }}</span>
        </span>
        <Icon name="chevronRight" size="md" class="md3-action-chevron" />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const router = useRouter()
const { t } = useI18n()

const actions = computed(() => [
  {
    to: '/keys',
    icon: 'key' as const,
    iconClass: 'md3-action-icon-key',
    title: t('dashboard.createApiKey'),
    description: t('dashboard.generateNewKey')
  },
  {
    to: '/usage',
    icon: 'chart' as const,
    iconClass: 'md3-action-icon-usage',
    title: t('dashboard.viewUsage'),
    description: t('dashboard.checkDetailedLogs')
  },
  {
    to: '/redeem',
    icon: 'gift' as const,
    iconClass: 'md3-action-icon-redeem',
    title: t('dashboard.redeemCode'),
    description: t('dashboard.addBalanceWithCode')
  }
])
</script>

<style scoped>
.md3-actions-panel {
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.dark .md3-actions-panel {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-actions-header {
  padding: 16px 18px;
  border-bottom: 1px solid var(--md-outline-variant);
}

.dark .md3-actions-header {
  border-color: var(--md-outline-variant);
}

.md3-actions-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  font-weight: 760;
}

.dark .md3-actions-header h2 {
  color: var(--md-on-surface);
}

.md3-actions-list {
  display: grid;
  gap: 8px;
  padding: 12px;
}

.md3-action-row {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  gap: 12px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 12px;
  text-align: left;
  transition: background-color 160ms ease, border-color 160ms ease;
}

.md3-action-row:hover {
  border-color: var(--md-outline-variant);
  background: var(--md-state-hover);
}

.dark .md3-action-row {
  background: var(--md-surface-container-low);
}

.dark .md3-action-row:hover {
  border-color: var(--md-outline-variant);
  background: var(--md-state-hover);
}

.md3-action-icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}

.md3-action-icon-key {
  background: rgb(204 251 241);
  color: rgb(15 118 110);
}

.md3-action-icon-usage {
  background: rgb(209 250 229);
  color: rgb(4 120 87);
}

.md3-action-icon-redeem {
  background: rgb(254 243 199);
  color: rgb(180 83 9);
}

.dark .md3-action-icon-key {
  background: rgb(20 184 166 / 0.18);
  color: rgb(94 234 212);
}

.dark .md3-action-icon-usage {
  background: rgb(6 95 70 / 0.26);
  color: rgb(110 231 183);
}

.dark .md3-action-icon-redeem {
  background: rgb(217 119 6 / 0.22);
  color: rgb(252 211 77);
}

.md3-action-copy {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
}

.md3-action-copy strong {
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-action-copy span {
  overflow: hidden;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-action-chevron {
  flex: 0 0 auto;
  color: color-mix(in srgb, var(--md-on-surface-variant) 70%, transparent);
  transition: color 160ms ease;
}

.md3-action-row:hover .md3-action-chevron {
  color: var(--md-primary);
}

.dark .md3-action-copy strong {
  color: var(--md-on-surface);
}

.dark .md3-action-copy span {
  color: var(--md-on-surface-variant);
}

.dark .md3-action-row:hover .md3-action-chevron {
  color: var(--md-primary);
}
</style>
