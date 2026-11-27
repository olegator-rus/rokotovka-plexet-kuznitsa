// Спецификация покупателя: позиции, количества, происхождение, итог, текст для
// копирования, тело POST /api/quotes, хранение. Без DOM: tests/spec.test.ts.
import type { Companion, QuoteOrigin, QuoteRequest } from '@/api/typesP2'
import type { SearchItemBase } from '@/api/types'
import { formatPrice, isPositivePrice, plural } from './format.ts'
import { parsePurchase, purchaseNote, purchaseQuantity, type SpecPurchase } from './procurement.ts'
import { tzSourceLabel } from './tz.ts'

export const SPEC_MAX_QTY = 100000

export interface SpecLine {
  product_id: string
  title: string
  vendor: string
  category: string
  license_line: string
  price_rub: number | null
  /** fixed | from | quote_required | customer_specific | unavailable. */
  price_mode: string
  quantity: number
  origin: QuoteOrigin
  /** Класс компаньона, если позиция пришла из блока или строки условно обязательного. */
  companion_class?: string
  /** К какой позиции добавлено (название якоря) — для пометки происхождения. */
  anchor?: string
  /** Контур доводки: исходный запрос и параметры закупки — уходят в заявку КП. */
  purchase?: SpecPurchase
  /** Контур ТЗ: из какого ТЗ и каких его пунктов строка («по ТЗ, п. 1 и 4»). */
  tz?: SpecTz
}

export interface SpecTz {
  hash: string
  file_name: string
  positions: number[]
}

/** Режим цены позиции выдачи: поле контура коммерции, иначе по старому флагу. */
export function priceModeOf(item: Pick<SearchItemBase, 'price_on_request' | 'price_mode'>): string {
  if (item.price_mode) return item.price_mode
  return item.price_on_request ? 'quote_required' : 'fixed'
}

/** Цена известна (её можно сложить в итог): fixed или «от», и число есть. */
export function hasKnownPrice(line: Pick<SpecLine, 'price_rub' | 'price_mode'>): boolean {
  return (
    (line.price_mode === 'fixed' || line.price_mode === 'from') && isPositivePrice(line.price_rub)
  )
}

/** Цена за единицу для показа по режиму цены. */
export function priceModeLabel(line: Pick<SpecLine, 'price_rub' | 'price_mode'>): string {
  if (line.price_mode === 'unavailable') return 'Нет в наличии'
  if (!hasKnownPrice(line)) return 'Цена по запросу'
  const p = formatPrice(line.price_rub)
  return line.price_mode === 'from' ? `от ${p}` : p
}

export function clampQty(n: unknown): number {
  const v = typeof n === 'number' ? n : Number(n)
  if (!Number.isFinite(v)) return 1
  return Math.min(SPEC_MAX_QTY, Math.max(1, Math.round(v)))
}

export function lineFromItem(
  item: SearchItemBase,
  quantity = 1,
  purchase: SpecPurchase | null = null,
): SpecLine {
  return {
    ...(purchase ? { purchase } : {}),
    product_id: item.product_id,
    title: item.title,
    vendor: item.vendor,
    category: item.category,
    license_line: item.license_line,
    price_rub: item.price_rub,
    price_mode: priceModeOf(item),
    quantity: clampQty(quantity),
    origin: 'search',
  }
}

export function lineFromCompanion(c: Companion, quantity: number | null, anchor = ''): SpecLine {
  return {
    product_id: c.product_id,
    title: c.title,
    vendor: c.vendor,
    category: c.category,
    license_line: c.license_line,
    price_rub: c.price_rub,
    price_mode: c.price_mode || 'fixed',
    quantity: clampQty(quantity ?? 1),
    origin: 'companion',
    companion_class: c.class,
    anchor: anchor || undefined,
  }
}

/** Строка из позиции ТЗ: количество и условия — из ТЗ, происхождение «из ТЗ», номер пункта. */
export function lineFromTz(
  item: SearchItemBase,
  quantity: number,
  purchase: SpecPurchase | null,
  tz: { hash: string; file_name: string; number: number },
): SpecLine {
  return {
    ...lineFromItem(item, quantity, purchase),
    origin: 'tz',
    tz: { hash: tz.hash, file_name: tz.file_name, positions: [tz.number] },
  }
}

/**
 * Добавить позицию: та же позиция уже есть — количество не складывается, а берётся новое (покупатель
 * второй раз добавляет ту же лицензию с уточнённым числом). Порядок сохраняется.
 * Контур ТЗ: тот же товар из другого пункта того же ТЗ — одна строка, количество складывается, номер пункта
 * дописывается («по ТЗ, п. 1 и 4»); повтор из того же пункта — как раньше, без дубля.
 */
export function addLine(lines: readonly SpecLine[], line: SpecLine): SpecLine[] {
  const i = lines.findIndex((l) => l.product_id === line.product_id)
  if (i < 0) return [...lines, { ...line, quantity: clampQty(line.quantity) }]
  const next = [...lines]
  const prev = next[i] as SpecLine
  if (line.origin === 'tz' && line.tz) {
    const fresh = line.tz.positions.filter((n) => !(prev.tz?.positions ?? []).includes(n))
    if (prev.tz && prev.tz.hash === line.tz.hash && fresh.length) {
      next[i] = {
        ...prev,
        origin: 'tz',
        quantity: clampQty(prev.quantity + line.quantity),
        tz: { ...prev.tz, positions: [...prev.tz.positions, ...fresh].sort((a, b) => a - b) },
      }
      return next
    }
    // Строка из поиска или из другого ТЗ становится строкой этого пункта: подпись и отметка — по нему.
    next[i] = {
      ...prev,
      origin: 'tz',
      quantity: clampQty(line.quantity),
      tz: prev.tz?.hash === line.tz.hash ? prev.tz : line.tz,
      ...(line.purchase ? { purchase: line.purchase } : {}),
    }
    return next
  }
  next[i] = { ...prev, quantity: clampQty(line.quantity) }
  return next
}

/** Пункты ТЗ с хешем hash, из которых в спецификации есть хотя бы один товар. */
export function tzPositionsInSpec(lines: readonly SpecLine[], hash: string): Set<number> {
  const out = new Set<number>()
  for (const l of lines) if (l.tz?.hash === hash) for (const n of l.tz.positions) out.add(n)
  return out
}

export function removeLine(lines: readonly SpecLine[], productId: string): SpecLine[] {
  return lines.filter((l) => l.product_id !== productId)
}

export function setQuantity(lines: readonly SpecLine[], productId: string, qty: unknown) {
  return lines.map((l) => (l.product_id === productId ? { ...l, quantity: clampQty(qty) } : l))
}

/** Пометка происхождения строки. */
export function originLabel(line: Pick<SpecLine, 'origin' | 'companion_class'>): string {
  if (line.origin === 'template') return 'из шаблона'
  if (line.origin === 'tz') return 'из ТЗ'
  if (line.origin === 'companion') {
    if (line.companion_class === 'conditional_required') return 'условно обязательное'
    // Старшая редакция — замена базовой позиции, а не дополнение к ней.
    if (line.companion_class === 'upgrade') return 'старшая редакция'
    return 'дополнение'
  }
  return 'из поиска'
}

/**
 * К какой позиции добавлена строка («к «Сервер …»») или '' — у старшей редакции якоря нет: базовой позиции в
 * спецификации может не быть, а ссылка на отсутствующую путает.
 */
export function anchorOf(line: Pick<SpecLine, 'anchor' | 'companion_class'>): string {
  if (!line.anchor || line.companion_class === 'upgrade') return ''
  return line.anchor
}

/**
 * Подпись условий строки: у строки из ТЗ «по ТЗ, п. N» встаёт на место «по запросу «…»» (TZ-UI.md §9), условия
 * закупки остаются.
 */
export function sourceNote(line: Pick<SpecLine, 'purchase' | 'tz'>): string {
  if (!line.tz) return purchaseNote(line.purchase)
  const terms = line.purchase ? purchaseNote({ ...line.purchase, query: '' }) : ''
  return [terms, tzSourceLabel(line.tz.positions)].filter(Boolean).join(' · ')
}

export interface SpecSummary {
  positions: number
  onRequest: number
  /** Сумма известных цен × количество; null — известных цен нет. */
  knownRub: number | null
  /** Все цены известны и точные (без «от»). */
  exact: boolean
  text: string
}

export function summarize(lines: readonly SpecLine[]): SpecSummary {
  const positions = lines.length
  let onRequest = 0
  let known = 0
  let anyKnown = false
  let anyFrom = false
  for (const l of lines) {
    if (!hasKnownPrice(l)) {
      onRequest++
      continue
    }
    anyKnown = true
    if (l.price_mode === 'from') anyFrom = true
    known += (l.price_rub as number) * l.quantity
  }
  const knownRub = anyKnown ? known : null
  const exact = positions > 0 && onRequest === 0 && !anyFrom
  const head = `${positions} ${plural(positions, ['позиция', 'позиции', 'позиций'])}`
  let text: string
  if (positions === 0) text = 'Позиций нет'
  else if (onRequest === 0) text = `${head} · итого ${anyFrom ? 'от ' : ''}${formatPrice(known)}`
  else if (knownRub === null) text = `${head} · у ${onRequest} цена по запросу`
  else
    text = `${head} · у ${onRequest} цена по запросу · известная часть от ${formatPrice(knownRub)}`
  return { positions, onRequest, knownRub, exact, text }
}

/** Текст «Скопировать спецификацию»: строка на позицию и итог. */
export function specText(lines: readonly SpecLine[]): string {
  const rows = lines.map((l, i) => {
    const price = priceModeLabel(l)
    const lic = l.license_line ? ` (${l.license_line})` : ''
    const anchor = anchorOf(l) ? ` к «${anchorOf(l)}»` : ''
    const tz = l.tz ? `, ${tzSourceLabel(l.tz.positions)}` : ''
    return `${i + 1}. ${l.title}${lic} × ${l.quantity} — ${price} — ${originLabel(l)}${anchor}${tz}`
  })
  return ['Спецификация', ...rows, summarize(lines).text].join('\n')
}

/** Тело POST /api/quotes. */
export function quoteRequest(
  lines: readonly SpecLine[],
  ctx: { sessionId: string; clientId: string | null; comment?: string },
): QuoteRequest {
  const body: QuoteRequest = {
    session_id: ctx.sessionId,
    client_id: ctx.clientId,
    items: lines.map((l) => ({
      product_id: l.product_id,
      quantity: l.quantity,
      origin: l.origin,
      ...(l.companion_class ? { companion_class: l.companion_class } : {}),
      ...(l.purchase ? { purchase: l.purchase } : {}),
    })),
  }
  if (ctx.comment?.trim()) body.comment = ctx.comment.trim()
  return body
}

const ORIGINS: readonly string[] = ['search', 'companion', 'template', 'tz']

function parseTz(x: unknown): SpecTz | undefined {
  if (!x || typeof x !== 'object') return undefined
  const r = x as Record<string, unknown>
  if (typeof r.hash !== 'string' || !r.hash || !Array.isArray(r.positions)) return undefined
  const positions = [
    ...new Set(r.positions.filter((n): n is number => Number.isInteger(n) && (n as number) > 0)),
  ]
  if (!positions.length) return undefined
  return {
    hash: r.hash.slice(0, 128),
    file_name: typeof r.file_name === 'string' ? r.file_name.slice(0, 300) : '',
    positions: positions.sort((a, b) => a - b),
  }
}

/** Разбор сохранённой спецификации: битое и чужое отбрасывается построчно. */
export function parseStored(raw: string | null): SpecLine[] {
  if (!raw) return []
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(data)) return []
  const out: SpecLine[] = []
  for (const x of data) {
    if (!x || typeof x !== 'object') continue
    const r = x as Record<string, unknown>
    if (typeof r.product_id !== 'string' || !r.product_id || typeof r.title !== 'string') continue
    if (out.some((l) => l.product_id === r.product_id)) continue
    out.push({
      product_id: r.product_id,
      title: r.title,
      vendor: typeof r.vendor === 'string' ? r.vendor : '',
      category: typeof r.category === 'string' ? r.category : '',
      license_line: typeof r.license_line === 'string' ? r.license_line : '',
      price_rub: typeof r.price_rub === 'number' ? r.price_rub : null,
      price_mode: typeof r.price_mode === 'string' ? r.price_mode : 'quote_required',
      quantity: clampQty(r.quantity),
      origin: ORIGINS.includes(r.origin as string) ? (r.origin as QuoteOrigin) : 'search',
      ...(typeof r.companion_class === 'string' ? { companion_class: r.companion_class } : {}),
      ...(typeof r.anchor === 'string' && r.anchor ? { anchor: r.anchor } : {}),
      ...(parsePurchase(r.purchase) ? { purchase: parsePurchase(r.purchase) } : {}),
      ...(parseTz(r.tz) ? { tz: parseTz(r.tz) } : {}),
    })
  }
  return out
}

/**
 * Количество для позиции из выдачи по числу из запроса («на 50 пользователей», чипы license_quantity и
 * license_metric): поштучная лицензия — само число, пакет на N мест той же метрики — ceil(число / N), серверная
 * лицензия на ядра — 1 (покупатель поправит в панели). Правило одно с «Выбрано по запросу» (purchaseQuantity).
 */
export function quantityForItem(
  item: Pick<SearchItemBase, 'license_line' | 'attrs'>,
  chips: readonly { field: string; value: string }[],
): number {
  const chip = chips.find((c) => c.field === 'license_quantity')
  const n = chip ? Number(chip.value) : NaN
  if (!Number.isFinite(n) || n <= 0) return 1
  const metric = chips.find((c) => c.field === 'license_metric')?.value
  return clampQty(purchaseQuantity(item, n, metric))
}
