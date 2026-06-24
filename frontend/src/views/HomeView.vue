<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="md3-home min-h-screen" :class="{ 'md3-home-dark': isDark }">
    <header class="md3-top-bar">
      <nav class="md3-container flex h-full items-center justify-between gap-4">
        <router-link to="/home" class="md3-brand" :aria-label="siteName">
          <span class="md3-brand-logo">
            <img :src="siteLogo || '/logo.png'" alt="" class="h-full w-full object-contain" />
          </span>
          <span class="min-w-0 truncate text-sm font-semibold">{{ siteName }}</span>
        </router-link>

        <div class="flex items-center gap-1 sm:gap-2">
          <div class="md3-locale">
            <LocaleSwitcher />
          </div>

          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="md3-icon-button"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <button
            type="button"
            class="md3-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-pressed="isDark"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link v-if="isAuthenticated" :to="dashboardPath" class="md3-tonal-button">
            <span class="md3-avatar">{{ userInitial }}</span>
            <span>{{ t('home.dashboard') }}</span>
          </router-link>
          <router-link v-else to="/login" class="md3-filled-button">
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section class="md3-container md3-hero">
        <div class="md3-hero-copy">
          <div class="md3-assist-chip">
            <Icon name="sparkles" size="sm" />
            <span>{{ t('home.heroSubtitle') }}</span>
          </div>

          <h1>{{ siteName }}</h1>
          <p class="md3-hero-subtitle">{{ siteSubtitle }}</p>
          <p class="md3-hero-description">{{ t('home.heroDescription') }}</p>

          <div class="flex flex-col gap-3 sm:flex-row">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="md3-hero-action">
              <span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}</span>
              <Icon name="arrowRight" size="md" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="md3-hero-secondary"
            >
              <Icon name="book" size="md" />
              <span>{{ t('home.docs') }}</span>
            </a>
          </div>
        </div>

        <div class="md3-gateway-panel" aria-label="Gateway preview">
          <div class="md3-panel-header">
            <div>
              <p class="md3-panel-label">Gateway</p>
              <h2>/v1/messages</h2>
            </div>
            <span class="md3-status-chip">
              <span class="md3-status-dot"></span>
              200 OK
            </span>
          </div>

          <div class="md3-request-block">
            <div class="flex items-center gap-2">
              <span class="md3-method">POST</span>
              <span class="truncate text-sm">claude-opus / gpt-5.5 / gemini</span>
            </div>
            <div class="mt-4 grid gap-3 sm:grid-cols-3">
              <div v-for="metric in heroMetrics" :key="metric.label" class="md3-metric">
                <span>{{ metric.label }}</span>
                <strong>{{ metric.value }}</strong>
              </div>
            </div>
          </div>

          <div class="space-y-2">
            <div v-for="route in routeRows" :key="route.name" class="md3-route-row">
              <span class="md3-route-icon">
                <Icon :name="route.icon" size="sm" />
              </span>
              <div class="min-w-0 flex-1">
                <p>{{ route.name }}</p>
                <span class="md3-route-detail">{{ route.detail }}</span>
              </div>
              <strong>{{ route.value }}</strong>
            </div>
          </div>
        </div>
      </section>

      <section class="md3-container">
        <div class="md3-chip-row">
          <span v-for="tag in featureTags" :key="tag.label" class="md3-filter-chip">
            <Icon :name="tag.icon" size="sm" />
            {{ tag.label }}
          </span>
        </div>
      </section>

      <section class="md3-container md3-section">
        <div class="md3-section-heading">
          <p>{{ t('home.solutions.subtitle') }}</p>
          <h2>{{ t('home.solutions.title') }}</h2>
        </div>

        <div class="grid gap-4 md:grid-cols-3">
          <article v-for="feature in featureCards" :key="feature.title" class="md3-feature-card">
            <span class="md3-feature-icon">
              <Icon :name="feature.icon" size="lg" />
            </span>
            <h3>{{ feature.title }}</h3>
            <p>{{ feature.description }}</p>
          </article>
        </div>
      </section>

      <section class="md3-container md3-section">
        <div class="md3-comparison">
          <div class="md3-comparison-heading">
            <p>{{ t('home.comparison.title') }}</p>
            <h2>{{ t('home.features.balanceQuota') }} · {{ t('home.features.multiAccount') }}</h2>
          </div>

          <div class="md3-comparison-list">
            <div v-for="item in comparisonItems" :key="item.feature" class="md3-comparison-row">
              <span>{{ item.feature }}</span>
              <p>{{ item.official }}</p>
              <strong>{{ item.us }}</strong>
            </div>
          </div>
        </div>
      </section>

      <section class="md3-container md3-section">
        <div class="md3-section-heading">
          <p>{{ t('home.providers.description') }}</p>
          <h2>{{ t('home.providers.title') }}</h2>
        </div>

        <div class="md3-provider-grid">
          <div
            v-for="provider in providers"
            :key="provider.name"
            class="md3-provider"
            :class="{ 'md3-provider-muted': provider.soon }"
          >
            <span class="md3-provider-mark">{{ provider.mark }}</span>
            <span class="min-w-0 flex-1 truncate">{{ provider.name }}</span>
            <small>{{ provider.soon ? t('home.providers.soon') : t('home.providers.supported') }}</small>
          </div>
        </div>
      </section>

      <section class="md3-container md3-cta-section">
        <div class="md3-cta">
          <div>
            <p>{{ t('home.cta.description') }}</p>
            <h2>{{ t('home.cta.title') }}</h2>
          </div>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="md3-cta-button">
            <span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.cta.button') }}</span>
            <Icon name="arrowRight" size="md" />
          </router-link>
        </div>
      </section>
    </main>

    <footer class="md3-footer">
      <div class="md3-container flex flex-col gap-3 py-8 sm:flex-row sm:items-center sm:justify-between">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div class="flex items-center gap-5">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
            {{ t('home.docs') }}
          </a>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

type IconName = 'chart' | 'creditCard' | 'database' | 'server' | 'shield' | 'swap' | 'sync' | 'users'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const preferredColorScheme = window.matchMedia('(prefers-color-scheme: dark)')
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const currentYear = computed(() => new Date().getFullYear())

const heroMetrics = computed(() => [
  { label: t('home.tags.subscriptionToApi'), value: '1 key' },
  { label: t('home.tags.stickySession'), value: 'stable' },
  { label: t('home.tags.realtimeBilling'), value: 'live' }
])

const routeRows = computed<Array<{ name: string; detail: string; value: string; icon: IconName }>>(() => [
  {
    name: t('home.features.unifiedGateway'),
    detail: t('home.features.unifiedGatewayDesc'),
    value: 'API',
    icon: 'server'
  },
  {
    name: t('home.features.multiAccount'),
    detail: t('home.features.multiAccountDesc'),
    value: 'pool',
    icon: 'sync'
  },
  {
    name: t('home.features.balanceQuota'),
    detail: t('home.features.balanceQuotaDesc'),
    value: 'quota',
    icon: 'creditCard'
  }
])

const featureTags = computed<Array<{ label: string; icon: IconName }>>(() => [
  { label: t('home.tags.subscriptionToApi'), icon: 'swap' },
  { label: t('home.tags.stickySession'), icon: 'shield' },
  { label: t('home.tags.realtimeBilling'), icon: 'chart' }
])

const featureCards = computed<Array<{ title: string; description: string; icon: IconName }>>(() => [
  {
    title: t('home.features.unifiedGateway'),
    description: t('home.features.unifiedGatewayDesc'),
    icon: 'server'
  },
  {
    title: t('home.features.multiAccount'),
    description: t('home.features.multiAccountDesc'),
    icon: 'users'
  },
  {
    title: t('home.features.balanceQuota'),
    description: t('home.features.balanceQuotaDesc'),
    icon: 'database'
  }
])

const comparisonItems = computed(() => [
  {
    feature: t('home.comparison.items.pricing.feature'),
    official: t('home.comparison.items.pricing.official'),
    us: t('home.comparison.items.pricing.us')
  },
  {
    feature: t('home.comparison.items.models.feature'),
    official: t('home.comparison.items.models.official'),
    us: t('home.comparison.items.models.us')
  },
  {
    feature: t('home.comparison.items.management.feature'),
    official: t('home.comparison.items.management.official'),
    us: t('home.comparison.items.management.us')
  },
  {
    feature: t('home.comparison.items.control.feature'),
    official: t('home.comparison.items.control.official'),
    us: t('home.comparison.items.control.us')
  }
])

const providers = computed(() => [
  { name: t('home.providers.claude'), mark: 'C', soon: false },
  { name: 'GPT', mark: 'G', soon: false },
  { name: t('home.providers.gemini'), mark: 'G', soon: false },
  { name: t('home.providers.antigravity'), mark: 'A', soon: false },
  { name: t('home.providers.more'), mark: '+', soon: true }
])

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
  if (event.key === 'theme') {
    syncThemePreference()
  }
}

function handleSystemThemeChange() {
  if (!localStorage.getItem('theme')) {
    syncThemePreference()
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
})

onBeforeUnmount(() => {
  window.removeEventListener('storage', handleThemeStorage)
  preferredColorScheme.removeEventListener('change', handleSystemThemeChange)
})
</script>

<style scoped>
.md3-home {
  --md-primary: #202124;
  --md-on-primary: #ffffff;
  --md-primary-container: #e8eaed;
  --md-on-primary-container: #202124;
  --md-secondary-container: #f1f3f4;
  --md-on-secondary-container: #3c4043;
  --md-surface: #ffffff;
  --md-surface-container-low: #f8fafd;
  --md-surface-container: #f1f3f4;
  --md-surface-container-high: #e8eaed;
  --md-on-surface: #202124;
  --md-on-surface-variant: #5f6368;
  --md-outline: #9aa0a6;
  --md-outline-variant: #dadce0;
  --md-shadow: 0 1px 2px rgb(0 0 0 / 0.14), 0 1px 3px 1px rgb(0 0 0 / 0.08);
  background: var(--md-surface);
  color: var(--md-on-surface);
}

.md3-home-dark {
  --md-primary: #f1f3f4;
  --md-on-primary: #202124;
  --md-primary-container: #3f3f3f;
  --md-on-primary-container: #f1f3f4;
  --md-secondary-container: #383838;
  --md-on-secondary-container: #f1f3f4;
  --md-surface: #1f1f1f;
  --md-surface-container-low: #2a2a2a;
  --md-surface-container: #303030;
  --md-surface-container-high: #3f3f3f;
  --md-on-surface: #f1f3f4;
  --md-on-surface-variant: #c7c7c7;
  --md-outline: #8e8e8e;
  --md-outline-variant: #4d4d4d;
  --md-shadow: none;
}

.md3-container {
  width: min(1120px, calc(100% - 32px));
  margin-inline: auto;
}

.md3-top-bar {
  position: sticky;
  top: 0;
  z-index: 30;
  height: 72px;
  border-bottom: 1px solid var(--md-outline-variant);
  background: color-mix(in srgb, var(--md-surface) 92%, transparent);
  backdrop-filter: blur(16px);
}

.md3-brand {
  display: inline-flex;
  min-width: 0;
  max-width: 48vw;
  align-items: center;
  gap: 12px;
  color: var(--md-on-surface);
}

.md3-brand-logo {
  display: grid;
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  place-items: center;
  overflow: hidden;
  border-radius: 8px;
  background: var(--md-surface-container-high);
}

.md3-icon-button {
  display: inline-grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 20px;
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease;
}

.md3-icon-button:hover {
  background: color-mix(in srgb, var(--md-on-surface) 8%, transparent);
  color: var(--md-on-surface);
}

.md3-locale :deep(button) {
  height: 40px;
  border-radius: 20px;
  color: var(--md-on-surface-variant);
}

.md3-locale :deep(button:hover) {
  background: color-mix(in srgb, var(--md-on-surface) 8%, transparent);
}

.md3-filled-button,
.md3-tonal-button,
.md3-hero-action,
.md3-hero-secondary,
.md3-cta-button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 20px;
  padding: 0 20px;
  font-size: 0.875rem;
  font-weight: 600;
  transition: background-color 160ms ease, box-shadow 160ms ease, transform 160ms ease;
}

.md3-filled-button,
.md3-hero-action,
.md3-cta-button {
  background: var(--md-primary);
  color: var(--md-on-primary);
  box-shadow: var(--md-shadow);
}

.md3-filled-button:hover,
.md3-hero-action:hover,
.md3-cta-button:hover {
  background: color-mix(in srgb, var(--md-primary) 92%, var(--md-on-primary));
  box-shadow: 0 2px 6px rgb(0 0 0 / 0.16);
}

.md3-tonal-button,
.md3-hero-secondary {
  background: var(--md-secondary-container);
  color: var(--md-on-secondary-container);
}

.md3-tonal-button:hover,
.md3-hero-secondary:hover {
  background: color-mix(in srgb, var(--md-secondary-container) 88%, var(--md-on-secondary-container));
}

.md3-avatar {
  display: grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 50%;
  background: var(--md-primary);
  color: var(--md-on-primary);
  font-size: 0.75rem;
}

.md3-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 32px;
  padding-top: 64px;
  padding-bottom: 48px;
}

.md3-hero-copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
}

.md3-assist-chip,
.md3-filter-chip {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 0 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  font-weight: 500;
}

.md3-hero h1 {
  margin-top: 24px;
  max-width: 760px;
  color: var(--md-on-surface);
  font-size: clamp(2.5rem, 6vw, 4.75rem);
  font-weight: 700;
  line-height: 1.02;
}

.md3-hero-subtitle {
  margin-top: 18px;
  color: var(--md-primary);
  font-size: 1.25rem;
  font-weight: 600;
}

.md3-hero-description {
  margin-top: 12px;
  max-width: 620px;
  color: var(--md-on-surface-variant);
  font-size: 1rem;
  line-height: 1.75;
}

.md3-hero-action,
.md3-hero-secondary {
  margin-top: 28px;
  min-height: 48px;
  border-radius: 24px;
}

.md3-gateway-panel {
  align-self: center;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  box-shadow: var(--md-shadow);
  padding: 20px;
}

.md3-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--md-outline-variant);
  padding-bottom: 16px;
}

.md3-panel-label,
.md3-section-heading p,
.md3-comparison-heading p,
.md3-cta p {
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
  font-weight: 600;
}

.md3-panel-header h2 {
  margin-top: 4px;
  color: var(--md-on-surface);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 1rem;
  font-weight: 700;
}

.md3-status-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 8px;
  background: var(--md-primary-container);
  padding: 6px 10px;
  color: var(--md-on-primary-container);
  font-size: 0.75rem;
  font-weight: 700;
}

.md3-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.md3-request-block {
  margin: 16px 0;
  border-radius: 8px;
  background: var(--md-surface);
  padding: 16px;
}

.md3-method {
  border-radius: 6px;
  background: var(--md-secondary-container);
  padding: 4px 8px;
  color: var(--md-on-secondary-container);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.75rem;
  font-weight: 800;
}

.md3-metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-left: 3px solid var(--md-primary);
  padding-left: 10px;
}

.md3-metric span,
.md3-route-row span,
.md3-footer {
  color: var(--md-on-surface-variant);
}

.md3-metric strong {
  color: var(--md-on-surface);
  font-size: 0.9375rem;
}

.md3-route-row {
  display: flex;
  align-items: center;
  gap: 12px;
  border-radius: 8px;
  padding: 12px;
  transition: background-color 160ms ease;
}

.md3-route-row:hover {
  background: color-mix(in srgb, var(--md-on-surface) 6%, transparent);
}

.md3-route-icon,
.md3-feature-icon {
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: var(--md-primary-container);
  color: var(--md-on-primary-container);
}

.md3-route-icon {
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
}

.md3-route-row p {
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-route-detail {
  display: block;
  overflow: hidden;
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-route-row strong {
  color: var(--md-primary);
  font-size: 0.75rem;
  text-transform: uppercase;
}

.md3-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 8px 0 24px;
}

.md3-section {
  padding-top: 56px;
  padding-bottom: 16px;
}

.md3-section-heading {
  margin-bottom: 20px;
}

.md3-section-heading h2,
.md3-comparison-heading h2,
.md3-cta h2 {
  margin-top: 4px;
  color: var(--md-on-surface);
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
}

.md3-feature-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 20px;
  transition: background-color 160ms ease, box-shadow 160ms ease;
}

.md3-feature-card:hover {
  background: var(--md-surface-container);
  box-shadow: var(--md-shadow);
}

.md3-feature-icon {
  width: 48px;
  height: 48px;
  margin-bottom: 18px;
}

.md3-feature-card h3 {
  color: var(--md-on-surface);
  font-size: 1rem;
  font-weight: 700;
}

.md3-feature-card p {
  margin-top: 8px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  line-height: 1.65;
}

.md3-comparison {
  display: grid;
  gap: 20px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 20px;
}

.md3-comparison-list {
  display: grid;
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-outline-variant);
}

.md3-comparison-row {
  display: grid;
  gap: 8px;
  background: var(--md-surface);
  padding: 14px;
}

.md3-comparison-row span {
  color: var(--md-on-surface);
  font-weight: 700;
}

.md3-comparison-row p {
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
}

.md3-comparison-row strong {
  color: var(--md-primary);
  font-size: 0.875rem;
}

.md3-provider-grid {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
}

.md3-provider {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 12px;
}

.md3-provider-muted {
  opacity: 0.68;
}

.md3-provider-mark {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 8px;
  background: var(--md-secondary-container);
  color: var(--md-on-secondary-container);
  font-weight: 800;
}

.md3-provider small {
  border-radius: 6px;
  background: var(--md-primary-container);
  padding: 4px 8px;
  color: var(--md-on-primary-container);
  font-size: 0.6875rem;
  font-weight: 700;
}

.md3-cta-section {
  padding-top: 64px;
  padding-bottom: 48px;
}

.md3-cta {
  display: flex;
  flex-direction: column;
  gap: 20px;
  border-radius: 8px;
  background: var(--md-primary-container);
  padding: 24px;
  color: var(--md-on-primary-container);
}

.md3-cta h2,
.md3-cta p {
  color: var(--md-on-primary-container);
}

.md3-cta-button {
  align-self: flex-start;
  background: var(--md-on-primary-container);
  color: var(--md-primary-container);
}

.md3-footer {
  border-top: 1px solid var(--md-outline-variant);
  font-size: 0.875rem;
}

.md3-footer a {
  color: var(--md-on-surface-variant);
  font-weight: 600;
}

.md3-footer a:hover {
  color: var(--md-primary);
}

@media (min-width: 768px) {
  .md3-comparison {
    grid-template-columns: minmax(220px, 0.72fr) minmax(0, 1.28fr);
    align-items: start;
  }

  .md3-comparison-row {
    grid-template-columns: 0.7fr 1fr 1fr;
    align-items: center;
  }
}

@media (min-width: 1024px) {
  .md3-hero {
    grid-template-columns: minmax(0, 1.05fr) minmax(360px, 0.95fr);
    gap: 56px;
    padding-top: 88px;
    padding-bottom: 56px;
  }
}

@media (max-width: 640px) {
  .md3-container {
    width: min(100% - 24px, 1120px);
  }

  .md3-top-bar {
    height: 64px;
  }

  .md3-brand {
    max-width: 38vw;
  }

  .md3-hero {
    padding-top: 40px;
  }

  .md3-hero h1 {
    font-size: 2.625rem;
  }

  .md3-hero-action,
  .md3-hero-secondary,
  .md3-cta-button {
    width: 100%;
  }
}
</style>
