import type { GPTAutoContextDiagnostics, GPTAutoContextInfo, GPTAutoContextSection, GPTAutoContextSelectionStep } from '@/api/admin/gptAuto'

// View helpers for context-diagnostics-v1. They only reshape what the server
// recorded; when a field is missing they return null instead of a guess.

export type DiagnosticsAvailability = 'available' | 'context_only' | 'none'

/**
 * available: the decision carries diagnostics.
 * context_only: state-v3 extraction metadata without diagnostics (created
 *   before diagnostics were enabled).
 * none: no context metadata at all (state v1/v2).
 */
export function diagnosticsAvailability(context?: GPTAutoContextInfo | null): DiagnosticsAvailability {
  if (context?.diagnostics?.diagnostics_version) return 'available'
  return context ? 'context_only' : 'none'
}

export type BudgetSegmentKey = 'latest' | 'continues' | 'original' | 'agent' | 'recent' | 'unused'

export interface BudgetSegment {
  key: BudgetSegmentKey
  chars: number
  /** Share of the configured budget, 0–1. */
  share: number
}

const segmentSections: Record<Exclude<BudgetSegmentKey, 'unused'>, GPTAutoContextSection[]> = {
  latest: ['latest_user_request'],
  continues: ['continues_request'],
  original: ['original_request', 'history_summary'],
  agent: ['agent_activity'],
  recent: ['recent_context'],
}

/**
 * Splits the character budget by what the builder actually charged to each
 * section. Returns null without diagnostics or without a positive budget.
 */
export function contextBudgetSegments(context?: GPTAutoContextInfo | null): BudgetSegment[] | null {
  const steps = context?.diagnostics?.selection
  const budget = context?.budget_chars ?? 0
  if (!steps || !(budget > 0)) return null
  const charsOf = (sections: GPTAutoContextSection[]) =>
    steps.filter((step) => sections.includes(step.section)).reduce((sum, step) => sum + step.chars, 0)
  const segments: BudgetSegment[] = (Object.keys(segmentSections) as Exclude<BudgetSegmentKey, 'unused'>[]).map((key) => {
    const chars = charsOf(segmentSections[key])
    return { key, chars, share: chars / budget }
  })
  const unused = Math.max(budget - (context?.used_chars ?? 0), 0)
  segments.push({ key: 'unused', chars: unused, share: unused / budget })
  return segments
}

/** Timeline rows: every step the builder ran except those that did not apply. */
export function contextTimeline(diagnostics?: GPTAutoContextDiagnostics | null): GPTAutoContextSelectionStep[] {
  return (diagnostics?.selection ?? []).filter((step) => step.status !== 'not_applicable')
}

export type ContextWarning =
  | 'context_truncated'
  | 'continues_task'
  | 'task_unresolved'
  | 'tool_failures'
  | 'previous_response_unavailable'
  | 'external_history_unavailable'
  | 'images_not_included'
  | 'context_insufficient'
  | 'not_scanned'

/** Neutral, evidence-based warnings derived from recorded facts only. */
export function contextWarnings(context?: GPTAutoContextInfo | null): ContextWarning[] {
  const diagnostics = context?.diagnostics
  if (!context || !diagnostics) return []
  if (diagnostics.scan_status !== 'scanned') return ['not_scanned']
  const limitations = context.limitations ?? []
  const warnings: ContextWarning[] = []
  if (diagnostics.context_truncated) warnings.push('context_truncated')
  const resolution = diagnostics.follow_up?.resolution
  if (resolution === 'linked_request' || resolution === 'agent_activity_only') warnings.push('continues_task')
  if (resolution === 'unresolved') warnings.push('task_unresolved')
  if ((diagnostics.agent?.tool_failures_detected ?? 0) > 0) warnings.push('tool_failures')
  if (limitations.includes('history_by_reference')) warnings.push('previous_response_unavailable')
  if (limitations.includes('item_references') || limitations.includes('history_compacted')) warnings.push('external_history_unavailable')
  if (limitations.includes('images_not_forwarded')) warnings.push('images_not_included')
  if (diagnostics.completeness === 'important_unavailable') warnings.push('context_insufficient')
  return warnings
}

/** Abbreviated hash for display; empty input stays empty. */
export function shortHash(hash?: string | null, length = 10): string {
  return hash ? hash.slice(0, length) : ''
}
