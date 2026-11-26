// Закупка и честное «нет» (контур доводки): выбранный по запросу вариант в карточке, параметры закупки
// для спецификации и заявки, статус реестра, ответ справки, поставка отсутствующего. Без DOM:
// tests/procurement.test.ts.
import type { Purchase, Reestr, SearchItemBase, SearchResponse } from '@/api/types'
import { formatPrice, isPositivePrice, plural } from './format.ts'
import { kindFromLine, LICENSE_KIND_LABELS } from './license.ts'

/** Параметры закупки строки спецификации — уходят в заявку КП вместе с исходным запросом. */
export interface SpecPurchase {
  query: string
  deal?: string
  quantity?: number
  metric?: string
  duration_months?: number
  org_type?: string
  edition?: string
}

const PER_UNIT = /(^|[\s·])1\s+(пользовател|устройств|пк|рабоч|компьютер|место|узел|сервер)/i

/** Позиция продаётся поштучно на единицу (строка лицензии «… · 1 устройство · …»). */
export function isPerUnitLine(licenseLine: string | undefined): boolean {
  return PER_UNIT.test(licenseLine ?? '')
}

/** Запрос назвал условия закупки: тип покупки, количество или срок. */
export function hasPurchaseTerms(p: Purchase | null | undefined): p is Purchase {
  return Boolean(p && (p.deal || p.quantity || p.duration_months))
}

/** Метрики, которые считаются штуками: на них число из запроса делится на размер пакета. */
const COUNTED_METRICS = new Set(['user', 'device', 'server'])

/**
 * Сколько позиций купить под число из запроса («на 50 пользователей»). Пакет на N мест (атрибут
 * license_quantity при той же метрике) — ceil(число / N): CAL на 5 пользователей при 50 — 10 пакетов, пакет на
 * 10 ПК при 10 — 1. Поштучная позиция — само число. Ядра, организация и пакет другой метрики — 1, покупатель
 * поправит в спецификации.
 */
export function purchaseQuantity(
  item: Pick<SearchItemBase, 'license_line' | 'attrs'>,
  quantity: number | null | undefined,
  metric?: string | null,
): number {
  const n = typeof quantity === 'number' && Number.isFinite(quantity) ? Math.round(quantity) : 0
  if (n <= 0) return 1
  const a = item.attrs
  const pack = a?.license_quantity
  if (
    a &&
    typeof pack === 'number' &&
    pack > 0 &&
    COUNTED_METRICS.has(a.license_metric) &&
    (!metric || metric === a.license_metric)
  )
    return Math.ceil(n / pack)
  return isPerUnitLine(item.license_line) ? n : 1
}

export interface SelectedOffer {
  /** «Продление · 2 года · Стандартный». */
  label: string
  quantity: number
  unitPrice: number
  total: number
  /** «× 100 устройств = 429 000 ₽» или цена позиции. */
  sum: string
}

function durationText(months: number | null | undefined): string {
  if (!months) return ''
  if (months % 12 === 0) {
    const y = months / 12
    return `${y} ${plural(y, ['год', 'года', 'лет'])}`
  }
  return `${months} ${plural(months, ['месяц', 'месяца', 'месяцев'])}`
}

const UNIT_FORMS: Record<string, [string, string, string]> = {
  device: ['устройство', 'устройства', 'устройств'],
  user: ['пользователь', 'пользователя', 'пользователей'],
  server: ['сервер', 'сервера', 'серверов'],
  core: ['ядро', 'ядра', 'ядер'],
}

/**
 * Вариант, с которым открывается карточка-семейство: голова семейства подошла под условия запроса (сервер выбрал
 * её по типу покупки, количеству и сроку). Поштучная позиция умножается на количество из запроса. null — запрос
 * условий не называл, голова не подходит или цены нет. Пакет на N мест берётся ceil(число / N) раз.
 */
export function selectedOffer(
  item: SearchItemBase,
  purchase: Purchase | null | undefined,
): SelectedOffer | null {
  if (!hasPurchaseTerms(purchase) || !item.matches_query) return null
  if (item.price_on_request || !isPositivePrice(item.price_rub)) return null
  const a = item.attrs
  const kind =
    LICENSE_KIND_LABELS[item.license_kind ?? ''] ||
    kindFromLine(item.license_line) ||
    (a?.license_type === 'renewal' ? 'Продление' : purchase.deal_label || 'Новая лицензия')
  const parts = [kind, durationText(a?.duration_months ?? null), a?.edition ?? ''].filter(Boolean)
  const quantity = purchaseQuantity(item, purchase.quantity, purchase.metric)
  const total = item.price_rub * quantity
  const forms = UNIT_FORMS[a?.license_metric ?? purchase.metric ?? ''] ?? ['шт.', 'шт.', 'шт.']
  const pack = a && COUNTED_METRICS.has(a.license_metric) ? (a.license_quantity ?? 1) : 1
  let sum = formatPrice(total)
  if (quantity > 1 && pack > 1)
    sum = `${formatPrice(item.price_rub)} × ${quantity} ${plural(quantity, ['пакет', 'пакета', 'пакетов'])} по ${pack} ${plural(pack, forms)} = ${formatPrice(total)}`
  else if (quantity > 1)
    sum = `${formatPrice(item.price_rub)} × ${quantity} ${plural(quantity, forms)} = ${formatPrice(total)}`
  return { label: parts.join(' · '), quantity, unitPrice: item.price_rub, total, sum }
}

/** Параметры закупки для строки спецификации; null — запрос ничего не назвал. */
export function specPurchase(p: Purchase | null | undefined): SpecPurchase | null {
  if (!p) return null
  const out: SpecPurchase = { query: p.query }
  if (p.deal) out.deal = p.deal
  if (p.quantity) out.quantity = p.quantity
  if (p.metric) out.metric = p.metric
  if (p.duration_months) out.duration_months = p.duration_months
  if (p.org_type) out.org_type = p.org_type
  if (p.edition) out.edition = p.edition
  return out
}

const DEAL_LABELS: Record<string, string> = {
  new: 'новая лицензия',
  renewal: 'продление',
  extension: 'расширение',
  migration: 'миграция',
}

/** Подпись строки спецификации: «продление · 100 · 24 мес. · по запросу «…»». */
export function purchaseNote(p: SpecPurchase | undefined): string {
  if (!p) return ''
  const parts: string[] = []
  if (p.deal) parts.push(DEAL_LABELS[p.deal] ?? p.deal)
  if (p.edition) parts.push(`редакция ${p.edition}`)
  if (p.duration_months) parts.push(durationText(p.duration_months))
  if (p.query) parts.push(`по запросу «${p.query}»`)
  return parts.join(' · ')
}

/** Разбор сохранённых параметров закупки строки: чужое и битое отбрасывается. */
export function parsePurchase(x: unknown): SpecPurchase | undefined {
  if (!x || typeof x !== 'object') return undefined
  const r = x as Record<string, unknown>
  if (typeof r.query !== 'string') return undefined
  const out: SpecPurchase = { query: r.query.slice(0, 300) }
  for (const k of ['deal', 'metric', 'org_type', 'edition'] as const)
    if (typeof r[k] === 'string' && r[k]) out[k] = r[k] as string
  for (const k of ['quantity', 'duration_months'] as const)
    if (typeof r[k] === 'number' && Number.isFinite(r[k]) && (r[k] as number) > 0)
      out[k] = r[k] as number
  return out
}

/** Тон бейджа реестра: подтверждено — сильный, остальное — нейтральный. */
export function reestrTone(r: Reestr | null | undefined): 'ok' | 'muted' | null {
  if (!r) return null
  return r.status === 'confirmed' ? 'ok' : 'muted'
}

/** Ответ справки вместо товаров. */
export function helpAnswerShown(resp: Pick<SearchResponse, 'help' | 'items'> | null): boolean {
  return Boolean(resp?.help && resp.help.mode === 'answer' && !resp.items.length)
}

/** Строка справки рядом с товаром («Как продлить в кабинете»). */
export function helpHintShown(resp: Pick<SearchResponse, 'help' | 'items'> | null): boolean {
  return Boolean(resp?.help && resp.help.mode === 'hint' && resp.items.length)
}

/** Кнопка «Запросить поставку»: товара нет в каталоге. */
export function supplyShown(resp: Pick<SearchResponse, 'empty' | 'help'> | null): boolean {
  return Boolean(resp?.empty?.supply && !resp.help)
}
