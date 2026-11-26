// Вклад компонентов (контур доводки): GET /api/evaluations/contribution (открыто), фоновый прогон и режим «без
// моделей» (бэк-офис). Форма — backend/out/eval_contribution.go.
import { http, type RequestOptions } from './http'
import type { EvalChange, EvalJob, EvalMetrics } from './types'

export type ContributionSet = 'test' | 'tune' | 'regression'

export interface ContributionCost {
  llm_rub_per_1k: number
  llm_calls: number
  cache_hits: number
  route_d_share: number
  cold_llm_rub_per_1k: number | null
  embed_queries: number
  rerank_queries: number
}

export interface ContributionRow {
  id: string
  kind: 'ladder' | 'single_change'
  label: string
  config: string
  off: { llm: boolean; vectors: boolean; rerank: boolean }
  against: string
  metrics: EvalMetrics
  cost: ContributionCost
  regressions: number
  improvements: number
  regression_head: EvalChange[]
  improvement_head: EvalChange[]
  duration_ms: number
}

export interface ContributionCheck {
  id: string
  label: string
  passed: boolean
  value: string
  blocking: boolean
}

export interface ContributionDecision {
  component: 'rerank' | 'vectors' | 'llm'
  label: string
  delta_ndcg: number | null
  delta_p95_ms: number
  improvements: number
  regressions: number
  verdict: 'keep' | 'remove' | 'borderline'
  text: string
  preliminary: boolean
}

export interface ContributionRun {
  run_id: string
  set: ContributionSet
  set_hash: string
  queries: number
  commit: string
  started_at: string
  duration_ms: number
  rules_version: number
  catalog_version: string
  rows: ContributionRow[]
  checks: ContributionCheck[]
  decisions: ContributionDecision[]
  parse_cost_rub: number
  parse_cost_source: 'measured' | 'assumed' | 'request'
  notes: string[]
  repro: string
}

export interface VectorsState {
  mode: 'on' | 'off'
  online: boolean
  changed_at: string | null
  offline: boolean
  banner: string | null
}

export interface ModelsState {
  llm: { mode: 'on' | 'off'; online: boolean; banner: string | null }
  vectors: VectorsState
}

export interface ContributionResponse {
  set: ContributionSet
  run: ContributionRun | null
  running: EvalJob | null
  last_job: EvalJob | null
  modes: ModelsState
  notes: string[]
}

export const getContribution = (set: ContributionSet, opts?: RequestOptions) =>
  http.get<ContributionResponse>(`/evaluations/contribution?set=${encodeURIComponent(set)}`, opts)

export const runContribution = (set: ContributionSet) =>
  http.post<EvalJob>('/evaluations/contribution/run', { set })

export const setOfflineMode = (mode: 'on' | 'off') =>
  http.post<ModelsState>('/admin/offline-mode', { mode })
