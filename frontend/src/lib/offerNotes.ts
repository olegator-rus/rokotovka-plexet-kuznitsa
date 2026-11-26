// Акции и строка сравнения на витрине фида. Без DOM: tests/offerNotes.test.ts.
// Сервер присылает готовые тексты; фронт только приводит их к виду интерфейса: без дат, без оценок
// («лучше», «выгоднее»), ссылка на условия — отдельным словом.
import type { Comparison } from '@/api/types'
import type { Promo } from '@/api/typesP2'
import { noDates } from './companions.ts'
import { formatPrice } from './format.ts'

export interface PromoView {
  id: string
  /** «Акция: −50% при переходе с антивируса другого производителя». */
  text: string
  /** Ссылка «условия» — только http(s); нет — строка без ссылки. */
  url: string
}

const safeHttp = (u: string | null | undefined) =>
  /^https?:\/\//i.test(u ?? '') ? (u as string) : ''

/** «Акция: X, условия по ссылке» → «Акция: X» + ссылка «условия». Пустая акция отбрасывается. */
export function promoView(p: Promo | null | undefined): PromoView | null {
  if (!p) return null
  let body = noDates(p.text || p.title || '')
    .replace(/[,.;]?\s*условия\s+по\s+ссылке\.?$/i, '')
    .replace(/^акция\s*[:—-]\s*/i, '')
    .trim()
  if (!body) return null
  body = body.charAt(0).toUpperCase() + body.slice(1)
  return { id: p.id || body, text: `Акция: ${body}`, url: safeHttp(p.url) }
}

/** Акции позиции к показу: без пустых и повторов, не больше двух строк. */
export function promoViews(list: readonly Promo[] | null | undefined): PromoView[] {
  const out: PromoView[] = []
  for (const p of list ?? []) {
    const v = promoView(p)
    if (v && !out.some((x) => x.id === v.id || x.text === v.text)) out.push(v)
  }
  return out.slice(0, 2)
}

export interface ComparisonView {
  /** «При этих условиях Стандартный на 31% дешевле, чем Расширенный, за узел». */
  text: string
  /** «Продление · 2 года · 100–149 устройств · для коммерческих организаций». */
  conditions: string
  /** Цены пары за место: «Стандартный — 3 070,80 ₽, Расширенный — 4 451,94 ₽». */
  prices: string
}

const JUDGEMENT = /(лучш|выгодн|оптимальн|рекоменду)/i

/**
 * Строка сравнения: текст сервера; нет текста — собирается из пары и процента. Оценочные слова не пропускаются:
 * такой текст заменяется собранным. Нечего сравнивать — null.
 */
export function comparisonView(c: Comparison | null | undefined): ComparisonView | null {
  if (!c) return null
  const [a, b] = c.items ?? []
  const pct =
    typeof c.percent === 'number' && Number.isFinite(c.percent) ? Math.round(c.percent) : 0
  const built =
    a?.name && b?.name && pct > 0
      ? `При этих условиях ${a.name} на ${pct}% дешевле, чем ${b.name}, за место`
      : ''
  let text = noDates(c.text ?? '')
  if (!text || JUDGEMENT.test(text)) text = built
  if (!text) return null
  const prices =
    a && b && a.per_place_rub > 0 && b.per_place_rub > 0
      ? `${a.name} — ${formatPrice(a.per_place_rub)}, ${b.name} — ${formatPrice(b.per_place_rub)}`
      : ''
  return { text, conditions: noDates(c.conditions ?? ''), prices }
}

/** Подпись цены за место для позиции со ступенью количества: «за узел», «за пользователя». */
export function perPlaceLabel(metric: string | null | undefined): string {
  switch (metric) {
    case 'user':
      return 'за пользователя'
    case 'server':
      return 'за сервер'
    case 'core':
      return 'за ядро'
    default:
      return 'за узел'
  }
}

/** Ступень количества у позиции: price_rub — за одно место. */
export function hasTier(item: { tier_min?: number | null }): boolean {
  return typeof item.tier_min === 'number' && item.tier_min > 0
}

/** «100–149», «от 250». */
export function tierLabel(item: { tier_min?: number | null; tier_max?: number | null }): string {
  if (!hasTier(item)) return ''
  const max = item.tier_max
  return typeof max === 'number' && max >= (item.tier_min as number)
    ? max === item.tier_min
      ? `${item.tier_min}`
      : `${item.tier_min}–${max}`
    : `от ${item.tier_min}`
}
