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

  <div v-else class="stitch-page">
    <div class="stitch-grid" aria-hidden="true"></div>
    <div class="stitch-vignette" aria-hidden="true"></div>

    <header class="stitch-header">
      <nav class="stitch-nav" :aria-label="t('home.primaryNavigation')">
        <div class="stitch-nav-start">
          <router-link to="/home" class="stitch-brand" :aria-label="siteName">
            <img v-if="siteLogo" :src="siteLogo" alt="" class="stitch-brand-logo" />
            <span>{{ siteName }}</span>
          </router-link>

          <div class="stitch-nav-links">
            <router-link to="/docs" class="stitch-nav-link">
              {{ t('home.docs') }}
            </router-link>
            <router-link to="/pricing" class="stitch-nav-link">
              {{ t('home.pricing') }}
            </router-link>
            <router-link
              v-if="publicTransitEnabled"
              to="/public/transit"
              class="stitch-nav-link"
            >
              {{ t('publicTransit.nav') }}
            </router-link>
            <router-link to="/models" class="stitch-nav-link">
              {{ t('home.modelMarketplace') }}
            </router-link>
          </div>
        </div>

        <div class="stitch-nav-actions">
          <div class="stitch-locale"><LocaleSwitcher /></div>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="stitch-login-button"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
          <button
            type="button"
            class="stitch-mobile-menu-button"
            :aria-label="mobileMenuOpen ? t('home.closeNavigation') : t('home.openNavigation')"
            :aria-expanded="mobileMenuOpen"
            aria-controls="stitch-mobile-menu"
            @click="mobileMenuOpen = !mobileMenuOpen"
          >
            <Icon :name="mobileMenuOpen ? 'x' : 'menu'" size="md" />
          </button>
        </div>
      </nav>

      <nav
        v-if="mobileMenuOpen"
        id="stitch-mobile-menu"
        class="stitch-mobile-menu"
        :aria-label="t('home.mobileNavigation')"
      >
        <router-link to="/docs" @click="mobileMenuOpen = false">
          {{ t('home.docs') }}
        </router-link>
        <router-link to="/pricing" @click="mobileMenuOpen = false">
          {{ t('home.pricing') }}
        </router-link>
        <router-link
          v-if="publicTransitEnabled"
          to="/public/transit"
          @click="mobileMenuOpen = false"
        >
          {{ t('publicTransit.nav') }}
        </router-link>
        <router-link to="/models" @click="mobileMenuOpen = false">
          {{ t('home.modelMarketplace') }}
        </router-link>
      </nav>
    </header>

    <main class="stitch-main">
      <section class="stitch-hero" aria-labelledby="home-hero-title">
        <div class="stitch-hero-copy">
          <div class="stitch-chip">{{ t('home.heroSubtitle') }}</div>
          <h1 id="home-hero-title" class="stitch-headline">{{ t('home.heroTitle') }}</h1>
          <p class="stitch-description">{{ t('home.heroDescription') }}</p>

          <div class="stitch-hero-actions">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="stitch-primary-button"
            >
              <span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.heroCta') }}</span>
              <Icon name="arrowRight" size="sm" />
            </router-link>
            <router-link to="/docs" class="stitch-secondary-button">
              <Icon name="book" size="sm" />
              <span>{{ t('home.heroCtaDocs') }}</span>
            </router-link>
          </div>
        </div>

        <div id="api-example" class="stitch-code-shell">
          <div class="stitch-code-glow" aria-hidden="true"></div>
          <div class="stitch-code-window">
            <div class="stitch-code-titlebar">
              <div class="stitch-window-dots" aria-hidden="true">
                <span class="stitch-window-dot stitch-window-dot-red"></span>
                <span class="stitch-window-dot stitch-window-dot-neutral"></span>
                <span class="stitch-window-dot stitch-window-dot-green"></span>
              </div>
              <span class="stitch-code-filename">{{ t('home.codeBlockTitle') }}</span>
              <button
                type="button"
                class="stitch-copy-button"
                :title="copied ? t('home.codeCopied') : t('home.copyCode')"
                :aria-label="copied ? t('home.codeCopied') : t('home.copyCode')"
                @click="copyCode"
              >
                <Icon :name="copied ? 'check' : 'copy'" size="sm" />
              </button>
            </div>
            <div class="stitch-code-body">
              <pre><code><span class="code-keyword">import</span> { Gateway } <span class="code-keyword">from</span> <span class="code-string">'@your-code/sdk'</span>;

<span class="code-keyword">const</span> ai = <span class="code-keyword">new</span> Gateway({
  apiKey: <span class="code-string">'yc_xxxxxx'</span>
});

<span class="code-comment">// Instantly switch models</span>
<span class="code-keyword">const</span> response = <span class="code-keyword">await</span> ai.complete({
  model: <span class="code-string">'anthropic/claude-3-opus'</span>,
  messages: [{ role: <span class="code-string">'user'</span>, content: <span class="code-string">'Hello world'</span> }]
});</code></pre>
            </div>
            <p class="sr-only" aria-live="polite">{{ copied ? t('home.codeCopied') : '' }}</p>
          </div>
        </div>
      </section>

      <section class="stitch-features" aria-labelledby="home-features-title">
        <h2 id="home-features-title" class="stitch-section-title">
          {{ t('home.models.title') }}
        </h2>

        <div class="stitch-feature-grid">
          <article v-for="feature in features" :key="feature.id" class="stitch-feature-card">
            <Icon :name="feature.watermark" size="xl" class="stitch-feature-watermark" aria-hidden="true" />
            <span class="stitch-feature-icon" aria-hidden="true">
              <Icon :name="feature.icon" size="md" />
            </span>
            <h3>{{ feature.title }}</h3>
            <p>{{ feature.description }}</p>
          </article>
        </div>
      </section>

      <section class="stitch-visual" :aria-label="t('home.heroImageAlt')">
        <img
          src="/gateway-data-streams.png"
          :alt="t('home.heroImageAlt')"
          class="stitch-visual-image"
        />
        <div class="stitch-visual-overlay" aria-hidden="true"></div>
      </section>
    </main>

    <footer class="stitch-footer">
      <p>&copy; {{ currentYear }} {{ siteName }} - {{ t('home.footer.allRightsReserved') }}</p>
      <nav
        v-if="legalDocuments.length || icpFilingNumber || publicSecurityFilingNumber"
        class="stitch-footer-links"
        :aria-label="t('home.footerNavigation')"
      >
        <router-link
          v-for="document in legalDocuments"
          :key="document.id"
          :to="`/legal/${encodeURIComponent(document.id)}`"
        >
          {{ footerDocumentTitle(document.id, document.title) }}
        </router-link>
        <a
          v-if="icpFilingNumber"
          href="https://beian.miit.gov.cn/"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ icpFilingNumber }}
        </a>
        <a
          v-if="publicSecurityFilingNumber"
          href="https://beian.mps.gov.cn/#/query/webSearch"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ publicSecurityFilingNumber }}
        </a>
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

type FeatureIcon = 'bolt' | 'link' | 'chartBar' | 'globe' | 'server' | 'trendingUp'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const codeSnippet = `import { Gateway } from '@your-code/sdk';

const ai = new Gateway({
  apiKey: 'yc_xxxxxx'
});

// Instantly switch models
const response = await ai.complete({
  model: 'anthropic/claude-3-opus',
  messages: [{ role: 'user', content: 'Hello world' }]
});`

const copied = ref(false)
const mobileMenuOpen = ref(false)
let copyFeedbackTimer: number | undefined

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const icpFilingNumber = computed(
  () => appStore.cachedPublicSettings?.site_icp_filing_number?.trim() || ''
)
const publicSecurityFilingNumber = computed(
  () => appStore.cachedPublicSettings?.site_public_security_filing_number?.trim() || ''
)
const publicTransitEnabled = computed(() =>
  appStore.cachedPublicSettings?.public_transit_enabled === true &&
  appStore.cachedPublicSettings?.public_transit_page_enabled === true
)
const legalDocuments = computed(() =>
  (appStore.cachedPublicSettings?.login_agreement_documents || [])
    .filter((document) => document.id?.trim() && document.title?.trim())
    .slice(0, 3)
)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const currentYear = computed(() => new Date().getFullYear())

const features = computed<Array<{
  id: string
  icon: FeatureIcon
  watermark: FeatureIcon
  title: string
  description: string
}>>(() => [
  {
    id: 'speed',
    icon: 'globe',
    watermark: 'bolt',
    title: t('home.features.unifiedGateway'),
    description: t('home.features.unifiedGatewayDesc')
  },
  {
    id: 'api',
    icon: 'link',
    watermark: 'server',
    title: t('home.features.multiAccount'),
    description: t('home.features.multiAccountDesc')
  },
  {
    id: 'monitoring',
    icon: 'chartBar',
    watermark: 'trendingUp',
    title: t('home.features.balanceQuota'),
    description: t('home.features.balanceQuotaDesc')
  }
])

function footerDocumentTitle(id: string, title: string): string {
  if (id === 'terms') return t('home.footer.terms')
  if (id === 'privacy') return t('home.footer.privacy')
  return title
}

async function copyCode(): Promise<void> {
  try {
    await navigator.clipboard.writeText(codeSnippet)
    copied.value = true
    if (copyFeedbackTimer) window.clearTimeout(copyFeedbackTimer)
    copyFeedbackTimer = window.setTimeout(() => {
      copied.value = false
    }, 1800)
  } catch {
    copied.value = false
  }
}

function handleNavigationKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') mobileMenuOpen.value = false
}

onMounted(() => {
  document.addEventListener('keydown', handleNavigationKeydown)
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleNavigationKeydown)
  if (copyFeedbackTimer) window.clearTimeout(copyFeedbackTimer)
})
</script>

<style scoped>
.stitch-page {
  position: relative;
  isolation: isolate;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #0b141c;
  color: #dae3ee;
  font-family: 'Geist', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  font-size: 16px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

.stitch-grid,
.stitch-vignette {
  position: fixed;
  inset: 0;
  z-index: -2;
  pointer-events: none;
}

.stitch-grid {
  background-image:
    linear-gradient(rgba(59, 74, 63, 0.14) 1px, transparent 1px),
    linear-gradient(90deg, rgba(59, 74, 63, 0.14) 1px, transparent 1px);
  background-size: 32px 32px;
}

.stitch-vignette {
  z-index: -1;
  background: radial-gradient(circle at 50% 30%, rgba(11, 20, 28, 0.2) 0%, #0b141c 76%);
}

.stitch-header {
  position: fixed;
  inset: 0 0 auto;
  z-index: 30;
  height: 64px;
  background: rgba(6, 15, 22, 0.86);
  border-bottom: 1px solid #3b4a3f;
  backdrop-filter: blur(12px);
}

.stitch-nav {
  width: 100%;
  height: 100%;
  padding: 0 20px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.stitch-nav-start,
.stitch-nav-actions,
.stitch-nav-links {
  display: flex;
  align-items: center;
}

.stitch-nav-start {
  min-width: 0;
  gap: 32px;
}

.stitch-nav-actions {
  flex: 0 0 auto;
  gap: 8px;
}

.stitch-nav-links {
  display: none;
  gap: 24px;
}

.stitch-brand {
  min-width: 0;
  max-width: 42vw;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  overflow: hidden;
  color: #00e38b;
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: 0;
  text-decoration: none;
}

.stitch-brand span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stitch-brand-logo {
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 4px;
  object-fit: contain;
}

.stitch-nav-link {
  color: #b9cbbc;
  font-size: 15px;
  text-decoration: none;
  transition: color 160ms ease;
}

.stitch-nav-link:hover,
.stitch-nav-link:focus-visible {
  color: #00ff9d;
}

.stitch-locale :deep(> div > button) {
  min-width: 42px;
  min-height: 36px;
  justify-content: center;
  border-radius: 4px;
  color: #b9cbbc;
}

.stitch-locale :deep(> div > button:hover),
.stitch-locale :deep(> div > button:focus-visible) {
  background: #182028;
  color: #f4fff3;
}

.stitch-locale :deep(.absolute) {
  border-color: #3b4a3f;
  border-radius: 4px;
  background: #060f16;
}

.stitch-locale :deep(.absolute button) {
  border-radius: 0;
  color: #dae3ee;
}

.stitch-locale :deep(.absolute button:hover) {
  background: #182028;
}

.stitch-login-button {
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 7px 14px;
  border: 1px solid #3b4a3f;
  border-radius: 4px;
  color: #00e38b;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  text-decoration: none;
  transition: border-color 160ms ease, background-color 160ms ease;
}

.stitch-login-button:hover,
.stitch-login-button:focus-visible {
  border-color: #00e38b;
  background: rgba(0, 227, 139, 0.06);
}

.stitch-mobile-menu-button {
  width: 36px;
  height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid #3b4a3f;
  border-radius: 4px;
  background: transparent;
  color: #b9cbbc;
  cursor: pointer;
}

.stitch-mobile-menu-button:hover,
.stitch-mobile-menu-button:focus-visible {
  border-color: #00e38b;
  color: #00e38b;
}

.stitch-mobile-menu {
  position: fixed;
  inset: 64px 0 auto;
  z-index: 29;
  display: grid;
  gap: 2px;
  padding: 12px 20px 16px;
  border-bottom: 1px solid #3b4a3f;
  background: rgba(6, 15, 22, 0.98);
  box-shadow: 0 20px 36px rgba(0, 0, 0, 0.28);
}

.stitch-mobile-menu a {
  min-height: 42px;
  display: flex;
  align-items: center;
  padding: 8px 10px;
  border-radius: 4px;
  color: #dae3ee;
  text-decoration: none;
}

.stitch-mobile-menu a:hover,
.stitch-mobile-menu a:focus-visible {
  background: #182028;
  color: #00e38b;
}

.stitch-main {
  width: min(100%, 1280px);
  flex: 1;
  margin: 0 auto;
  padding: 120px 20px 96px;
}

.stitch-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: center;
  gap: 48px;
  margin-bottom: 112px;
}

.stitch-hero-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 22px;
}

.stitch-chip {
  max-width: 100%;
  padding: 4px 11px;
  overflow-wrap: anywhere;
  border: 1px solid #3b4a3f;
  border-radius: 4px;
  background: rgba(11, 20, 28, 0.6);
  color: #00e38b;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: 0;
  text-transform: uppercase;
}

.stitch-headline {
  max-width: 650px;
  margin: 0;
  color: #f4fff3;
  font-size: 34px;
  font-weight: 700;
  line-height: 1.16;
  letter-spacing: 0;
  text-wrap: balance;
}

.stitch-description {
  max-width: 600px;
  margin: 0;
  color: #b9cbbc;
  font-size: 16px;
  line-height: 1.65;
}

.stitch-hero-actions {
  width: 100%;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px;
  margin-top: 6px;
}

.stitch-primary-button,
.stitch-secondary-button {
  min-height: 46px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  padding: 11px 22px;
  border-radius: 4px;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 14px;
  line-height: 1;
  text-decoration: none;
  transition: border-color 160ms ease, color 160ms ease, background-color 160ms ease;
}

.stitch-primary-button {
  border: 1px solid #00ff9d;
  background: #00ff9d;
  color: #00391f;
  font-weight: 700;
}

.stitch-primary-button:hover,
.stitch-primary-button:focus-visible {
  background: #56ffa8;
}

.stitch-secondary-button {
  border: 1px solid #3b4a3f;
  color: #dae3ee;
  font-weight: 500;
}

.stitch-secondary-button:hover,
.stitch-secondary-button:focus-visible {
  border-color: #508eff;
  color: #aec6ff;
}

.stitch-code-shell {
  position: relative;
  min-width: 0;
  width: 100%;
  scroll-margin-top: 88px;
}

.stitch-code-glow {
  position: absolute;
  inset: 8%;
  z-index: -1;
  background: rgba(0, 255, 157, 0.1);
  filter: blur(56px);
}

.stitch-code-window {
  position: relative;
  overflow: hidden;
  border: 1px solid #3b4a3f;
  border-radius: 8px;
  background: #010409;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.34);
}

.stitch-code-titlebar {
  position: relative;
  min-height: 40px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  border-bottom: 1px solid #3b4a3f;
  background: #060f16;
}

.stitch-window-dots {
  display: flex;
  gap: 7px;
}

.stitch-window-dot {
  width: 11px;
  height: 11px;
  border-radius: 9999px;
}

.stitch-window-dot-red { background: #ffb4ab; }
.stitch-window-dot-neutral { background: #849587; }
.stitch-window-dot-green { background: #00e38b; }

.stitch-code-filename {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  color: #b9cbbc;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1;
}

.stitch-copy-button {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin: -3px -6px -3px 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: #b9cbbc;
  cursor: pointer;
  transition: color 160ms ease, background-color 160ms ease;
}

.stitch-copy-button:hover,
.stitch-copy-button:focus-visible {
  background: #182028;
  color: #00e38b;
}

.stitch-code-body {
  overflow-x: auto;
  padding: 22px;
}

.stitch-code-body pre {
  min-width: 540px;
  margin: 0;
  color: #dae3ee;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  font-weight: 450;
  line-height: 1.65;
  tab-size: 2;
}

.code-keyword { color: #aec6ff; }
.code-string { color: #00e38b; }
.code-comment { color: #737a82; }

.stitch-features {
  margin-bottom: 112px;
}

.stitch-section-title {
  margin: 0 0 42px;
  color: #f4fff3;
  font-size: 24px;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: 0;
}

.stitch-feature-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 22px;
}

.stitch-feature-card {
  position: relative;
  min-width: 0;
  min-height: 246px;
  overflow: hidden;
  padding: 28px;
  border: 1px solid #3b4a3f;
  border-radius: 8px;
  background: rgba(11, 20, 28, 0.82);
  transition: border-color 180ms ease, background-color 180ms ease;
}

.stitch-feature-card:hover {
  border-color: #00e38b;
  background: #0d1821;
}

.stitch-feature-watermark {
  position: absolute;
  top: 18px;
  right: 18px;
  width: 58px;
  height: 58px;
  color: #00e38b;
  opacity: 0.1;
  transition: opacity 180ms ease;
}

.stitch-feature-card:hover .stitch-feature-watermark {
  opacity: 0.2;
}

.stitch-feature-icon {
  width: 46px;
  height: 46px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 22px;
  border: 1px solid #3b4a3f;
  border-radius: 4px;
  background: #222b33;
  color: #00e38b;
}

.stitch-feature-card h3 {
  position: relative;
  margin: 0 0 10px;
  color: #dae3ee;
  font-size: 18px;
  font-weight: 600;
  line-height: 1.35;
  letter-spacing: 0;
}

.stitch-feature-card p {
  position: relative;
  margin: 0;
  color: #b9cbbc;
  font-size: 14px;
  line-height: 1.55;
}

.stitch-visual {
  position: relative;
  overflow: hidden;
  width: 100%;
  aspect-ratio: 16 / 9;
  margin-bottom: 32px;
  border: 1px solid #3b4a3f;
  border-radius: 8px;
  background: #060f16;
  box-shadow: 0 0 70px rgba(0, 255, 157, 0.06);
}

.stitch-visual-image {
  width: 100%;
  height: 100%;
  display: block;
  object-fit: cover;
  object-position: center;
}

.stitch-visual-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(to top, rgba(11, 20, 28, 0.44), transparent 52%);
  pointer-events: none;
}

.stitch-footer {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 28px 20px;
  border-top: 1px solid #3b4a3f;
  background: #060f16;
  color: #737a82;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.stitch-footer p {
  margin: 0;
  text-align: center;
}

.stitch-footer-links {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 12px 22px;
}

.stitch-footer a {
  color: #737a82;
  text-decoration: none;
  transition: color 160ms ease;
}

.stitch-footer a:hover,
.stitch-footer a:focus-visible {
  color: #f4fff3;
}

.stitch-page :where(a, button):focus-visible {
  outline: 2px solid #aec6ff;
  outline-offset: 3px;
}

@media (min-width: 640px) {
  .stitch-main {
    padding-inline: 32px;
  }

  .stitch-headline {
    font-size: 42px;
  }

  .stitch-visual {
    aspect-ratio: 3 / 1;
  }
}

@media (min-width: 768px) {
  .stitch-nav {
    padding-inline: 48px;
  }

  .stitch-nav-links {
    display: flex;
  }

  .stitch-mobile-menu-button,
  .stitch-mobile-menu {
    display: none;
  }

  .stitch-main {
    padding: 128px 48px 112px;
  }

  .stitch-feature-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .stitch-footer {
    flex-direction: row;
    justify-content: space-between;
    padding: 30px 48px;
  }

  .stitch-footer p {
    text-align: left;
  }
}

@media (min-width: 1024px) {
  .stitch-nav {
    padding-inline: 64px;
  }

  .stitch-main {
    padding-inline: 64px;
  }

  .stitch-hero {
    grid-template-columns: minmax(0, 1fr) minmax(440px, 1fr);
    gap: 64px;
    margin-bottom: 124px;
  }

  .stitch-headline {
    font-size: 48px;
    line-height: 1.1;
  }

  .stitch-features {
    margin-bottom: 124px;
  }

  .stitch-footer {
    padding-inline: 64px;
  }
}

@media (max-width: 479px) {
  .stitch-locale :deep(> div > button span:nth-child(2)),
  .stitch-locale :deep(> div > button svg) {
    display: none;
  }

  .stitch-primary-button,
  .stitch-secondary-button {
    width: 100%;
  }

  .stitch-code-body {
    padding: 18px;
  }

  .stitch-code-body pre {
    font-size: 12px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .stitch-page *,
  .stitch-page *::before,
  .stitch-page *::after {
    scroll-behavior: auto;
    transition-duration: 0.01ms;
  }
}
</style>
