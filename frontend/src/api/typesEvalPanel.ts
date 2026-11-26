// Прогон панели по префиксам и учёт намерений:
// GET /api/evaluations?set=panel.

/** Доля: числитель, знаменатель, значение (null — знаменатель 0, не посчитано). */
export interface IntentRate {
  num: number
  den: number
  value: number | null
}

export interface IntentOutcome {
  outcome: 'commit' | 'suggestion' | 'product' | 'category' | 'vendor' | 'abandon' | string
  label: string
  count: number
  share: number | null
  converted: number
}

export interface IntentAnalytics {
  intents: number
  open: number
  outcomes: IntentOutcome[]
  conversion_legacy: IntentRate
  conversion_unified: IntentRate
  empty_results: IntentRate
  empty_suggestions: IntentRate
  sources: Record<string, number>
}

export interface PanelMetrics {
  prefixes: number
  errors: number
  suggest_n: number
  suggest_mrr5: number | null
  suggest_success5: number | null
  product_n: number
  product_mrr5: number | null
  product_success5: number | null
  saved_targets: number
  saved_keystrokes: number | null
  saved_share: number | null
  empty_suggest: number
  empty_suggest_rate: number | null
  product_prefixes: number
  family_dup_prefixes: number
  family_dup_rate: number | null
  distinct_families: number | null
  exact_checks: number
  exact_displaced: number
  suggestions_checked: number
  suggestions_to_empty: number
  to_empty_examples: string[]
  p50_ms: number | null
  p95_ms: number | null
  p50_ms_cached: number | null
  p95_ms_cached: number | null
  products_p95_ms: number | null
  partial: number
  llm_calls: number
  embed_calls: number
  rerank_calls: number
}

export interface PanelPrefixResult {
  id: string
  prefix: string
  type: string
  target_query: string
  target_family?: string
  suggestions: string[]
  families: string[]
  suggest_rank: number
  product_rank: number
  family_dup: boolean
  exact_ok: boolean | null
  ms: number
  error?: string
}

export interface PanelModeResult {
  mode: 'old' | 'new' | string
  label: string
  available: boolean
  reason?: string
  metrics: PanelMetrics
  by_type: Record<string, PanelMetrics>
  per_prefix: PanelPrefixResult[]
}

export interface PanelChange {
  id: string
  prefix: string
  type: string
  target: string
  section: 'suggestions' | 'products' | string
  old_rank: number
  new_rank: number
  kind: 'improvement' | 'regression' | string
}

export interface PanelSplit {
  late_from: string | null
  late_to: string | null
  late_targets: number
  note: string
}

export interface PanelEvalRun {
  run_id: string
  set: string
  set_hash: string
  prefixes: number
  commit: string
  catalog_version: string
  started_at: string
  duration_ms: number
  split: PanelSplit
  modes: PanelModeResult[]
  changes: PanelChange[]
  repro: string
  warnings: string[]
}

export interface PanelEvaluation {
  set: 'panel'
  run: PanelEvalRun | null
  intents: IntentAnalytics | null
  intents_days: number
}
