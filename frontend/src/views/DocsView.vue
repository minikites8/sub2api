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
      <div class="docs-content-layout" :class="{ 'docs-content-layout-with-models': isVideoSection }">
        <aside
          v-if="isVideoSection"
          class="docs-model-nav"
          :aria-label="t('docsPage.videoModels')"
        >
          <p class="docs-model-nav-label">{{ t('docsPage.videoModels') }}</p>
          <div class="docs-model-tabs" role="tablist">
            <button
              v-for="model in videoDocModels"
              :key="model.id"
              :id="`video-doc-tab-${model.id}`"
              type="button"
              role="tab"
              class="docs-model-tab"
              :class="{ 'docs-model-tab-active': activeVideoModel === model.id }"
              :aria-selected="activeVideoModel === model.id"
              :tabindex="activeVideoModel === model.id ? 0 : -1"
              @click="selectVideoModel(model.id)"
              @keydown="handleVideoModelKeydown($event, model.id)"
            >
              {{ model.label }}
            </button>
          </div>
        </aside>

        <div class="docs-content-column">
          <div
            class="docs-article"
            :role="isVideoSection ? 'tabpanel' : undefined"
            :aria-labelledby="isVideoSection ? `video-doc-tab-${activeVideoModel}` : undefined"
          >
            <header class="docs-article-header">
              <p class="docs-eyebrow">API / {{ activeItem.label }}</p>
              <h1>{{ activeArticle.title }}</h1>
              <p>{{ activeArticle.description }}</p>
            </header>

            <div class="docs-quick-start">
              <section
                v-for="step in activeArticle.steps"
                :key="step.key"
                class="docs-section docs-quick-start-section"
                :aria-labelledby="`${activeSection}-${step.key}-heading`"
              >
                <h2 :id="`${activeSection}-${step.key}-heading`">{{ step.title }}</h2>
                <p>{{ step.description }}</p>
                <router-link v-if="step.action" :to="step.action.to" class="docs-quick-start-action">
                  <span>{{ step.action.label }}</span>
                  <Icon name="arrowRight" size="sm" />
                </router-link>
                <CodeBlock
                  v-if="step.example"
                  :language="step.example.language"
                  :code="step.example.code"
                  :copy-label="copiedBlock === `${activeSection}-${step.key}` ? t('docsPage.copied') : t('docsPage.copy')"
                  :copied="copiedBlock === `${activeSection}-${step.key}`"
                  @copy="copyCode(`${activeSection}-${step.key}`, step.example.code)"
                />
              </section>
            </div>
          </div>

          <PublicSiteFooter :description="t('docsPage.footerDescription')" theme="docs" />
        </div>
      </div>
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
import PublicSiteFooter from '@/components/public/PublicSiteFooter.vue'
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
  | 'video-generation'
  | 'model-list'

type VideoDocModelId = 'happyhorse' | 'seedance' | 'veo'

type DocsIcon = 'grid' | 'cpu' | 'key' | 'chartBar' | 'cog' | 'lock' | 'chatBubble' | 'sparkles' | 'video' | 'document'

interface NavigationItem {
  id: SectionId
  label: string
  description: string
  icon: DocsIcon
}

interface ArticleExample {
  language: string
  code: string
}

interface ArticleStep {
  key: string
  title: string
  description: string
  action: { to: string; label: string } | null
  example: ArticleExample | null
}

interface ArticleStepDefinition {
  key: string
  actionTo?: string
}

interface CodeExampleDefinition {
  stepKey: string
  language: string
  code: string
}

interface ArticleContent {
  title: string
  description: string
  steps: ArticleStep[]
}

interface VideoDocModelDefinition {
  id: VideoDocModelId
  label: string
  steps: ArticleStepDefinition[]
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
  'video-generation',
  'model-list'
]

function sectionFromHash(hash: string): SectionId {
  const id = hash.replace(/^#/, '') as SectionId
  return validSections.includes(id) ? id : 'overview'
}

const activeSection = ref<SectionId>(sectionFromHash(route.hash))
const sidebarOpen = ref(false)
const searchOpen = ref(false)
const searchQuery = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const copiedBlock = ref<string | null>(null)
const activeVideoModel = ref<VideoDocModelId>('happyhorse')
let copyFeedbackTimer: number | undefined

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '',
  { allowRelative: true, allowDataUrl: true }
))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
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
  createNavigationItem('video-generation', 'video', 'videoGeneration'),
  createNavigationItem('model-list', 'document', 'modelList')
])

const allNavigationItems = computed(() => [...navigationItems.value, ...apiReferenceItems.value])
const activeItem = computed<NavigationItem>(() => (
  allNavigationItems.value.find((item) => item.id === activeSection.value)
  || createNavigationItem('overview', 'grid', 'overview')
))
const isVideoSection = computed(() => activeSection.value === 'video-generation')

const articleKeys: Record<SectionId, string> = {
  overview: 'overview',
  models: 'models',
  'api-keys': 'apiKeys',
  usage: 'usage',
  settings: 'settings',
  authentication: 'authentication',
  'chat-completion': 'chatCompletion',
  'image-generation': 'imageGeneration',
  'video-generation': 'videoGeneration',
  'model-list': 'modelList'
}

const articleStepLayouts: Record<SectionId, ArticleStepDefinition[]> = {
  overview: [
    { key: 'apiKey', actionTo: '/keys' },
    { key: 'model', actionTo: '/models' },
    { key: 'request' }
  ],
  models: [
    { key: 'browse', actionTo: '/models' },
    { key: 'copy' },
    { key: 'use' }
  ],
  'api-keys': [
    { key: 'create', actionTo: '/keys' },
    { key: 'save' },
    { key: 'use' }
  ],
  usage: [
    { key: 'open', actionTo: '/usage' },
    { key: 'range' },
    { key: 'quota' }
  ],
  settings: [
    { key: 'install' },
    { key: 'request' }
  ],
  authentication: [
    { key: 'header' },
    { key: 'unauthorized' },
    { key: 'limited' }
  ],
  'chat-completion': [
    { key: 'model', actionTo: '/models' },
    { key: 'request' },
    { key: 'response' }
  ],
  'image-generation': [
    { key: 'model', actionTo: '/models' },
    { key: 'request' },
    { key: 'result' }
  ],
  'video-generation': [
    { key: 'model', actionTo: '/models' }
  ],
  'model-list': [
    { key: 'request' },
    { key: 'read' },
    { key: 'use' }
  ]
}

const videoDocModels: readonly VideoDocModelDefinition[] = [
  {
    id: 'happyhorse',
    label: 'HappyHorse 1.1',
    steps: [
      { key: 'model', actionTo: '/models' },
      { key: 'textToVideo' },
      { key: 'firstFrameToVideo' },
      { key: 'referenceToVideo' },
      { key: 'videoEdit' },
      { key: 'result' }
    ]
  },
  {
    id: 'seedance',
    label: 'Seedance 2.0',
    steps: [
      { key: 'model', actionTo: '/models' },
      { key: 'seedanceTextToVideo' },
      { key: 'seedanceImageToVideo' },
      { key: 'seedanceReferenceVideo' },
      { key: 'result' }
    ]
  },
  {
    id: 'veo',
    label: 'Veo 3.1',
    steps: [
      { key: 'model', actionTo: '/models' },
      { key: 'veoTextToVideo' },
      { key: 'veoHeadTailToVideo' },
      { key: 'veoReferenceToVideo' },
      { key: 'result' }
    ]
  }
]

const activeVideoDocModel = computed(() => (
  videoDocModels.find((model) => model.id === activeVideoModel.value) || videoDocModels[0]
))

const chatRequestCode = `curl https://api.your-code.cc/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_MODEL",
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'`

const articleExamples: Partial<Record<SectionId, CodeExampleDefinition[]>> = {
  overview: [{
    stepKey: 'request',
    language: 'bash',
    code: chatRequestCode
  }],
  models: [{
    stepKey: 'use',
    language: 'json',
    code: `{\n  "model": "YOUR_MODEL",\n  "messages": [\n    {"role": "user", "content": "Hello"}\n  ]\n}`
  }],
  'api-keys': [{
    stepKey: 'use',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/models \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }],
  settings: [{
    stepKey: 'request',
    language: 'typescript',
    code: `import OpenAI from 'openai'\n\nconst client = new OpenAI({\n  apiKey: 'YOUR_API_KEY',\n  baseURL: 'https://api.your-code.cc/v1'\n})\n\nconst response = await client.chat.completions.create({\n  model: 'YOUR_MODEL',\n  messages: [{ role: 'user', content: 'Hello' }]\n})\n\nconsole.log(response.choices[0].message.content)`
  }],
  authentication: [{
    stepKey: 'header',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/models \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }],
  'chat-completion': [{
    stepKey: 'request',
    language: 'bash',
    code: chatRequestCode
  }],
  'image-generation': [{
    stepKey: 'request',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/images/generations \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "YOUR_IMAGE_MODEL",\n    "prompt": "A quiet mountain lake at sunrise"\n  }'`
  }],
  'video-generation': [{
    stepKey: 'textToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "happyhorse-1.1-t2v",\n    "prompt": "A white horse running along the coast at sunset",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 5\n  }'`
  }, {
    stepKey: 'firstFrameToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "happyhorse-1.1-i2v",\n    "prompt": "Slowly move the camera forward while the subject blinks naturally",\n    "first_frame": "https://example.com/first-frame.jpg",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 5\n  }'`
  }, {
    stepKey: 'referenceToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "happyhorse-1.1-r2v",\n    "prompt": "Keep the referenced character and outfit while walking through a city street",\n    "reference_images": [\n      "https://example.com/character.jpg",\n      "https://example.com/outfit.jpg"\n    ],\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 5\n  }'`
  }, {
    stepKey: 'videoEdit',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos/edits \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "happyhorse-1.1-video-edit",\n    "prompt": "Dress the subject in the striped sweater from the reference image",\n    "video": "https://example.com/input.mp4",\n    "image": "https://example.com/sweater.jpg",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 5\n  }'`
  }, {
    stepKey: 'seedanceTextToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "doubao-seedance-2-0-mini-260615",\n    "prompt": "A paper boat drifts along a quiet stream at sunrise",\n    "resolution": "480P",\n    "ratio": "16:9",\n    "seconds": 4,\n    "generate_audio": false\n  }'`
  }, {
    stepKey: 'seedanceImageToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "doubao-seedance-2-0-mini-260615",\n    "prompt": "The camera slowly moves forward while the flowers sway in the wind",\n    "first_frame": "https://example.com/first-frame.jpg",\n    "resolution": "480P",\n    "ratio": "16:9",\n    "seconds": 4,\n    "generate_audio": false\n  }'`
  }, {
    stepKey: 'seedanceReferenceVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "doubao-seedance-2-0-260128",\n    "prompt": "Preserve the movement rhythm and create a cinematic city scene",\n    "video": "https://example.com/reference-video.mp4",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 4,\n    "generate_audio": false\n  }'`
  }, {
    stepKey: 'veoTextToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "veo-3.1-fast-silent",\n    "prompt": "A paper airplane flies through a quiet modern library",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 4\n  }'`
  }, {
    stepKey: 'veoHeadTailToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "veo-3.1-lite",\n    "prompt": "The camera moves smoothly from the opening scene to the closing scene",\n    "image": "https://example.com/first-frame.jpg",\n    "last_frame": "https://example.com/last-frame.jpg",\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 4,\n    "generate_audio": false\n  }'`
  }, {
    stepKey: 'veoReferenceToVideo',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "veo-3.1",\n    "prompt": "Keep the referenced character appearance while walking through a city street",\n    "reference_images": [\n      "https://example.com/character-reference.jpg"\n    ],\n    "resolution": "720P",\n    "ratio": "16:9",\n    "seconds": 4,\n    "generate_audio": false\n  }'`
  }, {
    stepKey: 'result',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/videos/video_TASK_ID \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }],
  'model-list': [{
    stepKey: 'request',
    language: 'bash',
    code: `curl https://api.your-code.cc/v1/models \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
  }]
}

const activeArticle = computed<ArticleContent>(() => {
  const articleKey = articleKeys[activeSection.value]
  const exampleDefinitions = articleExamples[activeSection.value] || []
  const stepDefinitions = isVideoSection.value
    ? activeVideoDocModel.value.steps
    : articleStepLayouts[activeSection.value]
  return {
    title: t(`docsPage.articles.${articleKey}.title`),
    description: t(`docsPage.articles.${articleKey}.description`),
    steps: stepDefinitions.map((step, index) => {
      const exampleDefinition = exampleDefinitions.find((example) => example.stepKey === step.key)
      const translatedTitle = t(`docsPage.articles.${articleKey}.steps.${step.key}.title`)
      return {
        key: step.key,
        title: isVideoSection.value ? `${index + 1}. ${translatedTitle}` : translatedTitle,
        description: t(`docsPage.articles.${articleKey}.steps.${step.key}.description`),
        action: step.actionTo
          ? { to: step.actionTo, label: t(`docsPage.articles.${articleKey}.steps.${step.key}.action`) }
          : null,
        example: exampleDefinition
          ? {
              language: exampleDefinition.language,
              code: exampleDefinition.code.split('https://api.your-code.cc').join(apiBaseUrl.value)
            }
          : null
      }
    })
  }
})

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

function selectVideoModel(id: VideoDocModelId): void {
  activeVideoModel.value = id
  copiedBlock.value = null
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

async function handleVideoModelKeydown(event: KeyboardEvent, id: VideoDocModelId): Promise<void> {
  const currentIndex = videoDocModels.findIndex((model) => model.id === id)
  let nextIndex = currentIndex
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') nextIndex = (currentIndex + 1) % videoDocModels.length
  if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') nextIndex = (currentIndex - 1 + videoDocModels.length) % videoDocModels.length
  if (event.key === 'Home') nextIndex = 0
  if (event.key === 'End') nextIndex = videoDocModels.length - 1
  if (nextIndex === currentIndex) return

  event.preventDefault()
  const nextModel = videoDocModels[nextIndex]
  if (!nextModel) return
  selectVideoModel(nextModel.id)
  await nextTick()
  document.getElementById(`video-doc-tab-${nextModel.id}`)?.focus()
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

.docs-content-layout,
.docs-content-column {
  min-width: 0;
}

.docs-content-layout-with-models {
  display: grid;
  grid-template-columns: 216px minmax(0, 1fr);
}

.docs-model-nav {
  position: sticky;
  z-index: 20;
  top: 64px;
  height: calc(100vh - 64px);
  align-self: start;
  overflow-y: auto;
  border-right: 1px solid var(--docs-outline);
  padding: 56px 18px 32px;
  background: rgb(11 20 28 / 88%);
  backdrop-filter: blur(10px);
}

.docs-model-nav-label {
  margin: 0 10px 16px;
  color: var(--docs-muted);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
  letter-spacing: 0;
  text-transform: uppercase;
}

.docs-model-tabs {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.docs-model-tab {
  display: flex;
  width: 100%;
  min-height: 42px;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 10px 12px;
  color: var(--docs-muted);
  background: transparent;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.4;
  text-align: left;
  cursor: pointer;
  transition: border-color 150ms ease, color 150ms ease, background 150ms ease;
}

.docs-model-tab:hover,
.docs-model-tab:focus-visible {
  color: var(--docs-text);
  background: var(--docs-surface-high);
  outline: none;
}

.docs-model-tab-active {
  border-left-color: var(--docs-primary);
  color: #f4fff3;
  background: var(--docs-surface-high);
  font-weight: 700;
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

.docs-quick-start {
  max-width: 800px;
}

.docs-quick-start-section {
  margin-bottom: 52px;
}

.docs-quick-start-section > h2 {
  margin-bottom: 16px;
}

.docs-quick-start-section > p {
  margin: 0 0 18px;
  color: var(--docs-muted);
  font-size: 15px;
  line-height: 1.7;
}

.docs-quick-start-action {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--docs-primary-dim);
  border-radius: 4px;
  padding: 9px 13px;
  color: var(--docs-primary);
  background: var(--docs-surface-lowest);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
  transition: border-color 160ms ease, background 160ms ease;
}

.docs-quick-start-action:hover {
  border-color: var(--docs-primary);
  background: var(--docs-surface-high);
}

.docs-quick-start-section .docs-code-block {
  margin-top: 20px;
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

}

@media (max-width: 1200px) {
  .docs-content-layout-with-models {
    display: block;
  }

  .docs-model-nav {
    top: 64px;
    display: flex;
    width: 100%;
    height: auto;
    align-items: center;
    gap: 16px;
    overflow: hidden;
    border-right: 0;
    border-bottom: 1px solid var(--docs-outline);
    padding: 12px 24px;
  }

  .docs-model-nav-label {
    flex: 0 0 auto;
    margin: 0;
  }

  .docs-model-tabs {
    min-width: 0;
    flex: 1;
    flex-direction: row;
    gap: 8px;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .docs-model-tabs::-webkit-scrollbar {
    display: none;
  }

  .docs-model-tab {
    width: auto;
    min-width: max-content;
    flex: 0 0 auto;
    padding: 8px 12px;
  }

  .docs-model-tab-active {
    border-left-color: transparent;
    border-bottom-color: var(--docs-primary);
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

  .docs-model-nav {
    gap: 10px;
    padding: 10px 20px;
  }

  .docs-model-nav-label {
    display: none;
  }

  .docs-model-tab {
    padding: 8px 10px;
    font-size: 12px;
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
