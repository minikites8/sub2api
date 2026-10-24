import type { GPTAutoAdjustment, GPTAutoConfig, GPTAutoEconomics, GPTAutoModelProfile, GPTAutoPolicyDecision, UpdateGPTAutoConfig } from '@/api/admin/gptAuto'

/** Removes binary floating-point noise, e.g. 0.09999999999999999 → 0.1. */
export function cleanNumber(value: number): number {
  return Number(value.toPrecision(12))
}

/**
 * Formats USD with about four significant digits for sub-dollar amounts and
 * two to four decimals above, trimming trailing zeros. Tiny per-request
 * figures stay readable ($0.00005292) and float noise never shows.
 */
export function formatUSD(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  const clean = cleanNumber(value)
  if (clean === 0) return '$0.00'
  const abs = Math.abs(clean)
  const decimals = abs >= 1 ? 4 : Math.min(12, Math.max(2, Math.ceil(-Math.log10(abs)) + 3))
  const text = abs.toFixed(decimals).replace(/(\.\d{2}\d*?)0+$/, '$1')
  return `${clean < 0 ? '-' : ''}$${text}`
}

/** Per-1M-token rate. */
export function formatRate(value: number | null | undefined): string {
  return formatUSD(value)
}

export function formatMultiplier(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${cleanNumber(value)}×`
}

export type MarginStatus = 'unknown' | 'loss' | 'below_target' | 'meets_target'

/** Mirrors GPTAutoMarginStatus on the backend. */
export function marginStatus(margin: number | null | undefined, target: number): MarginStatus {
  if (margin === null || margin === undefined || !Number.isFinite(margin)) return 'unknown'
  if (margin < 0) return 'loss'
  if (margin < target) return 'below_target'
  return 'meets_target'
}

export function marginStatusClass(status: MarginStatus): string {
  switch (status) {
    case 'loss': return 'text-red-600 dark:text-red-400'
    case 'below_target': return 'text-amber-600 dark:text-amber-400'
    case 'meets_target': return 'text-emerald-600 dark:text-emerald-400'
    default: return 'text-gray-500 dark:text-gray-400'
  }
}

export interface EconomicsView {
  quality: string
  modelMargin: number | null
  modelStatus: MarginStatus
  fullMargin: number | null
  fullStatus: MarginStatus
  legacy: boolean
}

/**
 * Normalises current and legacy economics. Legacy rows (before the model /
 * fully loaded split) only stored the fully loaded margin; their model margin
 * is derived from revenue and upstream cost, and their quality is unknown.
 */
export function economicsView(economics: GPTAutoEconomics): EconomicsView {
  const legacy = economics.estimate_quality === undefined
  const quality = economics.pricing_available ? (economics.estimate_quality ?? 'legacy') : 'insufficient_data'
  const insufficient = quality === 'insufficient_data'
  let modelMargin: number | null = economics.model_gross_margin ?? null
  if (legacy && economics.pricing_available && economics.estimated_revenue > 0) {
    modelMargin = (economics.estimated_revenue - economics.estimated_upstream_cost) / economics.estimated_revenue
  }
  const fullMargin = insufficient ? null : (economics.estimated_gross_margin ?? null)
  if (insufficient) modelMargin = null
  return {
    quality,
    modelMargin,
    modelStatus: economics.model_margin_status && !legacy ? economics.model_margin_status : marginStatus(modelMargin, economics.target_margin),
    fullMargin,
    fullStatus: economics.fully_loaded_margin_status && !legacy ? economics.fully_loaded_margin_status : marginStatus(fullMargin, economics.target_margin),
    legacy,
  }
}

/** Rules that never change a recommendation (shown as warnings). */
const informationalRules = new Set([
  'margin_below_target', 'no_output_history', 'pricing_unavailable',
  'model_margin_below_target', 'model_margin_loss', 'fully_loaded_margin_below_target', 'fully_loaded_margin_loss',
  'insufficient_estimate_data', 'heuristic_estimate', 'low_model_confidence', 'low_effort_confidence',
  'model_probabilities_missing', 'choice_not_most_probable', 'effort_probabilities_missing',
])

/**
 * Splits route modifications from warnings. Decisions stored before the split
 * listed margin and estimate notes among the adjustments.
 */
export function splitPolicyRules(decision: GPTAutoPolicyDecision): { adjustments: GPTAutoAdjustment[]; warnings: GPTAutoAdjustment[] } {
  const adjustments: GPTAutoAdjustment[] = []
  const warnings: GPTAutoAdjustment[] = [...(decision.warnings ?? [])]
  for (const adjustment of decision.adjustments ?? []) {
    if (informationalRules.has(adjustment.rule)) warnings.push(adjustment)
    else adjustments.push(adjustment)
  }
  return { adjustments, warnings }
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
    capabilities: profile.capabilities ? { ...profile.capabilities } : undefined,
  }))
}

/** Editable copy of a config with the server-only fields removed. */
export function toEditableConfig(config: GPTAutoConfig): UpdateGPTAutoConfig {
  const rest: Partial<GPTAutoConfig> = { ...config }
  delete rest.classifier_api_key_configured
  delete rest.classifier_api_key_masked
  // Saved through its own endpoint; the config save ignores it anyway.
  delete rest.classifier_instructions
  return {
    ...(rest as Omit<GPTAutoConfig, 'classifier_api_key_configured' | 'classifier_api_key_masked'>),
    endpoints: [...(config.endpoints ?? [])],
    group_ids: config.group_ids ? [...config.group_ids] : [],
    model_filter: config.model_filter ? [...config.model_filter] : [],
    model_profiles: cloneProfiles(config.model_profiles ?? []),
    live: config.live
      ? {
          ...config.live,
          allowed_groups: [...(config.live.allowed_groups ?? [])],
          allowed_api_keys: [...(config.live.allowed_api_keys ?? [])],
          endpoints: [...(config.live.endpoints ?? [])],
        }
      : undefined,
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

/** Saves a downloaded blob under the server-chosen file name. */
export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
