// GET /api/evaluations/feed — сводка «На данных заказчика» (backend out/eval_target.go). Все числа —
// из прогонов наборов «по цели» и трасс; чего не прогоняли — null или пустой список.
import type { CostsParam } from './types'

export interface FeedCatalog {
  /** feed | mock | unknown */
  source: string
  label: string
  offers: number
}

export interface FeedConfig {
  id: string
  label: string
  note: string
}

export interface FeedScore {
  set: string
  config: string
  matched: number
  total: number
  success1: number | null
  top4: number | null
  run_tail: string
}

/** Счёт группы запросов отложенного набора: сколько их и у скольких цель первым ответом у BM25 и у live. */
export interface FeedClosedPair {
  total: number
  bm25: number
  live: number
}

/** …и у скольких запросов полной конфигурации нужная страница в первых четырёх. */
export interface FeedClosedCount extends FeedClosedPair {
  top4: number
}

/** Строка по виду запросов; kind — ключ вида как в наборе («склонение», «транслит», …). */
export interface FeedClosedKind extends FeedClosedCount {
  kind: string
}

/** Последний прогон отложенного набора по видам запросов; счёт всего набора — в полях самого объекта. */
export interface FeedClosedDetail extends FeedClosedCount {
  /** На скольких запросах полная конфигурация вызывала языковую модель. */
  model_searches: number
  p50_ms: number
  kinds: FeedClosedKind[]
  /** Искажённое написание: склонение, транслит, опечатка, раскладка, ТЗ в словоформе. */
  spelling: FeedClosedCount
  /** Запросы с товарной целью: все виды, кроме сервисных и мусорных. */
  product: FeedClosedPair
}

export interface FeedFour {
  config: string
  sets: string[]
  answers: number
  families4_n: number
  skus4_n: number
  families4: number | null
  skus4: number | null
}

export interface FeedCostRun {
  /** cold | warm */
  mode: string
  sets: string[]
  searches: number
  route_d_label: number
  route_d_no_model: number
  p: number
  h: number
  c_llm: number | null
  llm_calls: number
  input_tokens: number
  output_tokens: number
  llm_rub: number
  p50_ms: number
  p95_ms: number
  llm_per_request_rub: number
  fixed_per_request_rub: number
  per_request_rub: number
  month_rub: number
  infra_share: number
  agent_share: number
  support_share: number
  llm_share: number
}

export interface FeedCost {
  formula: string
  requests: number
  params: CostsParam[]
  cold: FeedCostRun | null
  warm: FeedCostRun | null
  /** Для сравнения: журнал витрины и трафика на этом каталоге (search_events + llm_usage). */
  journal: FeedCostRun | null
  notes: string[]
}

export interface TargetCost {
  mode: string
  searches: number
  route_d: number
  route_d_no_model: number
  model_searches: number
  cache_searches: number
  llm_calls: number
  input_tokens: number
  output_tokens: number
  llm_rub: number
  trace_rub: number
  usage_joined: boolean
}

export interface TargetMetrics {
  answers: number
  judged: number
  success1: number | null
  success1_n: number
  top4: number | null
  top4_n: number
  mrr10: number | null
  pass: number | null
  pass_n: number
  families4: number | null
  families4_n: number
  skus4: number | null
  skus4_n: number
  cards4: number | null
  cards4_n: number
  hard_queries: number
  hard_violated: number
  violation_items: number
  cost?: TargetCost
}

export interface FeedRow {
  set: string
  config: string
  queries: number
  run_tail: string
  mode?: string
  metrics: TargetMetrics
  p50_ms: number
  p95_ms: number
  rub_per_query: number | null
}

/** Диалог режима подбора (контур подбора): последняя оценка полного подбора. */
export interface FeedGuide {
  dialogs: number
  runs: number
  task_all: number
  gate: boolean
  dialog_cost_rub: number
  dialog_llm_cost_rub: number
  dialogs_with_llm: number
  llm_calls: number
  ms_total_p95: number
  run_tail: string
}

export interface FeedSummary {
  catalog: FeedCatalog
  etalon: FeedScore | null
  closed: FeedScore[]
  /** null — прогона отложенного набора у BM25 или полной конфигурации нет. */
  closed_detail?: FeedClosedDetail | null
  four: FeedFour | null
  cost: FeedCost | null
  table: FeedRow[]
  configs: FeedConfig[]
  /** null — оценку подбора не прогоняли. */
  guide?: FeedGuide | null
  mock_runs: boolean
}
