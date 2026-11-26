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
