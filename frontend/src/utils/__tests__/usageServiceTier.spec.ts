import { describe, expect, it } from 'vitest'

import {
  formatUsageServiceTier,
  formatUsageServiceTierMultiplier,
  getUsageServiceTierLabel,
  getUsageServiceTierLabelWithMultiplier,
  getUsageServiceTierMultiplier,
  normalizeUsageServiceTier,
} from '@/utils/usageServiceTier'

describe('usageServiceTier utils', () => {
  it('normalizes fast/default aliases', () => {
    expect(normalizeUsageServiceTier('fast')).toBe('priority')
    expect(normalizeUsageServiceTier(' default ')).toBe('standard')
    expect(normalizeUsageServiceTier('STANDARD')).toBe('standard')
  })

  it('preserves supported tiers', () => {
    expect(normalizeUsageServiceTier('priority')).toBe('priority')
    expect(normalizeUsageServiceTier('flex')).toBe('flex')
  })

  it('formats empty values as standard', () => {
    expect(formatUsageServiceTier()).toBe('standard')
    expect(formatUsageServiceTier('')).toBe('standard')
  })

  it('passes through unknown non-empty tiers for display fallback', () => {
    expect(normalizeUsageServiceTier('custom-tier')).toBe('custom-tier')
    expect(formatUsageServiceTier('custom-tier')).toBe('custom-tier')
  })

  it('maps tiers to translated labels', () => {
    const translate = (key: string) => ({
      'usage.serviceTierPriority': 'Fast',
      'usage.serviceTierFlex': 'Flex',
      'usage.serviceTierStandard': 'Standard',
    })[key] ?? key

    expect(getUsageServiceTierLabel('fast', translate)).toBe('Fast')
    expect(getUsageServiceTierLabel('flex', translate)).toBe('Flex')
    expect(getUsageServiceTierLabel(undefined, translate)).toBe('Standard')
    expect(getUsageServiceTierLabel('custom-tier', translate)).toBe('custom-tier')
  })

  it('maps tiers to billing multipliers', () => {
    expect(getUsageServiceTierMultiplier('fast')).toBe(2)
    expect(getUsageServiceTierMultiplier('priority')).toBe(2)
    expect(getUsageServiceTierMultiplier('flex')).toBe(0.5)
    expect(getUsageServiceTierMultiplier('standard')).toBe(1)
    expect(getUsageServiceTierMultiplier('custom-tier')).toBeNull()
  })

  it('formats labels with multipliers', () => {
    const translate = (key: string) => ({
      'usage.serviceTierPriority': 'Fast',
      'usage.serviceTierFlex': 'Flex',
      'usage.serviceTierStandard': 'Standard',
    })[key] ?? key

    expect(formatUsageServiceTierMultiplier(2)).toBe('2x')
    expect(formatUsageServiceTierMultiplier(0.5)).toBe('0.5x')
    expect(getUsageServiceTierLabelWithMultiplier('fast', translate)).toBe('Fast 2x')
    expect(getUsageServiceTierLabelWithMultiplier(undefined, translate)).toBe('Standard 1x')
    expect(getUsageServiceTierLabelWithMultiplier('custom-tier', translate)).toBe('custom-tier')
  })
})
