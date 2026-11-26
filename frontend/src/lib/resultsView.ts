// Страница результатов контура выдачи: заголовок, сортировки, применённые фильтры,
// уточнения, строка атрибутов карточки, устойчивый порядок фасетов. Без DOM: tests/resultsView.test.ts.
import type { Chip, SearchItemBase, SearchResponse } from '@/api/types'
import type { Facet, SearchFilter } from '@/api/typesP2'
import type { ItemAttrs, Refinement, ResultsSort } from '@/api/typesResults'
import { formatInt, formatPrice, plural } from './format.ts'

export const SORT_LABELS: Record<ResultsSort, string> = {
  recommended: 'Рекомендуемые',
  relevance: 'По соответствию запросу',
  price_asc: 'Сначала дешевле',
  price_desc: 'Сначала дороже',
  delivery: 'По способу поставки',
}

/** Пояснение у «Рекомендуемых», если сервер не прислал своё. */
export const RECOMMENDED_NOTE =
  'Порядок учитывает соответствие запросу, доступность и коммерческие факторы; точные совпадения стоят первыми.'

/** «По запросу "антивирус" — 7 продуктов · 32 варианта»; без счётчиков сервера — null (контур коммерции). */
export function resultsHeading(resp: Pick<SearchResponse, 'query' | 'counts'>): string | null {
  const c = resp.counts
  if (!c) return null
  const products = `${formatInt(c.families)} ${plural(c.families, ['продукт', 'продукта', 'продуктов'])}`
  const variants = `${formatInt(c.variants)} ${plural(c.variants, ['вариант', 'варианта', 'вариантов'])}`
  return `По запросу «${resp.query}» — ${products} · ${variants}`
}

/** Подписи ключей фильтров выдачи (как у фасетов сервера). */
const FILTER_KEY_LABELS: Record<string, string> = {
  vendor: 'Вендор',
  license_type: 'Тип',
  license_metric: 'Метрика',
  license_quantity: 'Количество',
  duration_months: 'Срок',
  in_reestr: 'Реестр',
  price_rub: 'Цена',
  has_price: 'Цена',
  delivery: 'Поставка',
}

const OP_SIGNS: Record<string, string> = { gte: 'от', lte: 'до', not: 'не' }

/**
 * Применённый фильтр чипом: «Срок: 1 год», «Цена: от 5 000 ₽». Подпись значения берётся из фасета ответа
 * (сервер знает «renewal» → «Продление»); фасета нет — значение как есть.
 */
export function filterChipLabel(f: SearchFilter, facets: readonly Facet[]): string {
  const facet = facets.find((x) => x.key === f.key)
  const name = facet?.label ?? FILTER_KEY_LABELS[f.key] ?? f.key
  const valueLabel = (v: SearchFilter['value']) => {
    const hit = facet?.values?.find((x) => String(x.value) === String(v))
    if (hit?.label) return hit.label
    if (f.key === 'price_rub' && typeof v === 'number') return formatPrice(v)
    if (v === true) return 'да'
    if (v === false) return 'нет'
    return facet?.unit ? `${String(v)} ${facet.unit}` : String(v)
  }
  if (f.op === 'range')
    return `${name}: ${valueLabel(f.value)} — ${valueLabel(f.value2 ?? f.value)}`
  const sign = OP_SIGNS[f.op]
  return `${name}: ${sign ? `${sign} ` : ''}${valueLabel(f.value)}`
}

/** Уточнение → фильтр выдачи; уточнение без поля (дописывает текст) — null. */
export function refinementFilter(r: Refinement): SearchFilter | null {
  if (!r.field || r.value === null) return null
  const n = Number(r.value)
  const value =
    r.value === 'true'
      ? true
      : r.value === 'false'
        ? false
        : r.value.trim() !== '' && Number.isFinite(n)
          ? n
          : r.value
  const op = r.op === 'not' || r.op === 'gte' || r.op === 'lte' ? r.op : 'eq'
  return { key: r.field, op, value }
}

/** Ячейка строки атрибутов: подпись, значение («—», если нет) и выделение, если атрибут назван в запросе. */
export interface AttrCell {
  key: string
  label: string
  value: string
  highlight: boolean
}

const TYPE_LABELS: Record<string, string> = {
  subscription: 'Новая · подписка',
  perpetual: 'Новая · бессрочная',
  renewal: 'Продление',
  upgrade: 'Обновление',
  oem: 'OEM',
  cloud: 'Облачная',
}

const METRIC_FORMS: Record<string, [string, string, string]> = {
  device: ['устройство', 'устройства', 'устройств'],
  user: ['пользователь', 'пользователя', 'пользователей'],
  server: ['сервер', 'сервера', 'серверов'],
  core: ['ядро', 'ядра', 'ядер'],
}

const DELIVERY_LABELS: Record<string, string> = {
  electronic: 'Электронная',
  oem: 'OEM',
  physical: 'Физическая',
}

/** Поля чипов «как поняли запрос» → атрибут карточки, который выделяется. */
const CHIP_ATTR: Record<string, string> = {
  license_quantity: 'quantity',
  license_metric: 'quantity',
  license_type: 'type',
  renewal: 'type',
  license_duration_months: 'duration',
  origin: 'reestr',
}

export function namedAttrs(chips: readonly Pick<Chip, 'field'>[]): Set<string> {
  const out = new Set<string>()
  for (const c of chips) {
    const a = CHIP_ATTR[c.field]
    if (a) out.add(a)
  }
  return out
}

export function durationLabel(months: number | null): string {
  if (months === null || months === 0) return 'Бессрочно'
  if (months % 12 === 0) {
    const y = months / 12
    return `${y} ${plural(y, ['год', 'года', 'лет'])}`
  }
  return `${months} ${plural(months, ['месяц', 'месяца', 'месяцев'])}`
}

/**
 * Строка атрибутов карточки ПО: редакция, тип (новая / продление), метрика и количество, срок, поставка,
 * реестр. Набор одинаков у всех карточек — отсутствующее значение прочерком, а не пропуском.
 */
export function attrCells(attrs: ItemAttrs, named: ReadonlySet<string>): AttrCell[] {
  const dash = '—'
  let quantity = dash
  if (attrs.license_metric === 'org') quantity = 'На организацию'
  else if (attrs.license_quantity !== null && METRIC_FORMS[attrs.license_metric])
    quantity = `${formatInt(attrs.license_quantity)} ${plural(attrs.license_quantity, METRIC_FORMS[attrs.license_metric]!)}`
  else if (attrs.license_quantity !== null) quantity = formatInt(attrs.license_quantity)
  const cells: AttrCell[] = [
    { key: 'edition', label: 'Редакция', value: attrs.edition || dash, highlight: false },
    {
      key: 'type',
      label: 'Лицензия',
      value: TYPE_LABELS[attrs.license_type] ?? (attrs.license_type || dash),
      highlight: false,
    },
    { key: 'quantity', label: 'Количество', value: quantity, highlight: false },
    {
      key: 'duration',
      label: 'Срок',
      value: attrs.license_type ? durationLabel(attrs.duration_months) : dash,
      highlight: false,
    },
    {
      key: 'delivery',
      label: 'Поставка',
      value: DELIVERY_LABELS[attrs.delivery] ?? dash,
      highlight: false,
    },
    {
      key: 'reestr',
      label: 'Реестр ПО РФ',
      value: attrs.in_reestr ? 'В реестре' : dash,
      highlight: false,
    },
  ]
  for (const c of cells) c.highlight = named.has(c.key) && c.value !== dash
  return cells
}

/** Действие карточки по состоянию. */
export type CardAction = 'choose' | 'spec' | 'price_request'

export function cardAction(item: SearchItemBase, variantsCount: number): CardAction {
  if (variantsCount > 0) return 'choose'
  if (item.price_on_request || item.price_rub === null) return 'price_request'
  return 'spec'
}

/** Цена карточки семейства: «от N ₽», если у вариантов разные цены; у позиции — точная или «Цена по запросу». */
export function familyPriceLabel(item: SearchItemBase, variantsCount: number): string {
  const from = item.price_from_rub
  if (variantsCount > 0 && from !== undefined && from !== null) return `от ${formatPrice(from)}`
  if (item.price_on_request || item.price_rub === null) return 'Цена по запросу'
  return formatPrice(item.price_rub)
}

/**
 * Устойчивый порядок фасетов: фасет, с которым пользователь работает, не меняет места. Ключи прошлого
 * порядка остаются на своих местах (пропавшие выпадают), новые — в порядке сервера в конце.
 */
export function stableFacetOrder(prev: readonly string[], next: readonly Facet[]): string[] {
  const nextKeys = next.map((f) => f.key)
  const kept = prev.filter((k) => nextKeys.includes(k))
  return [...kept, ...nextKeys.filter((k) => !kept.includes(k))]
}

/** Поиск внутри фасета — с 15 значений. */
export const FACET_SEARCH_FROM = 15

export function filterFacetValues<T extends { value: unknown; label?: string }>(
  values: readonly T[],
  needle: string,
): T[] {
  const q = needle.trim().toLowerCase()
  if (!q) return [...values]
  return values.filter((v) => (v.label ?? String(v.value)).toLowerCase().includes(q))
}
