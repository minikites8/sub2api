export function normalizeUsageServiceTier(serviceTier?: string | null): string | null {
  const value = serviceTier?.trim().toLowerCase()
  if (!value) return null
  if (value === 'fast') return 'priority'
  if (value === 'default' || value === 'standard') return 'standard'
  if (value === 'priority' || value === 'flex') return value
  return value
}

export function formatUsageServiceTier(serviceTier?: string | null): string {
  const normalized = normalizeUsageServiceTier(serviceTier)
  if (!normalized) return 'standard'
  return normalized
}

export function getUsageServiceTierLabel(
  serviceTier: string | null | undefined,
  translate: (key: string) => string,
): string {
  const tier = formatUsageServiceTier(serviceTier)
  if (tier === 'priority') return translate('usage.serviceTierPriority')
  if (tier === 'flex') return translate('usage.serviceTierFlex')
  if (tier === 'standard') return translate('usage.serviceTierStandard')
  return tier
}

export function getUsageServiceTierMultiplier(serviceTier?: string | null): number | null {
  const tier = formatUsageServiceTier(serviceTier)
  if (tier === 'priority') return 2
  if (tier === 'flex') return 0.5
  if (tier === 'standard') return 1
  return null
}

export function formatUsageServiceTierMultiplier(multiplier: number): string {
  return `${Number.isInteger(multiplier) ? multiplier.toFixed(0) : String(multiplier)}x`
}

export function getUsageServiceTierLabelWithMultiplier(
  serviceTier: string | null | undefined,
  translate: (key: string) => string,
): string {
  const label = getUsageServiceTierLabel(serviceTier, translate)
  const multiplier = getUsageServiceTierMultiplier(serviceTier)
  if (multiplier == null) return label
  return `${label} ${formatUsageServiceTierMultiplier(multiplier)}`
}
