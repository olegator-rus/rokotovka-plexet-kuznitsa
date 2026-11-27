// Варианты на странице товара: таблица «чем варианты отличаются» — как таблица вариантов карточки
// выдачи (FamilyCard): редакция, вид лицензии, срок, количество, для кого, цена. Колонка показывается, только если
// значения в ней различаются; текущая позиция — строка таблицы с пометкой. Без DOM: tests/productVariants.test.ts.
import type { ProductLicense } from '@/api/typesProduct'
import { formatInt, formatPrice, plural } from './format.ts'
import { LICENSE_KIND_LABELS, isPurchasable, priceLabel } from './license.ts'
import { perPlaceLabel } from './offerNotes.ts'
import { LICENSE_TYPES, METRIC_FORMS, termText } from './product.ts'

/** Позиция семейства: текущая (ProductPage) или вариант (ProductVariant). */
export interface VariantSource {
  product_id: string
  title: string
  license_line: string
  license: ProductLicense
  price_rub: number | null
  old_price_rub?: number | null
  price_on_request: boolean
  price_mode: string
}

export type VariantKey = 'edition' | 'kind' | 'term' | 'quantity' | 'org' | 'title'

export interface VariantRow {
  productId: string
  current: boolean
  cells: Record<VariantKey, string>
  /** «12 705 ₽» / «Цена по запросу». */
  price: string
  /** «за узел» — у ступени цены за место; иначе ''. */
  perPlace: string
  /** Ключ группы «тот же вид и та же организация» — для свёрнутого списка. */
  group: string
  sort: (string | number)[]
}

export interface VariantColumn {
  key: VariantKey
  label: string
}

const COLUMNS: VariantColumn[] = [
  { key: 'edition', label: 'Редакция' },
  { key: 'kind', label: 'Лицензия' },
  { key: 'term', label: 'Срок' },
  { key: 'quantity', label: 'Количество' },
  { key: 'org', label: 'Для кого' },
]

const DASH = '—'

/** Порядок видов: новая лицензия, затем продление, обновление, переход, поддержка, модуль, курс. */
const KIND_ORDER = [
  LICENSE_TYPES.subscription,
  LICENSE_TYPES.perpetual,
  LICENSE_TYPES.cloud,
  LICENSE_TYPES.oem,
  LICENSE_KIND_LABELS.renewal,
  LICENSE_KIND_LABELS.upgrade,
  LICENSE_KIND_LABELS.migration,
  LICENSE_KIND_LABELS.support,
  LICENSE_KIND_LABELS.module,
  LICENSE_KIND_LABELS.course,
] as string[]

const ORG_ORDER: Record<string, number> = {
  commercial: 0,
  '': 0,
  home: 1,
  academic: 2,
  government: 3,
  nonprofit: 4,
}

/** Вид лицензии: вид фида (продление, обновление…) главнее типа; базовая — «Подписка» / «Бессрочная». */
export function variantKind(l: ProductLicense): string {
  const kind = l.kind ?? ''
  if (LICENSE_KIND_LABELS[kind]) return LICENSE_KIND_LABELS[kind]
  if (!l.type) return ''
  return LICENSE_TYPES[l.type] ?? l.type
}

/** Количество: «На организацию», ступень «100–149 устройств» / «от 250 устройств», «2 устройства», число. */
export function variantQuantity(l: ProductLicense): string {
  if (l.metric === 'org') return 'На организацию'
  const forms = l.metric ? METRIC_FORMS[l.metric] : undefined
  const min = l.tier_min
  if (typeof min === 'number' && min > 0) {
    const max = typeof l.tier_max === 'number' && l.tier_max >= min ? l.tier_max : null
    const range =
      max === null
        ? `от ${formatInt(min)}`
        : max === min
          ? formatInt(min)
          : `${formatInt(min)}–${formatInt(max)}`
    return forms ? `${range} ${plural(max ?? min, forms)}` : range
  }
  if (typeof l.quantity === 'number' && l.quantity > 0)
    return forms ? `${formatInt(l.quantity)} ${plural(l.quantity, forms)}` : formatInt(l.quantity)
  return ''
}

/** Для кого: подпись сервера (образование, госучреждение…); коммерческий и «для дома» — словами, иначе ''. */
export function variantOrg(l: ProductLicense): string {
  if (l.org_label) return l.org_label
  if (l.org_type === 'commercial') return 'Коммерческая организация'
  if (l.org_type === 'home') return 'Для дома'
  return ''
}

function hasTierPrice(v: VariantSource): boolean {
  return isPurchasable(v) && typeof v.license.tier_min === 'number' && v.license.tier_min > 0
}

/**
 * Название в колонке «Позиция»: без производителя в начале (он один у всего семейства и уже в шапке) и без
 * хвостовой запятой фида («…Win/Mac),»).
 */
export function shortTitle(title: string, vendor = ''): string {
  let t = title.trim()
  if (vendor && t.toLowerCase().startsWith(`${vendor.toLowerCase()} `))
    t = t.slice(vendor.length).trim()
  return t.replace(/[\s,;]+$/, '') || title
}

const capitalize = (t: string) => (t ? t.charAt(0).toUpperCase() + t.slice(1) : t)

function toRow(v: VariantSource, current: boolean, vendor: string): VariantRow {
  const l = v.license
  const kind = variantKind(l)
  const months = l.duration_months
  const cells: Record<VariantKey, string> = {
    edition: l.edition || DASH,
    kind: kind || DASH,
    term: capitalize(termText(l.type, months)) || DASH,
    quantity: variantQuantity(l) || DASH,
    org: variantOrg(l) || DASH,
    title: shortTitle(v.title, vendor),
  }
  const kindRank = KIND_ORDER.indexOf(kind)
  const price = isPurchasable(v) ? (v.price_rub as number) : Number.POSITIVE_INFINITY
  return {
    productId: v.product_id,
    current,
    cells,
    price: priceLabel(v),
    perPlace: hasTierPrice(v) ? perPlaceLabel(l.metric) : '',
    group: `${kind}|${ORG_ORDER[l.org_type ?? ''] ?? 9}`,
    sort: [
      kindRank < 0 ? 99 : kindRank,
      ORG_ORDER[l.org_type ?? ''] ?? 9,
      l.edition || '',
      months !== null && months > 0 ? months : Number.MAX_SAFE_INTEGER,
      l.tier_min ?? l.quantity ?? 0,
      price,
      v.title,
    ],
  }
}

function compareSort(a: (string | number)[], b: (string | number)[]): number {
  for (let i = 0; i < a.length; i++) {
    const x = a[i]
    const y = b[i]
    if (x === y) continue
    if (typeof x === 'number' && typeof y === 'number') return x - y
    return String(x).localeCompare(String(y), 'ru')
  }
  return 0
}

export interface VariantTable {
  rows: VariantRow[]
  /** Только колонки с различающимися значениями; «Позиция» — если строки иначе не различить. */
  columns: VariantColumn[]
}

/**
 * Таблица вариантов: текущая позиция и остальные, порядок — вид лицензии (новая → продление → …), организация,
 * редакция, срок, количество, цена. Колонка с одинаковым значением во всех строках не выводится; если после этого
 * две строки совпадают, а названия разные (Windows и Mac), добавляется колонка «Позиция» с названием.
 */
export function variantTable(
  current: VariantSource,
  others: readonly VariantSource[],
  vendor = '',
): VariantTable {
  const rows = [toRow(current, true, vendor), ...others.map((v) => toRow(v, false, vendor))].sort(
    (a, b) => compareSort(a.sort, b.sort),
  )
  return { rows, columns: variantColumns(rows) }
}

/**
 * Колонки для показанных строк: только те, где значения различаются; если две строки иначе не различить, а названия
 * разные (Windows и Mac), — «Позиция» с названием первой колонкой. Свёрнутый список (один вид и одна организация)
 * получает свои колонки: одинаковое у всех строк не повторяется в каждой.
 */
export function variantColumns(rows: readonly VariantRow[]): VariantColumn[] {
  const columns = COLUMNS.filter((c) => new Set(rows.map((r) => r.cells[c.key])).size > 1)
  const sig = (r: VariantRow) => columns.map((c) => r.cells[c.key]).join('|')
  const bySig = new Map<string, Set<string>>()
  for (const r of rows) {
    const s = sig(r)
    if (!bySig.has(s)) bySig.set(s, new Set())
    bySig.get(s)!.add(r.cells.title)
  }
  if ([...bySig.values()].some((titles) => titles.size > 1))
    columns.unshift({ key: 'title', label: 'Позиция' })
  return columns
}

/** Строк в свёрнутом списке. */
export const VARIANTS_COLLAPSED = 10

/**
 * Свёрнутый список: всё, если строк немного; иначе — строки того же вида и той же организации, что у текущей
 * (вокруг текущей, не больше VARIANTS_COLLAPSED). Текущая видна всегда.
 */
export function collapsedRows(
  rows: readonly VariantRow[],
  limit = VARIANTS_COLLAPSED,
): VariantRow[] {
  if (rows.length <= limit + 2) return [...rows]
  const cur = rows.find((r) => r.current)
  const same = cur ? rows.filter((r) => r.group === cur.group) : []
  const pool = same.length >= 3 ? same : [...rows]
  if (pool.length <= limit) return pool
  const i = Math.max(
    0,
    pool.findIndex((r) => r.current),
  )
  const start = Math.max(0, Math.min(i - Math.floor(limit / 2), pool.length - limit))
  return pool.slice(start, start + limit)
}

/** Старая цена и бейдж скидки: только если старая выше текущей и цена есть; процент — округлённый, от 1%. */
export function discountView(
  v: Pick<VariantSource, 'price_rub' | 'price_on_request' | 'old_price_rub'> & {
    price_mode?: string
  },
): { old: string; badge: string } {
  const none = { old: '', badge: '' }
  const old = v.old_price_rub
  if (!isPurchasable(v) || typeof old !== 'number' || !(old > (v.price_rub as number))) return none
  const pct = Math.round(((old - (v.price_rub as number)) / old) * 100)
  return { old: formatPrice(old), badge: pct >= 1 ? `Скидка ${pct}%` : 'Скидка' }
}

/** Вопросы помощнику со страницы: подпись кнопки и текст первого хода (сервер получит «<продукт>: <текст>»). */
export interface GuideQuestion {
  label: string
  message: string
}

/**
 * Подсказки по позиции: «Чем отличаются редакции?» — если в семействе 2+ редакции, иначе «Чем отличаются варианты?» —
 * если вариантов больше одного; цена на число мест — с числом и единицей текущей позиции («на 100 устройств»),
 * без числа — общий вопрос без шаблона «N»; если мест нет ни у одной позиции семейства (ПАК «без ограничений
 * по пользователям»), вопроса о числе мест нет.
 */
export function guideQuestions(
  current: VariantSource,
  others: readonly VariantSource[],
): GuideQuestion[] {
  const res: GuideQuestion[] = []
  const editions = new Set(
    [current, ...others].map((v) => v.license.edition).filter((e): e is string => !!e),
  )
  if (editions.size > 1)
    res.push({ label: 'Чем отличаются редакции?', message: 'Чем отличаются редакции?' })
  else if (others.length)
    res.push({
      label: 'Чем отличаются варианты?',
      message: 'Чем отличаются варианты лицензии этого продукта?',
    })
  const l = current.license
  const n = typeof l.tier_min === 'number' && l.tier_min > 0 ? l.tier_min : l.quantity
  const forms = l.metric ? METRIC_FORMS[l.metric] : undefined
  if (typeof n === 'number' && n > 1 && forms) {
    const q = `Сколько стоит на ${formatInt(n)} ${plural(n, forms)}?`
    res.push({ label: q, message: q })
  } else if (
    [current, ...others].some((v) => v.license.metric || v.license.quantity || v.license.tier_min)
  ) {
    // Число мест в семействе есть, но у этой позиции не известно — общий вопрос, без шаблона «N».
    res.push({
      label: 'Посчитать на моё число мест',
      message: 'Сколько будет стоить на нужное мне число мест?',
    })
  }
  return res
}

/** Количество в спецификацию: у ступени цены за место — нижняя граница ступени (меньше по этой цене не купить), иначе 1. */
export function specQuantity(l: ProductLicense): number {
  return typeof l.tier_min === 'number' && l.tier_min > 1 ? l.tier_min : 1
}

/**
 * Названия в колонке «Позиция» без общего начала: у семейства фида названия совпадают на первые 5–15 слов и
 * различаются хвостом («…Корпоративная премиальная» / «…расширенная»). Общее начало от трёх слов заменяется на «…»;
 * иначе — название целиком. Ключ — productId.
 */
export function titleTails(rows: readonly VariantRow[]): Map<string, string> {
  const words = rows.map((r) => r.cells.title.split(/\s+/))
  let common = 0
  if (words.length > 1) {
    const first = words[0]!
    while (
      common < first.length &&
      words.every((w) => w.length > common + 1 && w[common] === first[common])
    )
      common++
  }
  const res = new Map<string, string>()
  rows.forEach((r, i) => {
    const w = words[i]!
    res.set(r.productId, common >= 3 ? `…${w.slice(common).join(' ')}` : r.cells.title)
  })
  return res
}
