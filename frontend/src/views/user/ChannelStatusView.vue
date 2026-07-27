<template>
  <AppLayout>
    <section class="model-marketplace mx-auto w-full max-w-[1440px] pb-10">
      <header class="mb-6 flex flex-col gap-5 xl:flex-row xl:items-end xl:justify-between">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="text-3xl font-bold tracking-normal text-gray-950 dark:text-white md:text-4xl">
              {{ t('modelMarketplace.title') }}
            </h1>
            <span
              v-if="snapshot?.schema_version"
              class="rounded-full border border-emerald-400/20 bg-emerald-400/5 px-2.5 py-1 font-jetbrains-mono text-xs text-emerald-300"
            >
              v{{ snapshot.schema_version }}
            </span>
          </div>
          <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 font-jetbrains-mono text-xs text-gray-500 dark:text-dark-400">
            <span class="inline-flex items-center gap-1.5">
              <span class="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              {{ t('modelMarketplace.publicAccess') }}
            </span>
            <span>{{ t('modelMarketplace.creditRule') }}</span>
            <span v-if="snapshot">{{ t('modelMarketplace.updatedAt', { time: formatDate(snapshot.generated_at) }) }}</span>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-dark-700 bg-dark-700 sm:grid-cols-4 xl:min-w-[520px]">
          <div class="bg-dark-900 px-4 py-3">
            <div class="font-jetbrains-mono text-xs uppercase text-dark-400">{{ t('modelMarketplace.stats.models') }}</div>
            <div class="mt-1 text-xl font-semibold text-white">{{ models.length }}</div>
          </div>
          <div class="bg-dark-900 px-4 py-3">
            <div class="font-jetbrains-mono text-xs uppercase text-dark-400">{{ t('modelMarketplace.stats.providers') }}</div>
            <div class="mt-1 text-xl font-semibold text-white">{{ providerOptions.length - 1 }}</div>
          </div>
          <div class="bg-dark-900 px-4 py-3">
            <div class="font-jetbrains-mono text-xs uppercase text-dark-400">{{ t('modelMarketplace.stats.operational') }}</div>
            <div class="mt-1 text-xl font-semibold text-emerald-300">{{ operationalCount }}</div>
          </div>
          <div class="bg-dark-900 px-4 py-3">
            <div class="font-jetbrains-mono text-xs uppercase text-dark-400">{{ t('modelMarketplace.stats.monitored') }}</div>
            <div class="mt-1 text-xl font-semibold text-white">{{ monitoringCoverage }}</div>
          </div>
        </div>
      </header>

      <div class="mb-5 rounded-lg border border-dark-700 bg-dark-900/80 p-3">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <label class="relative block min-w-0 flex-1 lg:max-w-md">
            <span class="sr-only">{{ t('modelMarketplace.searchPlaceholder') }}</span>
            <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-dark-400" />
            <input
              v-model.trim="search"
              type="search"
              class="h-10 w-full rounded border border-dark-700 bg-dark-950 pl-9 pr-3 font-jetbrains-mono text-sm text-white outline-none transition focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500/30"
              :placeholder="t('modelMarketplace.searchPlaceholder')"
            >
          </label>

          <div class="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center">
            <select v-model="providerFilter" class="market-select" :aria-label="t('modelMarketplace.providerFilter')">
              <option v-for="option in providerOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
            <select v-model="billingFilter" class="market-select" :aria-label="t('modelMarketplace.billingFilter')">
              <option v-for="option in billingOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
            <select v-model="statusFilter" class="market-select" :aria-label="t('modelMarketplace.statusFilter')">
              <option v-for="option in statusOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
            <label class="col-span-2 inline-flex h-10 cursor-pointer items-center justify-center gap-2 rounded border border-dark-700 bg-dark-950 px-3 text-xs text-dark-300 sm:col-span-1">
              <input v-model="showProviders" type="checkbox" class="peer sr-only">
              <span class="relative h-5 w-9 rounded-full bg-dark-700 transition peer-checked:bg-emerald-600 after:absolute after:left-0.5 after:top-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4" />
              <span>{{ t('modelMarketplace.showProviders') }}</span>
            </label>
            <div class="col-span-2 inline-flex h-10 items-center rounded border border-dark-700 bg-dark-950 p-1 sm:col-span-1">
              <button
                v-for="windowOption in windows"
                :key="windowOption"
                type="button"
                class="h-8 min-w-12 rounded px-2 font-jetbrains-mono text-xs transition"
                :class="monitorWindow === windowOption ? 'bg-dark-700 text-emerald-300' : 'text-dark-400 hover:text-white'"
                @click="monitorWindow = windowOption"
              >
                {{ windowOption }}
              </button>
            </div>
            <button
              type="button"
              class="col-span-2 inline-flex h-10 items-center justify-center gap-2 rounded border border-dark-700 bg-dark-950 px-3 text-sm text-dark-200 transition hover:border-emerald-500/60 hover:text-white disabled:cursor-wait disabled:opacity-60 sm:col-span-1 sm:w-10 sm:px-0"
              :disabled="loading"
              :title="t('modelMarketplace.refresh')"
              @click="loadSnapshot"
            >
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span class="sm:hidden">{{ t('modelMarketplace.refresh') }}</span>
            </button>
          </div>
        </div>
      </div>

      <div v-if="error" class="rounded-lg border border-red-500/30 bg-red-950/20 p-5 text-sm text-red-200">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <span>{{ error }}</span>
          <button type="button" class="btn btn-secondary" @click="loadSnapshot">{{ t('modelMarketplace.retry') }}</button>
        </div>
      </div>

      <div v-else-if="loading && !snapshot" class="space-y-2" aria-live="polite">
        <div v-for="index in 6" :key="index" class="h-[92px] animate-pulse rounded-lg border border-dark-800 bg-dark-900" />
      </div>

      <div v-else-if="filteredModels.length === 0" class="rounded-lg border border-dark-700 bg-dark-900 px-6 py-16 text-center">
        <Icon name="cpu" size="xl" class="mx-auto text-dark-400" />
        <h2 class="mt-4 text-lg font-semibold text-white">{{ t('modelMarketplace.empty.title') }}</h2>
        <p class="mt-2 text-sm text-dark-400">{{ t('modelMarketplace.empty.description') }}</p>
      </div>

      <template v-else>
        <div class="hidden overflow-hidden rounded-lg border border-dark-700 bg-dark-950 md:block">
          <div class="overflow-x-auto">
            <table class="w-full min-w-[960px] border-collapse text-left">
              <thead class="border-b border-dark-700 bg-dark-900">
                <tr class="font-jetbrains-mono text-xs uppercase text-dark-400">
                  <th class="px-5 py-3 font-semibold">{{ t('modelMarketplace.columns.model') }}</th>
                  <th class="px-4 py-3 font-semibold">{{ t('modelMarketplace.columns.billing') }}</th>
                  <th class="px-4 py-3 font-semibold">{{ t('modelMarketplace.columns.pricing') }}</th>
                  <th class="px-4 py-3 font-semibold">{{ t('modelMarketplace.columns.groups') }}</th>
                  <th class="px-4 py-3 font-semibold">{{ t('modelMarketplace.columns.latency') }}</th>
                  <th class="w-14 px-3 py-3"><span class="sr-only">{{ t('modelMarketplace.columns.actions') }}</span></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-dark-800">
                <template v-for="model in filteredModels" :key="model.id">
                  <tr class="market-row group bg-dark-950 transition-colors hover:bg-dark-900/70">
                    <td class="px-5 py-4">
                      <div class="flex min-w-[280px] items-start gap-3.5">
                        <div class="flex h-11 w-11 flex-none items-center justify-center rounded border border-dark-700 bg-dark-900 text-dark-200">
                          <PlatformIcon :platform="model.platforms[0]" size="lg" />
                        </div>
                        <div class="min-w-0">
                          <button type="button" class="flex max-w-[260px] items-center gap-1.5 text-left text-lg font-semibold text-white hover:text-emerald-300" @click="toggleExpanded(model.id)">
                            <span class="truncate">{{ model.name }}</span>
                          </button>
                          <div class="mt-1 flex items-center gap-2 font-jetbrains-mono text-xs">
                            <span class="text-dark-400">{{ model.developer || t('modelMarketplace.unknownDeveloper') }}</span>
                            <span v-if="showProviders" class="border-l border-dark-700 pl-2 text-dark-500">{{ platformNames(model.platforms) }}</span>
                            <span :class="statusTextClass(model.monitoring.status)">{{ formatAvailability(model) }}</span>
                            <button type="button" class="opacity-0 transition hover:text-white group-hover:opacity-100 focus:opacity-100" :title="t('modelMarketplace.copyModelId')" @click="copyModelId(model.name)">
                              <Icon name="copy" size="xs" />
                            </button>
                          </div>
                          <div
                            class="mt-2 flex gap-[2px]"
                            :aria-label="`${t('modelMarketplace.recentChecks')}: ${statusLabel(model.monitoring.status)} ${formatAvailability(model)}`"
                          >
                            <span
                              v-for="sample in monitorSquares(model)"
                              :key="sample.key"
                              class="h-[5px] w-[5px] flex-none rounded-[1px]"
                              :class="statusSquareClass(sample.status)"
                              :title="sample.title"
                            />
                          </div>
                        </div>
                      </div>
                    </td>
                    <td class="px-4 py-4">
                      <div class="flex flex-wrap gap-1.5">
                        <span v-for="mode in model.billingModes" :key="mode" class="rounded border border-dark-700 bg-dark-900 px-2 py-1 font-jetbrains-mono text-xs text-dark-200">
                          {{ billingModeLabel(mode) }}
                        </span>
                      </div>
                    </td>
                    <td class="min-w-[340px] px-4 py-4">
                      <div class="space-y-2 font-jetbrains-mono text-xs">
                        <div v-if="videoPrices(model).length > 0" class="flex flex-wrap items-center gap-1.5">
                          <span class="mr-1 text-[11px] uppercase text-dark-400">{{ t('modelMarketplace.units.video') }}</span>
                          <span
                            v-for="price in videoPrices(model)"
                            :key="price.resolution"
                            class="whitespace-nowrap rounded border border-emerald-400/20 bg-emerald-400/5 px-2 py-1 text-emerald-200"
                          >
                            <span class="text-dark-400">{{ formatResolution(price.resolution) }}</span>
                            {{ priceRange(price.values) }}
                          </span>
                        </div>
                        <div v-if="hasTokenPrices(model)" class="flex flex-wrap gap-x-3 gap-y-1 text-dark-200">
                          <span class="text-[11px] uppercase text-dark-400">{{ t('modelMarketplace.units.token') }}</span>
                          <span><span class="text-dark-400">IN</span> {{ tokenPriceRange(model, 'input_usd_per_token') }}</span>
                          <span><span class="text-dark-400">OUT</span> {{ tokenPriceRange(model, 'output_usd_per_token') }}</span>
                          <span v-if="hasCachePrices(model)"><span class="text-dark-400">W/R</span> {{ tokenPriceRange(model, 'cache_write_usd_per_token') }} / {{ tokenPriceRange(model, 'cache_read_usd_per_token') }}</span>
                        </div>
                        <div v-if="videoPrices(model).length === 0 && !hasTokenPrices(model)" class="text-dark-200">
                          {{ requestPriceRange(model) }}
                        </div>
                      </div>
                    </td>
                    <td class="px-4 py-4">
                      <div class="text-sm font-medium text-white">{{ model.profiles.length }}</div>
                      <div class="mt-1 font-jetbrains-mono text-xs text-dark-400">{{ multiplierRange(model) }}</div>
                    </td>
                    <td class="px-4 py-4">
                      <div class="font-jetbrains-mono text-xs text-white">{{ formatLatency(model.monitoring.latestLatencyMs) }}</div>
                      <div v-if="model.monitoring.avgLatency7dMs != null" class="mt-1 text-xs text-dark-400">
                        {{ t('modelMarketplace.avg7d') }} {{ formatLatency(model.monitoring.avgLatency7dMs) }}
                      </div>
                    </td>
                    <td class="px-3 py-4 text-right">
                      <button
                        type="button"
                        class="inline-flex h-8 w-8 items-center justify-center rounded text-dark-400 transition hover:bg-dark-800 hover:text-white"
                        :title="expanded.has(model.id) ? t('modelMarketplace.collapse') : t('modelMarketplace.details')"
                        @click="toggleExpanded(model.id)"
                      >
                        <Icon name="chevronDown" size="sm" class="transition-transform" :class="expanded.has(model.id) ? 'rotate-180' : ''" />
                      </button>
                    </td>
                  </tr>
                  <tr v-if="expanded.has(model.id)" class="bg-dark-950">
                    <td colspan="6" class="px-5 py-5">
                      <ModelPricingDetail :model="model" />
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>
        </div>

        <div class="space-y-3 md:hidden">
          <article v-for="model in filteredModels" :key="model.id" class="overflow-hidden rounded-lg border border-dark-700 bg-dark-950">
            <button type="button" class="flex w-full items-start justify-between gap-3 p-4 text-left" @click="toggleExpanded(model.id)">
              <span class="flex min-w-0 items-center gap-3.5">
                <span class="flex h-11 w-11 flex-none items-center justify-center rounded border border-dark-700 bg-dark-900 text-dark-200">
                  <PlatformIcon :platform="model.platforms[0]" size="lg" />
                </span>
                <span class="min-w-0">
                  <span class="block truncate text-lg font-semibold text-white">{{ model.name }}</span>
                  <span class="mt-1 block font-jetbrains-mono text-xs text-dark-400">
                    {{ model.developer || t('modelMarketplace.unknownDeveloper') }}
                    <template v-if="showProviders"> · {{ platformNames(model.platforms) }}</template>
                    · {{ billingModeNames(model.billingModes) }}
                  </span>
                  <span class="mt-2 flex items-center gap-2">
                    <span class="font-jetbrains-mono text-xs" :class="statusTextClass(model.monitoring.status)">{{ formatAvailability(model) }}</span>
                    <span
                      class="flex gap-[2px]"
                      :aria-label="`${t('modelMarketplace.recentChecks')}: ${statusLabel(model.monitoring.status)} ${formatAvailability(model)}`"
                    >
                      <span
                        v-for="sample in monitorSquares(model)"
                        :key="sample.key"
                        class="h-[5px] w-[5px] flex-none rounded-[1px]"
                        :class="statusSquareClass(sample.status)"
                        :title="sample.title"
                      />
                    </span>
                  </span>
                </span>
              </span>
              <Icon name="chevronDown" size="sm" class="mt-2 flex-none text-dark-400 transition-transform" :class="expanded.has(model.id) ? 'rotate-180' : ''" />
            </button>
            <div class="grid grid-cols-2 gap-px border-y border-dark-800 bg-dark-800">
              <div class="col-span-2 bg-dark-950 p-3">
                <div class="font-jetbrains-mono text-[11px] uppercase text-dark-400">{{ t('modelMarketplace.columns.pricing') }}</div>
                <div class="mt-2 space-y-2 font-jetbrains-mono text-xs">
                  <div v-if="videoPrices(model).length > 0" class="flex flex-wrap gap-1.5">
                    <span
                      v-for="price in videoPrices(model)"
                      :key="price.resolution"
                      class="whitespace-nowrap rounded border border-emerald-400/20 bg-emerald-400/5 px-2 py-1 text-emerald-200"
                    >
                      <span class="text-dark-400">{{ formatResolution(price.resolution) }}</span>
                      {{ priceRange(price.values) }} {{ t('modelMarketplace.units.video') }}
                    </span>
                  </div>
                  <div v-if="hasTokenPrices(model)" class="flex flex-wrap gap-x-3 gap-y-1 text-white">
                    <span><span class="text-dark-400">IN</span> {{ tokenPriceRange(model, 'input_usd_per_token') }}</span>
                    <span><span class="text-dark-400">OUT</span> {{ tokenPriceRange(model, 'output_usd_per_token') }}</span>
                    <span class="text-dark-400">{{ t('modelMarketplace.units.token') }}</span>
                  </div>
                  <div v-if="videoPrices(model).length === 0 && !hasTokenPrices(model)" class="text-white">
                    {{ requestPriceRange(model) }}
                  </div>
                </div>
              </div>
              <div class="bg-dark-950 p-3">
                <div class="font-jetbrains-mono text-[11px] uppercase text-dark-400">{{ t('modelMarketplace.columns.groups') }}</div>
                <div class="mt-1 font-jetbrains-mono text-xs text-white">{{ model.profiles.length }} · {{ multiplierRange(model) }}</div>
              </div>
              <div class="bg-dark-950 p-3">
                <div class="font-jetbrains-mono text-[11px] uppercase text-dark-400">{{ t('modelMarketplace.columns.latency') }}</div>
                <div class="mt-1 font-jetbrains-mono text-xs text-white">{{ formatLatency(model.monitoring.latestLatencyMs) }}</div>
              </div>
            </div>
            <div v-if="expanded.has(model.id)" class="p-4">
              <ModelPricingDetail :model="model" compact />
            </div>
          </article>
        </div>

        <div class="mt-4 flex flex-col gap-2 text-xs text-dark-400 sm:flex-row sm:items-center sm:justify-between">
          <span>{{ t('modelMarketplace.showing', { shown: filteredModels.length, total: models.length }) }}</span>
          <span>{{ t('modelMarketplace.priceNote') }}</span>
        </div>
      </template>
    </section>
    <PublicSiteFooter :description="t('home.footer.allRightsReserved')" theme="models" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PublicSiteFooter from '@/components/public/PublicSiteFooter.vue'
import { getPublicTransitSnapshot, type PublicTransitModel, type PublicTransitSnapshot } from '@/api/publicTransit'
import { useClipboard } from '@/composables/useClipboard'
import {
  availabilityForWindow,
  buildMarketplaceModels,
  effectiveTokenPrices,
  effectiveVideoPrices,
  hasTokenPricing,
  usdToCredits,
  VIDEO_RESOLUTION_ORDER,
  type MarketplaceModel,
  type MarketplaceStatus,
  type MarketplaceWindow,
  type TokenPriceField,
} from '@/utils/modelMarketplace'

const props = defineProps<{ previewSnapshot?: PublicTransitSnapshot }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const snapshot = ref<PublicTransitSnapshot | null>(props.previewSnapshot || null)
const loading = ref(false)
const error = ref('')
const search = ref('')
const providerFilter = ref('')
const showProviders = ref(false)
const billingFilter = ref('')
const statusFilter = ref('')
const monitorWindow = ref<MarketplaceWindow>('7d')
const expanded = ref(new Set<string>())
const windows: MarketplaceWindow[] = ['7d', '15d', '30d']
const MONITOR_SAMPLE_COUNT = 24
let abortController: AbortController | null = null

interface MonitorSquare {
  key: string
  status: MarketplaceStatus
  title: string
}

const models = computed(() => snapshot.value ? buildMarketplaceModels(snapshot.value) : [])
const operationalCount = computed(() => models.value.filter((model) => model.monitoring.status === 'operational').length)
const monitoredCount = computed(() => models.value.filter((model) => model.monitoring.status !== 'unmonitored').length)
const monitoringCoverage = computed(() => models.value.length > 0 ? `${Math.round(monitoredCount.value / models.value.length * 100)}%` : '0%')

const providerOptions = computed(() => {
  const values = Array.from(new Set(models.value.flatMap((model) => model.platforms))).sort()
  return [
    { value: '', label: t('modelMarketplace.filters.allProviders') },
    ...values.map((value) => ({ value, label: platformLabel(value) })),
  ]
})

const billingOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allBilling') },
  { value: 'token', label: t('modelMarketplace.billing.token') },
  { value: 'per_request', label: t('modelMarketplace.billing.perRequest') },
  { value: 'video', label: t('modelMarketplace.billing.video') },
  { value: 'video_token', label: t('modelMarketplace.billing.videoToken') },
])

const statusOptions = computed(() => [
  { value: '', label: t('modelMarketplace.filters.allStatus') },
  { value: 'operational', label: t('modelMarketplace.status.operational') },
  { value: 'degraded', label: t('modelMarketplace.status.degraded') },
  { value: 'unavailable', label: t('modelMarketplace.status.unavailable') },
  { value: 'unmonitored', label: t('modelMarketplace.status.unmonitored') },
])

const filteredModels = computed(() => {
  const query = search.value.toLowerCase()
  return models.value.filter((model) => {
    const searchable = [
      model.name,
      model.developer,
      ...model.rawModels,
      ...model.platforms,
      ...model.supportedProtocols,
      ...model.profiles.map((profile) => profile.groupName),
    ].join(' ').toLowerCase()
    return (!query || searchable.includes(query))
      && (!providerFilter.value || model.platforms.some((platform) => platform === providerFilter.value))
      && (!billingFilter.value || model.billingModes.includes(billingFilter.value))
      && (!statusFilter.value || model.monitoring.status === statusFilter.value)
  })
})

function platformLabel(platform: string): string {
  const labels: Record<string, string> = {
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    gemini: 'Google Gemini',
    antigravity: 'Antigravity',
    kiro: 'Kiro',
    grok: 'xAI',
    baidu_vod: 'Baidu VOD',
  }
  return labels[platform] || platform
}

function platformNames(platforms: string[]): string {
  return platforms.map(platformLabel).join(' / ')
}

function billingModeLabel(mode: string): string {
  if (mode === 'token') return t('modelMarketplace.billing.token')
  if (mode === 'per_request') return t('modelMarketplace.billing.perRequest')
  if (mode === 'video') return t('modelMarketplace.billing.video')
  if (mode === 'video_token') return t('modelMarketplace.billing.videoToken')
  return mode
}

function billingModeNames(modes: string[]): string {
  return modes.map(billingModeLabel).join(' / ')
}

function statusLabel(status: MarketplaceStatus): string {
  return t(`modelMarketplace.status.${status}`)
}

function statusTextClass(status: MarketplaceStatus): string {
  if (status === 'operational') return 'text-emerald-300'
  if (status === 'degraded') return 'text-amber-300'
  if (status === 'unavailable') return 'text-red-300'
  return 'text-dark-400'
}

function statusSquareClass(status: MarketplaceStatus): string {
  if (status === 'operational') return 'bg-emerald-400 shadow-[0_0_4px_rgba(52,211,153,0.45)]'
  if (status === 'degraded') return 'bg-amber-400'
  if (status === 'unavailable') return 'bg-red-400'
  return 'bg-dark-700'
}

function formatNumber(value: number, maximumFractionDigits = 3): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits }).format(value)
}

function formatCreditPrice(value: number): string {
  if (value >= 1000) return formatNumber(value, 2)
  if (value >= 1) return formatNumber(value, 3)
  return formatNumber(value, 6)
}

function tokenPriceRange(model: MarketplaceModel, field: TokenPriceField): string {
  const values = effectiveTokenPrices(model, field)
  if (values.length === 0) return '-'
  const minimum = values[0]
  const maximum = values.at(-1) ?? minimum
  return minimum === maximum ? formatCreditPrice(minimum) : `${formatCreditPrice(minimum)}-${formatCreditPrice(maximum)}`
}

function priceRange(values: number[]): string {
  if (values.length === 0) return '-'
  const minimum = values[0]
  const maximum = values.at(-1) ?? minimum
  return minimum === maximum ? formatCreditPrice(minimum) : `${formatCreditPrice(minimum)}-${formatCreditPrice(maximum)}`
}

function videoPrices(model: MarketplaceModel) {
  return effectiveVideoPrices(model)
}

function formatResolution(resolution: string): string {
  return resolution.toUpperCase()
}

function hasTokenPrices(model: MarketplaceModel): boolean {
  return hasTokenPricing(model)
}

function hasCachePrices(model: MarketplaceModel): boolean {
  return effectiveTokenPrices(model, 'cache_write_usd_per_token').length > 0
    || effectiveTokenPrices(model, 'cache_read_usd_per_token').length > 0
}

function requestPriceRange(model: MarketplaceModel): string {
  const values = model.profiles.flatMap((profile) => {
    const prices = [
      profile.model.price?.per_request_usd,
      ...Object.values(profile.model.price?.image_size_prices || {}),
    ].filter((value): value is number => value != null && Number.isFinite(value))
    return prices.map((value) => usdToCredits(value, profile.multiplier))
  })
  return priceRange(Array.from(new Set(values)).sort((a, b) => a - b))
}

function formatMultiplier(value: number): string {
  return `${formatNumber(value, 3)}x`
}

function multiplierRange(model: MarketplaceModel): string {
  const values = Array.from(new Set(model.profiles.flatMap((profile) => {
    if (profile.model.billing_mode === 'video_token') return [profile.videoMultiplier]
    const hasVideo = Object.values(profile.model.price?.video_resolution_prices || {}).some((value) => typeof value === 'number')
    const hasToken = [
      profile.model.price?.input_usd_per_token,
      profile.model.price?.output_usd_per_token,
      profile.model.price?.cache_write_usd_per_token,
      profile.model.price?.cache_read_usd_per_token,
    ].some((value) => typeof value === 'number')
    if (hasVideo && hasToken) return [profile.multiplier, profile.videoMultiplier]
    return [hasVideo ? profile.videoMultiplier : profile.multiplier]
  }))).sort((a, b) => a - b)
  const minimum = values[0]
  const maximum = values.at(-1) ?? minimum
  return minimum === maximum ? formatMultiplier(minimum) : `${formatMultiplier(minimum)}-${formatMultiplier(maximum)}`
}

function formatAvailability(model: MarketplaceModel): string {
  const value = availabilityForWindow(model.monitoring, monitorWindow.value)
  return value == null ? '-' : `${value.toFixed(2)}%`
}

function monitorSquares(model: MarketplaceModel): MonitorSquare[] {
  const actual = model.monitoring.samples.slice(-MONITOR_SAMPLE_COUNT).map((sample, index) => ({
    key: `actual-${sample.checkedAt}-${index}`,
    status: sample.status,
    title: t('modelMarketplace.checkTooltip', {
      time: formatDateTime(sample.checkedAt),
      status: statusLabel(sample.status),
    }),
  }))
  if (actual.length > 0) {
    const padding = Array.from({ length: MONITOR_SAMPLE_COUNT - actual.length }, (_, index) => ({
      key: `empty-${index}`,
      status: 'unmonitored' as const,
      title: t('modelMarketplace.noCheckData'),
    }))
    return [...padding, ...actual]
  }

  const availability = availabilityForWindow(model.monitoring, monitorWindow.value)
  if (availability == null) {
    const squares = Array.from({ length: MONITOR_SAMPLE_COUNT }, (_, index) => ({
      key: `empty-${index}`,
      status: 'unmonitored' as MarketplaceStatus,
      title: t('modelMarketplace.noCheckData'),
    }))
    if (model.monitoring.status !== 'unmonitored') {
      squares[MONITOR_SAMPLE_COUNT - 1] = {
        key: 'latest-status',
        status: model.monitoring.status,
        title: t('modelMarketplace.latestStatusTooltip', { status: statusLabel(model.monitoring.status) }),
      }
    }
    return squares
  }

  let issueCount = Math.round((100 - Math.max(0, Math.min(100, availability))) / 100 * MONITOR_SAMPLE_COUNT)
  if (model.monitoring.status === 'degraded' || model.monitoring.status === 'unavailable') {
    issueCount = Math.max(1, issueCount)
  }
  const issueStatus: MarketplaceStatus = model.monitoring.status === 'unavailable' ? 'unavailable' : 'degraded'
  return Array.from({ length: MONITOR_SAMPLE_COUNT }, (_, index) => {
    const status: MarketplaceStatus = index >= MONITOR_SAMPLE_COUNT - issueCount ? issueStatus : 'operational'
    return {
      key: `estimate-${index}`,
      status,
      title: t('modelMarketplace.estimateTooltip', {
        window: monitorWindow.value,
        status: statusLabel(status),
      }),
    }
  })
}

function formatLatency(value?: number): string {
  return value != null && Number.isFinite(value) && value > 0 ? `${formatNumber(value, 0)} ms` : '-'
}

function formatDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

function toggleExpanded(id: string) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}

async function copyModelId(model: string) {
  await copyToClipboard(model, t('modelMarketplace.modelIdCopied'))
}

async function loadSnapshot() {
  if (props.previewSnapshot) {
    snapshot.value = props.previewSnapshot
    error.value = ''
    return
  }
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  error.value = ''
  try {
    snapshot.value = await getPublicTransitSnapshot({ signal: controller.signal })
  } catch (err: unknown) {
    const requestError = err as { name?: string; code?: string; message?: string }
    if (requestError.name === 'AbortError' || requestError.code === 'ERR_CANCELED') return
    error.value = requestError.message || t('modelMarketplace.loadError')
  } finally {
    if (abortController === controller) {
      loading.value = false
      abortController = null
    }
  }
}

const ModelPricingDetail = defineComponent({
  name: 'ModelPricingDetail',
  props: {
    model: { type: Object as PropType<MarketplaceModel>, required: true },
    compact: { type: Boolean, default: false },
  },
  setup(detailProps) {
    const money = (value?: number, multiplier = 1, perMillion = true) => {
      if (value == null || !Number.isFinite(value)) return '-'
      const credits = usdToCredits(perMillion ? value * 1_000_000 : value, multiplier)
      return formatCreditPrice(credits)
    }
    const imagePrices = (model: PublicTransitModel, multiplier: number) => {
      const entries = Object.entries(model.price?.image_size_prices || {}).filter((entry): entry is [string, number] => typeof entry[1] === 'number')
      return entries.map(([size, value]) => `${size.toUpperCase()} ${money(value, multiplier, false)}`).join(' · ') || '-'
    }
    const requestAndImagePrices = (model: PublicTransitModel, multiplier: number) => {
      const values: string[] = []
      if (model.price?.per_request_usd != null) values.push(`${money(model.price.per_request_usd, multiplier, false)} / req`)
      const sizes = imagePrices(model, multiplier)
      if (sizes !== '-') values.push(sizes)
      return values.join(' · ') || '-'
    }
    const profileVideoPrices = (model: PublicTransitModel, multiplier: number) => {
      const order = new Map<string, number>(VIDEO_RESOLUTION_ORDER.map((resolution, index) => [resolution, index]))
      return Object.entries(model.price?.video_resolution_prices || {})
        .filter((entry): entry is [string, number] => typeof entry[1] === 'number')
        .sort((a, b) => (order.get(a[0].toLowerCase()) ?? Number.MAX_SAFE_INTEGER) - (order.get(b[0].toLowerCase()) ?? Number.MAX_SAFE_INTEGER))
        .map(([resolution, value]) => `${formatResolution(resolution)} ${formatCreditPrice(usdToCredits(value, multiplier))}`)
    }
    const profileMultiplier = (profile: MarketplaceModel['profiles'][number]) => {
      if (profile.model.billing_mode === 'video_token') return formatMultiplier(profile.videoMultiplier)
      const hasVideo = profileVideoPrices(profile.model, profile.videoMultiplier).length > 0
      return formatMultiplier(hasVideo ? profile.videoMultiplier : profile.multiplier)
    }
    const profilePrice = (profile: MarketplaceModel['profiles'][number]) => {
      const rows = []
      const video = profileVideoPrices(profile.model, profile.videoMultiplier)
      if (video.length > 0) {
        rows.push(h('div', { class: 'flex flex-wrap gap-1.5' }, video.map((value) => h('span', {
          class: 'whitespace-nowrap rounded border border-emerald-400/20 bg-emerald-400/5 px-2 py-1 text-emerald-200',
        }, `${value} ${t('modelMarketplace.units.video')}`))))
      }
      const token = [
        [label('input'), profile.model.price?.input_usd_per_token],
        [label('output'), profile.model.price?.output_usd_per_token],
        [label('cacheWrite'), profile.model.price?.cache_write_usd_per_token],
        [label('cacheRead'), profile.model.price?.cache_read_usd_per_token],
      ].filter((entry): entry is [string, number] => typeof entry[1] === 'number')
      if (token.length > 0) {
        rows.push(h('div', { class: 'flex flex-wrap gap-x-3 gap-y-1' }, token.map(([name, value]) => h('span', {}, [
          h('span', { class: 'text-dark-400' }, `${name} `),
          money(value, profile.multiplier),
        ]))))
      }
      if (profile.model.billing_mode === 'video_token') {
        const values = (profile.model.intervals || [])
          .map((interval) => interval.output_usd_per_token)
          .filter((value): value is number => typeof value === 'number')
          .map((value) => usdToCredits(value * 1_000_000, profile.videoMultiplier))
          .sort((a, b) => a - b)
        if (values.length > 0) {
          rows.push(h('div', {}, `${values.length} ${label('videoTokenPricing')} · ${priceRange(values)} ${t('modelMarketplace.units.token')}`))
        }
      }
      if (video.length === 0) {
        const request = requestAndImagePrices(profile.model, profile.multiplier)
        if (request !== '-') rows.push(h('div', {}, request))
      }
      return rows.length > 0 ? rows : '-'
    }
    const tierRange = (minimum: number, maximum?: number) => maximum == null ? `${formatNumber(minimum, 0)}+` : `${formatNumber(minimum, 0)}-${formatNumber(maximum, 0)}`
    const label = (key: string) => t(`modelMarketplace.detail.${key}`)
    const videoTokenTierLabel = (tier: string) => {
      const [resolution = tier, inputType = ''] = tier.split(':')
      const resolutionLabel = resolution.toLowerCase() === 'default' ? t('admin.channels.form.defaultResolution') : resolution.toUpperCase()
      const inputLabel = inputType.toLowerCase() === 'video' ? label('inputWithVideo') : label('inputWithoutVideo')
      return `${resolutionLabel} · ${inputLabel}`
    }

    return () => h('div', { class: 'space-y-5' }, [
      h('div', { class: 'flex flex-wrap gap-2' }, [
        ...detailProps.model.rawModels.map((rawModel) => h('span', { class: 'rounded border border-dark-800 px-2 py-1 font-jetbrains-mono text-xs text-dark-400' }, rawModel)),
      ]),
      h('div', { class: 'overflow-x-auto rounded border border-dark-800' }, [
        h('table', { class: `w-full border-collapse text-left ${detailProps.compact ? 'min-w-[560px]' : 'min-w-[640px]'}` }, [
          h('thead', { class: 'bg-dark-900 font-jetbrains-mono text-[11px] uppercase text-dark-400' }, [
            h('tr', {}, [label('group'), label('mode'), label('multiplier'), label('pricing')].map((text) => h('th', { class: 'px-3 py-2 font-medium' }, text))),
          ]),
          h('tbody', { class: 'divide-y divide-dark-800 font-jetbrains-mono text-xs' }, detailProps.model.profiles.map((profile) => h('tr', { class: 'text-dark-200' }, [
            h('td', { class: 'px-3 py-3' }, [h('div', { class: 'font-sans text-xs font-medium text-white' }, profile.groupName), h('div', { class: 'mt-1 text-[11px] text-dark-400' }, platformLabel(profile.platform))]),
            h('td', { class: 'px-3 py-3' }, billingModeLabel(profile.model.billing_mode)),
            h('td', { class: 'px-3 py-3 text-emerald-300' }, profileMultiplier(profile)),
            h('td', { class: 'min-w-[300px] px-3 py-3' }, h('div', { class: 'space-y-2' }, profilePrice(profile))),
          ]))),
        ]),
      ]),
      ...detailProps.model.profiles.flatMap((profile) => (profile.model.intervals || []).length > 0 && (profile.model.billing_mode === 'token' || profile.model.billing_mode === 'video_token')
        ? [h('div', { class: 'rounded border border-dark-800 bg-dark-900/50 p-3' }, [
            h('div', { class: 'mb-2 text-xs font-medium text-white' }, `${profile.groupName} · ${label(profile.model.billing_mode === 'video_token' ? 'videoTokenPricing' : 'tierPricing')}`),
            h('div', { class: 'grid gap-2 sm:grid-cols-2 xl:grid-cols-3' }, (profile.model.intervals || []).map((interval) => h('div', { class: 'rounded border border-dark-800 bg-dark-950 p-3 font-jetbrains-mono text-xs text-dark-300' }, [
              h('div', { class: 'mb-2 text-white' }, profile.model.billing_mode === 'video_token' ? videoTokenTierLabel(interval.tier_label || '') : interval.tier_label || tierRange(interval.min_tokens, interval.max_tokens)),
              ...(profile.model.billing_mode === 'video_token'
                ? [h('div', {}, `${label('output')}: ${money(interval.output_usd_per_token, profile.videoMultiplier)} ${t('modelMarketplace.units.token')}`)]
                : [
                    h('div', {}, `${label('tokenRange')}: ${tierRange(interval.min_tokens, interval.max_tokens)}`),
                    h('div', { class: 'mt-1' }, `${label('input')}: ${money(interval.input_usd_per_token, profile.multiplier)} · ${label('output')}: ${money(interval.output_usd_per_token, profile.multiplier)}`),
                  ]),
            ]))),
          ])]
        : []),
      h('div', { class: 'font-jetbrains-mono text-xs text-dark-400' }, t('modelMarketplace.detail.unitNote')),
    ])
  },
})

onMounted(() => {
  if (!props.previewSnapshot) void loadSnapshot()
})

onBeforeUnmount(() => abortController?.abort())
</script>

<style scoped>
.market-select {
  height: 2.5rem;
  min-width: 0;
  border: 1px solid rgb(55 65 81);
  border-radius: 0.25rem;
  background: var(--md-app-bg);
  padding: 0 2rem 0 0.75rem;
  color: rgb(229 231 235);
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 0.75rem;
  outline: none;
}

.market-select:focus {
  border-color: rgb(16 185 129);
  box-shadow: 0 0 0 1px rgb(16 185 129 / 0.3);
}

.market-row:hover {
  box-shadow: inset 2px 0 0 rgb(52 211 153);
}

@media (max-width: 639px) {
  .market-select {
    width: 100%;
  }
}
</style>
