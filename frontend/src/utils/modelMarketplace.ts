import type {
  PublicTransitModel,
  PublicTransitModelPrice,
  PublicTransitMonitor,
  PublicTransitPriceInterval,
  PublicTransitSnapshot,
} from '@/api/publicTransit'
import type { GroupPlatform } from '@/types'

export type MarketplaceStatus = 'operational' | 'degraded' | 'unavailable' | 'unmonitored'
export type MarketplaceWindow = '7d' | '15d' | '30d'
export type TokenPriceField =
  | 'input_usd_per_token'
  | 'output_usd_per_token'
  | 'cache_write_usd_per_token'
  | 'cache_read_usd_per_token'

export interface MarketplacePriceProfile {
  key: string
  groupName: string
  platform: GroupPlatform
  multiplier: number
  subscriptionType?: string
  exclusive: boolean
  model: PublicTransitModel
}

export interface MarketplaceMonitoring {
  status: MarketplaceStatus
  availability7d?: number
  availability15d?: number
  availability30d?: number
  latestLatencyMs?: number
  avgLatency7dMs?: number
  lastCheckedAt?: string
  samples: MarketplaceMonitorSample[]
}

export interface MarketplaceMonitorSample {
  status: MarketplaceStatus
  checkedAt: string
  latencyMs?: number
}

export interface MarketplaceModel {
  id: string
  name: string
  rawModels: string[]
  platforms: GroupPlatform[]
  billingModes: string[]
  supportedProtocols: string[]
  profiles: MarketplacePriceProfile[]
  monitoring: MarketplaceMonitoring
}

interface MonitorObservation {
  status: string
  availability7d?: number
  availability15d?: number
  availability30d?: number
  latestLatencyMs?: number
  avgLatency7dMs?: number
  lastCheckedAt?: string
}

interface MonitoringIndex {
  observations: Map<string, Map<string, MonitorObservation>>
  samples: Map<string, Map<string, MarketplaceMonitorSample>>
}

function normalizedModelName(value: string): string {
  return value.trim().toLowerCase()
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

function observationKey(item: MonitorObservation): string {
  return [
    item.status,
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
  if (!existing || statusSeverity(sample.status) > statusSeverity(existing.status)) {
    samples.set(sample.checkedAt, sample)
  } else if (existing.status === sample.status && (sample.latencyMs ?? 0) > (existing.latencyMs ?? 0)) {
    samples.set(sample.checkedAt, sample)
  }
  target.set(key, samples)
}

function buildMonitoringIndex(monitors: PublicTransitMonitor[]): MonitoringIndex {
  const observations = new Map<string, Map<string, MonitorObservation>>()
  const samples = new Map<string, Map<string, MarketplaceMonitorSample>>()

  for (const monitor of monitors) {
    for (const model of monitor.models || []) {
      registerObservation(observations, model.model, {
        status: model.latest_status,
        availability7d: model.availability_7d,
        availability15d: model.availability_15d,
        availability30d: model.availability_30d,
        latestLatencyMs: model.latest_latency_ms,
        avgLatency7dMs: model.avg_latency_7d_ms,
        lastCheckedAt: monitor.last_checked_at,
      })
    }

    registerObservation(observations, monitor.primary_model, {
      status: monitor.primary_status,
      availability7d: monitor.availability_7d,
      availability15d: monitor.availability_15d,
      availability30d: monitor.availability_30d,
      latestLatencyMs: monitor.latest_latency_ms,
      avgLatency7dMs: monitor.avg_latency_7d_ms,
      lastCheckedAt: monitor.last_checked_at,
    })

    for (const point of monitor.timeline || []) {
      registerSample(samples, monitor.primary_model, {
        status: normalizeStatus(point.status),
        checkedAt: point.checked_at,
        latencyMs: point.latency_ms,
      })
    }

    for (const model of monitor.extra_models || []) {
      registerObservation(observations, model.model, {
        status: model.status,
        latestLatencyMs: model.latency_ms,
        lastCheckedAt: monitor.last_checked_at,
      })
    }
  }

  return { observations, samples }
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
      if (!existing || statusSeverity(sample.status) > statusSeverity(existing.status)) {
        sampleByTime.set(sample.checkedAt, sample)
      } else if (existing.status === sample.status && (sample.latencyMs ?? 0) > (existing.latencyMs ?? 0)) {
        sampleByTime.set(sample.checkedAt, sample)
      }
    }
  }
  const values = Array.from(unique.values())
  const samples = Array.from(sampleByTime.values())
    .sort((a, b) => a.checkedAt.localeCompare(b.checkedAt))
    .slice(-90)
  if (values.length === 0) return { status: 'unmonitored', samples }

  const checkedAt = values
    .map((item) => item.lastCheckedAt)
    .filter((value): value is string => Boolean(value))
    .sort()
    .at(-1)

  return {
    status: aggregateStatus(values),
    availability7d: minimum(values.map((item) => item.availability7d)),
    availability15d: minimum(values.map((item) => item.availability15d)),
    availability30d: minimum(values.map((item) => item.availability30d)),
    latestLatencyMs: maximum(values.map((item) => item.latestLatencyMs)),
    avgLatency7dMs: maximum(values.map((item) => item.avgLatency7dMs)),
    lastCheckedAt: checkedAt,
    samples,
  }
}

export function buildMarketplaceModels(snapshot: PublicTransitSnapshot): MarketplaceModel[] {
  const monitoringIndex = buildMonitoringIndex(snapshot.monitoring || [])
  const models = new Map<string, Omit<MarketplaceModel, 'monitoring'>>()

  for (const group of snapshot.groups || []) {
    for (const model of group.models || []) {
      const id = normalizedModelName(model.standard_model)
      const current = models.get(id) || {
        id,
        name: model.standard_model,
        rawModels: [],
        platforms: [],
        billingModes: [],
        supportedProtocols: [],
        profiles: [],
      }
      if (!current.rawModels.includes(model.raw_model)) current.rawModels.push(model.raw_model)
      if (!current.platforms.includes(model.platform)) current.platforms.push(model.platform)
      if (!current.billingModes.includes(model.billing_mode)) current.billingModes.push(model.billing_mode)
      for (const protocol of model.supported_protocols || []) {
        if (!current.supportedProtocols.includes(protocol)) current.supportedProtocols.push(protocol)
      }
      current.profiles.push({
        key: `${group.platform}:${group.name}:${model.raw_model}`,
        groupName: group.name,
        platform: group.platform,
        multiplier: group.rate_multiplier,
        subscriptionType: group.subscription_type,
        exclusive: group.is_exclusive,
        model,
      })
      models.set(id, current)
    }
  }

  return Array.from(models.values())
    .map((model) => ({
      ...model,
      rawModels: model.rawModels.sort(),
      platforms: model.platforms.sort(),
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
  if (window === '15d') return monitoring.availability15d
  if (window === '30d') return monitoring.availability30d
  return monitoring.availability7d
}

export function usdToCredits(value: number, multiplier = 1): number {
  return value * 100 * multiplier
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
    return [...base, ...intervals].map((value) => usdToCredits(value * 1_000_000, profile.multiplier))
  })
  return Array.from(new Set(values)).sort((a, b) => a - b)
}
