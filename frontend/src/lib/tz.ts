// Поиск по ТЗ (контур ТЗ, docs TZ-UI.md, TZ-CORE.md §2, §6): проверка файла, этапы разбора, опрос операции с
// отменой, поиск по позициям не больше четырёх одновременно, статус позиции, тексты. Без DOM: tests/tz.test.ts.
import type { Purchase, SearchItem, SearchItemBase } from '@/api/types'
import type { TzDocument, TzOperation, TzPosition, TzStage } from '@/api/typesTz'
import { plural } from './format.ts'
import { purchaseQuantity } from './procurement.ts'
import { serializeFilter } from './facets.ts'

/** 20 МБ — предел файла (TZ-UI.md §10; на сервере 20 МиБ). */
export const TZ_MAX_BYTES = 20 * 1024 * 1024
/** Не больше четырёх поисков по позициям одновременно (TZ-CORE.md §6). */
export const TZ_SEARCH_CONCURRENCY = 4
export const TZ_POLL_MS = 1000
export const TZ_SEARCH_LIMIT = 5
/** «Похожие» — до четырёх карточек; «Найдено» — первая и до трёх под «ещё». */
export const TZ_SHOWN_MAX = 4
export const TZ_ACCEPT = '.pdf,.docx'

/** Тексты TZ-UI.md §11. */
export const TZ_TEXT = {
  tooltip: 'Поиск по техническому заданию',
  accessibleLabel: 'Загрузить техническое задание',
  formats: 'PDF или DOCX до 20 МБ',
  suggestionTitle: 'Найти товары по ТЗ',
  suggestionDescription: 'Загрузите PDF или DOCX, чтобы найти товары по всем позициям документа',
  dropTitle: 'Перетащите ТЗ сюда',
  dropDescription: 'Найдём товары по позициям документа',
  read: 'Читаю документ',
  ocr: 'Распознаю текст',
  search: 'Ищу товары',
  pending: 'Ожидает поиска',
  cancel: 'Отменить',
  longOcr:
    'Распознавание сканов может занять больше времени. Вы можете продолжить пользоваться поиском.',
  replace: 'Загрузить другое ТЗ',
  exit: 'Выйти из режима ТЗ',
  navTitle: 'Позиции ТЗ',
  found: 'Найдено',
  similar: 'Похожие',
  unavailable: 'Нет в каталоге',
  error: 'Ошибка поиска',
  similarDescription: 'Подобраны товары по описанию. Проверьте их соответствие требованиям ТЗ.',
  quoteExpand: 'Показать цитату из ТЗ',
  quoteCollapse: 'Свернуть цитату',
  quoteCopy: 'Копировать',
  variantsCollapse: 'Свернуть варианты',
  openSearch: 'Открыть поиск',
  empty: 'В каталоге нет подходящих товаров для этой позиции.',
  manager: 'Спросить менеджера',
  added: 'Товар из позиции добавлен в спецификацию',
  specOrigin: 'из ТЗ',
  errPosition: 'Не удалось подобрать товары. Повторите поиск по этой позиции.',
  retry: 'Повторить',
  chooseFile: 'Выбрать файл',
  expiredTitle: 'Результаты по ТЗ недоступны',
  expiredDescription:
    'Результаты хранятся неделю. Возможно, срок хранения закончился или ссылка неверна.',
  expiredHint: 'Загрузите документ заново, чтобы подобрать товары.',
  expiredAction: 'Загрузить ТЗ',
  noMatches: 'По позициям ТЗ не найдено подходящих товаров',
  noMatchesHint:
    'Вы можете открыть поиск по отдельной позиции, изменить запрос или обратиться к менеджеру.',
  regularSearch: 'Перейти к обычному поиску',
  remainderAction: 'Загрузить остаток отдельным файлом',
} as const

export type TzUploadError =
  'format' | 'size' | 'multiple' | 'damaged' | 'password' | 'ocr' | 'no_positions' | 'general'

export const TZ_ERROR_TEXT: Record<TzUploadError, string> = {
  format: 'Поддерживаются только PDF и DOCX. Выберите файл другого формата.',
  size: 'Размер файла превышает 20 МБ. Выберите файл меньшего размера.',
  multiple: 'Загрузите один файл технического задания.',
  damaged: 'Не удалось прочитать документ. Попробуйте загрузить другой файл.',
  password: 'Документ защищён паролем. Загрузите файл без защиты.',
  ocr: 'Не удалось распознать текст. Загрузите более чёткий скан или документ с текстом.',
  no_positions:
    'В документе не найдено товарных позиций. Попробуйте другой файл или воспользуйтесь обычным поиском.',
  general: 'Не удалось обработать ТЗ. Попробуйте ещё раз.',
}

/** Действие под ошибкой (TZ-UI.md §10). */
export function errorAction(e: TzUploadError): { label: string; kind: 'pick' | 'search' } {
  switch (e) {
    case 'format':
      return { label: TZ_TEXT.chooseFile, kind: 'pick' }
    case 'size':
      return { label: 'Выбрать другой файл', kind: 'pick' }
    case 'multiple':
      return { label: 'Выбрать один файл', kind: 'pick' }
    case 'damaged':
      return { label: 'Загрузить другой файл', kind: 'pick' }
    case 'password':
      return { label: 'Загрузить документ без пароля', kind: 'pick' }
    case 'ocr':
      return { label: 'Загрузить более чёткий документ', kind: 'pick' }
    case 'no_positions':
      return { label: TZ_TEXT.regularSearch, kind: 'search' }
    default:
      return { label: TZ_TEXT.retry, kind: 'pick' }
  }
}

/** Причина ошибки операции → ключ текста; незнакомая — общая. */
export function reasonToError(reason: string | undefined | null): TzUploadError {
  const known: readonly string[] = [
    'format',
    'size',
    'damaged',
    'password',
    'ocr',
    'no_positions',
    'general',
  ]
  return reason && known.includes(reason) ? (reason as TzUploadError) : 'general'
}

/** Проверка на фронте до отправки: одно имя с расширением .pdf/.docx и размер до 20 МБ. */
export function validateTzFile(f: { name: string; size: number }): TzUploadError | null {
  const name = f.name.toLowerCase().trim()
  if (!name.endsWith('.pdf') && !name.endsWith('.docx')) return 'format'
  if (f.size > TZ_MAX_BYTES) return 'size'
  if (f.size <= 0) return 'damaged'
  return null
}

/** Выбор нескольких файлов: не отправляется ни один. */
export function validateTzFiles(files: readonly { name: string; size: number }[]): {
  file: number | null
  error: TzUploadError | null
} {
  if (files.length !== 1) return { file: null, error: files.length > 1 ? 'multiple' : null }
  const error = validateTzFile(files[0] as { name: string; size: number })
  return { file: error ? null : 0, error }
}

// ── Этапы разбора ──

export type TzStepKey = 'read' | 'ocr' | 'search'
export interface TzStep {
  key: TzStepKey
  label: string
  state: 'done' | 'active' | 'todo'
}

/** Этап сервера → шаг полосы; ocr — только если сервер его прислал. */
export function stageStep(stage: TzStage | undefined | null): TzStepKey {
  if (stage === 'ocr') return 'ocr'
  if (stage === 'searching' || stage === 'search') return 'search'
  return 'read'
}

/**
 * Шаги полосы: «Читаю документ» → [«Распознаю текст» — только для скана] → «Ищу товары». Ширина шага ничего не
 * значит, процентов нет.
 */
export function stageSteps(current: TzStepKey, sawOcr: boolean): TzStep[] {
  const keys: TzStepKey[] =
    sawOcr || current === 'ocr' ? ['read', 'ocr', 'search'] : ['read', 'search']
  const idx = keys.indexOf(current)
  const label: Record<TzStepKey, string> = {
    read: TZ_TEXT.read,
    ocr: TZ_TEXT.ocr,
    search: TZ_TEXT.search,
  }
  return keys.map((key, i) => ({
    key,
    label: label[key],
    state: i < idx ? 'done' : i === idx ? 'active' : 'todo',
  }))
}

// ── Опрос операции ──

export interface PollDeps {
  get: (id: string, signal: AbortSignal) => Promise<TzOperation>
  wait: (ms: number, signal: AbortSignal) => Promise<void>
  onStage?: (stage: TzStage | undefined) => void
  intervalMs?: number
  /** Сколько сетевых сбоев подряд терпим, прежде чем назвать операцию ошибкой. */
  maxFailures?: number
}

/**
 * Опрашивает операцию раз в секунду до completed или error. Отменённый сигнал — null: поздний ответ не
 * используется (отмена и обычный поиск во время разбора не переводят на страницу ТЗ).
 */
export async function pollOperation(
  id: string,
  deps: PollDeps,
  signal: AbortSignal,
): Promise<TzOperation | null> {
  const interval = deps.intervalMs ?? TZ_POLL_MS
  const maxFailures = deps.maxFailures ?? 3
  let failures = 0
  while (!signal.aborted) {
    let op: TzOperation
    try {
      op = await deps.get(id, signal)
    } catch (e) {
      if (signal.aborted) return null
      const status = (e as { status?: number } | null)?.status
      // 404 — операция забыта сервером (перезапуск или срок жизни): ждать нечего.
      if (status === 404) return { status: 'error', reason: 'general' }
      failures++
      if (failures >= maxFailures) return { status: 'error', reason: 'general' }
      await deps.wait(interval, signal).catch(() => undefined)
      continue
    }
    if (signal.aborted) return null
    failures = 0
    if (op.status !== 'processing') return op
    deps.onStage?.(op.stage)
    await deps.wait(interval, signal).catch(() => undefined)
  }
  return null
}

/** Пауза, которую прерывает сигнал. */
export function abortableWait(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('aborted', 'AbortError'))
      return
    }
    const t = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    function onAbort() {
      clearTimeout(t)
      reject(new DOMException('aborted', 'AbortError'))
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

// ── Поиск по позициям ──

/**
 * Запускает worker для каждого элемента, не больше limit одновременно, в порядке списка. Ошибка одного элемента
 * не останавливает остальные (worker сам записывает свою ошибку). Отменённый сигнал — новые не начинаются.
 */
export async function runLimited<T>(
  items: readonly T[],
  limit: number,
  worker: (item: T, index: number) => Promise<void>,
  signal?: AbortSignal,
): Promise<void> {
  let next = 0
  const lanes = Math.max(1, Math.min(limit, items.length))
  async function lane() {
    while (next < items.length) {
      if (signal?.aborted) return
      const i = next++
      try {
        await worker(items[i] as T, i)
      } catch {
        // Ошибку позиции записывает worker; очередь идёт дальше.
      }
    }
  }
  await Promise.all(Array.from({ length: lanes }, lane))
}

export type TzMatchStatus = 'found' | 'similar' | 'not_found'
export type TzPositionState = 'pending' | 'searching' | 'ready' | 'error'

/** Товары позиции: без «Популярного в наличии» и без дописанного фильтром — это не находки по позиции. */
export function positionProducts(items: readonly SearchItem[] | undefined | null): SearchItem[] {
  return (items ?? []).filter((i) => i.match_type !== 'fallback' && !i.filter_fill)
}

function isExact(i: SearchItemBase): boolean {
  return i.match_type ? i.match_type === 'exact' : i.group === 'exact'
}

/**
 * Итог подбора по TZ-CORE.md §6: назван продукт и есть точное совпадение — «Найдено»; работа без точного
 * совпадения — «Нет в каталоге»; товары есть — «Похожие»; товаров нет — «Нет в каталоге». Упавший запрос сюда не
 * попадает: это состояние error, а не итог.
 */
export function matchStatus(
  position: Pick<TzPosition, 'named' | 'kind'>,
  items: readonly SearchItem[] | undefined | null,
): TzMatchStatus {
  const real = positionProducts(items)
  const exact = real.some(isExact)
  if (position.named && exact) return 'found'
  if (position.kind === 'service' && !exact) return 'not_found'
  if (!real.length) return 'not_found'
  return 'similar'
}

/** Карточки позиции: «Найдено» — первая и до трёх под «ещё»; «Похожие» — до четырёх сразу. */
export function shownProducts(
  status: TzMatchStatus | null,
  items: readonly SearchItem[],
): { first: SearchItem[]; more: SearchItem[] } {
  if (status === 'found') return { first: items.slice(0, 1), more: items.slice(1, TZ_SHOWN_MAX) }
  if (status === 'similar') return { first: items.slice(0, TZ_SHOWN_MAX), more: [] }
  return { first: [], more: [] }
}

export const STATUS_LABEL: Record<TzMatchStatus, string> = {
  found: TZ_TEXT.found,
  similar: TZ_TEXT.similar,
  not_found: TZ_TEXT.unavailable,
}

/** Подпись состояния позиции: незавершённое никогда не «Нет в каталоге». */
export function positionLabel(state: TzPositionState, status: TzMatchStatus | null): string {
  if (state === 'error') return TZ_TEXT.error
  if (state === 'pending') return TZ_TEXT.pending
  if (state === 'searching' || !status) return TZ_TEXT.search
  return STATUS_LABEL[status]
}

// ── Тексты с числами ──

export function positionsCount(n: number): string {
  return `${n} ${plural(n, ['позиция', 'позиции', 'позиций'])}`
}

/** «Ещё 1 вариант», «Ещё 2 варианта», «Ещё 5 вариантов». */
export function moreVariantsLabel(n: number): string {
  return `Ещё ${n} ${plural(n, ['вариант', 'варианта', 'вариантов'])}`
}

/** Заголовок: пока поиск идёт — без «найдено»; «найдено» — число позиций «Найдено» и «Похожие». */
export function resultsTitle(fileName: string, total: number, found: number | null): string {
  const head = `По ТЗ «${fileName}» — ${positionsCount(total)}`
  return found === null ? head : `${head} · найдено ${found}`
}

export function navProgress(added: number, total: number): string {
  return `${added} из ${total} в спецификации`
}

export function mobileNavLabel(current: number, total: number): string {
  return `Позиция ${current} из ${total}`
}

export function limitNotice(remaining: number): string {
  return `Показаны первые 50 позиций. Осталось необработанных позиций: ${remaining}.`
}

export function backLabel(fileName: string, current: number | null, total: number): string {
  const head = `Вернуться к ТЗ «${fileName}»`
  return current ? `${head} · позиция ${current} из ${total}` : head
}

export function unverifiedLabel(reqs: readonly string[]): string {
  return `Не проверено: ${reqs.join('; ')}`
}

/** «по ТЗ, п. 3», «по ТЗ, п. 1 и 4», «по ТЗ, п. 1, 3 и 4». */
export function tzSourceLabel(positions: readonly number[]): string {
  const ns = [...new Set(positions)].sort((a, b) => a - b)
  if (!ns.length) return 'по ТЗ'
  if (ns.length === 1) return `по ТЗ, п. ${ns[0]}`
  return `по ТЗ, п. ${ns.slice(0, -1).join(', ')} и ${ns[ns.length - 1]}`
}

// ── Условия позиции ──

/** Условия позиции как условия закупки поиска: карточка открывается с выбранным под них вариантом. */
export function positionPurchase(
  p: Pick<TzPosition, 'purchase' | 'search_query'>,
): Purchase | null {
  const src = p.purchase
  if (!src) return null
  const out: Purchase = { query: p.search_query }
  if (src.deal) out.deal = src.deal
  if (typeof src.quantity === 'number' && src.quantity > 0) out.quantity = src.quantity
  if (src.metric) out.metric = src.metric
  if (typeof src.duration_months === 'number' && src.duration_months > 0)
    out.duration_months = src.duration_months
  if (src.org_type) out.org_type = src.org_type
  if (src.edition) out.edition = src.edition
  return out
}

/** Метрики, которые считаются штуками (как в procurement.ts). */
const COUNTED_METRICS = new Set(['user', 'device', 'server'])

/**
 * Количество строки спецификации по количеству из ТЗ. Ступень, пакет на N мест и поштучная строка — то же правило,
 * что у «Выбрано по запросу» (purchaseQuantity). Отличие одно: у позиции каталога без своей метрики и без пакета
 * («Бессрочная», «Подписка · 1 год») количество берётся из ТЗ как есть — в ТЗ оно названо явно («2 шт.»). Лицензия на
 * ядра или организацию (метрика не штучная) — 1.
 */
export function positionQuantity(
  item: Pick<SearchItemBase, 'license_line' | 'attrs' | 'tier_min'>,
  p: Pick<TzPosition, 'purchase'>,
): number {
  const q = p.purchase?.quantity
  if (typeof q !== 'number' || !(q > 0)) return 1
  const n = Math.round(q)
  const byRule = purchaseQuantity(item, n, p.purchase?.metric)
  if (byRule !== 1 || n === 1) return byRule
  const metric = item.attrs?.license_metric ?? ''
  if (metric && !COUNTED_METRICS.has(metric)) return 1
  const pack = item.attrs?.license_quantity
  if (typeof pack === 'number' && pack > 1) return Math.ceil(n / pack)
  return n
}

/** Адрес полного поиска по позиции: /search?q=…&f=…&fromTz=<хеш>&tzItem=N. */
export function positionSearchQuery(
  hash: string,
  p: Pick<TzPosition, 'search_query' | 'filters' | 'number'>,
): Record<string, string | string[]> {
  const out: Record<string, string | string[]> = { q: p.search_query }
  if (p.filters?.length) out.f = p.filters.map(serializeFilter)
  out.fromTz = hash
  out.tzItem = String(p.number)
  return out
}

/** Сравнение текста запроса для привязки добавлений к позиции: регистр и лишние пробелы не важны. */
export function sameQuery(a: string, b: string): boolean {
  const n = (s: string) => s.trim().replace(/\s+/g, ' ').toLowerCase()
  return n(a) === n(b)
}

/** Хеш из адреса: только безопасные символы, иначе — неверная ссылка. */
export function parseTzHash(raw: unknown): string | null {
  const v = Array.isArray(raw) ? raw[0] : raw
  if (typeof v !== 'string') return null
  const s = v.trim()
  return /^[A-Za-z0-9_-]{1,128}$/.test(s) ? s : null
}

export function parseTzItem(raw: unknown): number | null {
  const v = Array.isArray(raw) ? raw[0] : raw
  const n = typeof v === 'string' ? Number(v) : NaN
  return Number.isInteger(n) && n > 0 ? n : null
}

/** Число позиций, которые не попали в разбор из-за предела 50 (из результата разбора). */
export function remainingPositions(doc: Pick<TzDocument, 'extracted_count' | 'processed_count'>) {
  return Math.max(0, (doc.extracted_count ?? 0) - (doc.processed_count ?? 0))
}

// ── Раскрытые блоки и прокрутка (sessionStorage, на хеш) ──

export interface TzViewState {
  scrollY: number
  quotes: string[]
  variants: string[]
}

export function viewKey(hash: string): string {
  return `plexet.tz.view.${hash}`
}

export function parseViewState(raw: string | null): TzViewState | null {
  if (!raw) return null
  try {
    const d = JSON.parse(raw) as Record<string, unknown>
    const ids = (x: unknown) =>
      Array.isArray(x) ? x.filter((v): v is string => typeof v === 'string').slice(0, 100) : []
    return {
      scrollY: typeof d.scrollY === 'number' && d.scrollY >= 0 ? d.scrollY : 0,
      quotes: ids(d.quotes),
      variants: ids(d.variants),
    }
  } catch {
    return null
  }
}
