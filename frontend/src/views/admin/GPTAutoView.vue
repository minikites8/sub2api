<template>
  <AppLayout>
    <div class="space-y-6">
      <SmartOpsNav />
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.title') }}</h1>
            <span class="rounded-full bg-primary-50 px-2.5 py-0.5 text-xs font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300" data-testid="gpt-auto-phase-badge">{{ t('gptAuto.phaseBadge') }}</span>
          </div>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.description') }}</p>
        </div>
      </header>

      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300" data-testid="gpt-auto-shadow-banner">
        <p class="font-medium">{{ t('gptAuto.shadowBanner.title') }}</p>
        <p class="mt-1 text-xs">{{ t('gptAuto.shadowBanner.body') }}</p>
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
      <GPTAutoDecisionLog v-else-if="activeTab === 'logs'" />
      <GPTAutoTestLab v-else />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import GPTAutoOverviewPanel from '@/components/admin/gptAuto/GPTAutoOverviewPanel.vue'
import GPTAutoConfigPanel from '@/components/admin/gptAuto/GPTAutoConfigPanel.vue'
import GPTAutoDecisionLog from '@/components/admin/gptAuto/GPTAutoDecisionLog.vue'
import GPTAutoTestLab from '@/components/admin/gptAuto/GPTAutoTestLab.vue'

type Tab = 'overview' | 'config' | 'logs' | 'lab'
const tabs: Tab[] = ['overview', 'config', 'logs', 'lab']

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const initial = String(route.query.tab ?? '') as Tab
const activeTab = ref<Tab>(tabs.includes(initial) ? initial : 'overview')

function selectTab(tab: Tab) {
  activeTab.value = tab
  void router.replace({ query: { ...route.query, tab } })
}
</script>
