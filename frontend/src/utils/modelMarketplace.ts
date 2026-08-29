import type {
  PublicTransitModel,
  PublicTransitModelPrice,
  PublicTransitMonitor,
  PublicTransitMonitorWindow,
  PublicTransitPriceInterval,
  PublicTransitSnapshot,
} from '@/api/publicTransit'
import type { GroupPlatform } from '@/types'

export type MarketplaceStatus = 'operational' | 'degraded' | 'unavailable' | 'unmonitored'
export type MarketplaceWindow = '90m' | '12h' | '1d' | '15d' | '7d' | '30d'
export type TokenPriceField =
  | 'input_usd_per_token'
  | 'output_usd_per_token'
  | 'cache_write_usd_per_token'
  | 'cache_read_usd_per_token'

export interface MarketplacePriceProfile {
  key: string
  groupName: string
  platform: GroupPlatform
  providerVisible: boolean
  multiplier: number
  subscriptionType?: string
  exclusive: boolean
  monitoringEnabled: boolean
  monitoring?: MarketplaceMonitoring
  model: PublicTransitModel
}

export interface MarketplaceMonitoringWindow {
  status: MarketplaceStatus
  hasRequests?: boolean
  availability?: number
  healthScore?: number
  latestLatencyMs?: number
  avgLatencyMs?: number
  lastCheckedAt?: string
  samples: MarketplaceMonitorSample[]
}

export interface MarketplaceMonitoring {
  status: MarketplaceStatus
  hasRequests?: boolean
  healthScore?: number
  availability7d?: number
  availability15d?: number
  availability30d?: number
  latestLatencyMs?: number
  avgLatency7dMs?: number
  lastCheckedAt?: string
  samples: MarketplaceMonitorSample[]
  windows?: Partial<Record<MarketplaceWindow, MarketplaceMonitoringWindow>>
}

export interface MarketplaceMonitorSample {
  status: MarketplaceStatus
  hasRequests?: boolean
  healthScore?: number
  checkedAt: string
  latencyMs?: number
  successRate?: number
}

export interface MarketplaceModel {
  id: string
  name: string
  developer: string
  rawModels: string[]
  platforms: GroupPlatform[]
  visiblePlatforms: GroupPlatform[]
  billingModes: string[]
  supportedProtocols: string[]
  profiles: MarketplacePriceProfile[]
  monitoring: MarketplaceMonitoring
}

interface MonitorObservation {
  status: string
  hasRequests?: boolean
  healthScore?: number
  availability7d?: number
  availability15d?: number
  availability30d?: number
  latestLatencyMs?: number
  avgLatency7dMs?: number
  lastCheckedAt?: string
  windows?: Partial<Record<MarketplaceWindow, MarketplaceMonitoringWindow>>
}

interface MonitoringIndex {
  observations: Map<string, Map<string, MonitorObservation>>
  samples: Map<string, Map<string, MarketplaceMonitorSample>>
}

interface MarketplaceIdentityIndex {
  resolve(modelName: string): string
  displayName(identity: string): string
}

function normalizedModelName(value: string): string {
  return value.trim().toLowerCase()
}

function pricingModelAliases(model: PublicTransitModel): string[] {
  const aliases = new Map<string, string>()
  for (const value of model.pricing_models || []) {
    const name = value.trim()
    const normalized = normalizedModelName(name)
    if (name && !name.endsWith('*') && !aliases.has(normalized)) aliases.set(normalized, name)
  }
  return Array.from(aliases.values())
}

// A channel pricing row defines one marketplace identity and all callable aliases for it.
function buildMarketplaceIdentityIndex(snapshot: PublicTransitSnapshot): MarketplaceIdentityIndex {
  const parents = new Map<string, string>()

  const ensure = (name: string): string => {
    const normalized = normalizedModelName(name)
    if (normalized && !parents.has(normalized)) parents.set(normalized, normalized)
    return normalized
  }
  const find = (name: string): string => {
    let current = ensure(name)
    if (!current) return ''
    while (parents.get(current) !== current) {
      const parent = parents.get(current)!
      const grandparent = parents.get(parent) || parent
      parents.set(current, grandparent)
      current = grandparent
    }
    return current
  }
  const union = (left: string, right: string): void => {
    const leftRoot = find(left)
    const rightRoot = find(right)
    if (leftRoot && rightRoot && leftRoot !== rightRoot) parents.set(rightRoot, leftRoot)
  }

  for (const group of snapshot.groups || []) {
    for (const model of group.models || []) {
      ensure(model.standard_model)
      const aliases = pricingModelAliases(model)
      for (const alias of aliases) ensure(alias)
      for (let index = 1; index < aliases.length; index += 1) union(aliases[0], aliases[index])
    }
  }

  const namesByIdentity = new Map<string, string>()
  for (const group of snapshot.groups || []) {
    for (const model of group.models || []) {
      const aliases = pricingModelAliases(model)
      if (aliases.length === 0) continue
      const identity = find(aliases[0])
      if (!namesByIdentity.has(identity)) namesByIdentity.set(identity, aliases[0])
    }
  }
  for (const group of snapshot.groups || []) {
    for (const model of group.models || []) {
      const identity = find(model.standard_model)
      if (!namesByIdentity.has(identity)) namesByIdentity.set(identity, model.standard_model)
    }
  }

  return {
    resolve: find,
    displayName: (identity: string) => namesByIdentity.get(identity) || identity,
  }
}

function appendUniqueModelName(target: string[], value: string): void {
  const name = value.trim()
  const normalized = normalizedModelName(name)
  if (name && !target.some((item) => normalizedModelName(item) === normalized)) target.push(name)
}

function profileIdentity(model: PublicTransitModel): string {
  const aliases = pricingModelAliases(model)
  if (aliases.length === 0) return normalizedModelName(model.standard_model)
  return aliases.map(normalizedModelName).sort().join('\x00')
}

const MODEL_DEVELOPER_RULES: Array<{ pattern: RegExp; developer: string }> = [
  { pattern: /(^|[-_.])claude([-_.]|$)/, developer: 'Anthropic' },
  {
    pattern:
      /(^|[-_.])(gpt|chatgpt|codex|dall-e|sora|whisper)([-_.]|$)|^o[134]([-_.]|$)|^text-embedding-3([-_.]|$)|^tts-1([-_.]|$)/,
    developer: 'OpenAI',
  },
  { pattern: /(^|[-_.])(gemini|gemma|imagen|veo)([-_.]|$)/, developer: 'Google' },
  { pattern: /(^|[-_.])(doubao|seedance|seedream)([-_.]|$)/, developer: 'ByteDance' },
  { pattern: /(^|[-_.])deepseek([-_.]|$)/, developer: 'DeepSeek' },
  { pattern: /(^|[-_.])grok([-_.]|$)/, developer: 'xAI' },
  { pattern: /(^|[-_.])(qwen|qwq|wan)([-_.]|$)/, developer: 'Alibaba' },
  { pattern: /(^|[-_.])(kimi|moonshot)([-_.]|$)/, developer: 'Moonshot AI' },
  { pattern: /(^|[-_.])(minimax|hailuo)([-_.]|$)/, developer: 'MiniMax' },
  { pattern: /(^|[-_.])(glm|chatglm|cogview|cogvideo)([-_.]|$)/, developer: 'Zhipu AI' },
  { pattern: /(^|[-_.])hunyuan([-_.]|$)/, developer: 'Tencent' },
  { pattern: /(^|[-_.])(ernie|wenxin)([-_.]|$)/, developer: 'Baidu' },
  { pattern: /(^|[-_.])llama([-_.]|$)/, developer: 'Meta' },
  {
    pattern: /(^|[-_.])(mistral|mixtral|codestral|pixtral)([-_.]|$)/,
    developer: 'Mistral AI',
  },
  { pattern: /(^|[-_.])(command-r|command-a)([-_.]|$)/, developer: 'Cohere' },
  { pattern: /(^|[-_.])phi([-_.]|$)/, developer: 'Microsoft' },
  { pattern: /(^|[-_.])(nova|titan)([-_.]|$)/, developer: 'Amazon' },
  { pattern: /(^|[-_.])flux([-_.]|$)/, developer: 'Black Forest Labs' },
  { pattern: /(^|[-_.])happyhorse([-_.]|$)/, developer: 'HappyHorse' },
  { pattern: /(^|[-_.])kling([-_.]|$)/, developer: 'Kuaishou' },
  { pattern: /(^|[-_.])vidu([-_.]|$)/, developer: 'ShengShu AI' },
  { pattern: /(^|[-_.])step([-_.]|$)/, developer: 'StepFun' },
  { pattern: /(^|[-_.])baichuan([-_.]|$)/, developer: 'Baichuan AI' },
  { pattern: /(^|[-_.])nemotron([-_.]|$)/, developer: 'NVIDIA' },
]

export function modelDeveloper(modelName: string): string {
  const normalized = normalizedModelName(modelName)
  return MODEL_DEVELOPER_RULES.find((rule) => rule.pattern.test(normalized))?.developer || ''
}

function finiteValues(values: Array<number | undefined>): number[] {
  return values.filter((value): value is number => value != null && Number.isFinite(value))
}

function minimum(values: Array<number | undefined>): number | undefined {
  const valid = finiteValues(values)
  return valid.length > 0 ? Math.min(...valid) : undefined
}

function maximum(values: Array<number | undefined>): number | undefined {
  const valid = finiteValues(values)
  return valid.length > 0 ? Math.max(...valid) : undefined
}

function normalizeStatus(value: string): MarketplaceStatus {
  switch (value.trim().toLowerCase()) {
    case 'operational':
    case 'healthy':
    case 'online':
    case 'ok':
      return 'operational'
    case 'degraded':
    case 'warning':
      return 'degraded'
    case 'failed':
    case 'error':
    case 'offline':
    case 'unavailable':
      return 'unavailable'
    default:
      return 'unmonitored'
  }
}

function statusSeverity(status: MarketplaceStatus): number {
  if (status === 'unavailable') return 3
  if (status === 'degraded') return 2
  if (status === 'operational') return 1
  return 0
}

function aggregateStatus(observations: MonitorObservation[]): MarketplaceStatus {
  const statuses = observations.map((item) => normalizeStatus(item.status))
  if (statuses.includes('unavailable')) return 'unavailable'
  if (statuses.includes('degraded')) return 'degraded'
  if (statuses.includes('operational')) return 'operational'
  return 'unmonitored'
}

function monitorHasRequests(value: { status: string; hasRequests?: boolean }): boolean {
  return value.hasRequests ?? normalizeStatus(value.status) !== 'unmonitored'
}

function monitoredObservations<T extends { status: string; hasRequests?: boolean }>(observations: T[]): T[] {
  return observations.filter(monitorHasRequests)
}

function publicMonitorStatus(status: string, healthOverall?: string): MarketplaceStatus {
  return normalizeStatus(healthOverall || status)
}

function publicMonitorAvailability(
  status: string,
  availability: number | undefined,
  metrics?: { has_requests: boolean; success_rate: number },
): number | undefined {
  if (metrics) return metrics.has_requests ? metrics.success_rate * 100 : undefined
  return normalizeStatus(status) === 'unmonitored' ? undefined : availability
}

function publicMonitorLatency(
  fallback: number | undefined,
  metrics?: { duration: { p50_ms?: number | null } },
): number | undefined {
  return metrics?.duration.p50_ms ?? fallback
}

function observationKey(item: MonitorObservation): string {
  return [
    item.status,
    item.hasRequests,
    item.healthScore,
    item.availability7d,
    item.availability15d,
    item.availability30d,
    item.latestLatencyMs,
    item.avgLatency7dMs,
    item.lastCheckedAt,
  ].join(':')
}

function registerObservation(
  target: Map<string, Map<string, MonitorObservation>>,
  model: string,
  observation: MonitorObservation,
) {
  const key = normalizedModelName(model)
  if (!key) return
  const observations = target.get(key) || new Map<string, MonitorObservation>()
  observations.set(observationKey(observation), observation)
  target.set(key, observations)
}

function registerSample(
  target: Map<string, Map<string, MarketplaceMonitorSample>>,
  model: string,
  sample: MarketplaceMonitorSample,
) {
  const key = normalizedModelName(model)
  if (!key || !sample.checkedAt) return
  const samples = target.get(key) || new Map<string, MarketplaceMonitorSample>()
  const existing = samples.get(sample.checkedAt)
  if (!existing || shouldReplaceMonitorSample(existing, sample)) samples.set(sample.checkedAt, sample)
  target.set(key, samples)
}

function shouldReplaceMonitorSample(
  existing: MarketplaceMonitorSample,
  candidate: MarketplaceMonitorSample,
): boolean {
  if (monitorHasRequests(candidate) !== monitorHasRequests(existing)) return monitorHasRequests(candidate)
  const severityDelta = statusSeverity(candidate.status) - statusSeverity(existing.status)
  if (severityDelta !== 0) return severityDelta > 0
  if (candidate.healthScore != null && existing.healthScore != null && candidate.healthScore !== existing.healthScore) {
    return candidate.healthScore < existing.healthScore
  }
  if (candidate.healthScore != null && existing.healthScore == null) return true
  return (candidate.latencyMs ?? 0) > (existing.latencyMs ?? 0)
}

function buildMonitoringIndex(monitors: PublicTransitMonitor[]): MonitoringIndex {
  const observations = new Map<string, Map<string, MonitorObservation>>()
  const samples = new Map<string, Map<string, MarketplaceMonitorSample>>()

  for (const monitor of monitors) {
    registerObservation(observations, monitor.model, {
      status: monitor.health?.overall || monitor.status,
      hasRequests: monitor.metrics?.has_requests,
      healthScore: monitor.health?.score ?? undefined,
      availability7d: publicMonitorAvailability(monitor.status, monitor.availability_7d, monitor.metrics),
      availability15d: monitor.availability_15d,
      availability30d: monitor.availability_30d,
      latestLatencyMs: publicMonitorLatency(monitor.latest_duration_p50_ms, monitor.metrics),
      avgLatency7dMs: monitor.metrics?.duration.avg_ms ?? monitor.duration_p50_7d_ms,
      lastCheckedAt: monitor.data_through,
      windows: Object.fromEntries(Object.entries(monitor.windows || {}).map(([key, value]) => [key, publicMonitorWindow(value)])),
    })

    for (const point of monitor.buckets || []) {
      registerSample(samples, monitor.model, {
        status: publicMonitorStatus(point.status, point.health?.overall),
        hasRequests: point.metrics?.has_requests,
        healthScore: point.health?.score ?? undefined,
        checkedAt: point.bucket_start,
        latencyMs: publicMonitorLatency(point.duration_p50_ms, point.metrics),
        successRate: publicMonitorAvailability(point.status, point.success_rate, point.metrics),
      })
    }
  }

  return { observations, samples }
}

function publicMonitorWindow(window: PublicTransitMonitorWindow): MarketplaceMonitoringWindow {
  return {
    status: publicMonitorStatus(window.status, window.health?.overall),
    hasRequests: window.metrics?.has_requests,
    healthScore: window.health?.score ?? undefined,
    availability: publicMonitorAvailability(window.status, window.availability, window.metrics),
    latestLatencyMs: publicMonitorLatency(window.latest_duration_p50_ms, window.metrics),
    avgLatencyMs: window.metrics?.duration.avg_ms ?? window.duration_p50_ms,
    lastCheckedAt: window.data_through,
    samples: (window.buckets || []).map((sample) => ({
      status: publicMonitorStatus(sample.status, sample.health?.overall),
      hasRequests: sample.metrics?.has_requests,
      healthScore: sample.health?.score ?? undefined,
      checkedAt: sample.bucket_start,
      latencyMs: publicMonitorLatency(sample.duration_p50_ms, sample.metrics),
      successRate: publicMonitorAvailability(sample.status, sample.success_rate, sample.metrics),
    })),
  }
}

function aggregateMonitoring(
  aliases: string[],
  index: MonitoringIndex,
): MarketplaceMonitoring {
  const unique = new Map<string, MonitorObservation>()
  const sampleByTime = new Map<string, MarketplaceMonitorSample>()
  for (const alias of aliases) {
    const key = normalizedModelName(alias)
    const observations = index.observations.get(key)
    for (const [key, observation] of observations || []) unique.set(key, observation)
    const samples = index.samples.get(key)
    for (const sample of samples?.values() || []) {
      const existing = sampleByTime.get(sample.checkedAt)
      if (!existing || shouldReplaceMonitorSample(existing, sample)) sampleByTime.set(sample.checkedAt, sample)
    }
  }
  const values = Array.from(unique.values())
  const samples = Array.from(sampleByTime.values())
    .sort((a, b) => a.checkedAt.localeCompare(b.checkedAt))
    .slice(-90)
  if (values.length === 0) return { status: 'unmonitored', samples }

  const windows: Partial<Record<MarketplaceWindow, MarketplaceMonitoringWindow>> = {}
  for (const window of ['90m', '12h', '1d', '15d'] as MarketplaceWindow[]) {
    const windowValues = values.flatMap((item) => item.windows?.[window] ? [item.windows[window]!] : [])
    if (windowValues.length === 0) continue
    const activeWindowValues = monitoredObservations(windowValues)
    const windowSamples = new Map<string, MarketplaceMonitorSample>()
    for (const item of windowValues) {
      for (const sample of item.samples) {
        const existing = windowSamples.get(sample.checkedAt)
        if (!existing || shouldReplaceMonitorSample(existing, sample)) windowSamples.set(sample.checkedAt, sample)
      }
    }
    windows[window] = {
      status: activeWindowValues.length > 0 ? aggregateStatus(activeWindowValues) : 'unmonitored',
      hasRequests: activeWindowValues.length > 0,
      healthScore: minimum(activeWindowValues.map((item) => item.healthScore)),
      availability: minimum(activeWindowValues.map((item) => item.availability)),
      latestLatencyMs: maximum(activeWindowValues.map((item) => item.latestLatencyMs)),
      avgLatencyMs: maximum(activeWindowValues.map((item) => item.avgLatencyMs)),
      lastCheckedAt: windowValues.map((item) => item.lastCheckedAt).filter((value): value is string => Boolean(value)).sort().at(-1),
      samples: Array.from(windowSamples.values()).sort((a, b) => a.checkedAt.localeCompare(b.checkedAt)).slice(-90),
    }
  }

  const checkedAt = values
    .map((item) => item.lastCheckedAt)
    .filter((value): value is string => Boolean(value))
    .sort()
    .at(-1)

  const activeValues = monitoredObservations(values)
  return {
    status: activeValues.length > 0 ? aggregateStatus(activeValues) : 'unmonitored',
    hasRequests: activeValues.length > 0,
    healthScore: minimum(activeValues.map((item) => item.healthScore)),
    availability7d: minimum(activeValues.map((item) => item.availability7d)),
    availability15d: minimum(activeValues.map((item) => item.availability15d)),
    availability30d: minimum(activeValues.map((item) => item.availability30d)),
    latestLatencyMs: maximum(activeValues.map((item) => item.latestLatencyMs)),
    avgLatency7dMs: maximum(activeValues.map((item) => item.avgLatency7dMs)),
    lastCheckedAt: checkedAt,
    samples,
    windows,
  }
}

export function buildMarketplaceModels(snapshot: PublicTransitSnapshot): MarketplaceModel[] {
  const monitoringIndex = buildMonitoringIndex(snapshot.monitoring || [])
  const identityIndex = buildMarketplaceIdentityIndex(snapshot)
  const models = new Map<string, Omit<MarketplaceModel, 'monitoring'>>()

  for (const group of snapshot.groups || []) {
    const groupMonitoringIndex = buildMonitoringIndex(group.monitoring || [])
    for (const model of group.models || []) {
      const identity = identityIndex.resolve(model.standard_model)
      const name = identityIndex.displayName(identity)
      const id = normalizedModelName(name)
      const current = models.get(id) || {
        id,
        name,
        developer: modelDeveloper(name),
        rawModels: [],
        platforms: [],
        visiblePlatforms: [],
        billingModes: [],
        supportedProtocols: [],
        profiles: [],
      }
      appendUniqueModelName(current.rawModels, model.raw_model)
      for (const alias of pricingModelAliases(model)) appendUniqueModelName(current.rawModels, alias)
      if (!current.platforms.includes(model.platform)) current.platforms.push(model.platform)
      if (group.provider_visible === true && !current.visiblePlatforms.includes(model.platform)) {
        current.visiblePlatforms.push(model.platform)
      }
      if (!current.billingModes.includes(model.billing_mode)) current.billingModes.push(model.billing_mode)
      for (const protocol of model.supported_protocols || []) {
        if (!current.supportedProtocols.includes(protocol)) current.supportedProtocols.push(protocol)
      }
      const profile: MarketplacePriceProfile = {
        key: `${group.platform}:${group.name}:${profileIdentity(model)}`,
        groupName: group.name,
        platform: group.platform,
        providerVisible: group.provider_visible === true,
        multiplier: model.rate_multiplier ?? group.rate_multiplier,
        subscriptionType: group.subscription_type,
        exclusive: group.is_exclusive,
        monitoringEnabled: group.monitoring_enabled === true,
        monitoring: group.monitoring_enabled === true
          ? aggregateMonitoring([model.standard_model, model.raw_model, ...pricingModelAliases(model)], groupMonitoringIndex)
          : undefined,
        model,
      }
      const profileIndex = current.profiles.findIndex((item) => item.key === profile.key)
      if (profileIndex < 0) {
        current.profiles.push(profile)
      } else if (
        normalizedModelName(model.standard_model) === normalizedModelName(current.name)
        && normalizedModelName(current.profiles[profileIndex].model.standard_model) !== normalizedModelName(current.name)
      ) {
        current.profiles[profileIndex] = profile
      }
      models.set(id, current)
    }
  }

  return Array.from(models.values())
    .map((model) => ({
      ...model,
      rawModels: model.rawModels.sort(),
      platforms: model.platforms.sort(),
      visiblePlatforms: model.visiblePlatforms.sort(),
      billingModes: model.billingModes.sort(),
      supportedProtocols: model.supportedProtocols.sort(),
      profiles: model.profiles.sort((a, b) => a.multiplier - b.multiplier || a.groupName.localeCompare(b.groupName)),
      monitoring: aggregateMonitoring([model.name, ...model.rawModels], monitoringIndex),
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export function availabilityForWindow(
  monitoring: MarketplaceMonitoring,
  window: MarketplaceWindow,
): number | undefined {
  const current = monitoring.windows?.[window]
  if (current) return monitorHasRequests(current) ? current.availability : undefined
  if (monitoring.hasRequests === false || (monitoring.hasRequests == null && monitoring.status === 'unmonitored')) return undefined
  if (window === '15d' || window === '30d') return window === '15d' ? monitoring.availability15d : monitoring.availability30d
  return monitoring.availability7d
}

export function monitoringForWindow(
  monitoring: MarketplaceMonitoring,
  window: MarketplaceWindow,
): MarketplaceMonitoringWindow {
  const current = monitoring.windows?.[window]
  if (current) {
    return monitorHasRequests(current) ? current : { ...current, availability: undefined }
  }
  return {
    status: monitoring.status,
    hasRequests: monitoring.hasRequests,
    healthScore: monitoring.healthScore,
    availability: availabilityForWindow(monitoring, window),
    latestLatencyMs: monitoring.latestLatencyMs,
    avgLatencyMs: monitoring.avgLatency7dMs,
    lastCheckedAt: monitoring.lastCheckedAt,
    samples: monitoring.samples,
  }
}

export function monitorSamplesForWindow(
  monitoring: MarketplaceMonitoring,
  window: MarketplaceWindow,
): MarketplaceMonitorSample[] {
  return monitoringForWindow(monitoring, window).samples
}

export function applyGroupMultiplier(value: number, multiplier = 1): number {
  return value * multiplier
}

function priceValuesFromRecord(
  record: PublicTransitModelPrice | PublicTransitPriceInterval | undefined,
  field: TokenPriceField,
): number[] {
  const value = record?.[field]
  return value != null && Number.isFinite(value) ? [value] : []
}

export function effectiveTokenPrices(
  model: MarketplaceModel,
  field: TokenPriceField,
): number[] {
  const values = model.profiles.flatMap((profile) => {
    const base = priceValuesFromRecord(profile.model.price, field)
    const intervals = (profile.model.intervals || []).flatMap((interval) => priceValuesFromRecord(interval, field))
    return [...base, ...intervals].map((value) => applyGroupMultiplier(value * 1_000_000, profile.multiplier))
  })
  return Array.from(new Set(values)).sort((a, b) => a - b)
}

export function hasTokenPricing(model: MarketplaceModel): boolean {
  const fields: TokenPriceField[] = [
    'input_usd_per_token',
    'output_usd_per_token',
    'cache_write_usd_per_token',
    'cache_read_usd_per_token',
  ]
  return fields.some((field) => effectiveTokenPrices(model, field).length > 0)
}
