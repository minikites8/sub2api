import { describe, expect, it } from 'vitest'
import { economicsView, formatMultiplier, formatRate, formatUSD, marginStatus, splitPolicyRules } from '../gptAuto'
import type { GPTAutoEconomics, GPTAutoPolicyDecision } from '@/api/admin/gptAuto'

const economics = (overrides: Partial<GPTAutoEconomics> = {}): GPTAutoEconomics => ({
  hypothetical: true, model: 'gpt-6-luna', effort: 'none', billing_multiplier: 1.1,
  estimated_input_tokens: 1, estimated_output_tokens: 10, output_estimate_source: 'admin_assumption', output_estimate_samples: 0,
  list_input_per_m: 0.5, list_output_per_m: 0.55, upstream_input_per_m: 0.5, upstream_output_per_m: 0.45, upstream_cost_source: 'configured_channel',
  estimated_revenue: 0.000006, estimated_upstream_cost: 0.000005, estimated_classifier_cost: 0.00005292,
  estimated_gross_profit: -0.00005192, estimated_gross_margin: -8.653333, target_margin: 0.35, below_target: true, pricing_available: true,
  estimate_quality: 'admin_assumption', model_gross_margin: 0.166667, model_margin_status: 'below_target', fully_loaded_margin_status: 'loss',
  ...overrides,
})

describe('GPT Auto money formatting', () => {
  it('removes floating-point artifacts and keeps tiny amounts readable', () => {
    expect(formatUSD(0.09999999999999999)).toBe('$0.10')
    expect(formatRate(0.1 / 1e6 * 1e6)).toBe('$0.10')
    expect(formatUSD(0.00005292)).toBe('$0.00005292')
    expect(formatUSD(0.000006)).toBe('$0.000006')
    expect(formatUSD(0.015)).toBe('$0.015')
    expect(formatUSD(12.5)).toBe('$12.50')
    expect(formatUSD(-0.00005192)).toBe('-$0.00005192')
    expect(formatUSD(0)).toBe('$0.00')
    expect(formatUSD(undefined)).toBe('—')
    expect(formatMultiplier(1.1000000000000001)).toBe('1.1×')
  })
})

describe('GPT Auto margin semantics', () => {
  it('distinguishes loss, positive-below-target, meeting target and unknown', () => {
    expect(marginStatus(-8.653, 0.43)).toBe('loss')
    expect(marginStatus(0.331, 0.43)).toBe('below_target')
    expect(marginStatus(0.5, 0.43)).toBe('meets_target')
    expect(marginStatus(undefined, 0.43)).toBe('unknown')
    expect(marginStatus(null, 0.43)).toBe('unknown')
  })

  it('never shows a margin for insufficient estimates', () => {
    const view = economicsView(economics({ estimate_quality: 'insufficient_data', estimated_gross_margin: undefined, model_gross_margin: undefined, model_margin_status: 'unknown', fully_loaded_margin_status: 'unknown' }))
    expect(view.modelMargin).toBeNull()
    expect(view.fullMargin).toBeNull()
    expect(view.fullStatus).toBe('unknown')
  })

  it('derives the model margin for legacy rows', () => {
    const legacy = economics({ estimate_quality: undefined, model_gross_margin: undefined, model_margin_status: undefined, fully_loaded_margin_status: undefined })
    const view = economicsView(legacy)
    expect(view.legacy).toBe(true)
    expect(view.modelMargin).toBeCloseTo(1 / 6, 6)
    expect(view.modelStatus).toBe('below_target')
    expect(view.fullStatus).toBe('loss')
  })
})

describe('GPT Auto policy rules', () => {
  it('separates warnings from route modifications, including legacy rows', () => {
    const decision: GPTAutoPolicyDecision = {
      policy_version: 'shadow-v1', status: 'accepted', jev_model: 'gpt-6-luna', jev_effort: 'low', final_model: 'gpt-6-luna', final_effort: 'low', candidates: [],
      adjustments: [{ rule: 'margin_below_target', to: 'gpt-6-luna' }, { rule: 'effort_unsupported', from: 'none', to: 'low' }],
      warnings: [{ rule: 'low_model_confidence' }],
    }
    const split = splitPolicyRules(decision)
    expect(split.adjustments.map((a) => a.rule)).toEqual(['effort_unsupported'])
    expect(split.warnings.map((w) => w.rule)).toEqual(['low_model_confidence', 'margin_below_target'])
  })
})
