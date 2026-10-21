import type { GPTAutoConfig, GPTAutoModelProfile, UpdateGPTAutoConfig } from '@/api/admin/gptAuto'

export function formatUSD(value: number | null | undefined, digits = 6): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  if (value !== 0 && Math.abs(value) < 10 ** -digits) return `$${value.toExponential(2)}`
  return `$${value.toFixed(digits)}`
}

export function formatPercent(value: number | null | undefined, digits = 1): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${(value * 100).toFixed(digits)}%`
}

export function formatMs(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${Math.round(value)} ms`
}

export function formatNumber(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return value.toLocaleString()
}

export function cloneProfiles(profiles: GPTAutoModelProfile[]): GPTAutoModelProfile[] {
  return profiles.map((profile) => ({
    ...profile,
    supported_efforts: [...(profile.supported_efforts ?? [])],
    endpoints: profile.endpoints ? [...profile.endpoints] : undefined,
    upstream_costs: (profile.upstream_costs ?? []).map((cost) => ({ ...cost })),
  }))
}

/** Editable copy of a config with the server-only fields removed. */
export function toEditableConfig(config: GPTAutoConfig): UpdateGPTAutoConfig {
  const rest: Partial<GPTAutoConfig> = { ...config }
  delete rest.classifier_api_key_configured
  delete rest.classifier_api_key_masked
  return {
    ...(rest as Omit<GPTAutoConfig, 'classifier_api_key_configured' | 'classifier_api_key_masked'>),
    endpoints: [...(config.endpoints ?? [])],
    group_ids: config.group_ids ? [...config.group_ids] : [],
    model_filter: config.model_filter ? [...config.model_filter] : [],
    model_profiles: cloneProfiles(config.model_profiles ?? []),
    classifier_api_key: '',
    clear_classifier_api_key: false,
  }
}

export function parseIdList(text: string): number[] {
  return text
    .split(/[\s,，]+/)
    .map((part) => Number(part.trim()))
    .filter((id) => Number.isInteger(id) && id > 0)
}

export function parseStringList(text: string): string[] {
  return text
    .split(/[\s,，]+/)
    .map((part) => part.trim())
    .filter(Boolean)
}

/** Sorted [key, probability] pairs, highest first. */
export function sortedProbabilities(probabilities?: Record<string, number>): [string, number][] {
  return Object.entries(probabilities ?? {}).sort((a, b) => b[1] - a[1])
}
