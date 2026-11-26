// Контур автономии: автономный агент — триггеры, план, бюджет, итог запуска, настройки и память трёх слоёв.

export type RunTrigger = 'schedule' | 'spike' | 'catalog_change' | 'manual'

/** Кластер в плане запуска: модель выбрала его и объяснила почему. */
export interface RunPlanItem {
  cluster_id: string
  label: string
  score?: number
  why?: string
}

/** Последняя редакция плана (§14.3). */
export interface RunPlan {
  revision: number
  source: 'model' | 'fallback' | string
  summary: string
  clusters?: RunPlanItem[] | null
  tools?: string[] | null
  checks?: string[] | null
  dropped?: RunPlanItem[] | null
  change?: string | null
}

export interface BudgetAmounts {
  steps: number
  tokens: number
  cost_rub: number
  seconds: number
}

/** Бюджет запуска: лимиты и расход (у идущего запуска — на сейчас). */
export interface RunBudget {
  limits: BudgetAmounts
  used: BudgetAmounts
  exhausted?: string | boolean | null
}

/** Итог запуска счётчиками (§14.3). */
export interface RunOutcome {
  proposals: number
  tasks: number
  rollbacks: number
  drafts: number
  skipped_memory: number
  effect_checks: number
  helped: number
  no_effect: number
  harmed: number
  checked: number
  stale: number
}

export interface RunUnprocessed {
  cluster_id: string
  label: string
  score?: number
  reason?: string
}

/** GET /api/agent/settings */
export interface AgentSettings {
  read_only: boolean
  mode_label: string
  schedule: { enabled: boolean; at: string; interval: string; days: number; next_at: string | null }
  spike: {
    enabled: boolean
    window: string
    min_searches: number
    empty_share: number
    reform_share: number
    query_freq: number
    cooldown: string
  }
  catalog_trigger: boolean
  max_parallel: number
  running: number
  budget: { steps: number; tokens: number; cost_rub: number; duration_sec: number }
  query_budget: { steps: number; tokens: number; cost_rub: number; duration_sec: number }
  /** Режим, расписание и пороги одной строкой — для шапки «Запусков». */
  line: string
}

export type MemoryLayerId = 'decisions' | 'suppress' | 'episodes'

export interface MemoryDecision {
  id: string
  proposal_id: string
  run_id: string | null
  trigger: string
  cluster_label: string
  cluster_key: string
  diagnosis: string
  cause: string
  cause_label: string
  agent_reason: string
  proposal: string
  action: string
  decision: 'accepted' | 'rejected' | 'rolled_back' | 'suppress_removed' | string
  reason: string
  reason_label: string
  comment: string
  decided_by: string
  decided_by_label: string
  ts: string
}

export interface MemorySuppress {
  id: string
  cluster_key: string
  cluster_label: string
  scope: 'cluster' | 'patch' | string
  patch_keys: string[] | null
  diagnosis: string
  why: string
  reason: string
  comment: string
  decided_at: string
  proposal_id: string
  run_id: string
  return_condition: string
  failures_at: number
  return_failures: number
  current_failures: number
  catalog_at: string
  checked_at: string | null
  checked_run_id: string | null
  status: 'active' | 'lifted' | 'removed' | string
  lifted_run_id: string | null
  lifted_what: string
  removed_reason: string
  removed_by: string
  removed_at: string | null
}

export interface MemoryEpisode {
  id: string
  run_id: string
  trigger: string
  kind: 'investigation' | 'effect_check' | string
  cluster_key: string
  cluster_label: string
  outcome: string
  diagnosis: string
  cause: string
  cause_label: string
  proposal_id: string | null
  effect: 'helped' | 'no_effect' | 'harmed' | null | string
  details: Record<string, unknown> | null
  note: string
  ts: string
}

/** GET /api/agent/memory?layer= — заполнен только запрошенный слой (пустой слой сервер не присылает). */
export interface MemoryLayerResponse {
  layer: MemoryLayerId
  decisions?: MemoryDecision[] | null
  suppress?: MemorySuppress[] | null
  episodes?: MemoryEpisode[] | null
}

/** Откат, предложенный агентом: на какое предложение и версию. */
export interface RollbackOf {
  proposal_id: string
  config_version: number | null
  label: string
}
