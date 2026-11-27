// Страница результатов по ТЗ (контур ТЗ, TZ-CORE.md §2, §6): позиции по хешу (GET /api/tz/:hash — без повторного
// разбора) и товары по каждой позиции существующим POST /api/search с source "tz", не больше четырёх одновременно.
// Позиции видны сразу в порядке документа; готовая заменяет свою заглушку, порядок не меняется. Ошибка позиции —
// только её состояние, повтор — только для неё. Результат последнего открытого ТЗ держится в памяти вкладки: возврат
// из полного поиска по позиции показывает ту же страницу без новых запросов, и место прокрутки восстанавливается.
import { computed, onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue'
import { postSearch } from '@/api/search'
import { getTzDocument } from '@/api/tz'
import type { SearchItem, SearchRequest, SearchResponse } from '@/api/types'
import type { TzDocument, TzPosition } from '@/api/typesTz'
import { isApiError } from '@/lib/apiError'
import { GUEST_CLIENT_ID } from '@/lib/searchState'
import {
  TZ_SEARCH_CONCURRENCY,
  TZ_SEARCH_LIMIT,
  type TzMatchStatus,
  type TzPositionState,
  matchStatus,
  positionProducts,
  runLimited,
} from '@/lib/tz'
import { useAppStore } from '@/stores/app'
import { useTzStore } from '@/stores/tz'

export interface TzRow {
  position: TzPosition
  state: TzPositionState
  status: TzMatchStatus | null
  items: SearchItem[]
  requestId: string | null
}

export type TzDocState = 'loading' | 'ready' | 'unavailable' | 'error'

interface CacheEntry {
  doc: TzDocument
  rows: TzRow[]
}
/** Последние открытые ТЗ вкладки (не больше трёх): возврат из полного поиска без новых запросов. */
const cache = new Map<string, CacheEntry>()
const CACHE_MAX = 3

function remember(hash: string, entry: CacheEntry) {
  cache.delete(hash)
  cache.set(hash, entry)
  while (cache.size > CACHE_MAX) cache.delete(cache.keys().next().value as string)
}

/** Позиции ТЗ из памяти вкладки (для строки «Вернуться к ТЗ…» над обычной выдачей). */
export function cachedTzDocument(hash: string): TzDocument | null {
  return cache.get(hash)?.doc ?? null
}

/** Тело POST /api/search для позиции (TZ-CORE.md §6): запрос, фильтры позиции, source "tz". */
export function positionSearchRequest(
  p: Pick<TzPosition, 'search_query' | 'filters'>,
  ctx: { sessionId: string; clientId: string | null; config: SearchRequest['config'] },
): SearchRequest {
  return {
    query: p.search_query,
    client_id: ctx.clientId ?? GUEST_CLIENT_ID,
    session_id: ctx.sessionId,
    config: ctx.config,
    as_typed: false,
    removed_chips: [],
    clarify: {},
    limit: TZ_SEARCH_LIMIT,
    group: 'family',
    source: 'tz',
    ...(p.filters?.length ? { filters: p.filters.map((f) => ({ ...f })) } : {}),
  }
}

/** Пустые списки сервер на Go может прислать как null: страница работает с массивами. */
export function normalizeTzDocument(d: TzDocument): TzDocument {
  return {
    ...d,
    positions: (d.positions ?? []).map((p) => ({
      ...p,
      short_title: p.short_title || p.title,
      attributes: p.attributes ?? [],
      filters: p.filters ?? [],
      source_quote: p.source_quote ?? '',
      unverified_requirements: p.unverified_requirements ?? [],
    })),
  }
}

function pendingRows(doc: TzDocument): TzRow[] {
  return doc.positions.map((position) => ({
    position,
    state: 'pending',
    status: null,
    items: [],
    requestId: null,
  }))
}

export function useTzResults(hash: Ref<string | null>) {
  const app = useAppStore()
  const tz = useTzStore()
  const docState = ref<TzDocState>('loading')
  const doc = shallowRef<TzDocument | null>(null)
  const rows = ref<TzRow[]>([])
  /** Страница показана из памяти вкладки: место прокрутки можно восстановить сразу. */
  const fromCache = ref(false)
  let ctrl: AbortController | null = null

  const settled = computed(
    () =>
      rows.value.length > 0 && rows.value.every((r) => r.state === 'ready' || r.state === 'error'),
  )
  /** «Найдено N» — позиции со статусом «Найдено» или «Похожие»; пока поиск идёт — null. */
  const found = computed(() =>
    settled.value
      ? rows.value.filter((r) => r.status === 'found' || r.status === 'similar').length
      : null,
  )
  const nothingFound = computed(
    () =>
      settled.value &&
      rows.value.every((r) => r.state === 'ready') &&
      rows.value.every((r) => r.status === 'not_found'),
  )

  function patch(i: number, next: Partial<TzRow>) {
    const cur = rows.value[i]
    if (!cur) return
    rows.value[i] = { ...cur, ...next }
  }

  async function searchOne(i: number, signal: AbortSignal) {
    const row = rows.value[i]
    if (!row) return
    patch(i, { state: 'searching' })
    try {
      if (!app.sessionId) await app.loadMeta()
      const body = positionSearchRequest(row.position, {
        sessionId: app.sessionId,
        clientId: app.clientId || GUEST_CLIENT_ID,
        config: app.defaultConfig,
      })
      const resp: SearchResponse = await postSearch(body, { signal })
      if (signal.aborted) return
      patch(i, {
        state: 'ready',
        status: matchStatus(row.position, resp.items),
        items: positionProducts(resp.items),
        requestId: resp.request_id ?? null,
      })
    } catch (e) {
      if (signal.aborted) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      // Технический сбой — «Ошибка поиска» с повтором, а не «Нет в каталоге».
      patch(i, { state: 'error', status: null, items: [] })
    }
  }

  async function searchAll(indices: number[], c: AbortController) {
    await runLimited(indices, TZ_SEARCH_CONCURRENCY, (i) => searchOne(i, c.signal), c.signal)
    if (ctrl !== c) return
    const h = hash.value
    if (h && doc.value) remember(h, { doc: doc.value, rows: rows.value })
    if (h) tz.finishSearching(h)
  }

  async function load(h: string) {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    const hit = cache.get(h)
    if (hit && hit.rows.every((r) => r.state === 'ready' || r.state === 'error')) {
      doc.value = hit.doc
      rows.value = hit.rows.map((r) => ({ ...r }))
      fromCache.value = true
      docState.value = 'ready'
      tz.finishSearching(h)
      return
    }
    fromCache.value = false
    docState.value = 'loading'
    doc.value = null
    rows.value = []
    let d: TzDocument
    try {
      d = normalizeTzDocument(await getTzDocument(h, { signal: c.signal }))
    } catch (e) {
      if (ctrl !== c) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      docState.value =
        isApiError(e) && (e.status === 404 || e.status === 410) ? 'unavailable' : 'error'
      tz.finishSearching(h)
      return
    }
    if (ctrl !== c) return
    doc.value = d
    rows.value = pendingRows(d)
    docState.value = 'ready'
    if (!rows.value.length) {
      tz.finishSearching(h)
      return
    }
    await searchAll(
      rows.value.map((_, i) => i),
      c,
    )
  }

  /** Повтор только для позиции с ошибкой. */
  async function retry(i: number) {
    const c = ctrl ?? new AbortController()
    ctrl = c
    await searchOne(i, c.signal)
    const h = hash.value
    if (h && doc.value && settled.value) remember(h, { doc: doc.value, rows: rows.value })
  }

  function reload() {
    if (hash.value) void load(hash.value)
  }

  watch(
    hash,
    (h) => {
      if (h) void load(h)
      else {
        ctrl?.abort()
        docState.value = 'unavailable'
      }
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    ctrl?.abort()
    ctrl = null
  })

  return { docState, doc, rows, settled, found, nothingFound, fromCache, retry, reload }
}

/**
 * Позиции ТЗ для полного поиска по позиции (/search?q=…&fromTz=<хеш>&tzItem=N): из памяти вкладки, иначе
 * GET /api/tz/:hash — разбор не повторяется. Нет хеша или ссылка недоступна — null.
 */
export function useTzDocument(hash: Ref<string | null>) {
  const doc = shallowRef<TzDocument | null>(null)
  let ctrl: AbortController | null = null
  watch(
    hash,
    async (h) => {
      ctrl?.abort()
      ctrl = null
      if (!h) {
        doc.value = null
        return
      }
      const hit = cachedTzDocument(h)
      if (hit) {
        doc.value = hit
        return
      }
      if (doc.value?.hash === h) return
      const c = new AbortController()
      ctrl = c
      try {
        const d = normalizeTzDocument(await getTzDocument(h, { signal: c.signal }))
        if (ctrl === c) doc.value = d
      } catch {
        if (ctrl === c) doc.value = null
      }
    },
    { immediate: true },
  )
  onBeforeUnmount(() => ctrl?.abort())
  return doc
}
