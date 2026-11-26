// Типы API расширенных функций контура коммерции — по DTO
// бэкенда (backend/out/effect_commerce.go, out/analytics.go, services/margin_opportunity.go,
// services/agent_commerce_stats.go). Поля, которых сервер может не прислать, — необязательные: экран тогда не
// показывает строку.

// ── Доказательства предложений по покупкам ──

/** Строка готовой таблицы доказательств (evidence.table). */
export interface EvidenceRow {
  label: string
  value: string
}

/** Ступень проверки гипотезы (evidence.ladder). */
export interface LadderRung {
  no: number
  name: string
  ok: boolean
  skipped?: boolean
  text: string
}

/** Вердикт судьи другого семейства (evidence.judge). */
export interface JudgeVerdict {
  available: boolean
  objection: boolean
  reason?: string
  model?: string
}

/** Оценка приоритета (evidence.priority): ₽, «оценка приоритета, не прогноз». */
export interface PriorityInfo {
  label?: string
  score?: number
  formula?: string
  anchor_orders?: number
  attach_gap?: number
  companion_margin_rub?: number
}

/** Гипотеза предложения (evidence.hypotheses). */
export interface StatsHypothesis {
  no: number
  hypothesis?: string
  passed: boolean
  rejected_reason?: string
  ladder?: LadderRung[]
}

export interface CopurchaseEvidence {
  source?: string
  table?: EvidenceRow[]
  strength?: string
  ladder?: LadderRung[]
  judge?: JudgeVerdict
  priority?: PriorityInfo
  hypotheses?: StatsHypothesis[]
  finding_text?: string
  technical_note?: string
  economic_note?: string
  synthetic?: boolean
  bundle?: BundleCard
}

// ── Комплект (action: bundle, patch.bundle) ──

export interface BundleAnchor {
  product_id: string
  family_id?: string
  title: string
  price_rub?: number | null
  price_mode?: string
  margin?: number | null
}

export interface BundleItem {
  product_id: string
  family_id?: string
  title: string
  category?: string
  class?: string
  relation_id?: string
  relation_status?: 'approved' | 'proposed' | string
  compatibility?: string
  condition?: string
  quantity?: number
  price_rub?: number | null
  price_mode?: string
  margin?: number | null
  attach_rate?: number | null
  p_value?: number | null
  q_value?: number | null
  repeated?: boolean
}

export interface BundleCard {
  anchor: BundleAnchor
  items: BundleItem[]
  totals?: {
    price_rub?: number | null
    margin_rub?: number | null
    margin_pct?: number | null
    priced_all?: boolean
  }
}

// ── Предложение по марже (action: margin_policy) ──

export interface MarginProposalPatch {
  category_id?: string
  category?: string
  weight_before?: number
  weight: number
  global_weight?: number
  policy_version?: number
  simulate?: { set?: string; category?: string; positions?: number[] }
  apply?: { weight?: number; categories?: Record<string, number> }
}

/** Показатели положения рычага (как MarginSimPoint, без списка затронутых). */
export interface MarginPointBrief {
  weight: number
  ndcg10?: number
  ndcg_delta_mean?: number
  margin_index?: number | null
  margin_index_relevant?: number | null
  share_over_eps?: number
  top1_changed?: number
  top3_changed?: number
  exact_top1_changed?: number
  queries_with_shifts?: number
  guards_fired?: number
  available?: boolean
}

export interface MarginScope {
  category_id: string
  category: string
  queries_in_scope: number
  judged_in_scope: number
  current_weight?: number
  others?: string
}

export interface MarginProposalEval {
  set?: string
  set_hash?: string
  catalog_version?: string
  rules_version?: number
  policy_version?: number
  judged?: number
  base_ndcg10?: number
  epsilon?: number
  scope?: MarginScope | null
  before?: MarginPointBrief
  after?: MarginPointBrief
  margin_index_delta?: number
  margin_index_relevant_delta?: number
  ndcg_delta?: number
  passed?: boolean
  synthetic?: boolean
}

// ── Модель эффекта пакета (GET /api/effect/commerce) ──

export interface EffectCommerceInput {
  key: string
  label: string
  value: number | null
  default: number | null
  unit: string
  /** measured | synthetic | assumed | literature | request */
  source: string
  source_ref: string
  source_url?: string
  scenarios?: Record<string, number>
  hint: string
}

export interface EffectCommerceTerm {
  key: 'rescued' | 'upsell' | 'mix' | 'anchor_loss' | 'returns' | string
  label: string
  kind: 'mechanism' | 'deduction' | string
  gp_rub_month: number | null
  orders_month?: number | null
  formula: string
}

export interface EffectIdentityCheck {
  o0: number
  g0: number
  delta_o: number
  delta_g: number
  do_g0: number
  o0_dg: number
  do_dg: number
  sum: number
  direct: number
  terms: number
  ok: boolean
  epsilon: number
}

export interface EffectCommerceScenario {
  id: string
  label: string
  rule: string
  terms: EffectCommerceTerm[]
  delta_gp_rub_month: number
  complete: boolean
  check: EffectIdentityCheck | null
  uplift: number | null
}

export interface EffectCommerceResponse {
  days: number
  from: string
  to: string
  title: string
  note: string
  synthetic: boolean
  synthetic_keys: string[]
  formula: string[]
  identity: string
  inputs: EffectCommerceInput[]
  scenarios: EffectCommerceScenario[]
  pilot: string
  missing: string[]
  warnings: string[]
}

// ── Блок «Допродажи» аналитики ──

export interface CommerceTile {
  key: string
  label: string
  value: number | null
  /** share | count | rub */
  unit: string
  /** stand_events | offline_eval | catalog | agent | synthetic */
  source: string
  note: string
  list?: string
  link?: string
}

export interface CommerceListRow {
  id: string
  title: string
  detail: string
  query?: string
}

export interface CommerceList {
  key: string
  label: string
  total: number
  items: CommerceListRow[]
}

export interface AnalyticsCommerce {
  role: string
  title: string
  note: string
  synthetic: boolean
  tiles: CommerceTile[]
  lists: CommerceList[]
  effect?: EffectCommerceResponse | null
  missing: string[]
}
