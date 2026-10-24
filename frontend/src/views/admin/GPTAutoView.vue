<template>
  <AppLayout>
    <div class="space-y-6">
      <SmartOpsNav />
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.title') }}</h1>
            <span
              class="rounded-full px-2.5 py-0.5 text-xs font-semibold"
              :class="liveActive ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200' : 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'"
              data-testid="gpt-auto-phase-badge"
            >{{ liveActive ? t('gptAuto.phaseBadgeLive') : t('gptAuto.phaseBadge') }}</span>
          </div>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.description') }}</p>
        </div>
        <dl v-if="meta" class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-auto-versions">
          <dt>{{ t('gptAuto.versions.state') }}</dt><dd class="font-mono">{{ meta.state_version || '—' }}</dd>
          <dt>{{ t('gptAuto.versions.schema') }}</dt><dd class="font-mono">{{ meta.classifier_schema || '—' }}</dd>
          <dt>{{ t('gptAuto.versions.policy') }}</dt><dd class="font-mono">{{ meta.policy_version || '—' }}</dd>
          <dt>{{ t('gptAuto.versions.pool') }}</dt><dd class="font-mono">{{ meta.model_pool_version || '—' }}</dd>
          <dd class="col-span-2 max-w-xs text-[11px]">{{ t('gptAuto.versions.note') }}</dd>
        </dl>
      </header>

      <div
        class="rounded-lg border p-3 text-sm"
        :class="liveActive ? 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700/60 dark:bg-amber-900/20 dark:text-amber-200' : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
        data-testid="gpt-auto-shadow-banner"
      >
        <p class="font-medium">{{ liveActive ? t('gptAuto.liveBanner.title') : t('gptAuto.shadowBanner.title') }}</p>
        <p class="mt-1 text-xs">{{ liveActive ? t('gptAuto.liveBanner.body') : t('gptAuto.shadowBanner.body') }}</p>
      </div>

      <nav class="flex flex-wrap gap-2 border-b border-gray-200 pb-2 dark:border-dark-700" role="tablist">
        <button
          v-for="tab in tabs"
          :key="tab"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab"
          class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
          :class="activeTab === tab ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700'"
          :data-testid="`gpt-auto-tab-${tab}`"
          @click="selectTab(tab)"
        >
          {{ t(`gptAuto.tabs.${tab}`) }}
        </button>
      </nav>

      <GPTAutoOverviewPanel v-if="activeTab === 'overview'" />
      <GPTAutoConfigPanel v-else-if="activeTab === 'config'" />
      <GPTAutoPricingPanel v-else-if="activeTab === 'pricing'" />
      <GPTAutoLivePanel v-else-if="activeTab === 'live'" />
      <GPTAutoClassifierPanel v-else-if="activeTab === 'classifier'" />
      <GPTAutoDecisionLog v-else-if="activeTab === 'logs'" />
      <GPTAutoTestLab v-else />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import GPTAutoOverviewPanel from '@/components/admin/gptAuto/GPTAutoOverviewPanel.vue'
import GPTAutoConfigPanel from '@/components/admin/gptAuto/GPTAutoConfigPanel.vue'
import GPTAutoClassifierPanel from '@/components/admin/gptAuto/GPTAutoClassifierPanel.vue'
import GPTAutoDecisionLog from '@/components/admin/gptAuto/GPTAutoDecisionLog.vue'
import GPTAutoTestLab from '@/components/admin/gptAuto/GPTAutoTestLab.vue'
import GPTAutoPricingPanel from '@/components/admin/gptAuto/GPTAutoPricingPanel.vue'
import GPTAutoLivePanel from '@/components/admin/gptAuto/GPTAutoLivePanel.vue'
import { gptAutoAPI, type GPTAutoMeta } from '@/api/admin/gptAuto'

type Tab = 'overview' | 'pricing' | 'live' | 'config' | 'classifier' | 'logs' | 'lab'
const tabs: Tab[] = ['overview', 'pricing', 'live', 'config', 'classifier', 'logs', 'lab']

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const initial = String(route.query.tab ?? '') as Tab
const activeTab = ref<Tab>(tabs.includes(initial) ? initial : 'overview')

function selectTab(tab: Tab) {
  activeTab.value = tab
  void router.replace({ query: { ...route.query, tab } })
}

// The mode shown here is what the backend node serving this page has cached;
// other nodes converge within the config cache TTL.
const meta = ref<GPTAutoMeta | null>(null)
const liveActive = computed(() => meta.value?.live_routing_active === true)
onMounted(async () => {
  try {
    meta.value = await gptAutoAPI.getMeta()
  } catch {
    meta.value = null
  }
})
</script>
