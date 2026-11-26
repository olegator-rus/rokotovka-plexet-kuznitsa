// Состояние экрана «Поиск» ↔ адрес страницы ↔ тело POST /api/search. Без DOM: tests/searchState.test.ts.
// Адрес — единственный источник состояния запроса: ссылку можно переслать, назад/вперёд работают.
//   /search?q=касперский&config=B3&client=client-a&trace=1
//   &as_typed=1            — «Искать как введено»
//   &rm=license_quantity:10 — снятый чип (повторяется)
//   &cl=license_metric:device — ответ на уточняющий вопрос (повторяется)
//   &f=ram_gb:gte:32       — фасет оборудования, контур коммерции (повторяется, lib/facets.ts)
import type { ChipRef, ConfigId, SearchRequest } from '@/api/types'
import type { SearchFilter } from '@/api/typesP2'
import { parseFilters, serializeFilter } from './facets.ts'

export const CONFIG_IDS: readonly ConfigId[] = ['live', 'B0', 'B1', 'B2', 'B3', 'B4']
export const SEARCH_LIMIT = 20
export const GUEST_CLIENT_ID = 'guest'

export interface SearchState {
  q: string
  /** null — конфигурация по умолчанию из /api/meta. */
  config: ConfigId | null
  /** null — гость. */
  client: string | null
  asTyped: boolean
  removed: ChipRef[]
  clarify: Record<string, string>
  trace: boolean
  /** Фасеты оборудования (контур коммерции); необязательное — старые вызовы без поля работают. */
  filters?: SearchFilter[]
}

export function emptyState(): SearchState {
  return {
    q: '',
    config: null,
    client: null,
    asTyped: false,
    removed: [],
    clarify: {},
    trace: false,
    filters: [],
  }
}

type RawQuery = Record<string, unknown>

function first(v: unknown): string {
  if (Array.isArray(v)) return first(v[0])
  return typeof v === 'string' ? v : ''
}

function all(v: unknown): string[] {
  if (Array.isArray(v)) return v.filter((x): x is string => typeof x === 'string')
  return typeof v === 'string' ? [v] : []
}

/** «field:value» → { field, value }; без двоеточия или с пустым полем — null. */
export function parsePair(raw: string): ChipRef | null {
  const i = raw.indexOf(':')
  if (i <= 0) return null
  const field = raw.slice(0, i).trim()
  const value = raw.slice(i + 1).trim()
  if (!field || !value) return null
  return { field, value }
}

export function isConfigId(v: unknown): v is ConfigId {
  return typeof v === 'string' && (CONFIG_IDS as readonly string[]).includes(v)
}

export function parseSearchQuery(query: RawQuery): SearchState {
  const configIn = first(query.config).trim()
  const configRaw = configIn.toLowerCase() === 'live' ? 'live' : configIn.toUpperCase()
  const client = first(query.client).trim()
  const clarify: Record<string, string> = {}
  for (const pair of all(query.cl).map(parsePair)) if (pair) clarify[pair.field] = pair.value
  const removed: ChipRef[] = []
  for (const pair of all(query.rm).map(parsePair)) {
    if (pair && !removed.some((c) => c.field === pair.field && c.value === pair.value))
      removed.push(pair)
  }
  return {
    q: first(query.q).trim(),
    config: isConfigId(configRaw) ? configRaw : null,
    client: client && client !== GUEST_CLIENT_ID ? client : null,
    asTyped: first(query.as_typed) === '1',
    removed,
    clarify,
    trace: first(query.trace) === '1',
    filters: parseFilters(all(query.f)),
  }
}

/** Обратно в query роутера. Пустые и умолчательные значения не пишутся. */
export function serializeSearchState(state: SearchState): Record<string, string | string[]> {
  const out: Record<string, string | string[]> = {}
  if (state.q) out.q = state.q
  if (state.config) out.config = state.config
  if (state.client && state.client !== GUEST_CLIENT_ID) out.client = state.client
  if (state.asTyped) out.as_typed = '1'
  if (state.removed.length) out.rm = state.removed.map((c) => `${c.field}:${c.value}`)
  const cl = Object.entries(state.clarify).map(([k, v]) => `${k}:${v}`)
  if (cl.length) out.cl = cl
  if (state.trace) out.trace = '1'
  if (state.filters?.length) out.f = state.filters.map(serializeFilter)
  return out
}

/** Новый текст запроса сбрасывает уточнения прошлого: снятые чипы, ответ на вопрос, «как введено». */
export function withQuery(state: SearchState, q: string): SearchState {
  return { ...state, q: q.trim(), asTyped: false, removed: [], clarify: {}, filters: [] }
}

export function withChipRemoved(state: SearchState, chip: ChipRef): SearchState {
  if (state.removed.some((c) => c.field === chip.field && c.value === chip.value)) return state
  return { ...state, removed: [...state.removed, { field: chip.field, value: chip.value }] }
}

export function withChipsRestored(state: SearchState): SearchState {
  return { ...state, removed: [] }
}

export function withClarify(state: SearchState, attribute: string, value: string): SearchState {
  return { ...state, clarify: { ...state.clarify, [attribute]: value } }
}

export function withAsTyped(state: SearchState): SearchState {
  return { ...state, asTyped: true }
}

/** Сравнение по смыслу запроса: переключение трассы новый поиск не запускает. */
export function sameRequest(a: SearchState, b: SearchState): boolean {
  const key = (s: SearchState) => JSON.stringify({ ...serializeSearchState(s), trace: undefined })
  return key(a) === key(b)
}

export interface RequestContext {
  sessionId: string
  defaultConfig: ConfigId
  limit?: number
}

/** Тело POST /api/search. */
export function buildSearchRequest(state: SearchState, ctx: RequestContext): SearchRequest {
  return {
    query: state.q,
    client_id: state.client ?? GUEST_CLIENT_ID,
    session_id: ctx.sessionId,
    config: state.config ?? ctx.defaultConfig,
    as_typed: state.asTyped,
    removed_chips: state.removed.map((c) => ({ field: c.field, value: c.value })),
    clarify: { ...state.clarify },
    limit: ctx.limit ?? SEARCH_LIMIT,
    // Контур коммерции: фасеты — только если выбраны, иначе тело запроса как в контуре поиска.
    ...(state.filters?.length ? { filters: state.filters.map((f) => ({ ...f })) } : {}),
  }
}
