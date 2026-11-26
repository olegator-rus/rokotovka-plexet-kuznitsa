// Типы HTTP API — строго по контракту API:
// поле меняется сначала там, потом здесь. Необязательные блоки ответа (`correction`, `clarify`, `empty`,
// `llm`) сервер может не прислать или прислать null — типы допускают оба варианта.
import type {
  Companion,
  CompanionBlock,
  Facet,
  Features,
  ItemCompatibility,
  ItemKind,
  P2EventType,
  PriceMode,
  SearchFilter,
  Solution,
} from './typesP2'

/** live — витрина: B3 на действующей (последней принятой) версии правил; лестница оценки — B0–B4. */
export type ConfigId = 'live' | 'B0' | 'B1' | 'B2' | 'B3' | 'B4'
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
  /** Подписанный сервером токен сессии покупателя: идёт в session_id поиска и событий. */
  session?: string
  /** Контур коммерции: флаги блоков и что в данных придумано (плашка «Синтетические данные»). */
  features?: Features
  synthetic?: string[]
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
  /** compare — поиск из «Одного запроса»: пишется в журнал, но не входит в аналитику и стоимость. */
  source?: 'live' | 'compare'
  /** Контур коммерции: фасеты оборудования; пустые не отправляются. */
  filters?: SearchFilter[]
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
  /** Контур коммерции: оператор и единица числового условия («Память ≥ 32 ГБ»); поле mpn — чип «Парт-номер». */
  op?: string
  unit?: string
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
  /** Контур коммерции: тип, парт-номер, режим цены, совместимость с целевой моделью, условно обязательное. */
  kind?: ItemKind | string
  mpn?: string
  price_mode?: PriceMode | string
  compatibility?: ItemCompatibility | null
  required?: Companion[]
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
  /** «С этим покупают»: связи complement для первой позиции, до 3 семейств; B0–B2 — пусто. */
  complements?: Complement[]
  /** Контур коммерции: «Дополните решение» (null — блока нет), шаблон решения, фасеты категории. */
  companions?: CompanionBlock | null
  solution?: Solution | null
  facets?: Facet[]
  trace: Trace
}

export interface Complement {
  family_id: string
  family_name: string
  product_id: string
  title: string
  vendor: string
  category: string
  license_line: string
  price_rub: number | null
  price_on_request: boolean
  /** Пояснение связи из справочника. */
  note: string
}

/** GET /api/traces/{request_id} — сохранённая трасса поиска и исходный запрос. */
export interface TraceRecord {
  request_id: string
  query: string
  normalized: string
  config: ConfigId | string
  route: Route | string
  source: string
  client_id: string | null
  created_at: string
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
  | P2EventType

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
  /** Набор holdout_week: исторические поиски окна (с весом частоты), из них размеченных. */
  searches?: number
  judged_searches?: number
  /** Доля размеченных поисков с релевантным в топ-10, с весом частоты. */
  relevant_top10_weighted?: number | null
  /** Поиски, где у предыдущей конфигурации прогона релевантного в топ-10 не было, а у этой есть (и наоборот). */
  rescued_searches?: number
  lost_searches?: number
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
  /** Выполнено поисков прогона и всего (запросы × конфигурации); total 0 — набор ещё не загружен. */
  done?: number
  total?: number
}

/** Последний прогон LLM-судьи на наборе: согласие с ручной разметкой и NDCG по обеим оценкам. */
export interface JudgeAgreement {
  run_id: string
  set: string
  set_hash: string
  model: string
  prompt_version: string
  configs: string[]
  created_at: string
  pairs: number
  judged: number
  /** До какой глубины выдачи пул пар полный (NDCG@depth). */
  depth: number
  exact: number
  within1: number
  kappa: number
  /** 4×4: строка — разметка, столбец — судья. */
  confusion: number[][]
  by_config: { config: string; ndcg_labels: number; ndcg_judge: number }[]
  same_order: boolean
  calls: number
  cost_rub: number
  duration_ms: number
}

export interface EvaluationsResponse {
  set: string
  configs: EvaluationView[]
  running: EvalJob | null
  last_job: EvalJob | null
  judge_agreement?: JudgeAgreement | null
  /** Честность правок агента в столбце B4 этого набора; null — нет B3 или B4. */
  agent_credit?: EvalAgentCredit | null
  /** Только holdout_week: доля спасённых для вилки эффекта (как measured.rescued в /api/effect). */
  rescued?: EffectRescued | null
}

export interface EvalAgentCredit {
  /** Все принятые правки B4 — из запусков после разреза журнала. */
  counted: boolean
  /** В B4 есть правки запусков до разреза: его числа — не независимая оценка. */
  pre_holdout: boolean
  note: string
  rules_version: number
}

/** Доля спасённых поисков на реплее отложенной недели (backend/out/effect.go EffectRescued). */
export interface EffectRescued {
  set: string
  set_hash: string
  source: string
  baseline: string
  /** До какой конфигурации засчитано: B3 (только поиск) или B4 (поиск и честные правки агента). */
  config: ConfigId
  run_baseline: string
  run_config: string
  run_agent: string
  rules_version: number
  agent_rules_version: number
  weight: string
  failed: number
  rescued: number
  failed_queries: number
  rescued_queries: number
  share: number
  search_share: number
  agent_share: number
  agent_counted: boolean
  agent_pre_holdout?: boolean
  agent_note: string
  ci_low: number
  ci_high: number
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
  proposal_status?: string | null
  proposal_diagnosis?: string | null
  /** Вывод агента: из предложения или итога кластера последних запусков. */
  agent?: AgentVerdict
}

export type AgentVerdictCode =
  'proposal' | 'answers' | 'closed' | 'no_offer' | 'earlier' | 'queued' | 'mostly_ok' | 'unseen'

export interface AgentVerdict {
  code: AgentVerdictCode | string
  label: string
  note?: string
  diagnosis?: string
  run_id?: string
  cluster_id?: string
  cluster_label?: string
}

/** GET /api/analytics/current: текущая выдача по проблемным запросам (поиск без журнала, действующие правила). */
export interface ProblemNow {
  query: string
  total: number
  exact: boolean
  not_in_catalog: boolean
  top_title: string
  route: string
  cached: boolean
  error?: string
}

export interface ProblemsNowResponse {
  days: number
  rules_version: number
  catalog_version: string
  checked_at: string
  items: ProblemNow[]
}

// ── Вилка эффекта: GET /api/effect (backend/out/effect.go) ──

export interface EffectParam {
  key: 'session_factor' | 'order_prob' | 'rescue_factor' | 'avg_check_rub' | string
  label: string
  value: number
  default: number
  source: 'measured' | 'assumed' | 'request' | string
  unit: string
  hint: string
}

export interface EffectScenario {
  id: 'conservative' | 'base' | 'optimistic' | string
  label: string
  rule: string
  rescued_share: number
  rescue_factor: number
  order_prob: number
  problem_sessions_month: number
  rescued_month: number
  orders_month: number
  revenue_rub_month: number
  /** Заказы сценария / текущие заказы поиска в месяц; null — заказов не было. */
  uplift: number | null
}

export interface EffectResponse {
  days: number
  from: string
  to: string
  note: string
  formula: string
  formula_keys: string
  measured: {
    searches: number
    searches_month: number
    problem_searches: number
    problem_share: number | null
    problem_sessions: number
    orders: number
    search_order_rate: number | null
    orders_month: number
    rescued: EffectRescued | null
  }
  assumptions: EffectParam[]
  scenarios: EffectScenario[]
  missing: string[]
  warnings: string[]
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
  /** Только role=head: вилка эффекта с допущениями по умолчанию. */
  effect?: EffectResponse | null
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

export type LlmCallKind = 'search' | 'eval' | 'agent' | 'judge' | 'other'

export interface LlmCall {
  id: string
  /** search — поиск покупателя, eval — прогон оценки, agent — запуск агента, other — ручной прогон. */
  kind: LlmCallKind | string
  created_at: string
  request_id: string | null
  /** Исходный запрос поиска по request_id; null — вызов не из записанного поиска. */
  query?: string | null
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

export interface CostsParam {
  key: string
  label: string
  value: number
  default: number
  source: 'measured' | 'assumed' | 'request' | string
  unit: string
  hint: string
}

export interface CostsScenario {
  requests: number
  llm_calls: number
  server_rub: number
  llm_rub: number
  agent_rub: number
  total_rub: number
  per_request_rub: number
  /** Онлайн-LLM на запрос: доля маршрута D × (1 − доля кэша) × стоимость разбора. */
  llm_per_request_rub: number
}

export interface CostsMonthly {
  formula: string
  params: CostsParam[]
  scenarios: CostsScenario[]
}

export interface CostsResponse {
  llm_mode: LlmMode
  pricing: CostsPricing
  summary: CostsSummary
  routes: CostRouteRow[]
  calls: CostsCalls
  /** Месячные сценарии сервера: из них блок «Месяц» берёт LLM на запрос и стоимость запуска. */
  monthly?: CostsMonthly
}

// ── SearchOps: агент, предложения, версии правил ──

export type AgentRunStatus = 'running' | 'done' | 'stopped' | 'failed'

export interface AgentRun {
  id: string
  status: AgentRunStatus | string
  trigger: 'manual' | 'schedule' | string
  started_at: string
  finished_at: string | null
  clusters_total: number
  clusters_done: number
  proposals_created: number
  tool_calls: number
  input_tokens: number
  output_tokens: number
  cost_rub: number
  stop_reason: string | null
  summary: string | null
  current_cluster: { id: string; label: string } | null
  /** Лимит вызовов инструментов на запуск (15 × выбранные кластеры). */
  max_tool_calls?: number
  days?: number
  max_clusters?: number
  model?: string
  /** Версия правил, на которой работал запуск. */
  rules_version?: number
  limits?: {
    tool_calls_per_cluster?: number
    hypotheses?: number
    tokens_per_run?: number
    timeout_sec?: number
  }
  episode?: RunEpisode | null
}

/** Итог запуска: метрики периода, итог каждого кластера, эффект принятых правок. */
export interface RunEpisode {
  metrics?: {
    days?: number
    searches?: number
    failed_queries?: number
    failed_searches?: number
    distinct_queries?: number
  }
  clusters?: {
    cluster_id: string
    label: string
    /** proposal | ticket | closed */
    outcome: string
    diagnosis: string
    hypotheses: number
    rejected: number
    tool_calls: number
    proposal_id: string
  }[]
  applied_effects?: AppliedEffect[] | null
}

export type AgentStepKind =
  'tool_call' | 'thought' | 'check' | 'memory' | 'diagnosis' | 'hypothesis' | 'proposal' | 'error'

export interface AgentStep {
  id: string
  run_id: string
  seq: number
  ts: string
  kind: AgentStepKind | string
  cluster_id: string | null
  tool: string | null
  title: string
  args: Record<string, unknown> | null
  result: Record<string, unknown> | null
  ok: boolean
  hypothesis_no: number | null
  rejected_reason: string | null
  ms: number | null
}

export interface AgentRunsResponse {
  items: AgentRun[]
}

export interface AgentRunDetail {
  run: AgentRun
  steps: AgentStep[]
}

export interface AgentRunRequest {
  days: number
  max_clusters: number
  trigger: 'manual' | 'schedule'
}

export type ProposalStatus = 'pending' | 'accepted' | 'rejected' | 'rolled_back' | 'stale'
export type ProposalRole = 'ecom' | 'product' | 'content'
export type Diagnosis =
  | 'ALIAS_GAP'
  | 'TYPO'
  | 'ASSORTMENT_GAP'
  | 'SUBSTITUTE_AVAILABLE'
  | 'CONTENT_GAP'
  | 'RANKING_FAILURE'
  | 'CONSTRAINT_CONFLICT'
  | 'NON_PRODUCT'
  | 'NOISE'
  | 'UNKNOWN'

export interface RulePatchItem {
  kind: string
  alias?: string
  canonical?: string
  target?: string
  target_id?: string | null
  [key: string]: unknown
}

export interface RulePatch {
  add: RulePatchItem[]
  remove: RulePatchItem[]
}

export interface FoundOf {
  found: number
  total: number
}

/** Запрос целевого кластера в прогоне: находился ли нужный товар до и после правки. */
export interface TargetQuery {
  text: string
  freq: number
  judged?: boolean
  before_found: boolean
  after_found: boolean
  before_top?: string[]
  after_top?: string[]
  after_empty?: boolean
}

export interface ProposalEvaluation {
  target: { before: FoundOf; after: FoundOf; worse?: number; queries?: TargetQuery[] }
  regression: {
    worse: number
    better: number
    total: number
    sets?: string[]
    worse_queries?: unknown[]
  }
  exact_worse: number
  constraint_violations_new: number
  false_rescue_new?: number
  passed: boolean
  reasons?: string[]
  rules_version?: number
  ms?: number
}

export interface ProposalCluster {
  id: string
  label: string
  queries: { text: string; freq: number; failures?: number }[]
  lost_demand: number
}

/** Гипотеза, которую агент проверил прогоном (в т. ч. отклонённая). */
export interface HypothesisEvidence {
  no: number
  hypothesis: string
  patch: RulePatch | null
  passed: boolean
  reason?: string | null
  evaluation?: ProposalEvaluation | null
}

/** Как отвечает действующий поиск на запрос кластера. */
export interface CurrentSearch {
  query: string
  route?: string
  total: number
  empty: boolean
  solved?: boolean
  top: { title: string; family_id: string; group: string }[]
}

export interface ProposalEvidence {
  terms?: string[]
  hypotheses?: HypothesisEvidence[] | null
  memory?: unknown[] | null
  current_search?: CurrentSearch[] | null
  reformulations?: { from: string; to: string; count: number; clicked: number }[] | null
  substitutes?: { name: string; vendor: string; family_id: string }[] | null
  /** Перепроверка при «Принять», если правила сменились после проверки агента. */
  recheck?: ProposalRecheck | null
  /** Прогон агента до перепроверки — когда перепроверка прошла и evaluation заменён новым. */
  initial_evaluation?: ProposalEvaluation | null
  [key: string]: unknown
}

export interface ProposalRecheck {
  from_version: number
  to_version: number
  passed: boolean
  reasons?: string[] | null
  evaluation?: ProposalEvaluation | null
  checked_at?: string
}

/** Эффект принятой правки: проверка verify_applied_change на действующих правилах. */
export interface AppliedEffect {
  before?: FoundOf
  promised?: FoundOf
  now?: FoundOf
  holds?: boolean
  summary?: string
  checked_at?: string
  cluster_label?: string
  queries?: { text: string; found: boolean; top?: string[] }[]
  [key: string]: unknown
}

export interface Proposal {
  id: string
  run_id: string
  cluster: ProposalCluster
  diagnosis: Diagnosis | string
  diagnosis_text: string
  role: ProposalRole | string
  action: 'rules_patch' | 'ticket' | string
  patch: RulePatch | null
  evaluation: ProposalEvaluation | null
  hypotheses_tried: number
  status: ProposalStatus | string
  created_at: string
  decided_at: string | null
  decided_reason: string | null
  decided_comment?: string | null
  config_version: number | null
  evidence?: ProposalEvidence | null
  base_version?: number
  effect?: AppliedEffect | null
  /** Запрос для «Проверить в поиске». */
  check_query?: string | null
  /** Почему предложение устарело (status: stale): код или текст сервера. */
  stale_reason?: string | null
}

export interface ProposalsResponse {
  items: Proposal[]
}

export type ProposalRejectReason = 'too_broad' | 'wrong_product' | 'not_now' | 'other'

export interface ApproveResponse {
  proposal: Proposal
  config_version: number | null
}

export interface ConfigVersion {
  version: number
  created_at: string
  author: 'agent+human' | 'human' | 'system' | string
  description: string
  proposal_id: string | null
  diff: RulePatch | null
  effect: AppliedEffect | null
  /** import | agent | rollback */
  source?: string
  /** Число алиасов в версии. */
  aliases?: number
}

/** POST /api/configs/rollback */
export interface RollbackResponse {
  version: number
  rolled_back_to: number
  rolled_back_proposals: string[]
}

export interface ConfigsResponse {
  active: number
  items: ConfigVersion[]
}

export interface MemoryItem {
  cluster_label: string
  proposal_id: string | null
  decision: string
  reason: string | null
  comment?: string | null
  diagnosis?: string
  cluster_key?: string
  ts: string
}

export interface MemoryResponse {
  items: MemoryItem[]
}
