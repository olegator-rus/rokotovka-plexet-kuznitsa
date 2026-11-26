// Типы HTTP API — строго по контракту API:
// поле меняется сначала там, потом здесь. Необязательные блоки ответа (`correction`, `clarify`, `empty`,
// `llm`) сервер может не прислать или прислать null — типы допускают оба варианта.

export type ConfigId = 'B0' | 'B1' | 'B2' | 'B3' | 'B4'
export type LlmMode = 'on' | 'off'
export type ClientSegment = 'smb' | 'enterprise' | 'gov' | 'individual'

/** GET /api/meta */
export interface MetaClient {
  id: string
  name: string
  segment?: ClientSegment | string
  /** Подсказка к клиенту: «SMB, покупал антивирус». */
  hint?: string
}

export interface Meta {
  llm_mode: LlmMode
  catalog_version: string
  rules_version: number
  configs: ConfigId[]
  default_config: ConfigId
  clients: MetaClient[]
}

/** Условие запроса, снятое пользователем крестиком на чипе. */
export interface ChipRef {
  field: string
  value: string
}

/** POST /api/search — тело запроса. */
export interface SearchRequest {
  query: string
  client_id: string | null
  session_id: string
  config: ConfigId
  as_typed: boolean
  removed_chips: ChipRef[]
  /** Ответ на уточняющий вопрос: { атрибут: выбранное значение }. */
  clarify: Record<string, string>
  limit: number
}

export interface Correction {
  from: string
  to: string
  confidence: 'high' | 'medium'
  applied: boolean
}

export interface Chip {
  field: string
  value: string
  label: string
  /** true — учтено строго (обязательное условие), false — по возможности. */
  strict: boolean
}

export interface ClarifyOption {
  value: string
  label: string
}

export interface Clarify {
  attribute: string
  question: string
  options: ClarifyOption[]
}

export type ItemGroup = 'exact' | 'match' | 'analog'

export interface SearchItemBase {
  product_id: string
  family_id: string
  title: string
  vendor: string
  category: string
  license_line: string
  price_rub: number | null
  price_on_request: boolean
  group: ItemGroup
  matches_query: boolean
  purchased_before: boolean
  final: number
}

export interface SearchItem extends SearchItemBase {
  /** Другие лицензии того же семейства (те же поля, без вложенных вариантов). */
  variants: SearchItemBase[]
}

export interface Relaxed {
  field: string
  message: string
}

export interface EmptyInfo {
  message: string
  show_manager: boolean
}

export interface SearchResponse {
  request_id: string
  query: string
  normalized: string
  config: ConfigId
  correction?: Correction | null
  chips: Chip[]
  clarify?: Clarify | null
  total: number
  items: SearchItem[]
  relaxed: Relaxed[]
  /** Только когда точного товара нет; `items` при этом может содержать аналоги. */
  empty?: EmptyInfo | null
  related_queries: string[]
  trace: Trace
}

/** §4 трасса */
export type Route = 'A' | 'B' | 'C' | 'D'
export type ParseSource = 'rules' | 'cache' | 'llm'

export interface NormalizeChange {
  kind: 'layout' | 'typo' | 'alias' | 'clean' | string
  from: string
  to: string
}

export interface FoundAttribute {
  field: string
  value: string
  strict: boolean
  source: string
}

export interface RankedCandidate {
  product_id: string
  rank: number
  score: number
}

export interface ScoreParts {
  relevance: number
  popularity: number
  margin: number
  context: number
}

/**
 * Шаг трассы. Общие поля — имя, время, пропуск; остальное зависит от шага (normalize → changes,
 * attributes → found/unresolved, route → route/signals, bm25/dense → top, fusion → k,
 * business → weights, relaxation → ladder). Неизвестные поля показываются как есть.
 */
export interface TraceStep {
  name: string
  ms: number
  skipped?: boolean
  /** Шаг завершился ошибкой, дальше пошёл запасной путь («Состояния экранов»). */
  error?: string
  /** Причина пропуска: not_implemented, not_in_config… (сервер присылает сверх контракта). */
  reason?: string
  changes?: NormalizeChange[]
  found?: FoundAttribute[]
  unresolved?: unknown[]
  route?: Route
  signals?: string[]
  top?: RankedCandidate[]
  k?: number
  weights?: Partial<ScoreParts>
  ladder?: unknown[]
  [extra: string]: unknown
}

export interface TraceItem {
  product_id: string
  final: number
  group: ItemGroup
  parts: ScoreParts
  bm25_rank: number | null
  dense_rank: number | null
}

export type RejectReason = 'license_metric_mismatch' | 'below_threshold' | 'version_mismatch'

export interface Rejected {
  product_id: string
  reason: RejectReason | string
  detail?: string
}

export interface Trace {
  request_id: string
  route: Route
  parse_source: ParseSource
  config: ConfigId
  /** Конфигурация, которая реально отработала, если сервер понизил запрошенную (сверх контракта). */
  effective_config?: ConfigId
  rules_version: number
  total_ms: number
  cost_rub: number
  steps: TraceStep[]
  items: TraceItem[]
  rejected: Rejected[]
  llm: Record<string, unknown> | null
}

/** GET /api/suggest */
export type SuggestType = 'query' | 'product' | 'vendor' | 'category'

export interface SuggestItem {
  text: string
  id: string | null
  /** [начало, конец) совпавшей части в `text`. */
  highlight?: [number, number] | null
}

export interface SuggestGroup {
  type: SuggestType
  label: string
  items: SuggestItem[]
}

export interface SuggestResponse {
  groups: SuggestGroup[]
}

/** POST /api/events */
export type EventType =
  | 'click'
  | 'add_to_cart'
  | 'order'
  | 'price_request'
  | 'manager_request'
  | 'suggest_pick'
  | 'correction_undo'
  | 'chip_remove'
  | 'clarify_answer'

export interface SearchEvent {
  type: EventType
  request_id: string | null
  session_id: string
  client_id: string | null
  product_id: string | null
  position: number | null
  payload: Record<string, unknown>
}

// ── «До / После»: GET /api/evaluations, POST /api/evaluations/run (backend/out/eval.go) ──
// Доли — 0..1; null — метрику не на чем считать (в наборе нет запросов нужного типа).

export interface EvalLatency {
  count: number
  p50: number
  p95: number
}

export interface EvalMetrics {
  queries: number
  empty_rate: number | null
  empty_rate_weighted: number | null
  no_relevant_top10: number | null
  relevant_top10: number | null
  ndcg10: number | null
  mrr10: number | null
  recall50: number | null
  exact_accuracy: number | null
  rescued_rate: number | null
  false_rescue_rate: number | null
  violation_rate: number | null
  violation_items: number
  /** Запросы, на которых поиск вернул ошибку (в метрики не входят). */
  errors: number
  latency: EvalLatency
  by_route?: Record<string, EvalLatency>
}

/** Запрос, по которому конфигурация хуже (или лучше) предыдущей в лестнице прогона. */
export interface EvalChange {
  query_id: string
  text: string
  type: string
  freq: number
  /** С какой конфигурацией сравнивали. */
  against: string
  ndcg_from: number | null
  ndcg_to: number | null
  ndcg_delta: number
  /** ndcg | lost_relevant | became_empty | lost_exact | new_violation | false_rescue (и обратные). */
  reasons: string[]
}

/** Сохранённый результат одной конфигурации (последний прогон). */
export interface EvaluationView {
  run_id: string
  set: string
  set_hash: string
  queries: number
  config: ConfigId
  rules_version: number
  rules_override: boolean
  catalog_version: string
  commit: string
  /** Строка воспроизводимости: набор@хэш · правила vN · каталог · коммит. */
  repro: string
  created_at: string
  duration_ms: number
  metrics: EvalMetrics
  by_type: Record<string, EvalMetrics>
  against?: string
  regressions: EvalChange[]
  improvements: EvalChange[]
}

export type EvalJobStatus = 'running' | 'done' | 'failed'

export interface EvalJob {
  id: string
  set: string
  configs: string[]
  status: EvalJobStatus
  started_at: string
  finished_at?: string
  run_id?: string
  error?: string
  /** Обработано запросов и всего — сервер пока не присылает; без них показываем время прогона. */
  done?: number
  total?: number
}

export interface EvaluationsResponse {
  set: string
  configs: EvaluationView[]
  running: EvalJob | null
  last_job: EvalJob | null
}

export interface EvalRunRequest {
  set: string
  configs: ConfigId[]
}

// ── «Аналитика»: GET /api/analytics?days=30&role=… (backend/out/analytics.go) ──

export interface AnalyticsTiles {
  searches: number
  empty: number
  empty_rate: number | null
  no_click: number
  no_click_rate: number | null
  orders: number
  order_rate: number | null
  pending_proposals: number
}

export interface FunnelStepRaw {
  step: 'searches' | 'clicks' | 'carts' | 'orders' | string
  label: string
  count: number
  /** Доля от поисков. */
  rate: number | null
}

export interface ProblemQuery {
  query: string
  freq: number
  empty: number
  no_click: number
  reformulated: number
  clicks: number
  orders: number
  failures: number
  /** Что происходит чаще всего: empty | reformulated | no_click. */
  outcome: string
  outcome_label: string
  diagnosis: string
  /** Предложение агента по запросу — появится с SearchOps. */
  proposal_id?: string | null
}

export interface AnalyticsResponse {
  role: string
  days: number
  from: string
  to: string
  tiles: AnalyticsTiles
  funnel: FunnelStepRaw[]
  problems: ProblemQuery[]
  /** Откуда события: live и mock_log (импорт исторических логов). */
  sources: Record<string, number>
}

// ── «Экономика»: GET /api/costs?limit=&offset= (backend/out/costs.go) ──

export interface CostsPrice {
  model: string
  input_rub_per_mtok: number
  output_rub_per_mtok: number
}

export interface CostsPricing {
  version: string
  date: string
  currency: string
  prices: CostsPrice[]
}

export interface CostsSummary {
  searches: number
  /** Стоимость вызовов модели, привязанных к поискам, на один поиск. */
  avg_cost_rub: number | null
  share_without_llm: number | null
  llm_parses: number
  cache_hits: number
  /** cache / (cache + llm) среди разборов маршрута D. */
  cache_hit_share: number | null
  search_llm_cost_rub: number
  llm_calls: number
  llm_cost_rub: number
  input_tokens: number
  output_tokens: number
  avg_llm_latency_ms: number | null
  /** Вызовы модели из прогонов оценки (request_id eval-…): в стоимость запроса не входят. */
  eval_calls: number
  eval_cost_rub: number
  agent_runs: number
  agent_run_avg_cost_rub: number | null
}

export interface CostRouteRow {
  route: string
  count: number
  share: number | null
  avg_ms: number | null
  avg_cost_rub: number | null
  llm_parses: number
  cache_hits: number
}

export type LlmCallKind = 'search' | 'eval' | 'agent' | 'other'

export interface LlmCall {
  id: string
  /** search — поиск покупателя, eval — прогон оценки, agent — запуск агента, other — ручной прогон. */
  kind: LlmCallKind | string
  created_at: string
  request_id: string | null
  agent_run_id: string | null
  provider: string
  model: string
  input_tokens: number
  output_tokens: number
  cached_tokens: number
  latency_ms: number
  cost_rub: number
  pricing_version: string
  status: string
}

export interface CostsCalls {
  total: number
  limit: number
  offset: number
  items: LlmCall[]
}

export interface CostsResponse {
  llm_mode: LlmMode
  pricing: CostsPricing
  summary: CostsSummary
  routes: CostRouteRow[]
  calls: CostsCalls
}
