<template>
  <div class="site-info-page min-h-screen" :class="{ 'site-info-page-dark': isDark }">
    <header class="site-info-top-bar">
      <nav class="site-info-container flex h-full items-center justify-between gap-4">
        <router-link to="/home" class="site-info-brand" :aria-label="siteName">
          <span class="site-info-brand-logo">
            <img :src="siteLogo || '/logo.png'" alt="" class="h-full w-full object-contain" />
          </span>
          <span class="min-w-0 truncate text-sm font-semibold">{{ siteName }}</span>
        </router-link>

        <div class="flex items-center gap-1 sm:gap-2">
          <div class="site-info-locale">
            <LocaleSwitcher />
          </div>

          <button
            type="button"
            class="site-info-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-pressed="isDark"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="site-info-filled-button">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="site-info-container py-8 md:py-10">
      <section class="flex flex-col gap-5 md:flex-row md:items-end md:justify-between">
        <div>
          <div class="site-info-assist-chip">
            <Icon name="chart" size="sm" />
            <span>{{ t('siteInfo.kicker') }}</span>
          </div>
          <h1 class="mt-5 text-3xl font-bold tracking-normal text-[var(--site-info-on-surface)] md:text-5xl">
            {{ t('siteInfo.title') }}
          </h1>
          <p class="mt-3 max-w-2xl text-sm leading-6 text-[var(--site-info-on-surface-variant)] md:text-base">
            {{ t('siteInfo.description') }}
          </p>
        </div>

        <div class="flex flex-col items-start gap-2 md:items-end">
          <button
            type="button"
            class="site-info-tonal-button"
            :disabled="loading"
            @click="loadSiteInfo"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            <span>{{ t('common.refresh') }}</span>
          </button>
          <div class="flex flex-wrap items-center gap-2 text-xs text-[var(--site-info-on-surface-variant)]">
            <Icon name="clock" size="sm" />
            <span>{{ generatedAtLabel }}</span>
          </div>
        </div>
      </section>

      <div
        v-if="loadError"
        class="mt-6 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
      >
        {{ loadError }}
      </div>

      <section class="mt-8 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div v-for="metric in overviewMetrics" :key="metric.key" class="site-info-metric">
          <div class="flex items-center justify-between gap-3">
            <span class="text-xs font-semibold uppercase tracking-widest text-[var(--site-info-on-surface-variant)]">
              {{ metric.label }}
            </span>
            <Icon :name="metric.icon" size="sm" class="text-[var(--site-info-primary)]" />
          </div>
          <div class="mt-3 text-2xl font-bold tabular-nums text-[var(--site-info-on-surface)]">
            {{ metric.value }}
          </div>
          <div class="mt-1 text-xs text-[var(--site-info-on-surface-variant)]">
            {{ metric.caption }}
          </div>
        </div>
      </section>

      <section class="mt-8">
        <div class="site-info-panel">
          <div class="site-info-panel-heading">
            <div>
              <h2>{{ t('siteInfo.groups.title') }}</h2>
              <p>{{ t('siteInfo.groups.description') }}</p>
            </div>
            <span class="site-info-count-chip">{{ groups.length }}</span>
          </div>

          <div v-if="loading && groups.length === 0" class="site-info-loading">
            {{ t('siteInfo.loading') }}
          </div>
          <div v-else-if="groups.length === 0" class="site-info-empty">
            {{ t('siteInfo.groups.empty') }}
          </div>
          <div v-else class="mt-4 overflow-x-auto">
            <table class="site-info-table">
              <thead>
                <tr>
                  <th>{{ t('siteInfo.groups.columns.name') }}</th>
                  <th>{{ t('siteInfo.groups.columns.platform') }}</th>
                  <th>{{ t('siteInfo.groups.columns.rate') }}</th>
                  <th>{{ t('siteInfo.groups.columns.imageRate') }}</th>
                  <th>{{ t('siteInfo.groups.columns.officialSavings') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="group in groups" :key="group.id">
                  <td>
                    <div class="font-semibold text-[var(--site-info-on-surface)]">{{ group.name }}</div>
                  </td>
                  <td>
                    <span class="site-info-subtle-chip">{{ group.platform || '-' }}</span>
                  </td>
                  <td class="font-mono font-semibold tabular-nums">{{ formatMultiplier(group.rate_multiplier) }}</td>
                  <td class="font-mono tabular-nums">
                    <span v-if="group.allow_image_generation">
                      {{ formatMultiplier(group.image_rate_multiplier) }}
                    </span>
                    <span v-else class="text-[var(--site-info-on-surface-variant)]">-</span>
                  </td>
                  <td class="font-mono font-semibold tabular-nums text-emerald-600 dark:text-emerald-300">
                    {{ formatOfficialSavings(group.rate_multiplier) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section class="mt-8">
        <div class="site-info-panel-heading mb-4">
          <div>
            <h2>{{ t('siteInfo.availability.title') }}</h2>
            <p>{{ t('siteInfo.availability.description') }}</p>
          </div>
          <span class="site-info-count-chip">{{ monitors.length }}</span>
        </div>

        <div v-if="loading && monitors.length === 0" class="site-info-loading">
          {{ t('siteInfo.loading') }}
        </div>
        <div v-else-if="monitors.length === 0" class="site-info-empty">
          {{ t('siteInfo.availability.empty') }}
        </div>
        <div v-else class="grid gap-4 lg:grid-cols-2">
          <article
            v-for="monitor in monitors"
            :key="`${monitor.provider}-${monitor.name}-${monitor.group_name}`"
            class="site-info-monitor-card"
          >
            <div class="flex items-start gap-3">
              <span
                class="grid h-10 w-10 flex-shrink-0 place-items-center rounded-lg ring-1 ring-black/5 dark:ring-white/10"
                :class="[providerGradient(monitor.provider), providerTintClass(monitor.provider)]"
              >
                <ProviderIcon :provider="monitor.provider" :size="22" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="truncate text-base font-semibold text-[var(--site-info-on-surface)]">
                    {{ monitor.name }}
                  </h3>
                  <span class="site-info-subtle-chip">{{ providerLabel(monitor.provider) }}</span>
                  <span v-if="monitor.group_name" class="site-info-subtle-chip">{{ monitor.group_name }}</span>
                </div>
                <p class="mt-1 text-xs text-[var(--site-info-on-surface-variant)]">
                  {{ t('siteInfo.availability.modelCount', { n: monitor.models.length }) }}
                </p>
              </div>
            </div>

            <div class="mt-4 overflow-x-auto">
              <table class="site-info-table">
                <thead>
                  <tr>
                    <th>{{ t('siteInfo.availability.columns.model') }}</th>
                    <th>{{ t('siteInfo.availability.columns.status') }}</th>
                    <th>{{ t('siteInfo.availability.columns.availability7d') }}</th>
                    <th>{{ t('siteInfo.availability.columns.availability15d') }}</th>
                    <th>{{ t('siteInfo.availability.columns.availability30d') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="model in monitor.models" :key="model.model">
                    <td class="font-mono text-xs font-semibold">{{ model.model }}</td>
                    <td>
                      <span class="rounded-full px-2 py-1 text-xs font-semibold" :class="statusBadgeClass(model.latest_status)">
                        {{ statusLabel(model.latest_status) }}
                      </span>
                    </td>
                    <td class="font-mono tabular-nums">{{ formatPercent(model.availability_7d) }}</td>
                    <td class="font-mono tabular-nums">{{ formatPercent(model.availability_15d) }}</td>
                    <td class="font-mono tabular-nums">{{ formatPercent(model.availability_30d) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <MonitorTimeline
              :buckets="monitor.timeline"
              :countdown-seconds="0"
              :length="60"
              :show-countdown="false"
            />
          </article>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import MonitorTimeline from '@/components/user/monitor/MonitorTimeline.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getPublicSiteInfo,
  type PublicSiteInfo,
  type PublicSiteModelAvailability,
  type PublicSiteMonitorAvailability,
  type PublicSiteMonitorTimelinePoint,
} from '@/api/publicSiteInfo'
import {
  providerGradient,
  useChannelMonitorFormat,
} from '@/composables/useChannelMonitorFormat'

type IconName = InstanceType<typeof Icon>['$props']['name']

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const {
  statusLabel,
  statusBadgeClass,
  providerLabel,
  formatPercent,
  formatRelativeTime,
} = useChannelMonitorFormat()

const data = ref<PublicSiteInfo | null>(null)
const loading = ref(false)
const loadError = ref('')
const isDark = ref(document.documentElement.classList.contains('dark'))
const preferredColorScheme = window.matchMedia('(prefers-color-scheme: dark)')
let abortController: AbortController | null = null

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const groups = computed(() => data.value?.groups ?? [])
const monitors = computed(() => mergeMonitorAvailability(data.value?.model_availability ?? []))
const allModels = computed(() => monitors.value.flatMap((monitor) => monitor.models))

const operationalModelCount = computed(() =>
  allModels.value.filter((model) => model.latest_status === 'operational').length
)

const averageAvailability7d = computed(() => {
  const values = allModels.value.map((model) => model.availability_7d).filter((value) => Number.isFinite(value))
  if (values.length === 0) return null
  return values.reduce((sum, value) => sum + value, 0) / values.length
})

const generatedAtLabel = computed(() => {
  if (!data.value?.generated_at) return t('siteInfo.generatedAtEmpty')
  return t('monitorCommon.updatedAt', { time: formatRelativeTime(data.value.generated_at) })
})

const overviewMetrics = computed(() => [
  {
    key: 'groups',
    label: t('siteInfo.metrics.groups'),
    value: groups.value.length,
    caption: t('siteInfo.metrics.publicGroups'),
    icon: 'grid' as IconName,
  },
  {
    key: 'monitors',
    label: t('siteInfo.metrics.monitors'),
    value: monitors.value.length,
    caption: t('siteInfo.metrics.monitorGroups'),
    icon: 'server' as IconName,
  },
  {
    key: 'models',
    label: t('siteInfo.metrics.models'),
    value: `${operationalModelCount.value}/${allModels.value.length}`,
    caption: t('siteInfo.metrics.operationalModels'),
    icon: 'checkCircle' as IconName,
  },
  {
    key: 'availability',
    label: t('siteInfo.metrics.availability'),
    value: averageAvailability7d.value == null ? '-' : formatPercent(averageAvailability7d.value),
    caption: t('siteInfo.metrics.availabilityCaption'),
    icon: 'chart' as IconName,
  },
])

function providerTintClass(provider: string): string {
  switch (provider) {
    case 'openai':
      return 'text-emerald-600 dark:text-emerald-300'
    case 'anthropic':
      return 'text-orange-600 dark:text-orange-300'
    case 'gemini':
      return 'text-sky-600 dark:text-sky-300'
    default:
      return 'text-gray-500 dark:text-gray-300'
  }
}

function formatMultiplier(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${Number(value).toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}x`
}

function formatOfficialSavings(value: number): string {
  if (!Number.isFinite(value)) return '-'
  const savingsPct = (1 - value / 7) * 100
  const percent = `${savingsPct.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`
  return t('siteInfo.groups.savingsValue', { percent })
}

function mergeMonitorAvailability(monitors: PublicSiteMonitorAvailability[]): PublicSiteMonitorAvailability[] {
  const grouped = new Map<string, {
    id: number
    name: string
    provider: PublicSiteMonitorAvailability['provider']
    groupNames: Set<string>
    models: Map<string, PublicSiteModelAvailability>
    timeline: PublicSiteMonitorTimelinePoint[]
  }>()

  for (const monitor of monitors) {
    const key = `${String(monitor.provider).toLowerCase()}::${monitor.name.trim().toLowerCase()}`
    let item = grouped.get(key)
    if (!item) {
      item = {
        id: monitor.id,
        name: monitor.name,
        provider: monitor.provider,
        groupNames: new Set<string>(),
        models: new Map<string, PublicSiteModelAvailability>(),
        timeline: [],
      }
      grouped.set(key, item)
    }

    if (monitor.group_name) {
      item.groupNames.add(monitor.group_name)
    }

    for (const model of monitor.models) {
      const existing = item.models.get(model.model)
      item.models.set(model.model, existing ? mergeModelAvailability(existing, model) : { ...model })
    }

    item.timeline.push(...(monitor.timeline ?? []))
  }

  return Array.from(grouped.values())
    .map((item) => ({
      id: item.id,
      name: item.name,
      provider: item.provider,
      group_name: Array.from(item.groupNames).sort((a, b) => a.localeCompare(b)).join(' / '),
      models: Array.from(item.models.values()).sort((a, b) => a.model.localeCompare(b.model)),
      timeline: mergeTimeline(item.timeline),
    }))
    .sort((a, b) => `${a.provider}-${a.name}`.localeCompare(`${b.provider}-${b.name}`))
}

function mergeModelAvailability(
  a: PublicSiteModelAvailability,
  b: PublicSiteModelAvailability,
): PublicSiteModelAvailability {
  return {
    model: a.model,
    latest_status: statusRank(b.latest_status) > statusRank(a.latest_status) ? b.latest_status : a.latest_status,
    availability_7d: Math.min(a.availability_7d, b.availability_7d),
    availability_15d: Math.min(a.availability_15d, b.availability_15d),
    availability_30d: Math.min(a.availability_30d, b.availability_30d),
  }
}

function statusRank(status: string): number {
  switch (status) {
    case 'operational':
      return 0
    case 'degraded':
      return 1
    case 'failed':
      return 2
    case 'error':
      return 3
    default:
      return 4
  }
}

function mergeTimeline(points: PublicSiteMonitorTimelinePoint[]): PublicSiteMonitorTimelinePoint[] {
  const seen = new Set<string>()
  const merged: PublicSiteMonitorTimelinePoint[] = []
  for (const point of points) {
    const key = `${point.checked_at}:${point.status}:${point.latency_ms ?? ''}:${point.ping_latency_ms ?? ''}`
    if (seen.has(key)) continue
    seen.add(key)
    merged.push(point)
  }
  return merged
    .sort((a, b) => parseCheckedAt(b.checked_at) - parseCheckedAt(a.checked_at))
    .slice(0, 60)
}

function parseCheckedAt(value: string): number {
  const ts = Date.parse(value)
  return Number.isNaN(ts) ? 0 : ts
}

function resolveThemePreference(): boolean {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark') return true
  if (savedTheme === 'light') return false
  return preferredColorScheme.matches
}

function applyTheme(dark: boolean, persist = true) {
  isDark.value = dark
  document.documentElement.classList.toggle('dark', dark)
  if (persist) {
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  }
}

function syncThemePreference() {
  applyTheme(resolveThemePreference(), false)
}

function toggleTheme() {
  applyTheme(!isDark.value)
}

function handleThemeStorage(event: StorageEvent) {
  if (event.key === 'theme') syncThemePreference()
}

function handleSystemThemeChange() {
  if (!localStorage.getItem('theme')) syncThemePreference()
}

async function loadSiteInfo() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true
  loadError.value = ''
  try {
    data.value = await getPublicSiteInfo({ signal: ctrl.signal })
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    loadError.value = extractApiErrorMessage(err, t('siteInfo.loadError'))
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

onMounted(() => {
  syncThemePreference()
  window.addEventListener('storage', handleThemeStorage)
  preferredColorScheme.addEventListener('change', handleSystemThemeChange)
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  void loadSiteInfo()
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
  window.removeEventListener('storage', handleThemeStorage)
  preferredColorScheme.removeEventListener('change', handleSystemThemeChange)
})
</script>

<style scoped>
.site-info-page {
  --site-info-primary: #202124;
  --site-info-on-primary: #ffffff;
  --site-info-primary-container: #e8eaed;
  --site-info-on-primary-container: #202124;
  --site-info-secondary-container: #f1f3f4;
  --site-info-on-secondary-container: #3c4043;
  --site-info-surface: #ffffff;
  --site-info-surface-low: #f8fafd;
  --site-info-surface-container: #f1f3f4;
  --site-info-surface-high: #e8eaed;
  --site-info-on-surface: #202124;
  --site-info-on-surface-variant: #5f6368;
  --site-info-outline-variant: #dadce0;
  --site-info-shadow: 0 1px 2px rgb(0 0 0 / 0.14), 0 1px 3px 1px rgb(0 0 0 / 0.08);
  background: var(--site-info-surface);
  color: var(--site-info-on-surface);
}

.site-info-page-dark {
  --site-info-primary: #f1f3f4;
  --site-info-on-primary: #202124;
  --site-info-primary-container: #3f3f3f;
  --site-info-on-primary-container: #f1f3f4;
  --site-info-secondary-container: #383838;
  --site-info-on-secondary-container: #f1f3f4;
  --site-info-surface: #1f1f1f;
  --site-info-surface-low: #2a2a2a;
  --site-info-surface-container: #303030;
  --site-info-surface-high: #3f3f3f;
  --site-info-on-surface: #f1f3f4;
  --site-info-on-surface-variant: #c7c7c7;
  --site-info-outline-variant: #4d4d4d;
  --site-info-shadow: none;
}

.site-info-container {
  width: min(1180px, calc(100% - 32px));
  margin-inline: auto;
}

.site-info-top-bar {
  position: sticky;
  top: 0;
  z-index: 30;
  height: 72px;
  border-bottom: 1px solid var(--site-info-outline-variant);
  background: color-mix(in srgb, var(--site-info-surface) 92%, transparent);
  backdrop-filter: blur(16px);
}

.site-info-brand {
  display: inline-flex;
  min-width: 0;
  max-width: 48vw;
  align-items: center;
  gap: 12px;
  color: var(--site-info-on-surface);
}

.site-info-brand-logo {
  display: grid;
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  place-items: center;
  overflow: hidden;
  border-radius: 8px;
  background: var(--site-info-surface-high);
}

.site-info-icon-button {
  display: inline-grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 20px;
  color: var(--site-info-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease;
}

.site-info-icon-button:hover {
  background: color-mix(in srgb, var(--site-info-on-surface) 8%, transparent);
  color: var(--site-info-on-surface);
}

.site-info-locale :deep(button) {
  height: 40px;
  border-radius: 20px;
  color: var(--site-info-on-surface-variant);
}

.site-info-locale :deep(button:hover) {
  background: color-mix(in srgb, var(--site-info-on-surface) 8%, transparent);
}

.site-info-filled-button,
.site-info-tonal-button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 20px;
  padding: 0 20px;
  font-size: 0.875rem;
  font-weight: 600;
  transition: background-color 160ms ease, box-shadow 160ms ease;
}

.site-info-filled-button {
  background: var(--site-info-primary);
  color: var(--site-info-on-primary);
  box-shadow: var(--site-info-shadow);
}

.site-info-tonal-button {
  background: var(--site-info-secondary-container);
  color: var(--site-info-on-secondary-container);
}

.site-info-filled-button:hover {
  background: color-mix(in srgb, var(--site-info-primary) 92%, var(--site-info-on-primary));
}

.site-info-tonal-button:hover {
  background: color-mix(in srgb, var(--site-info-secondary-container) 88%, var(--site-info-on-secondary-container));
}

.site-info-tonal-button:disabled {
  cursor: wait;
  opacity: 0.7;
}

.site-info-assist-chip,
.site-info-count-chip,
.site-info-subtle-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--site-info-outline-variant);
  border-radius: 8px;
  background: var(--site-info-surface-low);
  color: var(--site-info-on-surface-variant);
  font-size: 0.8125rem;
  font-weight: 600;
}

.site-info-assist-chip {
  min-height: 32px;
  padding: 0 12px;
}

.site-info-count-chip,
.site-info-subtle-chip {
  min-height: 28px;
  padding: 0 10px;
}

.site-info-metric,
.site-info-panel,
.site-info-monitor-card {
  border: 1px solid var(--site-info-outline-variant);
  border-radius: 8px;
  background: var(--site-info-surface-low);
  box-shadow: var(--site-info-shadow);
}

.site-info-metric,
.site-info-panel,
.site-info-monitor-card {
  padding: 20px;
}

.site-info-panel-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.site-info-panel-heading h2 {
  color: var(--site-info-on-surface);
  font-size: 1.125rem;
  font-weight: 700;
}

.site-info-panel-heading p {
  margin-top: 4px;
  color: var(--site-info-on-surface-variant);
  font-size: 0.875rem;
}

.site-info-loading,
.site-info-empty {
  margin-top: 16px;
  border: 1px dashed var(--site-info-outline-variant);
  border-radius: 8px;
  padding: 28px;
  text-align: center;
  color: var(--site-info-on-surface-variant);
  font-size: 0.875rem;
}

.site-info-table {
  width: 100%;
  min-width: max-content;
  border-collapse: collapse;
}

.site-info-table th,
.site-info-table td {
  border-bottom: 1px solid var(--site-info-outline-variant);
  padding: 12px 10px;
  text-align: left;
  font-size: 0.8125rem;
}

.site-info-table th {
  color: var(--site-info-on-surface-variant);
  font-weight: 700;
}

.site-info-table td {
  color: var(--site-info-on-surface);
}

.site-info-table tr:last-child td {
  border-bottom: 0;
}

@media (max-width: 640px) {
  .site-info-container {
    width: min(100% - 24px, 1180px);
  }

  .site-info-filled-button {
    padding-inline: 14px;
  }

  .site-info-panel-heading {
    flex-direction: column;
  }
}
</style>
