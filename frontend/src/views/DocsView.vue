<template>
  <div class="docs-page">
    <header class="docs-header">
      <nav class="docs-top-nav" :aria-label="t('docsPage.primaryNavigation')">
        <div class="docs-header-start">
          <router-link to="/home" class="docs-brand" :title="siteName">
            <img v-if="siteLogo" :src="siteLogo" alt="" class="docs-brand-logo" />
            <span>{{ siteName }}</span>
          </router-link>

          <div class="docs-top-links">
            <router-link to="/docs" class="docs-top-link docs-top-link-active">
              {{ t('docsPage.docs') }}
            </router-link>
            <router-link to="/pricing" class="docs-top-link">
              {{ t('docsPage.pricing') }}
            </router-link>
            <router-link to="/models" class="docs-top-link">
              {{ t('docsPage.models') }}
            </router-link>
          </div>
        </div>

        <div class="docs-header-actions">
          <div class="docs-desktop-locale">
            <LocaleSwitcher />
          </div>
          <button
            type="button"
            class="docs-icon-button"
            :aria-label="t('docsPage.search')"
            :title="t('docsPage.search')"
            @click="openSearch"
          >
            <Icon name="search" size="md" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="docs-login-button">
            {{ isAuthenticated ? t('docsPage.dashboard') : t('docsPage.login') }}
          </router-link>
          <button
            type="button"
            class="docs-icon-button docs-menu-button"
            :aria-label="sidebarOpen ? t('docsPage.closeNavigation') : t('docsPage.openNavigation')"
            :aria-expanded="sidebarOpen"
            aria-controls="docs-sidebar"
            @click="sidebarOpen = !sidebarOpen"
          >
            <Icon :name="sidebarOpen ? 'x' : 'menu'" size="md" />
          </button>
        </div>
      </nav>
    </header>

    <button
      v-if="sidebarOpen"
      type="button"
      class="docs-sidebar-backdrop"
      :aria-label="t('docsPage.closeNavigation')"
      @click="sidebarOpen = false"
    ></button>

    <aside id="docs-sidebar" class="docs-sidebar" :class="{ 'docs-sidebar-open': sidebarOpen }">
      <div class="docs-sidebar-content">
        <div class="docs-sidebar-group">
          <p class="docs-sidebar-label">{{ t('docsPage.navigation') }}</p>
          <button
            v-for="item in navigationItems"
            :key="item.id"
            type="button"
            class="docs-sidebar-link"
            :class="{ 'docs-sidebar-link-active': activeSection === item.id }"
            @click="selectSection(item.id)"
          >
            <Icon :name="item.icon" size="md" />
            <span>{{ item.label }}</span>
          </button>
        </div>

        <div class="docs-sidebar-group docs-sidebar-api-group">
          <p class="docs-sidebar-label">{{ t('docsPage.apiReference') }}</p>
          <button
            v-for="item in apiReferenceItems"
            :key="item.id"
            type="button"
            class="docs-sidebar-link"
            :class="{ 'docs-sidebar-link-active': activeSection === item.id }"
            @click="selectSection(item.id)"
          >
            <Icon :name="item.icon" size="md" />
            <span>{{ item.label }}</span>
          </button>
        </div>
      </div>

      <div class="docs-sidebar-footer">
        <div class="docs-sidebar-link docs-sidebar-static">
          <Icon name="chatBubble" size="md" />
          <span>{{ t('docsPage.contactSupport') }}</span>
        </div>
        <div class="docs-mobile-locale">
          <LocaleSwitcher />
        </div>
      </div>
    </aside>

    <main class="docs-main">
      <div class="docs-article">
        <header class="docs-article-header">
          <p class="docs-eyebrow">API / {{ activeItem.label }}</p>
          <h1>{{ activeArticle.title }}</h1>
          <p>{{ activeArticle.description }}</p>
        </header>

        <template v-if="activeSection === 'chat-completion'">
          <section class="docs-section" aria-labelledby="quick-start-heading">
            <h2 id="quick-start-heading">{{ t('docsPage.quickStart') }}</h2>
            <CodeBlock
              language="bash"
              :code="chatCompletionCode"
              :copy-label="copiedBlock === 'chat-completion' ? t('docsPage.copied') : t('docsPage.copy')"
              :copied="copiedBlock === 'chat-completion'"
              @copy="copyCode('chat-completion', chatCompletionCode)"
            />
          </section>

          <section class="docs-section" aria-labelledby="parameters-heading">
            <h2 id="parameters-heading">{{ t('docsPage.parameters') }}</h2>
            <div class="docs-table-wrap">
              <table class="docs-parameter-table">
                <thead>
                  <tr>
                    <th>{{ t('docsPage.parameter') }}</th>
                    <th>{{ t('docsPage.type') }}</th>
                    <th>{{ t('docsPage.description') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in parameterRows" :key="row.name">
                    <td>{{ row.name }}</td>
                    <td>{{ row.type }}</td>
                    <td>{{ row.description }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="docs-callout" aria-labelledby="rate-limits-heading">
            <Icon name="infoCircle" size="lg" />
            <div>
              <h2 id="rate-limits-heading">{{ t('docsPage.rateLimits') }}</h2>
              <p>{{ t('docsPage.rateLimitDescription') }}</p>
            </div>
          </section>
        </template>

        <template v-else>
          <section
            v-for="section in activeArticle.sections"
            :key="section.key"
            class="docs-section docs-guide-section"
            :aria-labelledby="`${activeSection}-${section.key}-heading`"
          >
            <h2 :id="`${activeSection}-${section.key}-heading`">{{ section.title }}</h2>
            <p class="docs-section-intro">{{ section.body }}</p>

            <div
              v-if="section.key === 'generation' && inlineImageExample"
              class="docs-inline-example"
            >
              <h3>{{ inlineImageExample.title }}</h3>
              <p>{{ inlineImageExample.description }}</p>
              <CodeBlock
                :language="inlineImageExample.language"
                :code="inlineImageExample.code"
                :copy-label="copiedBlock === `${activeSection}-${inlineImageExample.key}` ? t('docsPage.copied') : t('docsPage.copy')"
                :copied="copiedBlock === `${activeSection}-${inlineImageExample.key}`"
                @copy="copyCode(`${activeSection}-${inlineImageExample.key}`, inlineImageExample.code)"
              />
            </div>

            <div class="docs-detail-grid">
              <article v-for="(item, index) in section.items" :key="item.title" class="docs-detail-item">
                <span class="docs-detail-index">{{ String(index + 1).padStart(2, '0') }}</span>
                <h3>{{ item.title }}</h3>
                <p>{{ item.description }}</p>
              </article>
            </div>
          </section>

          <section
            v-for="example in standaloneExamples"
            :key="example.key"
            class="docs-section docs-example-section"
            :aria-labelledby="`${activeSection}-${example.key}-example-heading`"
          >
            <h2 :id="`${activeSection}-${example.key}-example-heading`">{{ example.title }}</h2>
            <p class="docs-section-intro">{{ example.description }}</p>
            <CodeBlock
              :language="example.language"
              :code="example.code"
              :copy-label="copiedBlock === `${activeSection}-${example.key}` ? t('docsPage.copied') : t('docsPage.copy')"
              :copied="copiedBlock === `${activeSection}-${example.key}`"
              @copy="copyCode(`${activeSection}-${example.key}`, example.code)"
            />
          </section>

          <section class="docs-callout" :aria-labelledby="`${activeSection}-callout-heading`">
            <Icon name="infoCircle" size="lg" />
            <div>
              <h2 :id="`${activeSection}-callout-heading`">{{ activeArticle.callout.title }}</h2>
              <p>{{ activeArticle.callout.body }}</p>
            </div>
          </section>
        </template>
      </div>

      <footer class="docs-footer">
        <p>&copy; {{ currentYear }} {{ siteName }} - {{ t('docsPage.footerDescription') }}</p>
      </footer>
    </main>

    <div v-if="searchOpen" class="docs-search-overlay" role="presentation" @click.self="closeSearch">
      <section
        class="docs-search-dialog"
        role="dialog"
        aria-modal="true"
        :aria-label="t('docsPage.search')"
      >
        <div class="docs-search-input-row">
          <Icon name="search" size="md" />
          <input
            ref="searchInput"
            v-model="searchQuery"
            type="search"
            :placeholder="t('docsPage.searchPlaceholder')"
          />
          <button
            type="button"
            class="docs-icon-button"
            :aria-label="t('docsPage.closeSearch')"
            @click="closeSearch"
          >
            <Icon name="x" size="sm" />
          </button>
        </div>
        <p class="docs-search-label">{{ t('docsPage.searchResults') }}</p>
        <div class="docs-search-results">
          <button
            v-for="item in searchResults"
            :key="item.id"
            type="button"
            @click="selectSearchResult(item.id)"
          >
            <span class="docs-search-result-icon"><Icon :name="item.icon" size="sm" /></span>
            <span>
              <strong>{{ item.label }}</strong>
              <small>{{ item.description }}</small>
            </span>
            <Icon name="arrowRight" size="sm" />
          </button>
          <p v-if="searchResults.length === 0" class="docs-search-empty">
            {{ t('docsPage.noResults') }}
          </p>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import CodeBlock from '@/components/docs/CodeBlock.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

type SectionId =
  | 'overview'
  | 'models'
  | 'api-keys'
  | 'usage'
  | 'settings'
  | 'authentication'
  | 'chat-completion'
  | 'image-generation'
  | 'model-list'

type DocsIcon = 'grid' | 'cpu' | 'key' | 'chartBar' | 'cog' | 'lock' | 'chatBubble' | 'sparkles' | 'document'

interface NavigationItem {
  id: SectionId
  label: string
  description: string
  icon: DocsIcon
}

type GuideSectionId = Exclude<SectionId, 'chat-completion'>

interface ArticleDetailItem {
  title: string
  description: string
}

interface ArticleSection {
  key: string
  title: string
  body: string
  items: ArticleDetailItem[]
}

interface ArticleExample {
  key: string
  title: string
  description: string
  language: string
  code: string
}

interface CodeExampleDefinition {
  key: string
  language: string
  code: string
}

interface ArticleContent {
  title: string
  description: string
  sections: ArticleSection[]
  examples: ArticleExample[]
  callout: {
    title: string
    body: string
  }
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const validSections: SectionId[] = [
  'overview',
  'models',
  'api-keys',
  'usage',
  'settings',
  'authentication',
  'chat-completion',
  'image-generation',
  'model-list'
]

function sectionFromHash(hash: string): SectionId {
  const id = hash.replace(/^#/, '') as SectionId
  return validSections.includes(id) ? id : 'chat-completion'
}

const activeSection = ref<SectionId>(sectionFromHash(route.hash))
const sidebarOpen = ref(false)
const searchOpen = ref(false)
const searchQuery = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const copiedBlock = ref<string | null>(null)
let copyFeedbackTimer: number | undefined

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '',
  { allowRelative: true, allowDataUrl: true }
))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())
const apiBaseUrl = computed(() => (
  appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || window.location.origin
).replace(/\/+$/, ''))

const navigationItems = computed<NavigationItem[]>(() => [
  createNavigationItem('overview', 'grid', 'overview'),
  createNavigationItem('models', 'cpu', 'models'),
  createNavigationItem('api-keys', 'key', 'apiKeys'),
  createNavigationItem('usage', 'chartBar', 'usage'),
  createNavigationItem('settings', 'cog', 'settings')
])

const apiReferenceItems = computed<NavigationItem[]>(() => [
  createNavigationItem('authentication', 'lock', 'authentication'),
  createNavigationItem('chat-completion', 'chatBubble', 'chatCompletion'),
  createNavigationItem('image-generation', 'sparkles', 'imageGeneration'),
  createNavigationItem('model-list', 'document', 'modelList')
])

const allNavigationItems = computed(() => [...navigationItems.value, ...apiReferenceItems.value])
const activeItem = computed<NavigationItem>(() => (
  allNavigationItems.value.find((item) => item.id === activeSection.value)
  || createNavigationItem('chat-completion', 'chatBubble', 'chatCompletion')
))

const articleKeys: Record<SectionId, string> = {
  overview: 'overview',
  models: 'models',
  'api-keys': 'apiKeys',
  usage: 'usage',
  settings: 'settings',
  authentication: 'authentication',
  'chat-completion': 'chatCompletion',
  'image-generation': 'imageGeneration',
  'model-list': 'modelList'
}

const articleLayouts: Record<GuideSectionId, string[]> = {
  overview: ['start', 'endpoints'],
  models: ['selection', 'routing'],
  'api-keys': ['lifecycle', 'controls'],
  usage: ['query', 'response'],
  settings: ['connection', 'production'],
  authentication: ['headers', 'errors'],
  'image-generation': ['generation', 'editing'],
  'model-list': ['catalog', 'refresh']
}

const codeExamples: Record<GuideSectionId, CodeExampleDefinition[]> = {
  overview: [{
    key: 'primary',
    language: 'typescript',
    code: `import OpenAI from 'openai'\n\nconst client = new OpenAI({\n  apiKey: process.env.SUB2API_KEY,\n  baseURL: 'https://api.your-code.cc/v1'\n})\n\nconst models = await client.models.list()\nconsole.log(models.data.map((model) => model.id))`
  }],
  models: [{
    key: 'primary',
    language: 'json',
    code: `{\n  "model": "MODEL_ID_FROM_V1_MODELS",\n  "messages": [\n    { "role": "user", "content": "Explain this repository." }\n  ],\n  "stream": true\n}`
  }],
  'api-keys': [{
    key: 'primary',
    language: 'env',
    code: `SUB2API_KEY=YOUR_API_KEY\nSUB2API_BASE_URL=https://api.your-code.cc/v1`
  }],
  usage: [{
    key: 'primary',
    language: 'bash',
    code: `curl https://api.your-code.cc/api/v1/usage \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }],
  settings: [{
    key: 'primary',
    language: 'typescript',
    code: `const client = new OpenAI({\n  apiKey: process.env.GATEWAY_API_KEY,\n  baseURL: 'https://api.your-code.cc/v1',\n  timeout: 60_000,\n  maxRetries: 2\n})`
  }],
  authentication: [{
    key: 'primary',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/models \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json"`
  }],
  'image-generation': [{
    key: 'primary',
    language: 'bash',
    code: `curl -X POST https://api.your-code.cc/v1/images/generations \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "IMAGE_MODEL_ID_FROM_V1_MODELS",\n    "prompt": "A precise isometric diagram of an AI gateway",\n    "size": "1024x1024",\n    "quality": "high",\n    "n": 1,\n    "response_format": "b64_json"\n  }'`
  }, {
    key: 'edit',
    language: 'bash',
    code: `curl -X POST https://api.your-code.cc/v1/images/edits \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -F "model=IMAGE_MODEL_ID_FROM_V1_MODELS" \\\n  -F "image=@reference.png" \\\n  -F "prompt=Replace the background with a clean studio backdrop" \\\n  -F "size=1024x1024" \\\n  -F "response_format=b64_json"`
  }, {
    key: 'response',
    language: 'json',
    code: `{\n  "created": 1710000000,\n  "data": [\n    {\n      "b64_json": "iVBORw0KGgoAAA...",\n      "revised_prompt": "A precise isometric diagram of an AI gateway"\n    }\n  ]\n}`
  }],
  'model-list': [{
    key: 'primary',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/models \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }]
}

const chatCompletionCode = `curl -X POST https://api.your-code.cc/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "high-perf-v2",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Analyze this dataset."}
    ],
    "temperature": 0.2
  }'`

const activeArticle = computed<ArticleContent>(() => {
  const articleKey = articleKeys[activeSection.value]
  if (activeSection.value === 'chat-completion') {
    return {
      title: t(`docsPage.articles.${articleKey}.title`),
      description: t(`docsPage.articles.${articleKey}.description`),
      sections: [],
      examples: [],
      callout: { title: '', body: '' }
    }
  }

  const guideId = activeSection.value as GuideSectionId
  return {
    title: t(`docsPage.articles.${articleKey}.title`),
    description: t(`docsPage.articles.${articleKey}.description`),
    sections: articleLayouts[guideId].map((sectionKey) => ({
      key: sectionKey,
      title: t(`docsPage.articles.${articleKey}.sections.${sectionKey}.title`),
      body: t(`docsPage.articles.${articleKey}.sections.${sectionKey}.body`),
      items: ['first', 'second', 'third'].map((itemKey) => ({
        title: t(`docsPage.articles.${articleKey}.sections.${sectionKey}.items.${itemKey}.title`),
        description: t(`docsPage.articles.${articleKey}.sections.${sectionKey}.items.${itemKey}.description`)
      }))
    })),
    examples: codeExamples[guideId].map((example) => ({
      key: example.key,
      title: t(`docsPage.articles.${articleKey}.examples.${example.key}.title`),
      description: t(`docsPage.articles.${articleKey}.examples.${example.key}.description`),
      language: example.language,
      code: example.code
        .split('https://api.your-code.cc').join(apiBaseUrl.value)
        .replace('/api/v1/usage', '/v1/usage?days=30&timezone=Asia%2FShanghai')
    })),
    callout: {
      title: t(`docsPage.articles.${articleKey}.callout.title`),
      body: t(`docsPage.articles.${articleKey}.callout.body`)
    }
  }
})

const inlineImageExample = computed<ArticleExample | null>(() => {
  if (activeSection.value !== 'image-generation') return null
  return activeArticle.value.examples.find((example) => example.key === 'primary') || null
})

const standaloneExamples = computed<ArticleExample[]>(() => {
  if (!inlineImageExample.value) return activeArticle.value.examples
  return activeArticle.value.examples.filter((example) => example.key !== inlineImageExample.value?.key)
})

const parameterRows = computed(() => [
  { name: 'model', type: 'string', description: t('docsPage.parameterRows.model') },
  { name: 'messages', type: 'array', description: t('docsPage.parameterRows.messages') },
  { name: 'temperature', type: 'number', description: t('docsPage.parameterRows.temperature') },
  { name: 'max_tokens', type: 'integer', description: t('docsPage.parameterRows.maxTokens') }
])

const searchResults = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  if (!query) return allNavigationItems.value
  return allNavigationItems.value.filter((item) => (
    `${item.label} ${item.description}`.toLocaleLowerCase().includes(query)
  ))
})

function createNavigationItem(id: SectionId, icon: DocsIcon, translationKey: string): NavigationItem {
  const articleKey = articleKeys[id]
  return {
    id,
    icon,
    label: t(`docsPage.nav.${translationKey}`),
    description: t(`docsPage.articles.${articleKey}.description`)
  }
}

async function selectSection(id: SectionId): Promise<void> {
  activeSection.value = id
  sidebarOpen.value = false
  await router.replace({ name: 'Docs', hash: `#${id}` })
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

async function selectSearchResult(id: SectionId): Promise<void> {
  closeSearch()
  await selectSection(id)
}

async function openSearch(): Promise<void> {
  searchOpen.value = true
  await nextTick()
  searchInput.value?.focus()
}

function closeSearch(): void {
  searchOpen.value = false
  searchQuery.value = ''
}

async function copyCode(id: string, code: string): Promise<void> {
  let copied = false
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(code)
      copied = true
    }
  } catch {
    copied = copyCodeWithTextarea(code)
  }

  if (!navigator.clipboard?.writeText) {
    copied = copyCodeWithTextarea(code)
  }

  if (!copied) {
    copiedBlock.value = null
    return
  }

  copiedBlock.value = id
  if (copyFeedbackTimer) window.clearTimeout(copyFeedbackTimer)
  copyFeedbackTimer = window.setTimeout(() => {
    copiedBlock.value = null
  }, 1800)
}

function copyCodeWithTextarea(code: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = code
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    textarea.remove()
  }
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    if (searchOpen.value) closeSearch()
    sidebarOpen.value = false
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLocaleLowerCase() === 'k') {
    event.preventDefault()
    openSearch()
  }
}

watch(() => route.hash, (hash) => {
  activeSection.value = sectionFromHash(hash)
})

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown)
  if (copyFeedbackTimer) window.clearTimeout(copyFeedbackTimer)
})
</script>

<style scoped>
.docs-page {
  --docs-bg: #0b141c;
  --docs-surface-lowest: #060f16;
  --docs-surface-low: #141c24;
  --docs-surface: #182028;
  --docs-surface-high: #222b33;
  --docs-surface-highest: #2d363e;
  --docs-text: #dae3ee;
  --docs-muted: #b9cbbc;
  --docs-outline: #3b4a3f;
  --docs-primary: #00ff9d;
  --docs-primary-dim: #00e38b;
  --docs-on-primary: #00391f;
  --docs-secondary: #aec6ff;
  min-height: 100vh;
  background-color: var(--docs-bg);
  background-image:
    linear-gradient(to right, rgb(48 54 61 / 20%) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(48 54 61 / 20%) 1px, transparent 1px);
  background-size: 32px 32px;
  color: var(--docs-text);
  font-family: 'Geist', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}

.docs-page,
.docs-page * {
  box-sizing: border-box;
}

.docs-header {
  position: fixed;
  inset: 0 0 auto;
  z-index: 50;
  height: 64px;
  border-bottom: 1px solid var(--docs-outline);
  background: rgb(6 15 22 / 92%);
  backdrop-filter: blur(12px);
}

.docs-top-nav {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 0 20px;
}

.docs-header-start,
.docs-header-actions,
.docs-top-links {
  display: flex;
  align-items: center;
}

.docs-header-start {
  min-width: 0;
  gap: 32px;
}

.docs-header-actions {
  flex: 0 0 auto;
  gap: 8px;
}

.docs-brand {
  display: inline-flex;
  min-width: 0;
  max-width: 42vw;
  align-items: center;
  gap: 10px;
  overflow: hidden;
  color: var(--docs-primary-dim);
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  text-decoration: none;
  white-space: nowrap;
}

.docs-brand span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.docs-brand-logo {
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 4px;
  object-fit: contain;
}

.docs-top-links {
  align-self: stretch;
  gap: 24px;
}

.docs-top-link {
  position: relative;
  display: inline-flex;
  height: 100%;
  align-items: center;
  color: var(--docs-muted);
  font-size: 15px;
  text-decoration: none;
  transition: color 160ms ease;
}

.docs-top-link:hover,
.docs-top-link:focus-visible {
  color: var(--docs-text);
}

.docs-top-link-active {
  color: var(--docs-primary);
}

.docs-top-link-active::after {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 2px;
  background: var(--docs-primary);
  content: '';
}

.docs-icon-button {
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 4px;
  color: var(--docs-muted);
  background: transparent;
  cursor: pointer;
  transition: border-color 160ms ease, color 160ms ease, background 160ms ease;
}

.docs-icon-button:hover,
.docs-icon-button:focus-visible {
  border-color: var(--docs-outline);
  color: var(--docs-primary);
  background: var(--docs-surface-low);
  outline: none;
}

.docs-menu-button {
  display: none;
  border-color: var(--docs-outline);
}

.docs-login-button {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--docs-outline);
  border-radius: 4px;
  padding: 7px 14px;
  color: var(--docs-primary-dim);
  background: transparent;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  text-decoration: none;
  transition: border-color 160ms ease, background-color 160ms ease;
}

.docs-login-button:hover,
.docs-login-button:focus-visible {
  border-color: var(--docs-primary-dim);
  background: rgb(0 227 139 / 6%);
  outline: none;
}

.docs-sidebar {
  position: fixed;
  z-index: 40;
  top: 64px;
  bottom: 0;
  left: 0;
  display: flex;
  width: 286px;
  flex-direction: column;
  border-right: 1px solid var(--docs-outline);
  background: var(--docs-surface-low);
}

.docs-sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 18px;
}

.docs-sidebar-group + .docs-sidebar-group {
  margin-top: 32px;
}

.docs-sidebar-label,
.docs-search-label,
.docs-eyebrow {
  margin: 0 0 12px;
  color: var(--docs-muted);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
  letter-spacing: 0;
  text-transform: uppercase;
}

.docs-sidebar-label {
  padding: 0 10px;
}

.docs-sidebar-link {
  display: flex;
  width: 100%;
  min-height: 42px;
  align-items: center;
  gap: 12px;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 9px 12px;
  color: var(--docs-muted);
  background: transparent;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.4;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
  transition: color 150ms ease, background 150ms ease, border-color 150ms ease;
}

.docs-sidebar-link + .docs-sidebar-link {
  margin-top: 5px;
}

.docs-sidebar-link:hover,
.docs-sidebar-link:focus-visible {
  color: var(--docs-text);
  background: var(--docs-surface-high);
  outline: none;
}

.docs-sidebar-link-active {
  border-left-color: var(--docs-primary);
  color: #f4fff3;
  background: var(--docs-surface-high);
  font-weight: 700;
  box-shadow: 0 0 15px rgb(0 255 157 / 8%);
}

.docs-sidebar-static {
  cursor: default;
}

.docs-sidebar-static:hover {
  color: var(--docs-muted);
  background: transparent;
}

.docs-sidebar-footer {
  border-top: 1px solid var(--docs-outline);
  padding: 18px;
}

.docs-mobile-locale {
  display: none;
}

.docs-main {
  min-height: 100vh;
  margin-left: 286px;
  padding-top: 64px;
}

.docs-article {
  width: min(100%, 1100px);
  min-height: calc(100vh - 153px);
  margin: 0 auto;
  padding: 70px 48px 72px;
}

.docs-article-header {
  max-width: 800px;
  margin-bottom: 56px;
}

.docs-eyebrow {
  color: var(--docs-primary-dim);
}

.docs-article-header h1 {
  margin: 0 0 18px;
  color: #f4fff3;
  font-size: 48px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: 0;
}

.docs-article-header > p:last-child {
  max-width: 760px;
  margin: 0;
  color: var(--docs-muted);
  font-size: 17px;
  line-height: 1.65;
}

.docs-section {
  margin-bottom: 68px;
}

.docs-section > h2 {
  margin: 0 0 28px;
  border-bottom: 1px solid var(--docs-outline);
  padding-bottom: 12px;
  color: var(--docs-text);
  font-size: 26px;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: 0;
}

.docs-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--docs-outline);
  border-radius: 6px;
  background: var(--docs-surface-low);
}

.docs-parameter-table {
  width: 100%;
  min-width: 720px;
  border-collapse: collapse;
  text-align: left;
}

.docs-parameter-table th {
  padding: 16px 18px;
  color: var(--docs-muted);
  background: var(--docs-surface-highest);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.docs-parameter-table td {
  border-top: 1px solid var(--docs-outline);
  padding: 17px 18px;
  color: var(--docs-muted);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  vertical-align: top;
}

.docs-parameter-table td:first-child {
  color: #f4fff3;
  font-weight: 700;
}

.docs-parameter-table td:nth-child(2) {
  color: var(--docs-secondary);
}

.docs-parameter-table tbody tr:hover {
  background: var(--docs-surface-lowest);
}

.docs-callout {
  display: flex;
  align-items: flex-start;
  gap: 18px;
  border: 1px solid var(--docs-outline);
  border-radius: 6px;
  padding: 24px;
  background: var(--docs-surface-lowest);
  transition: border-color 160ms ease;
}

.docs-callout:hover {
  border-color: var(--docs-primary-dim);
}

.docs-callout > svg {
  flex: 0 0 auto;
  color: var(--docs-primary);
}

.docs-callout h2 {
  margin: 0 0 8px;
  color: #f4fff3;
  font-size: 18px;
  font-weight: 600;
}

.docs-callout p,
.docs-section-intro,
.docs-detail-item p {
  margin: 0;
  color: var(--docs-muted);
  font-size: 15px;
  line-height: 1.7;
}

.docs-section-intro {
  max-width: 800px;
  margin-top: -6px;
}

.docs-detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 24px;
  margin-top: 30px;
}

.docs-inline-example {
  margin-top: 30px;
}

.docs-inline-example > h3 {
  margin: 0 0 8px;
  color: var(--docs-text);
  font-size: 18px;
  font-weight: 600;
  line-height: 1.4;
}

.docs-inline-example > p {
  max-width: 800px;
  margin: 0 0 18px;
  color: var(--docs-muted);
  font-size: 15px;
  line-height: 1.7;
}

.docs-detail-item {
  min-width: 0;
  border-top: 2px solid var(--docs-outline);
  padding-top: 18px;
}

.docs-detail-index {
  display: block;
  margin-bottom: 16px;
  color: var(--docs-primary);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
}

.docs-detail-item h3 {
  margin: 0 0 9px;
  color: var(--docs-text);
  font-size: 16px;
  font-weight: 600;
  line-height: 1.4;
}

.docs-example-section + .docs-example-section {
  margin-top: -20px;
}

.docs-footer {
  display: flex;
  min-height: 89px;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  border-top: 1px solid var(--docs-outline);
  padding: 24px 48px;
  color: rgb(185 203 188 / 68%);
  background: rgb(6 15 22 / 76%);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
}

.docs-footer p {
  margin: 0;
}

.docs-footer a {
  color: inherit;
  text-decoration: none;
}

.docs-footer a:hover,
.docs-footer a:focus-visible {
  color: var(--docs-primary);
}

.docs-search-overlay {
  position: fixed;
  z-index: 80;
  inset: 0;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 96px 20px 20px;
  background: rgb(0 7 11 / 74%);
  backdrop-filter: blur(5px);
}

.docs-search-dialog {
  width: min(100%, 620px);
  overflow: hidden;
  border: 1px solid var(--docs-outline);
  border-radius: 8px;
  background: rgb(11 20 28 / 98%);
  box-shadow: 0 24px 70px rgb(0 0 0 / 48%);
}

.docs-search-input-row {
  display: flex;
  height: 58px;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid var(--docs-outline);
  padding: 0 12px 0 18px;
  color: var(--docs-muted);
}

.docs-search-input-row input {
  width: 100%;
  min-width: 0;
  border: 0;
  padding: 0;
  color: var(--docs-text);
  background: transparent;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 14px;
  outline: none;
}

.docs-search-input-row input::placeholder {
  color: rgb(185 203 188 / 55%);
}

.docs-search-label {
  margin: 0;
  padding: 16px 18px 8px;
}

.docs-search-results {
  max-height: min(480px, calc(100vh - 220px));
  overflow-y: auto;
  padding: 0 10px 12px;
}

.docs-search-results > button {
  display: grid;
  width: 100%;
  grid-template-columns: 34px minmax(0, 1fr) 20px;
  align-items: center;
  gap: 12px;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 12px;
  color: var(--docs-muted);
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.docs-search-results > button:hover,
.docs-search-results > button:focus-visible {
  border-color: var(--docs-outline);
  color: var(--docs-primary);
  background: var(--docs-surface-high);
  outline: none;
}

.docs-search-result-icon {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--docs-outline);
  border-radius: 4px;
}

.docs-search-results strong,
.docs-search-results small {
  display: block;
}

.docs-search-results strong {
  color: var(--docs-text);
  font-size: 14px;
}

.docs-search-results small {
  margin-top: 3px;
  overflow: hidden;
  color: var(--docs-muted);
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.docs-search-empty {
  margin: 0;
  padding: 34px 20px 42px;
  color: var(--docs-muted);
  font-size: 14px;
  text-align: center;
}

.docs-sidebar-backdrop {
  display: none;
}

.docs-page :deep(.docs-desktop-locale button),
.docs-page :deep(.docs-mobile-locale button) {
  color: var(--docs-muted);
}

.docs-page :deep(.docs-desktop-locale > div > button) {
  min-width: 42px;
  min-height: 36px;
  justify-content: center;
  border-radius: 4px;
}

.docs-page :deep(.docs-desktop-locale button:hover),
.docs-page :deep(.docs-mobile-locale button:hover) {
  color: var(--docs-text);
  background: var(--docs-surface-high);
}

@media (min-width: 768px) {
  .docs-top-nav {
    padding-inline: 48px;
  }
}

@media (min-width: 1024px) {
  .docs-top-nav {
    padding-inline: 64px;
  }
}

@media (max-width: 900px) {
  .docs-top-nav {
    padding: 0 20px;
  }

  .docs-header-start {
    gap: 12px;
  }

  .docs-menu-button {
    display: inline-flex;
  }

  .docs-top-links,
  .docs-desktop-locale {
    display: none;
  }

  .docs-sidebar {
    z-index: 60;
    width: min(300px, 88vw);
    transform: translateX(-100%);
    transition: transform 180ms ease;
  }

  .docs-sidebar-open {
    transform: translateX(0);
  }

  .docs-sidebar-backdrop {
    position: fixed;
    z-index: 55;
    inset: 64px 0 0;
    display: block;
    width: 100%;
    border: 0;
    background: rgb(0 7 11 / 66%);
  }

  .docs-mobile-locale {
    display: block;
    margin-top: 8px;
  }

  .docs-main {
    margin-left: 0;
  }

  .docs-article {
    padding: 56px 32px 64px;
  }

  .docs-detail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 600px) {
  .docs-top-nav {
    gap: 10px;
    padding: 0 20px;
  }

  .docs-header-start,
  .docs-header-actions {
    gap: 8px;
  }

  .docs-brand {
    max-width: 42vw;
    font-size: 18px;
  }

  .docs-brand-logo {
    width: 30px;
    height: 30px;
    flex-basis: auto;
  }

  .docs-login-button {
    min-height: 36px;
    padding: 0 12px;
    font-size: 13px;
  }

  .docs-article {
    min-height: calc(100vh - 180px);
    padding: 42px 20px 54px;
  }

  .docs-article-header {
    margin-bottom: 44px;
  }

  .docs-article-header h1 {
    margin-bottom: 14px;
    font-size: 32px;
    line-height: 1.2;
  }

  .docs-article-header > p:last-child {
    font-size: 15px;
    line-height: 1.65;
  }

  .docs-section {
    margin-bottom: 52px;
  }

  .docs-section > h2 {
    margin-bottom: 20px;
    font-size: 22px;
  }

  .docs-callout {
    gap: 14px;
    padding: 18px;
  }

  .docs-detail-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 22px;
  }

  .docs-example-section + .docs-example-section {
    margin-top: -10px;
  }

  .docs-footer {
    min-height: 116px;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: 12px;
    padding: 24px 20px;
    line-height: 1.7;
  }

  .docs-search-overlay {
    padding: 76px 10px 10px;
  }

  .docs-search-dialog {
    border-radius: 6px;
  }
}

@media (max-width: 390px) {
  .docs-brand {
    max-width: 94px;
  }

  .docs-header-actions {
    margin-left: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .docs-page *,
  .docs-page *::before,
  .docs-page *::after {
    scroll-behavior: auto !important;
    transition-duration: 1ms !important;
  }
}
</style>
