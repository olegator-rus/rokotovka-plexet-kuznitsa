// Спецификация покупателя: позиции, количества, происхождение, итог, текст для
// копирования, тело POST /api/quotes, хранение. Без DOM: tests/spec.test.ts.
import type { Companion, QuoteOrigin, QuoteRequest } from '@/api/typesP2'
import type { SearchItemBase } from '@/api/types'
import { formatPrice, plural } from './format.ts'

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
}

/** Режим цены позиции выдачи: поле контура коммерции, иначе по старому флагу. */
export function priceModeOf(item: Pick<SearchItemBase, 'price_on_request' | 'price_mode'>): string {
  if (item.price_mode) return item.price_mode
  return item.price_on_request ? 'quote_required' : 'fixed'
}

/** Цена известна (её можно сложить в итог): fixed или «от», и число есть. */
export function hasKnownPrice(line: Pick<SpecLine, 'price_rub' | 'price_mode'>): boolean {
  return (
    (line.price_mode === 'fixed' || line.price_mode === 'from') &&
    typeof line.price_rub === 'number' &&
    Number.isFinite(line.price_rub)
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

export function lineFromItem(item: SearchItemBase, quantity = 1): SpecLine {
  return {
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

/**
 * Добавить позицию: та же позиция уже есть — количество не складывается, а берётся новое (покупатель
 * второй раз добавляет ту же лицензию с уточнённым числом). Порядок сохраняется.
 */
export function addLine(lines: readonly SpecLine[], line: SpecLine): SpecLine[] {
  const i = lines.findIndex((l) => l.product_id === line.product_id)
  if (i < 0) return [...lines, { ...line, quantity: clampQty(line.quantity) }]
  const next = [...lines]
  const prev = next[i] as SpecLine
  next[i] = { ...prev, quantity: clampQty(line.quantity) }
  return next
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
  if (line.origin === 'companion')
    return line.companion_class === 'conditional_required' ? 'условно обязательное' : 'дополнение'
  return 'из поиска'
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
    return `${i + 1}. ${l.title}${lic} × ${l.quantity} — ${price} — ${originLabel(l)}`
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
    })),
  }
  if (ctx.comment?.trim()) body.comment = ctx.comment.trim()
  return body
}

const ORIGINS: readonly string[] = ['search', 'companion', 'template']

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
    })
  }
  return out
}

/**
 * Количество для позиции из выдачи: число из запроса («на 50 пользователей») — только если лицензия
 * считается поштучно на пользователя или устройство («1 пользователь», «1 устройство»); серверная лицензия
 * на ядра или пакет получает 1 — покупатель поправит в панели.
 */
export function quantityForItem(
  item: Pick<SearchItemBase, 'license_line'>,
  chips: readonly { field: string; value: string }[],
): number {
  const chip = chips.find((c) => c.field === 'license_quantity')
  const n = chip ? Number(chip.value) : NaN
  if (!Number.isFinite(n) || n <= 0) return 1
  const perUnit = /(^|[\s·])1\s+(пользовател|устройств|пк|рабоч|компьютер|место)/i
  return perUnit.test(item.license_line ?? '') ? clampQty(n) : 1
}
