// Режим подбора в поиске (контур подбора, SPEC §1, §6, §8): когда предлагать подбор, сборка таблицы сравнения,
// «только различия», подписи происхождения условий, цена оффера, строка спецификации. Без DOM: tests/guide.test.ts.
import type { SearchResponse } from '@/api/types'
import type {
  GuideCell,
  GuideChip,
  GuideClaim,
  GuideComparison,
  GuideCondition,
  GuideOffer,
  GuideResponse,
  GuideRoute,
  GuideSpecLine,
} from '@/api/typesGuide'
import { formatDecimal, formatInt, formatPrice, isPositivePrice } from './format.ts'
import type { SpecPurchase } from './procurement.ts'
import { clampQty, type SpecLine } from './spec.ts'

// ── Три состояния строки (SPEC §1) ──

export type GuideMode = 'search' | 'offer' | 'guide'

/**
 * Вопрос, на который отвечает подбор: «чем отличается…», «что взять для…», «подбери», «самый дешёвый …» (проверяемое
 * утверждение пересчитывает код среди подходящих, SPEC §7).
 */
const QUESTION =
  /(чем\s+(\S+\s+){0,3}отлича|в\s+ч[её]м\s+разниц|разниц[аы]\s+между|что\s+(лучше\s+)?(взять|выбрать|купить)\s+(для|на|под)|подбер|помоги(те)?\s+(выбрать|подобрать)|какую\s+(редакцию|лицензию)\s+(взять|выбрать)|^сравни|сам(ый|ая|ое|ую)\s+(дешёв|дешев|недорог))/i

/** Запрос задачей, а не названием: «защита для офиса», «решение по безопасности». */
const TASK = /(защит|безопасност|для\s+офис|для\s+компани|для\s+предприят|решени[ея]\s+(для|по))/i

/** Число мест в тексте: «на 50 пк», «30 компьютеров», «для трёх админов» не считается. */
const COUNT = /\b\d+\s*(пк|компьютер|мест|пользовател|сотрудник|рабоч|устройств|узл|сервер|лиценз)/i

const KNOWN_MODES: readonly string[] = ['search', 'offer', 'guide']

/**
 * Режим строки для текущей выдачи. Есть признак сервера (`response.guide.mode`) — он главный. Иначе по запросу
 * и цели поиска:
 * - вопрос («чем отличается…», «что взять для…», «подбери») — сразу подбор;
 * - ответ справки, сервисная цель, пустая выдача — обычный поиск;
 * - полные условия (тип покупки, количество и срок разобрал нормализатор) — обычный поиск;
 * - цель — артикул или семейство без числа мест и без задачи — обычный поиск («криптопро csp 5.0»);
 * - иначе — выдача и предложение «Помочь выбрать» (категория, производитель, задача, число мест без продукта).
 */
export function guideMode(
  query: string,
  resp: Pick<SearchResponse, 'guide' | 'target' | 'purchase' | 'help' | 'items' | 'chips'> | null,
): GuideMode {
  const server = resp?.guide?.mode
  if (server && KNOWN_MODES.includes(server)) return server as GuideMode
  const q = query.trim()
  if (!q) return 'search'
  if (QUESTION.test(q)) return 'guide'
  if (!resp) return 'search'
  if (resp.help?.mode === 'answer') return 'search'
  if (resp.target?.type === 'service') return 'search'
  if (!resp.items?.length) return 'search'
  const p = resp.purchase
  if (p?.deal && p.quantity && p.duration_months) return 'search'
  const counted =
    Boolean(p?.quantity) ||
    (resp.chips ?? []).some((c) => c.field === 'license_quantity') ||
    COUNT.test(q)
  const task = TASK.test(q)
  const t = resp.target?.type
  if ((t === 'sku' || t === 'family') && !counted && !task) return 'search'
  return 'offer'
}

/** Режим из POST /api/guide/route: search_offer — «Поиск + предложение»; неизвестное — null (решают правила). */
export function routeMode(r: Pick<GuideRoute, 'mode'> | null | undefined): GuideMode | null {
  if (r?.mode === 'search_offer') return 'offer'
  if (r?.mode === 'search' || r?.mode === 'guide') return r.mode
  return null
}

// ── Чипы и условия ──

export interface ChipView {
  label: string
  value: string
  /** Какое условие задаёт чип; нет у чипа — ключ вопроса. */
  key: string
}

export function chipView(c: GuideChip, questionKey = ''): ChipView {
  if (typeof c === 'string') return { label: c, value: c, key: questionKey }
  return { label: c.label, value: c.value ?? c.label, key: c.key || questionKey }
}

/** «Другое» среди чипов вопроса — не ответ, а приглашение ввести число или текст. */
export function isOtherChip(c: ChipView): boolean {
  return /^(друго[ей]|другое количество|свой вариант)$/i.test(c.label.trim())
}

/** Подпись происхождения условия: введённое покупателем не подписывается. */
export function originLabel(origin: string): string {
  if (origin === 'default') return 'по умолчанию'
  if (origin === 'inferred') return 'по запросу'
  return ''
}

/**
 * Текст правки условия для поля message — запасной путь, пока ядро не принимает `condition`: «без условия «1 год»»
 * или новое значение как его ввёл покупатель.
 */
export function conditionMessage(c: Pick<GuideCondition, 'label' | 'value'>, value: string | null) {
  if (value === null) return `без условия «${c.label || c.value}»`
  return value
}

/** Текст чипа условия для экранного чтения: «1 год — по умолчанию». */
export function conditionAria(c: GuideCondition): string {
  const o = originLabel(c.origin)
  return `${c.label || c.value}${o ? ` — ${o}` : ''}`
}

// ── Таблица сравнения (SPEC §8) ──

export const CELL_TEXT = {
  yes: 'есть',
  no: 'нет по данным вендора',
  unknown: 'нет сведений',
  depends: 'зависит от ваших условий',
} as const

export type CellTone = 'yes' | 'no' | 'value' | 'unknown' | 'depends'

export interface CellView {
  tone: CellTone
  text: string
  factId: string | null
}

/** Строка с деньгами: число в ней — рубли (ядро отдаёт цену и итог числом). */
const MONEY_ROW = /(price|total|sum|rub|cost)/i

/** Число ячейки: в денежной строке — «2 813,40 ₽» (0 и меньше — пусто), иначе с разрядами. */
function numberText(n: number, money: boolean): string {
  if (money) return formatPrice(n)
  if (!Number.isFinite(n)) return ''
  return Number.isInteger(n) ? formatInt(n) : formatDecimal(n, 2)
}

/**
 * Ячейка: пустое значение — «нет сведений», а не «нет функции». rowKey — ключ строки: в денежной строке число
 * выводится ценой, «0 ₽» — никогда.
 */
export function cellView(cell: GuideCell | undefined, rowKey = ''): CellView {
  const factId = cell?.fact_id || null
  const kind = cell?.kind
  if (kind === 'yes') return { tone: 'yes', text: CELL_TEXT.yes, factId }
  if (kind === 'no') return { tone: 'no', text: CELL_TEXT.no, factId }
  if (kind === 'depends') return { tone: 'depends', text: CELL_TEXT.depends, factId }
  if (kind === 'value') {
    const v = cell?.value
    const text =
      typeof v === 'number'
        ? numberText(v, MONEY_ROW.test(rowKey))
        : v === null || v === undefined
          ? ''
          : String(v).trim()
    if (text) return { tone: 'value', text, factId }
  }
  return { tone: 'unknown', text: CELL_TEXT.unknown, factId }
}

export interface RowView {
  key: string
  label: string
  note: string
  cells: CellView[]
  /** Значения по колонкам расходятся. */
  differs: boolean
}

export interface TableView {
  columns: { id: string; title: string; url: string }[]
  rows: RowView[]
  /** Строк всего (до «только различия»). */
  total: number
  /** Строк с различием. */
  differing: number
}

/** Расходятся ли значения строки. «Нет сведений» везде — не различие; «нет сведений» против «есть» — различие. */
export function rowDiffers(cells: readonly CellView[]): boolean {
  if (cells.length < 2) return false
  const keys = new Set(cells.map((c) => `${c.tone}:${c.text.toLowerCase()}`))
  return keys.size > 1
}

/**
 * Таблица для показа. `columns` — индексы колонок (на телефоне две, остальные переключателем); различие
 * считается по показанным колонкам.
 */
export function buildTable(
  cmp: GuideComparison | null | undefined,
  opts: { onlyDiff?: boolean; columns?: readonly number[] } = {},
): TableView {
  const all = cmp?.columns ?? []
  const idx = (opts.columns ?? all.map((_, i) => i)).filter((i) => i >= 0 && i < all.length)
  const columns = idx.map((i) => {
    const c = all[i]!
    return { id: c.family_id, title: c.title, url: c.family_url }
  })
  const rows: RowView[] = (cmp?.rows ?? []).map((r) => {
    const cells = idx.map((i) => cellView(r.cells[i], r.key))
    return { key: r.key, label: r.label, note: r.note ?? '', cells, differs: rowDiffers(cells) }
  })
  const differing = rows.filter((r) => r.differs).length
  return {
    columns,
    rows: opts.onlyDiff ? rows.filter((r) => r.differs) : rows,
    total: rows.length,
    differing,
  }
}

/** Индексы колонок для показа: на узком экране — первая и выбранная, иначе все. */
export function visibleColumns(count: number, narrow: boolean, picked: number): number[] {
  const all = Array.from({ length: count }, (_, i) => i)
  if (!narrow || count <= 2) return all
  const second = picked > 0 && picked < count ? picked : 1
  return [0, second]
}

/** Факты ответа по идентификатору — для значка источника в ячейке. */
export function claimIndex(claims: readonly GuideClaim[] | null | undefined) {
  const m = new Map<string, GuideClaim>()
  for (const c of claims ?? []) if (c.fact_id) m.set(c.fact_id, c)
  return m
}

/** Факт проверен человеком: другие статусы помечаются «не проверено». */
export function claimVerified(c: Pick<GuideClaim, 'status'>): boolean {
  return c.status === 'verified' || c.status === 'проверен'
}

/** Пометка проверки у источника: человеком — без пометки, автоматически — так и пишем, иначе — «не проверено». */
export function claimMark(c: Pick<GuideClaim, 'status'>): string {
  if (claimVerified(c)) return ''
  if (c.status === 'verified_agent') return 'проверено автоматически'
  return 'не проверено'
}

/** Короткое имя источника для значка: домен страницы, если названия нет. */
export function sourceName(src: { title?: string; url?: string } | null | undefined): string {
  if (!src) return ''
  if (src.title?.trim()) return src.title.trim()
  try {
    return new URL(src.url ?? '').hostname.replace(/^www\./, '')
  } catch {
    return 'источник'
  }
}

// ── Цена и оффер (SPEC §8, «0 ₽» — никогда) ──

export interface OfferPriceView {
  /** «3 940,20 ₽», «от 1 850 ₽» или «Цена по запросу». */
  unit: string
  /** «197 010 ₽» или '' — итога нет. */
  total: string
  /** Цена известна и складывается в итог. */
  known: boolean
  /** Пометка состояния цены: '' у обычной цены каталога. */
  state: string
}

/** Состояния цены «по запросу»: ядро пишет on_request, выдача — quote_required. */
const ON_REQUEST = new Set(['quote_required', 'on_request', 'customer_specific'])

export function offerPrice(
  o: Pick<GuideOffer, 'price_per_unit' | 'total' | 'price_state'>,
): OfferPriceView {
  const st = o.price_state
  if (st === 'unavailable')
    return { unit: 'Нет в наличии', total: '', known: false, state: 'нет в наличии' }
  const priced = isPositivePrice(o.price_per_unit) && !ON_REQUEST.has(st)
  if (!priced) return { unit: 'Цена по запросу', total: '', known: false, state: 'цена по запросу' }
  const from = st === 'from'
  const unit = `${from ? 'от ' : ''}${formatPrice(o.price_per_unit)}`
  const total = isPositivePrice(o.total) ? `${from ? 'от ' : ''}${formatPrice(o.total)}` : ''
  return { unit, total, known: true, state: from ? 'цена от' : '' }
}

/** Число из условия «сколько» (count, seats, quantity…). */
export function conditionCount(conds: readonly GuideCondition[] | null | undefined): number | null {
  // Ядро пишет «count:workstation», «count:server»: первое такое условие.
  const c = (conds ?? []).find((x) =>
    /^(count|seats|quantity|objects|workstations)(:|$)/.test(x.key),
  )
  const n = c ? Number(String(c.value).replace(/[^\d]/g, '')) : NaN
  return Number.isFinite(n) && n > 0 ? n : null
}

/** Сколько единиц оффера класть в спецификацию: поле сервера → итог / цена → условие «сколько» → 1. */
export function offerQuantity(o: GuideOffer, conds: readonly GuideCondition[] = []): number {
  if (typeof o.quantity === 'number' && o.quantity > 0) return clampQty(o.quantity)
  if (isPositivePrice(o.total) && isPositivePrice(o.price_per_unit)) {
    const n = o.total / o.price_per_unit
    if (Math.abs(n - Math.round(n)) < 0.01) return clampQty(Math.round(n))
  }
  return clampQty(conditionCount(conds) ?? 1)
}

/** Параметры закупки из условий подбора — уходят в заявку КП вместе со строкой. */
export function purchaseFromConditions(
  query: string,
  conds: readonly GuideCondition[] = [],
): SpecPurchase {
  const out: SpecPurchase = { query }
  const n = conditionCount(conds)
  if (n) out.quantity = n
  for (const c of conds) {
    const v = String(c.value ?? '')
    if (c.key === 'operation' && v) out.deal = v
    if (c.key === 'org_type' && v) out.org_type = v
    if (c.key === 'edition' && v) out.edition = v
    if (c.key === 'term_months') {
      const m = Number(v.replace(/[^\d]/g, ''))
      if (m > 0) out.duration_months = m
    }
  }
  return out
}

/** Единица продажи без предлога: ядро пишет «за компьютер», выдача — «компьютер». */
export function unitText(unit: string | null | undefined): string {
  return (unit ?? '').trim().replace(/^за\s+/i, '')
}

/** Подпись к цене: «за компьютер», «за лицензию»; падеж берётся у сервера, предлог не задваивается. */
export function perUnit(unit: string | null | undefined): string {
  const u = (unit ?? '').trim()
  if (!u) return ''
  return /^за\s/i.test(u) ? u : `за ${u}`
}

/** «1 850 ₽ за рабочее место»; без цены — «Цена по запросу» без единицы. */
export function priceWithUnit(
  o: Pick<GuideOffer, 'price_per_unit' | 'total' | 'price_state' | 'unit'>,
) {
  const p = offerPrice(o)
  const u = perUnit(o.unit)
  return p.known && u ? `${p.unit} ${u}` : p.unit
}

export type OfferRole = 'match' | 'alternative' | 'rejected'

/** Роль оффера: нет поля или неизвестное значение — подходящий. */
export function offerRole(o: Pick<GuideOffer, 'role'>): OfferRole {
  return o.role === 'alternative' || o.role === 'rejected' ? o.role : 'match'
}

/** Офферы ответа по ролям: подходящие, альтернативы, «Не подходит». */
export function splitOffers(offers: readonly GuideOffer[] | null | undefined) {
  const out: Record<OfferRole, GuideOffer[]> = { match: [], alternative: [], rejected: [] }
  for (const o of offers ?? []) out[offerRole(o)].push(o)
  return out
}

/** Строка спецификации из подобранного оффера: та же спецификация и тот же КП, что у выдачи. */
export function lineFromOffer(
  o: GuideOffer,
  conds: readonly GuideCondition[],
  query: string,
): SpecLine {
  const st = o.price_state
  const mode =
    st === 'from' || st === 'unavailable'
      ? st
      : ON_REQUEST.has(st) || !isPositivePrice(o.price_per_unit)
        ? 'quote_required'
        : 'fixed'
  const parts = [o.sku ? `арт. ${o.sku}` : '', o.tier ? `ступень ${o.tier}` : '', perUnit(o.unit)]
  return {
    product_id: o.offer_id,
    title: o.title,
    vendor: '',
    category: '',
    license_line: parts.filter(Boolean).join(' · '),
    price_rub: isPositivePrice(o.price_per_unit) ? o.price_per_unit : null,
    price_mode: mode,
    quantity: offerQuantity(o, conds),
    origin: 'search',
    purchase: purchaseFromConditions(query, conds),
  }
}

/** Оффер с ценой и количеством, перечитанными из каталога ответом POST /api/guide/spec. */
export function withSpecLine(o: GuideOffer, line: GuideSpecLine | null | undefined): GuideOffer {
  if (!line || line.offer_id !== o.offer_id) return o
  return {
    ...o,
    price_per_unit: line.price_per_unit,
    total: line.total,
    price_state: line.price_state || o.price_state,
    quantity: line.quantity > 0 ? line.quantity : o.quantity,
    sku: line.sku || o.sku,
    tier: line.tier || o.tier,
  }
}

/** Идентификатор подбора из непрозрачного состояния сервера. */
export function selectionId(state: unknown): string {
  if (state && typeof state === 'object') {
    const id = (state as Record<string, unknown>).selection_id
    if (typeof id === 'string') return id
  }
  return ''
}

// ── Ответ целиком ──

/** Отказ подбора: ниже обычная выдача и чипы. */
export function isRefusal(r: Pick<GuideResponse, 'type'> | null | undefined): boolean {
  return r?.type === 'no_data' || r?.type === 'out_of_scope'
}

/** Есть таблица со строками. */
export function hasTable(r: Pick<GuideResponse, 'comparison'> | null | undefined): boolean {
  return Boolean(r?.comparison?.columns?.length && r.comparison.rows?.length)
}

/** Подписи шагов, пока ход идёт (SPEC §11: статус операции сразу, проверенные строки потом). */
export const PENDING_STEPS = [
  'Ищу подходящие лицензии в каталоге',
  'Сверяю различия с документацией вендора',
  'Проверяю правила лицензирования',
] as const

/** Подпись шага по времени ожидания: через каждые 1,5 с — следующий. */
export function pendingStep(elapsedMs: number): string {
  const i = Math.min(PENDING_STEPS.length - 1, Math.max(0, Math.floor(elapsedMs / 1500)))
  return PENDING_STEPS[i]!
}
