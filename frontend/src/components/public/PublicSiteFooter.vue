<template>
  <footer class="public-site-footer" :class="`public-site-footer--${theme}`">
    <p>&copy; {{ currentYear }} {{ siteName }} - {{ description }}</p>
    <nav v-if="hasFooterLinks" :aria-label="t('home.footerNavigation')">
      <router-link
        v-for="document in legalDocuments"
        :key="document.id"
        :to="`/legal/${encodeURIComponent(document.id)}`"
      >
        {{ documentTitle(document.id, document.title) }}
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
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'

type FooterTheme = 'home' | 'docs' | 'pricing' | 'models'

withDefaults(defineProps<{
  description: string
  theme?: FooterTheme
}>(), {
  theme: 'home',
})

const { t } = useI18n()
const appStore = useAppStore()

const currentYear = new Date().getFullYear()
const siteName = computed(() => (
  appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
))
const icpFilingNumber = computed(() => (
  appStore.cachedPublicSettings?.site_icp_filing_number?.trim() || ''
))
const publicSecurityFilingNumber = computed(() => (
  appStore.cachedPublicSettings?.site_public_security_filing_number?.trim() || ''
))
const legalDocuments = computed(() => (
  appStore.cachedPublicSettings?.login_agreement_documents || []
)
  .filter((document) => document.id?.trim() && document.title?.trim())
  .slice(0, 3))
const hasFooterLinks = computed(() => (
  legalDocuments.value.length > 0 ||
  Boolean(icpFilingNumber.value) ||
  Boolean(publicSecurityFilingNumber.value)
))

function documentTitle(id: string, title: string): string {
  if (id === 'terms') return t('home.footer.terms')
  if (id === 'privacy') return t('home.footer.privacy')
  return title
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) void appStore.fetchPublicSettings()
})
</script>

<style scoped>
.public-site-footer {
  --footer-border: #3b4a3f;
  --footer-background: #060f16;
  --footer-text: #737a82;
  --footer-hover: #f4fff3;

  box-sizing: border-box;
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 28px 20px;
  border-top: 1px solid var(--footer-border);
  background: var(--footer-background);
  color: var(--footer-text);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  letter-spacing: 0;
}

.public-site-footer p {
  margin: 0;
  text-align: center;
}

.public-site-footer nav {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 12px 22px;
}

.public-site-footer a {
  max-width: 100%;
  color: inherit;
  text-decoration: none;
  overflow-wrap: anywhere;
  transition: color 160ms ease;
}

.public-site-footer a:hover,
.public-site-footer a:focus-visible {
  color: var(--footer-hover);
}

.public-site-footer a:focus-visible {
  outline: 2px solid #aec6ff;
  outline-offset: 3px;
}

.public-site-footer--docs {
  --footer-border: var(--docs-outline, #31423a);
  --footer-background: rgb(6 15 22 / 76%);
  --footer-text: rgb(185 203 188 / 68%);
  --footer-hover: var(--docs-primary, #72f5a5);

  min-height: 89px;
  padding: 24px 48px;
  font-size: 11px;
}

.public-site-footer--pricing {
  --footer-border: var(--pricing-border, #27272a);
  --footer-background: #000000;
  --footer-text: var(--pricing-faint, #71717a);
  --footer-hover: var(--pricing-accent, #34d399);
}

.public-site-footer--models {
  --footer-border: rgb(55 65 81);
  --footer-background: transparent;
  --footer-text: rgb(107 114 128);
  --footer-hover: rgb(110 231 183);

  max-width: 1440px;
  margin: 8px auto 0;
  padding: 24px 0 4px;
}

@media (min-width: 768px) {
  .public-site-footer {
    flex-direction: row;
    justify-content: space-between;
    padding: 30px 48px;
  }

  .public-site-footer p {
    text-align: left;
  }

  .public-site-footer--docs {
    padding-block: 24px;
  }

  .public-site-footer--models {
    padding: 24px 0 4px;
  }
}

@media (min-width: 1024px) {
  .public-site-footer:not(.public-site-footer--models) {
    padding-inline: 64px;
  }
}

@media (max-width: 767px) {
  .public-site-footer--docs {
    min-height: 116px;
    align-items: flex-start;
    justify-content: center;
    gap: 12px;
    padding: 24px 20px;
    line-height: 1.7;
  }

  .public-site-footer--docs p {
    text-align: left;
  }

  .public-site-footer--docs nav {
    justify-content: flex-start;
  }
}
</style>
