import { useI18n } from 'vue-i18n'

/** Translates GPT-Auto codes, falling back to the raw code for unknown values. */
export function useGPTAutoLabels() {
  const { t, te } = useI18n()
  const lookup = (namespace: string, key: string | undefined | null) => {
    if (!key) return '—'
    const path = `gptAuto.${namespace}.${key}`
    return te(path) ? t(path) : key
  }
  return {
    effort: (key?: string | null) => lookup('efforts', key),
    category: (key?: string | null) => lookup('categories', key),
    rule: (key?: string | null) => (key ?? '').split(',').map((part) => lookup('rules', part)).join(' · '),
    evaluationStatus: (key?: string | null) => lookup('evaluationStatus', key),
    decisionStatus: (key?: string | null) => lookup('decisionStatus', key),
    errorClass: (key?: string | null) => lookup('errorClasses', key),
    endpoint: (key?: string | null) => lookup('endpoints', key),
    counter: (key?: string | null) => lookup('counters', key),
    costSource: (key?: string | null) => lookup('costSources', key),
    outputSource: (key?: string | null) => lookup('outputSources', key),
    complexity: (key?: string | null) => (key === '' || key === undefined || key === null ? '—' : lookup('complexityLevels', key)),
  }
}
