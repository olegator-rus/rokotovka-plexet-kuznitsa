// Панель поиска контура выдачи без DOM.
// Проверяют tests/panel.test.ts: отбор секций, линейный порядок элементов, выделение продолжения, недавние
// запросы, кэш и порядковый номер ответа, уточнения.
import type {
  PanelCategory,
  PanelProduct,
  PanelRecs,
  PanelRefinement,
  PanelSuggestResponse,
  PanelSuggestion,
  PanelTogether,
  PanelVendor,
} from '@/api/typesPanel'
import type { Companion } from '@/api/typesP2'
import type { SearchFilter } from '@/api/typesP2'
import { noDates } from './companions.ts'

export const PANEL_MIN_CHARS = 2
export const PANEL_DEBOUNCE_MS = 150
/** Скелет товаров — только если ожидание дольше этого (150–200 мс). */
export const PANEL_SKELETON_MS = 180
export const RECENT_MAX = 5
export const RECENT_KEY = 'plexet.recent-queries'

export interface PanelLimits {
  refinements: number
  suggestions: number
  categories: number
  vendors: number
  products: number
  recs: number
}

export const DESKTOP_LIMITS: PanelLimits = {
  refinements: 6,
  suggestions: 5,
  categories: 2,
  vendors: 2,
  products: 6,
  recs: 3,
}
export const MOBILE_LIMITS: PanelLimits = {
  refinements: 4,
  suggestions: 4,
  categories: 2,
  vendors: 2,
  products: 3,
  recs: 2,
}

/** Ответ сервера в виде для показа: числа — максимумы, пустое отброшено, семейства без дублей. */
export interface PanelView {
  /** Сервер контура коммерции (нет поля `suggestions`): панель показывает прежние группы. */
  legacy: boolean
  corrected: { from: string; to: string; kind: string } | null
  refinements: PanelRefinement[]
  suggestions: PanelSuggestion[]
  categories: PanelCategory[]
  vendors: PanelVendor[]
  products: PanelProduct[]
  /** «Вместе с этим» (§16.8): до двух дополнений к первому товару панели; иначе пусто. */
  together: PanelTogether[]
  totalHint: number | null
  /** Товары не успели на сервере: место под них держим, ответ дозапрашиваем. */
  partial: boolean
  serverMs: number | null
}

export function isPanelResponse(resp: PanelSuggestResponse | null | undefined): boolean {
  return Array.isArray(resp?.suggestions)
}

export function toPanelView(
  resp: PanelSuggestResponse | null | undefined,
  limits: PanelLimits = DESKTOP_LIMITS,
): PanelView {
  const legacy = !isPanelResponse(resp)
  const seenFamilies = new Set<string>()
  const products: PanelProduct[] = []
  for (const p of resp?.products ?? []) {
    if (!p?.product_id) continue
    const fam = p.family_id || p.product_id
    if (seenFamilies.has(fam)) continue
    seenFamilies.add(fam)
    products.push(p)
    if (products.length >= limits.products) break
  }
  // Точный артикул — один товар; точное совпадение всегда первым, его ничто не сдвигает.
  const exactSku = products.find((p) => p.sku_exact)
  const shown = exactSku ? [exactSku] : products
  return {
    legacy,
    corrected: resp?.corrected?.to ? { ...resp.corrected } : null,
    refinements: exactSku
      ? []
      : (resp?.refinements ?? []).filter((r) => r?.label).slice(0, limits.refinements),
    suggestions: (resp?.suggestions ?? []).filter((s) => s?.text).slice(0, limits.suggestions),
    categories: exactSku
      ? []
      : (resp?.categories ?? []).filter((c) => c?.id && c.name).slice(0, limits.categories),
    vendors: (resp?.vendors ?? []).filter((v) => v?.id && v.name).slice(0, limits.vendors),
    products: shown,
    together: togetherFor(shown[0], resp?.together),
    totalHint: typeof resp?.total_hint === 'number' ? resp.total_hint : null,
    partial: Boolean(resp?.meta?.partial),
    serverMs: typeof resp?.meta?.ms?.total === 'number' ? resp.meta.ms.total : null,
  }
}

/**
 * «Вместе с этим» к первому товару панели: только дополнения к его семейству (`anchor_family_id`), с названием,
 * без повторов семейства и без самого первого товара; не больше двух.
 */
export function togetherFor(
  first: PanelProduct | undefined,
  list: readonly PanelTogether[] | null | undefined,
): PanelTogether[] {
  if (!first || !Array.isArray(list)) return []
  const anchor = first.family_id || first.product_id
  const seen = new Set<string>([anchor])
  const out: PanelTogether[] = []
  for (const t of list) {
    if (!t?.product_id || !(t.family_name || t.title)) continue
    if (t.anchor_family_id && t.anchor_family_id !== anchor) continue
    const fam = t.family_id || t.product_id
    if (seen.has(fam)) continue
    seen.add(fam)
    out.push(t)
    if (out.length >= 2) break
  }
  return out
}

/** Подпись строки «Вместе с этим»: причина сервера без дат, нет причины — название класса дополнения. */
export function togetherReason(t: Pick<PanelTogether, 'reason' | 'label'>): string {
  const r = noDates(t.reason ?? '').trim()
  return r || t.label || ''
}

export function isEmptyView(v: PanelView): boolean {
  return (
    !v.refinements.length &&
    !v.suggestions.length &&
    !v.categories.length &&
    !v.vendors.length &&
    !v.products.length
  )
}

// ── Элементы списка: один линейный порядок для стрелок ──

export type OptionKind =
  | 'recent'
  | 'popular'
  | 'suggestion'
  | 'category'
  | 'vendor'
  | 'product'
  | 'product_add'
  | 'together'
  | 'rec'
  | 'rec_add'
  | 'all'
  | 'legacy'

export interface PanelOption {
  id: string
  kind: OptionKind
  /** Подпись группы — для объявления скринридеру вместе с элементом. */
  group: string
  label: string
  /** Позиция внутри группы, с 1. */
  position: number
  text?: string
  suggestion?: PanelSuggestion
  category?: PanelCategory
  vendor?: PanelVendor
  product?: PanelProduct
  together?: PanelTogether
  rec?: Companion
  legacyType?: string
}

export interface OptionsInput {
  /** Пустая строка или один символ: недавние и популярные. */
  idle: boolean
  recent: string[]
  popular: PanelSuggestion[]
  view: PanelView | null
  /** Товары прежнего запроса под новым текстом — не кликабельны и в обход не входят. */
  productsStale: boolean
  recs: PanelRecs | null
  recsLimit: number
  mobile: boolean
  /** Строка «Все результаты» — только когда есть текст. */
  withAll: boolean
  legacyRows?: { type: string; label: string; rows: { key: string; text: string }[] }[]
}

export const GROUP_LABELS = {
  recent: 'Недавние запросы',
  popular: 'Популярные запросы',
  suggestions: 'Подсказки',
  categories: 'Категории',
  vendors: 'Производители',
  products: 'Товары',
  together: 'Вместе с этим',
  interest: 'Вам может быть интересно',
  recs: 'Рекомендации',
  all: 'Все результаты',
} as const

const optId = (kind: OptionKind, i: number) => `panel-opt-${kind}-${i}`

/**
 * Линейный порядок элементов: сначала левая колонка (подсказки, категории, производители), потом товары,
 * рекомендации и «Все результаты». На мобильном порядок экрана другой: подсказки, товары, категории и
 * производители, «Все результаты», рекомендации.
 */
/**
 * Что стоит в нижнем блоке панели: дополнения к первому товару («Вместе с этим»). Проверенные дополнения к
 * спецификации и к точному артикулу важнее — тогда пусто, и блок показывает их; подборка по категории уступает.
 */
export function bottomTogether(
  together: PanelTogether[],
  recs: PanelRecs | null,
  limit: number,
): PanelTogether[] {
  if (recs?.items?.length && (recs.basis === 'spec' || recs.basis === 'exact_sku')) return []
  return together.slice(0, limit)
}

export function buildOptions(input: OptionsInput): PanelOption[] {
  const out: PanelOption[] = []
  const push = (o: Omit<PanelOption, 'id'>) =>
    out.push({ ...o, id: optId(o.kind, out.filter((x) => x.kind === o.kind).length) })

  const recsPart = () => {
    const together = bottomTogether(
      input.idle || input.productsStale || input.legacyRows ? [] : (input.view?.together ?? []),
      input.recs,
      input.recsLimit,
    )
    if (together.length) {
      together.forEach((t, j) =>
        push({
          kind: 'together',
          group: GROUP_LABELS.together,
          label: t.family_name || t.title,
          together: t,
          position: j + 1,
        }),
      )
      return
    }
    if (!input.recs?.items?.length) return
    const group = input.recs.title || GROUP_LABELS.recs
    input.recs.items.slice(0, input.recsLimit).forEach((rec, i) => {
      push({ kind: 'rec', group, label: rec.title, position: i + 1, rec })
      push({
        kind: 'rec_add',
        group,
        label: `Добавить в спецификацию: ${rec.title}`,
        position: i + 1,
        rec,
      })
    })
  }

  if (input.idle) {
    input.recent.forEach((text, i) =>
      push({ kind: 'recent', group: GROUP_LABELS.recent, label: text, text, position: i + 1 }),
    )
    input.popular.forEach((s, i) =>
      push({
        kind: 'popular',
        group: GROUP_LABELS.popular,
        label: s.text,
        text: s.text,
        suggestion: s,
        position: i + 1,
      }),
    )
    recsPart()
    return out
  }

  if (input.legacyRows) {
    for (const g of input.legacyRows)
      g.rows.forEach((r, i) =>
        push({
          kind: 'legacy',
          group: g.label,
          label: r.text,
          text: r.text,
          position: i + 1,
          legacyType: g.type,
        }),
      )
    if (input.withAll)
      push({ kind: 'all', group: GROUP_LABELS.all, label: GROUP_LABELS.all, position: 1 })
    return out
  }

  const v = input.view
  const suggestions = () =>
    v?.suggestions.forEach((s, i) =>
      push({
        kind: 'suggestion',
        group: GROUP_LABELS.suggestions,
        label: s.text,
        text: s.text,
        suggestion: s,
        position: i + 1,
      }),
    )
  const catsVendors = () => {
    v?.categories.forEach((c, i) =>
      push({
        kind: 'category',
        group: GROUP_LABELS.categories,
        label: c.name,
        category: c,
        position: i + 1,
      }),
    )
    v?.vendors.forEach((x, i) =>
      push({
        kind: 'vendor',
        group: GROUP_LABELS.vendors,
        label: x.name,
        vendor: x,
        position: i + 1,
      }),
    )
  }
  const products = () => {
    if (input.productsStale) return
    v?.products.forEach((p, i) => {
      push({
        kind: 'product',
        group: GROUP_LABELS.products,
        label: p.title,
        product: p,
        position: i + 1,
      })
      if (p.sku_exact)
        push({
          kind: 'product_add',
          group: GROUP_LABELS.products,
          label: `В спецификацию: ${p.title}`,
          product: p,
          position: i + 1,
        })
    })
  }
  const all = () => {
    if (input.withAll)
      push({ kind: 'all', group: GROUP_LABELS.all, label: GROUP_LABELS.all, position: 1 })
  }

  if (input.mobile) {
    suggestions()
    products()
    catsVendors()
    all()
    recsPart()
  } else {
    suggestions()
    catsVendors()
    products()
    recsPart()
    all()
  }
  return out
}

/** Следующий индекс при стрелке: -1 — ничего не выделено (Enter ведёт в выдачу). */
export function moveActive(current: number, count: number, dir: 1 | -1): number {
  if (count <= 0) return -1
  if (current < 0) return dir === 1 ? 0 : count - 1
  const next = current + dir
  // Выход за край возвращает фокус в строку: снова ничего не выделено.
  if (next < 0 || next >= count) return -1
  return next
}

// ── Выделение продолжения: набранная часть обычным начертанием, продолжение — жирным ──

export interface SuggestionPart {
  text: string
  /** true — предсказанное продолжение (выделяется), false — набранное. */
  continuation: boolean
}

export function suggestionParts(
  text: string,
  typed: string,
  ranges?: [number, number][] | null,
): SuggestionPart[] {
  let typedRanges: [number, number][] = []
  if (ranges?.length) {
    typedRanges = ranges
      .filter((r) => Array.isArray(r) && r.length === 2)
      .map(
        ([a, b]) =>
          [Math.max(0, Math.min(text.length, a)), Math.max(0, Math.min(text.length, b))] as [
            number,
            number,
          ],
      )
      .filter(([a, b]) => b > a)
      .sort((x, y) => x[0] - y[0])
  } else {
    const t = typed.trim().toLowerCase()
    if (t && text.toLowerCase().startsWith(t)) typedRanges = [[0, t.length]]
  }
  if (!typedRanges.length) return [{ text, continuation: false }]
  const parts: SuggestionPart[] = []
  let pos = 0
  for (const [a, b] of typedRanges) {
    if (a < pos) continue
    if (a > pos) parts.push({ text: text.slice(pos, a), continuation: true })
    parts.push({ text: text.slice(a, b), continuation: false })
    pos = b
  }
  if (pos < text.length) parts.push({ text: text.slice(pos), continuation: true })
  return parts.filter((p) => p.text.length > 0)
}

// ── Недавние запросы: localStorage, недоступное хранилище — пусто, без ошибок ──

export interface KeyValueStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export function parseRecent(raw: string | null): string[] {
  if (!raw) return []
  try {
    const v: unknown = JSON.parse(raw)
    if (!Array.isArray(v)) return []
    return v
      .filter((x): x is string => typeof x === 'string' && x.trim().length > 0)
      .slice(0, RECENT_MAX)
  } catch {
    return []
  }
}

export function loadRecent(storage: KeyValueStorage | null | undefined): string[] {
  try {
    return parseRecent(storage?.getItem(RECENT_KEY) ?? null)
  } catch {
    return []
  }
}

export function saveRecent(storage: KeyValueStorage | null | undefined, list: string[]): void {
  try {
    if (!storage) return
    if (list.length) storage.setItem(RECENT_KEY, JSON.stringify(list.slice(0, RECENT_MAX)))
    else storage.removeItem(RECENT_KEY)
  } catch {
    // Хранилище недоступно (приватный режим) — недавние живут до перезагрузки.
  }
}

/** Новый запрос наверх; повтор (без учёта регистра и пробелов) не дублируется. */
export function withRecent(list: string[], q: string): string[] {
  const t = q.trim()
  if (!t) return list
  const key = t.toLowerCase().replace(/\s+/g, ' ')
  return [t, ...list.filter((x) => x.trim().toLowerCase().replace(/\s+/g, ' ') !== key)].slice(
    0,
    RECENT_MAX,
  )
}

export function withoutRecent(list: string[], q: string): string[] {
  return list.filter((x) => x !== q)
}

// ── Сеть: ключ кэша и порядковый номер ответа (RESEARCH А «Клиент») ──

export function cacheKey(q: string): string {
  return q.trim().toLowerCase().replace(/\s+/g, ' ')
}

/** Кэш ответов на сессию вкладки: ограничен по размеру, старые вытесняются первыми. */
export class ResponseCache<T> {
  private map = new Map<string, T>()
  private max: number
  constructor(max = 200) {
    this.max = max
  }
  get(key: string): T | undefined {
    return this.map.get(key)
  }
  set(key: string, value: T) {
    this.map.delete(key)
    this.map.set(key, value)
    while (this.map.size > this.max) {
      const first = this.map.keys().next().value
      if (first === undefined) break
      this.map.delete(first)
    }
  }
  get size() {
    return this.map.size
  }
}

/** Порядковый номер запроса: показываем только ответ на последний (отмена сама гонку не исключает). */
export class Sequencer {
  private last = 0
  next(): number {
    this.last += 1
    return this.last
  }
  isLatest(n: number): boolean {
    return n === this.last
  }
}

// ── Уточнения ──

/** Структурированное уточнение (есть `field`) не меняет текст строки, остальные дописывают слово. */
export function isStructured(r: PanelRefinement): boolean {
  return Boolean(r.field) && r.value !== null && r.value !== undefined
}

export const sameRefinement = (a: PanelRefinement, b: PanelRefinement) =>
  a.field === b.field && a.value === b.value && a.label === b.label

/**
 * Текст для запроса панели с применёнными структурированными уточнениями: строку не трогаем, а запрос к
 * /api/suggest строим так, чтобы сервер видел условия (у каждого уточнения готовый `query`).
 */
export function refinedQuery(text: string, applied: PanelRefinement[]): string {
  let cur = text.trim()
  for (const r of applied) {
    const label = r.label.replace(/^\+\s*/, '').trim()
    const q = (r.query ?? '').trim()
    // Готовый `query` берём, только если он ровно «текущее + это уточнение»: снятое раньше условие в него
    // могло войти — тогда дописываем подпись.
    const rest = q.toLowerCase().startsWith(cur.toLowerCase()) ? q.slice(cur.length).trim() : null
    cur = rest !== null && rest.toLowerCase() === label.toLowerCase() ? q : `${cur} ${label}`.trim()
  }
  return cur
}

export function refinementFilters(applied: PanelRefinement[]): SearchFilter[] {
  return applied.filter(isStructured).map((r) => ({
    key: r.field as string,
    op: r.op ?? 'eq',
    value: r.value as string | number | boolean,
  }))
}

/** Бейджи к показу: уже применённые не повторяются в предложенных; ведущие в пустоту отброшены. */
export function offeredRefinements(
  all: PanelRefinement[],
  applied: PanelRefinement[],
): PanelRefinement[] {
  return all.filter((r) => r.hits !== 0 && !applied.some((a) => sameRefinement(a, r)))
}

// ── Намерение (§11.1) ──

export function newIntentId(): string {
  const c = (globalThis as { crypto?: Crypto }).crypto
  if (c?.randomUUID) return c.randomUUID()
  return `i-${Date.now().toString(16)}-${Math.random().toString(16).slice(2, 10)}`
}

/** Карточка товара в панели: строки по типу. */
export function productLines(p: PanelProduct): {
  primary: string
  secondary: string
  meta: string
  count: string
} {
  const hw = p.kind === 'hardware'
  return {
    primary: hw ? p.title : p.family_name || p.title,
    secondary: hw ? p.mpn || '' : p.vendor,
    meta: hw ? [p.specs_line, p.delivery_line].filter(Boolean).join(' · ') : p.license_line,
    count:
      p.variants_count > 1
        ? hw
          ? `${p.variants_count} ${plural(p.variants_count, 'конфигурация', 'конфигурации', 'конфигураций')}`
          : `${p.variants_count} ${plural(p.variants_count, 'вариант', 'варианта', 'вариантов')} лицензии`
        : '',
  }
}

export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10
  const m100 = n % 100
  if (m10 === 1 && m100 !== 11) return one
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few
  return many
}

/** Цена в карточке панели: «от N ₽», «N ₽» или «Цена по запросу». */
export function panelPrice(
  p: Pick<PanelProduct, 'price_from_rub' | 'price_mode'>,
  fmt: (n: number) => string,
): string {
  if (p.price_mode === 'unavailable') return 'Нет в наличии'
  // Нулевая цена — «по запросу», «0 ₽» не выводится.
  if (
    p.price_from_rub === null ||
    p.price_from_rub === undefined ||
    !(p.price_from_rub > 0) ||
    p.price_mode === 'quote_required'
  )
    return 'Цена по запросу'
  return p.price_mode === 'from' || p.price_mode === ''
    ? `от ${fmt(p.price_from_rub)}`
    : fmt(p.price_from_rub)
}
