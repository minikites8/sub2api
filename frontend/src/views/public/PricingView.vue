<template>
  <div class="pricing-page">
    <header class="pricing-header">
      <nav class="pricing-nav" :aria-label="t('pricingPage.primaryNavigation')">
        <div class="pricing-nav-start">
          <router-link to="/home" class="pricing-brand" :aria-label="siteName">
            <img v-if="siteLogo" :src="siteLogo" alt="" class="pricing-brand-logo" />
            <span>{{ siteName }}</span>
          </router-link>

          <div class="pricing-nav-links">
            <router-link to="/docs" class="pricing-nav-link">
              {{ t('pricingPage.docs') }}
            </router-link>
            <router-link to="/pricing" class="pricing-nav-link pricing-nav-link-active">
              {{ t('pricingPage.pricing') }}
            </router-link>
            <router-link to="/models" class="pricing-nav-link">
              {{ t('pricingPage.models') }}
            </router-link>
          </div>
        </div>

        <div class="pricing-nav-actions">
          <div class="pricing-locale"><LocaleSwitcher /></div>
          <router-link :to="dashboardLink" class="pricing-login-button">
            {{ isAuthenticated ? t('pricingPage.dashboard') : t('pricingPage.login') }}
          </router-link>
          <button
            type="button"
            class="pricing-menu-button"
            :aria-label="mobileMenuOpen ? t('pricingPage.closeNavigation') : t('pricingPage.openNavigation')"
            :aria-expanded="mobileMenuOpen"
            aria-controls="pricing-mobile-menu"
            @click="mobileMenuOpen = !mobileMenuOpen"
          >
            <Icon :name="mobileMenuOpen ? 'x' : 'menu'" size="md" />
          </button>
        </div>
      </nav>

      <nav
        v-if="mobileMenuOpen"
        id="pricing-mobile-menu"
        class="pricing-mobile-menu"
        :aria-label="t('pricingPage.mobileNavigation')"
      >
        <router-link to="/docs" @click="mobileMenuOpen = false">
          {{ t('pricingPage.docs') }}
        </router-link>
        <router-link to="/pricing" class="pricing-mobile-active" @click="mobileMenuOpen = false">
          {{ t('pricingPage.pricing') }}
        </router-link>
        <router-link to="/models" @click="mobileMenuOpen = false">
          {{ t('pricingPage.models') }}
        </router-link>
      </nav>
    </header>

    <main class="pricing-main">
      <header class="pricing-hero">
        <h1>{{ t('pricingPage.hero.title') }}</h1>
        <p>{{ t('pricingPage.hero.description') }}</p>
      </header>

      <section class="pricing-tier-grid" :aria-label="t('pricingPage.tiersLabel')">
        <article
          v-for="tier in tiers"
          :key="tier.id"
          class="pricing-tier"
          :class="{
            'pricing-tier-recommended': tier.recommended,
            'pricing-tier-pro': tier.id === 'pro'
          }"
        >
          <span v-if="tier.recommended" class="pricing-tier-badge">
            {{ t('pricingPage.recommended') }}
          </span>

          <div>
            <h2>{{ tier.name }}</h2>
            <div class="pricing-tier-price">
              <strong>{{ tier.price }}</strong>
              <span v-if="tier.unit">{{ tier.unit }}</span>
            </div>
            <p>{{ tier.description }}</p>
          </div>

          <ul>
            <li v-for="feature in tier.features" :key="feature">
              <span class="pricing-feature-check" aria-hidden="true">
                <Icon name="check" size="xs" :stroke-width="3" />
              </span>
              <span>{{ feature }}</span>
            </li>
          </ul>

          <router-link
            v-if="tier.actionTo"
            :to="tier.actionTo"
            class="pricing-tier-action"
            :class="{ 'pricing-tier-action-primary': tier.recommended }"
          >
            {{ tier.actionLabel }}
          </router-link>
          <button v-else type="button" class="pricing-tier-action">
            {{ tier.actionLabel }}
          </button>
        </article>
      </section>

      <section class="pricing-comparison" aria-labelledby="pricing-comparison-title">
        <h2 id="pricing-comparison-title">{{ t('pricingPage.comparison.title') }}</h2>
        <div class="pricing-table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">{{ t('pricingPage.comparison.feature') }}</th>
                <th scope="col" class="pricing-comparison-emphasis">{{ t('pricingPage.tiers.payg.name') }}</th>
                <th scope="col">{{ t('pricingPage.tiers.pro.name') }}</th>
                <th scope="col">{{ t('pricingPage.tiers.enterprise.name') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in comparisonRows" :key="row.id">
                <th scope="row">{{ row.label }}</th>
                <td
                  v-for="(value, index) in row.values"
                  :key="`${row.id}-${index}`"
                  :class="{ 'pricing-comparison-emphasis': index === 0 }"
                >
                  <template v-if="value.kind === 'check'">
                    <Icon name="check" size="sm" :stroke-width="2.4" aria-hidden="true" />
                    <span class="sr-only">{{ t('pricingPage.comparison.included') }}</span>
                  </template>
                  <span v-else>{{ value.text }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>

    <footer class="pricing-footer">
      <p>&copy; {{ currentYear }} {{ siteName }} - {{ t('pricingPage.footerDescription') }}</p>
      <nav aria-label="Footer navigation">
        <router-link
          v-for="document in legalDocuments"
          :key="document.id"
          :to="`/legal/${encodeURIComponent(document.id)}`"
        >
          {{ document.title }}
        </router-link>
      </nav>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

type ComparisonValue =
  | { kind: 'text'; text: string }
  | { kind: 'check' }

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const mobileMenuOpen = ref(false)

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '',
  { allowRelative: true, allowDataUrl: true }
))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const dashboardLink = computed(() => (isAuthenticated.value ? dashboardPath.value : '/login'))
const purchaseLink = computed(() => (
  isAuthenticated.value ? '/purchase' : `/login?redirect=${encodeURIComponent('/purchase')}`
))
const professionalLink = computed(() => {
  const target = '/purchase?tab=subscriptions'
  return isAuthenticated.value ? target : `/login?redirect=${encodeURIComponent(target)}`
})
const currentYear = computed(() => new Date().getFullYear())
const legalDocuments = computed(() =>
  (appStore.cachedPublicSettings?.login_agreement_documents || [])
    .filter((document) => document.id?.trim() && document.title?.trim())
    .slice(0, 3)
)

const tiers = computed(() => [
  {
    id: 'payg',
    recommended: true,
    name: t('pricingPage.tiers.payg.name'),
    price: t('pricingPage.tiers.payg.price'),
    unit: t('pricingPage.tiers.payg.unit'),
    description: t('pricingPage.tiers.payg.description'),
    features: [
      t('pricingPage.tiers.payg.features.usage'),
      t('pricingPage.tiers.payg.features.exchange'),
      t('pricingPage.tiers.payg.features.availability')
    ],
    actionLabel: t('pricingPage.tiers.payg.action'),
    actionTo: purchaseLink.value
  },
  {
    id: 'pro',
    recommended: false,
    name: t('pricingPage.tiers.pro.name'),
    price: t('pricingPage.tiers.pro.price'),
    unit: t('pricingPage.tiers.pro.unit'),
    description: t('pricingPage.tiers.pro.description'),
    features: [
      t('pricingPage.tiers.pro.features.credits'),
      t('pricingPage.tiers.pro.features.limits'),
      t('pricingPage.tiers.pro.features.priorityScheduling'),
      t('pricingPage.tiers.pro.features.records'),
      t('pricingPage.tiers.pro.features.controls'),
      t('pricingPage.tiers.pro.features.support')
    ],
    actionLabel: t('pricingPage.tiers.pro.action'),
    actionTo: professionalLink.value
  },
  {
    id: 'enterprise',
    recommended: false,
    name: t('pricingPage.tiers.enterprise.name'),
    price: t('pricingPage.tiers.enterprise.price'),
    unit: '',
    description: t('pricingPage.tiers.enterprise.description'),
    features: [
      t('pricingPage.tiers.enterprise.features.discount'),
      t('pricingPage.tiers.enterprise.features.sla'),
      t('pricingPage.tiers.enterprise.features.support'),
      t('pricingPage.tiers.enterprise.features.infrastructure')
    ],
    actionLabel: t('pricingPage.tiers.enterprise.action'),
    actionTo: ''
  }
])

const comparisonRows = computed<Array<{
  id: string
  label: string
  values: ComparisonValue[]
}>>(() => [
  {
    id: 'requests',
    label: t('pricingPage.comparison.rows.requests.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.requests.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.requests.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.requests.enterprise') }
    ]
  },
  {
    id: 'credit-price',
    label: t('pricingPage.comparison.rows.creditPrice.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.creditPrice.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.creditPrice.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.creditPrice.enterprise') }
    ]
  },
  {
    id: 'limits',
    label: t('pricingPage.comparison.rows.limits.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.limits.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.limits.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.limits.enterprise') }
    ]
  },
  {
    id: 'scheduling',
    label: t('pricingPage.comparison.rows.scheduling.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.scheduling.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.scheduling.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.scheduling.enterprise') }
    ]
  },
  {
    id: 'key-limit',
    label: t('pricingPage.comparison.rows.keyLimit'),
    values: [
      { kind: 'text', text: '—' },
      { kind: 'check' },
      { kind: 'check' }
    ]
  },
  {
    id: 'ip-allowlist',
    label: t('pricingPage.comparison.rows.ipAllowlist'),
    values: [
      { kind: 'text', text: '—' },
      { kind: 'check' },
      { kind: 'check' }
    ]
  },
  {
    id: 'request-logs',
    label: t('pricingPage.comparison.rows.requestLogs.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.requestLogs.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.requestLogs.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.requestLogs.enterprise') }
    ]
  },
  {
    id: 'billing-reports',
    label: t('pricingPage.comparison.rows.billingReports.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.billingReports.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.billingReports.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.billingReports.enterprise') }
    ]
  },
  {
    id: 'custom-models',
    label: t('pricingPage.comparison.rows.customModels'),
    values: [
      { kind: 'text', text: '—' },
      { kind: 'text', text: '—' },
      { kind: 'check' }
    ]
  },
  {
    id: 'support',
    label: t('pricingPage.comparison.rows.support.label'),
    values: [
      { kind: 'text', text: t('pricingPage.comparison.rows.support.payg') },
      { kind: 'text', text: t('pricingPage.comparison.rows.support.pro') },
      { kind: 'text', text: t('pricingPage.comparison.rows.support.enterprise') }
    ]
  }
])

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') mobileMenuOpen.value = false
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
:global(html:has(.pricing-page)),
:global(body:has(.pricing-page)) {
  overflow-x: clip;
}

.pricing-page {
  --pricing-bg: #000000;
  --pricing-surface: #0a0a0a;
  --pricing-surface-high: #171717;
  --pricing-border: #353a3d;
  --pricing-border-soft: rgba(132, 149, 135, 0.13);
  --pricing-text: #ffffff;
  --pricing-muted: #cccccc;
  --pricing-faint: #8d969f;
  --pricing-accent: #00e38b;
  --pricing-accent-bright: #00ff9d;
  --pricing-blue: #508eff;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  overflow-x: hidden;
  background-color: var(--pricing-bg);
  background-image:
    linear-gradient(var(--pricing-border-soft) 1px, transparent 1px),
    linear-gradient(90deg, var(--pricing-border-soft) 1px, transparent 1px);
  background-size: 32px 32px;
  color: var(--pricing-text);
  font-family: 'Geist', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  font-size: 16px;
  line-height: 1.6;
  letter-spacing: 0;
  -webkit-font-smoothing: antialiased;
}

.pricing-page :where(*) {
  letter-spacing: 0;
}

.pricing-header {
  position: fixed;
  inset: 0 0 auto;
  z-index: 30;
  height: 64px;
  border-bottom: 1px solid #3b4a3f;
  background: rgb(6 15 22 / 86%);
  backdrop-filter: blur(12px);
}

.pricing-nav {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 0 20px;
}

.pricing-nav-start,
.pricing-nav-actions,
.pricing-nav-links {
  display: flex;
  align-items: center;
}

.pricing-nav-start {
  min-width: 0;
  gap: 32px;
}

.pricing-brand {
  min-width: 0;
  max-width: 42vw;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  overflow: hidden;
  color: var(--pricing-accent);
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  text-decoration: none;
}

.pricing-brand span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pricing-brand-logo {
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 4px;
  object-fit: contain;
}

.pricing-nav-links {
  display: none;
  align-self: stretch;
  align-items: center;
  gap: 24px;
}

.pricing-nav-link {
  position: relative;
  height: 100%;
  display: flex;
  align-items: center;
  color: var(--pricing-muted);
  font-size: 15px;
  text-decoration: none;
  transition: color 160ms ease;
}

.pricing-nav-link:hover,
.pricing-nav-link:focus-visible,
.pricing-nav-link-active {
  color: var(--pricing-accent);
}

.pricing-nav-link-active::after {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 2px;
  background: var(--pricing-accent);
  content: '';
}

.pricing-nav-actions {
  flex: 0 0 auto;
  gap: 8px;
}

.pricing-locale :deep(> div > button) {
  min-width: 42px;
  min-height: 36px;
  justify-content: center;
  border-radius: 4px;
  color: var(--pricing-muted);
}

.pricing-locale :deep(> div > button:hover),
.pricing-locale :deep(> div > button:focus-visible) {
  background: var(--pricing-surface-high);
  color: var(--pricing-text);
}

.pricing-locale :deep(.absolute) {
  border-color: var(--pricing-border);
  border-radius: 4px;
  background: var(--pricing-surface);
}

.pricing-locale :deep(.absolute button) {
  border-radius: 0;
  color: var(--pricing-muted);
}

.pricing-locale :deep(.absolute button:hover) {
  background: var(--pricing-surface-high);
}

.pricing-login-button,
.pricing-menu-button {
  min-height: 36px;
  border: 1px solid var(--pricing-border);
  border-radius: 4px;
  background: transparent;
  color: var(--pricing-accent);
}

.pricing-login-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 7px 14px;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
}

.pricing-menu-button {
  width: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  cursor: pointer;
}

.pricing-login-button:hover,
.pricing-login-button:focus-visible,
.pricing-menu-button:hover,
.pricing-menu-button:focus-visible {
  border-color: var(--pricing-accent);
  color: var(--pricing-accent);
}

.pricing-mobile-menu {
  position: fixed;
  inset: 64px 0 auto;
  z-index: 29;
  display: grid;
  gap: 2px;
  padding: 12px 20px 16px;
  border-bottom: 1px solid #3b4a3f;
  background: rgb(6 15 22 / 98%);
  box-shadow: 0 20px 36px rgb(0 0 0 / 28%);
}

.pricing-mobile-menu a {
  min-height: 42px;
  display: flex;
  align-items: center;
  padding: 8px 10px;
  border-radius: 4px;
  color: var(--pricing-muted);
  text-decoration: none;
}

.pricing-mobile-menu a:hover,
.pricing-mobile-menu a:focus-visible,
.pricing-mobile-menu .pricing-mobile-active {
  background: var(--pricing-surface-high);
  color: var(--pricing-accent);
}

.pricing-main {
  width: min(100%, 1280px);
  min-width: 0;
  max-width: 100%;
  flex: 1;
  margin: 0 auto;
  padding: 132px 20px 104px;
}

.pricing-hero {
  max-width: 760px;
  margin: 0 auto 82px;
  text-align: center;
}

.pricing-hero h1 {
  margin: 0;
  color: var(--pricing-text);
  font-size: 34px;
  font-weight: 700;
  line-height: 1.15;
}

.pricing-hero p {
  margin: 18px 0 0;
  color: var(--pricing-muted);
  font-size: 16px;
  line-height: 1.65;
}

.pricing-tier-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 24px;
}

.pricing-tier {
  position: relative;
  min-width: 0;
  min-height: 424px;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 30px;
  border: 1px solid var(--pricing-border);
  border-radius: 8px;
  background: rgba(10, 10, 10, 0.96);
  transition: border-color 180ms ease, background-color 180ms ease;
}

.pricing-tier:hover {
  border-color: var(--pricing-accent);
  background: #0d1113;
}

.pricing-tier-recommended {
  border: 2px solid #d7fbed;
  box-shadow: 0 0 26px rgba(0, 255, 157, 0.2);
}

.pricing-tier-badge {
  position: absolute;
  top: 0;
  left: 50%;
  padding: 5px 13px;
  transform: translate(-50%, -50%);
  border-radius: 9999px;
  background: var(--pricing-accent-bright);
  color: #00391f;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
}

.pricing-tier h2 {
  margin: 0;
  color: var(--pricing-text);
  font-size: 23px;
  font-weight: 700;
  line-height: 1.3;
}

.pricing-tier-price {
  min-height: 68px;
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px 10px;
  margin-top: 14px;
}

.pricing-tier-price strong {
  max-width: 100%;
  overflow-wrap: anywhere;
  color: var(--pricing-text);
  font-size: 37px;
  font-weight: 700;
  line-height: 1.15;
}

.pricing-tier-pro .pricing-tier-price strong {
  font-size: 32px;
}

.pricing-tier-price span {
  color: var(--pricing-muted);
  font-size: 14px;
}

.pricing-tier p {
  margin: 5px 0 0;
  color: var(--pricing-muted);
  font-size: 14px;
  line-height: 1.55;
}

.pricing-tier ul {
  display: grid;
  align-content: start;
  gap: 13px;
  flex: 1;
  margin: 0;
  padding: 0;
  list-style: none;
}

.pricing-tier li {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  color: var(--pricing-muted);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.pricing-feature-check {
  width: 19px;
  height: 19px;
  display: inline-flex;
  flex: 0 0 19px;
  align-items: center;
  justify-content: center;
  margin-top: 1px;
  border-radius: 50%;
  background: var(--pricing-accent-bright);
  color: #00391f;
}

.pricing-tier-action {
  width: 100%;
  min-height: 48px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 10px 16px;
  border: 1px solid var(--pricing-border);
  border-radius: 4px;
  background: transparent;
  color: var(--pricing-text);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.2;
  text-align: center;
  text-decoration: none;
  cursor: pointer;
  transition: border-color 160ms ease, background-color 160ms ease, color 160ms ease;
}

.pricing-tier-action:hover,
.pricing-tier-action:focus-visible {
  border-color: var(--pricing-blue);
  color: #aec6ff;
}

.pricing-tier-action-primary {
  border-color: var(--pricing-accent-bright);
  background: var(--pricing-accent-bright);
  color: #00391f;
  font-weight: 700;
}

.pricing-tier-action-primary:hover,
.pricing-tier-action-primary:focus-visible {
  border-color: #56ffa8;
  background: #56ffa8;
  color: #002110;
}

.pricing-comparison {
  min-width: 0;
  max-width: 100%;
  margin-top: 112px;
}

.pricing-comparison h2 {
  margin: 0 0 28px;
  color: var(--pricing-text);
  font-size: 24px;
  font-weight: 600;
  line-height: 1.3;
}

.pricing-table-scroll {
  width: 100%;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-color: var(--pricing-border) transparent;
}

.pricing-comparison table {
  width: 100%;
  min-width: 760px;
  border-collapse: collapse;
  background: rgba(0, 0, 0, 0.7);
  text-align: left;
}

.pricing-comparison th,
.pricing-comparison td {
  width: 25%;
  padding: 15px 16px;
  border-bottom: 1px solid var(--pricing-border);
  vertical-align: middle;
}

.pricing-comparison thead th {
  border-bottom-width: 2px;
  color: var(--pricing-faint);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.3;
  text-transform: uppercase;
}

.pricing-comparison tbody th {
  color: var(--pricing-text);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  font-weight: 600;
}

.pricing-comparison td {
  color: var(--pricing-muted);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
}

.pricing-comparison td svg {
  color: currentColor;
}

.pricing-comparison .pricing-comparison-emphasis {
  color: var(--pricing-accent);
}

.pricing-footer {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 28px 20px;
  border-top: 1px solid var(--pricing-border);
  background: #000000;
  color: var(--pricing-faint);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.pricing-footer p {
  margin: 0;
  text-align: center;
}

.pricing-footer nav {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 12px 20px;
}

.pricing-footer a {
  color: var(--pricing-faint);
  text-decoration: none;
  transition: color 160ms ease;
}

.pricing-footer a:hover,
.pricing-footer a:focus-visible {
  color: var(--pricing-accent);
}

.pricing-page :where(a, button):focus-visible {
  outline: 2px solid #aec6ff;
  outline-offset: 3px;
}

@media (min-width: 640px) {
  .pricing-main {
    padding-inline: 32px;
  }

  .pricing-hero h1 {
    font-size: 42px;
  }

  .pricing-tier {
    padding: 32px;
  }
}

@media (min-width: 768px) {
  .pricing-nav {
    padding-inline: 48px;
  }

  .pricing-nav-links {
    display: flex;
  }

  .pricing-menu-button,
  .pricing-mobile-menu {
    display: none;
  }

  .pricing-main {
    padding: 142px 48px 120px;
  }

  .pricing-hero h1 {
    font-size: 48px;
  }

  .pricing-tier-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    align-items: stretch;
  }

  .pricing-tier-recommended {
    transform: translateY(-16px);
  }

  .pricing-footer {
    flex-direction: row;
    justify-content: space-between;
    padding: 30px 48px;
  }

  .pricing-footer p {
    text-align: left;
  }
}

@media (min-width: 1024px) {
  .pricing-nav,
  .pricing-main {
    padding-inline: 64px;
  }

  .pricing-footer {
    padding-inline: 64px;
  }
}

@media (max-width: 479px) {
  .pricing-brand {
    max-width: 42vw;
    font-size: 18px;
  }

  .pricing-locale :deep(> div > button span:nth-child(2)),
  .pricing-locale :deep(> div > button svg) {
    display: none;
  }

  .pricing-main {
    padding-top: 116px;
  }

  .pricing-hero {
    margin-bottom: 64px;
  }

  .pricing-tier {
    min-height: 0;
    padding: 26px 22px;
  }

  .pricing-comparison {
    margin-top: 84px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pricing-page *,
  .pricing-page *::before,
  .pricing-page *::after {
    scroll-behavior: auto;
    transition-duration: 0.01ms;
  }
}
</style>
