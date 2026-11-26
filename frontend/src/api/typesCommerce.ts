// Типы API контура коммерции для бэк-офиса — по DTO бэкенда
// (backend/out/margin.go, relations.go, commerce_eval.go, agent.go). Отдельный файл, чтобы не пересекаться с типами
// витрины в types.ts. Необязательные поля сервер может не прислать — экран тогда не показывает строку.
import type { Proposal } from './types'

// ── Маржа ──

/** Версия политики маржи; версия 0 — исходная (вес контура поиска). */
export interface MarginPolicyVersion {
  version: number
  weight: number
  /** default | human | rollback | agent */
  source?: string
  author?: string
  comment?: string
  created_at: string | null
  categories?: Record<string, number>
  proposal_id?: string | null
}

export interface MarginPolicy {
  version: number
  weight: number
  /** Вес контура поиска (0,05): относительно него считаются изменения качества. */
  base_weight?: number
  /** Положения рычага по возрастанию: [0, 0.03, 0.05, 0.08, 0.12, 0.15]. */
  positions: number[]
  guards: Record<string, unknown>
  epsilon: number
  max_shift: number
  /** Упор по последней симуляции; null — симуляции ещё не было. */
  stop_weight?: number | null
  synthetic: boolean
  /** Флаг FEATURE_MARGIN_POLICY: выключен — политика в поиске не применяется. */
  enabled?: boolean
  versions?: MarginPolicyVersion[]
  /** Веса по категориям поверх общего; {} — нет. */
  categories?: Record<string, number>
  category_names?: Record<string, string>
}

/** Позиция выдачи в «было / стало». */
export type MarginRankItem =
  | string
  | { product_id: string; title?: string; grade?: number | null; margin?: number; moved?: number }

export interface MarginAffected {
  query_id?: string
  query: string
  type?: string
  ndcg_before?: number | null
  ndcg_after?: number | null
  /** Изменение NDCG@10 запроса к выдаче контура поиска. */
  ndcg_delta?: number
  top1_changed?: boolean
  top3_changed?: boolean
  /** top1 | top3 | worst — почему запрос в списке (если сервер присылает). */
  reason?: string
  guards?: string[]
  before: MarginRankItem[]
  after: MarginRankItem[]
}

export interface MarginSimPoint {
  weight: number
  /** Среднее изменение NDCG@10 к выдаче контура поиска (вес 0,05). */
  ndcg_delta_mean: number
  /** Худший дециль изменения NDCG@10. */
  ndcg_delta_p10: number
  /** Доля размеченных запросов, где политика без ε теряет больше ε, 0..1. */
  share_over_eps: number
  margin_index: number | null
  margin_index_relevant: number | null
  top1_changed: number
  top3_changed: number
  exact_top1_changed?: number
  guards_fired: number
  guards_by_kind?: Record<string, number>
  epsilon_rollbacks?: number
  queries_with_shifts?: number
  /** Положение доступно (не за упором). */
  available?: boolean
  ndcg10?: number
  affected: MarginAffected[]
}

export interface MarginSimulateResponse {
  items: MarginSimPoint[]
  /** Упор вверх от 0,05: последнее положение, где share_over_eps = 0. */
  stop_weight: number
  /** То же вниз от 0,05. */
  min_weight?: number
  set?: string
  set_hash?: string
  queries?: number
  /** Размеченных запросов — на них считается оценка. */
  judged?: number
  epsilon?: number
  base_ndcg10?: number
  base_weight?: number
  current_weight?: number
  policy_version?: number
  catalog_version?: string
  rules_version?: number
  synthetic?: boolean
  cached?: boolean
  duration_ms?: number
  note?: string
  ran_at?: string
}

/** GET /api/margin/simulate/status — базовая выдача собирается (503 у simulate). */
export interface MarginSimStatus {
  running: boolean
  done: number
  total: number
}

export interface MarginApplied {
  version: number
  weight?: number
  version_before?: number
  weight_before?: number
  rolled_back_to?: number
  categories?: Record<string, number>
  categories_before?: Record<string, number>
  proposal_id?: string | null
}

// ── Связи ──

export interface Relation {
  id: string
  from_family: string
  to_family: string
  from_product?: string | null
  to_product?: string | null
  /** Подписи концов связи от сервера. */
  from_name?: string
  to_name?: string
  from_title?: string
  to_title?: string
  /** Сервер пометил «проверить» (старше stale_after_days без проверки). */
  needs_review?: boolean
  type: string
  note?: string
  requirement?: string
  compatibility?: string
  conditions?: string[]
  quantity_strategy?: string
  source?: string
  source_url?: string
  evidence_level?: string
  review_status?: string
  reviewed_at?: string | null
  upgrade_adds?: string[]
}

export interface RelationsResponse {
  version: number
  /** false — версий ещё нет, справочник = каталог. */
  materialized?: boolean
  catalog_version?: string
  stale_after_days?: number
  total?: number
  items: Relation[]
}

/** Изменённая связь: целиком (правка агента) или парой before/after (разница версий). */
export type RelationUpdate = Relation | { before?: Relation | null; after: Relation }

export interface RelationPatch {
  add?: Relation[]
  /** id удаляемых (правка агента) или связи целиком (разница версий). */
  remove?: (string | Relation)[]
  update?: RelationUpdate[]
}

export interface RelationVersion {
  version: number
  created_at: string
  author?: string
  source?: string
  description?: string
  proposal_id?: string | null
  diff?: RelationPatch | null
  relations?: number
  catalog_version?: string
  archived?: boolean
}

export interface RelationVersionsResponse {
  /** 0 — версий нет, справочник = каталог. */
  active: number
  items: RelationVersion[]
}

export interface RelationRollbackResponse {
  version: number
  rolled_back_to: number
}

// ── Коммерческая лестница ──

export type CommerceMetricKey =
  | 'compat_violations'
  | 'required_recall'
  | 'coverage'
  | 'block_precision_judge'
  | 'mpn_top1'
  | 'units_parse_acc'
  | 'exact_top1'
  | 'ndcg10'
  | 'margin_index'

export type CommerceMetrics = Partial<Record<CommerceMetricKey, number | null>> & {
  errors?: number
}

/** Пара «якорь → кандидат» разметки блока (companion_pairs.json). */
export interface CompanionPairRef {
  anchor_product: string
  candidate_product: string
  class: string
  query?: string
}

export interface CommerceRegression {
  query_id?: string
  text: string
  type?: string
  against?: string
  ndcg_from?: number | null
  ndcg_to?: number | null
  ndcg_delta?: number
  reasons?: string[]
}

export interface CommerceConfigRun {
  /** C0 | C1 | C2 | C3 | C0+companions | C3-hardware … */
  config: string
  /** У прогона с одним изменением: от какой конфигурации и что изменено (+companions, -hardware). */
  base?: string
  changed?: string
  against?: string
  rules_version?: number
  catalog_version?: string
  metrics: CommerceMetrics
  block?: { violations?: CompanionPairRef[] | null; [k: string]: unknown } | null
  regressions?: CommerceRegression[] | null
  /** Шаг ещё не готов — числа нет, строка с пометкой. */
  ready?: boolean
}

export interface CommerceResponse {
  run_id?: string
  set?: string
  set_hash?: string
  queries?: number
  pairs?: number
  commit?: string
  started_at?: string
  duration_ms?: number
  repro?: string
  synthetic: boolean
  margin_weight?: number
  /** Какие компоненты контура коммерции готовы к сравнению. */
  ready?: { hardware?: boolean; companions?: boolean; margin?: boolean }
  ladder: CommerceConfigRun[]
  single_changes: CommerceConfigRun[]
  notes?: string[] | null
  judge_agreement?: { agreement: number; n: number } | null
}

// ── Предложение relation_patch ──

/** Метрики блока на размеченных парах (services.CompanionScore). */
export interface CompanionsEval {
  required_recall?: number | null
  compat_violations?: number | null
  coverage?: number | null
  block_precision_judge?: number | null
}

/** Прогон правки связей (evaluate_companions). */
export interface RelationPatchEval {
  resolved?: boolean
  findings_before?: number
  findings_after?: number
  new_findings?: unknown[] | null
  block?: {
    pairs?: number
    before?: CompanionsEval
    after?: CompanionsEval
    enabled?: boolean
  } | null
  regression?: {
    set?: string
    total: number
    worse: number
    better?: number
    block_changed?: number
  } | null
  skipped?: string[] | null
  passed: boolean
  reasons?: string[] | null
  /** Старый вид: companions.before/after. */
  companions?: { before?: CompanionsEval; after?: CompanionsEval } | null
}

/** Proposal с полями контура коммерции. */
export type CommerceProposal = Proposal & {
  evidence_level?: string | null
  technical_status?: 'verified' | 'unverified' | 'failed' | string | null
  economic_status?: 'unknown' | 'estimated' | 'measured' | string | null
  priority_score?: number | null
}
