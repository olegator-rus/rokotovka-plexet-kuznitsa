// Рабочая область подбора (контур экрана, UI.md): экран берётся из `screen` ответа — фронт не выбирает,
// не считает и не переводит. Здесь только выборка полей и подписи условий.
// Без DOM: tests/guideView.test.ts.
import type {
  GuideCondition,
  GuideOffer,
  GuideResponse,
  GuideScreen,
  GuideSummary,
} from '@/api/typesGuide'
import { formatPrice, isPositivePrice, plural } from './format.ts'
import { offerName, offerPrice } from './guide.ts'

// ── Условия (UI 6.5) ──

export type CountKind = 'workstation' | 'server' | 'user' | 'host' | 'connection' | string

interface KindText {
  field: string
  question: string
  forms: [string, string, string]
  short: string
}

const KIND_TEXT: Record<string, KindText> = {
  workstation: {
    field: 'Компьютеры',
    question: 'Сколько компьютеров нужно защитить?',
    forms: ['компьютер', 'компьютера', 'компьютеров'],
    short: 'ПК',
  },
  server: {
    field: 'Серверы',
    question: 'Сколько серверов?',
    forms: ['сервер', 'сервера', 'серверов'],
    short: 'серв.',
  },
  user: {
    field: 'Пользователи',
    question: 'Сколько пользователей?',
    forms: ['пользователь', 'пользователя', 'пользователей'],
    short: 'польз.',
  },
  host: {
    field: 'Хосты',
    question: 'Сколько хостов?',
    forms: ['хост', 'хоста', 'хостов'],
    short: 'хост.',
  },
  connection: {
    field: 'Подключения',
    question: 'Сколько одновременных подключений?',
    forms: ['подключение', 'подключения', 'подключений'],
    short: 'подкл.',
  },
}

const FALLBACK_KIND: KindText = {
  field: 'Количество',
  question: 'На сколько рабочих мест нужна лицензия?',
  forms: ['место', 'места', 'мест'],
  short: 'шт.',
}

export function kindText(kind: CountKind): KindText {
  return KIND_TEXT[kind] ?? FALLBACK_KIND
}

/** Вид объекта из ключа «count:workstation»; '' — ключ не про количество. */
export function countKind(key: string | null | undefined): CountKind {
  const m = /^count:(\w+)$/.exec(key ?? '')
  return m ? m[1]! : ''
}

export interface CountFieldView {
  key: string
  kind: CountKind
  label: string
  value: number | null
  origin: string
}

/** Поля количества: по строке на объект (компьютеры, серверы, пользователи — отдельными строками). */
export function countFields(conds: readonly GuideCondition[] | null | undefined): CountFieldView[] {
  return (conds ?? [])
    .filter((c) => countKind(c.key))
    .map((c) => {
      const n = Number(String(c.value ?? '').replace(/[^\d]/g, ''))
      return {
        key: c.key,
        kind: countKind(c.key),
        label: kindText(countKind(c.key)).field,
        value: Number.isFinite(n) && n > 0 ? n : null,
        origin: c.origin,
      }
    })
}

/** Быстрые значения количества (UI 6.5): 10 · 20 · 30 · 50 · 100. */
export const QUICK_COUNTS = [10, 20, 30, 50, 100] as const

/** Количество: целое от 1 до 100 000; иначе null. */
export function clampCount(v: unknown): number | null {
  const n = Math.round(Number(v))
  if (!Number.isFinite(n) || n < 1) return null
  return Math.min(n, 100_000)
}

/** Варианты срока: 1–3 года; бессрочная — только если её прислал сервер. */
export const TERM_OPTIONS = [
  { value: '12', label: '1 год' },
  { value: '24', label: '2 года' },
  { value: '36', label: '3 года' },
] as const

export function termOptions(current: string | null | undefined) {
  const v = String(current ?? '')
  const known = TERM_OPTIONS.some((o) => o.value === v)
  return known || !v ? [...TERM_OPTIONS] : [...TERM_OPTIONS, { value: v, label: termLabel(v) }]
}

export function termLabel(months: string | number): string {
  const m = Number(months)
  if (!Number.isFinite(m) || m <= 0) return 'бессрочная'
  if (m % 12 === 0) {
    const y = m / 12
    return `${y} ${plural(y, ['год', 'года', 'лет'])}`
  }
  return `${m} ${plural(m, ['месяц', 'месяца', 'месяцев'])}`
}

/** Функции из условий: ключ «feature:<ось>». */
export function featureConditions(conds: readonly GuideCondition[] | null | undefined) {
  return (conds ?? []).filter((c) => c.key.startsWith('feature:'))
}

/** Подпись функции без «нужно:» и «нужно» в начале, с заглавной. */
export function featureLabel(label: string): string {
  const s = label.replace(/^нужн[оа]:?\s*/i, '').trim()
  return s ? s[0]!.toUpperCase() + s.slice(1) : s
}

/** Условия, которые показываются полями сами (количество, тип покупки, срок, функции) или в задаче. */
const OWN_FIELD = /^(count:|feature:|operation$|term_months$|direction$|vendor$|object$)/

/**
 * Прочие условия строкой «Платформа: Astra Linux». Тип организации — только если его задал покупатель (UI 6.5:
 * появляется, когда влияет на допустимость).
 */
export function otherConditions(conds: readonly GuideCondition[] | null | undefined) {
  return (conds ?? []).filter(
    (c) => !OWN_FIELD.test(c.key) && !(c.key === 'org_type' && c.origin === 'default'),
  )
}

/** Условия одной строкой для телефона: «50 ПК · Новая · 1 год». */
export function conditionsLine(conds: readonly GuideCondition[] | null | undefined): string {
  const list = conds ?? []
  const parts: string[] = []
  for (const f of countFields(list)) if (f.value) parts.push(`${f.value} ${kindText(f.kind).short}`)
  const op = list.find((c) => c.key === 'operation')
  if (op) parts.push(op.value === 'renewal' ? 'Продление' : 'Новая')
  const term = list.find((c) => c.key === 'term_months')
  if (term) parts.push(term.label || termLabel(term.value))
  for (const f of featureConditions(list)) parts.push(featureLabel(f.label))
  return parts.join(' · ')
}

// ── Ячейки сравнения (UI 6.3) ──

export const MARK_SIGN: Record<string, string> = {
  yes: '✓',
  no: '—',
  unknown: '?',
  conditional: '!',
}

export const LEGEND = [
  { mark: 'yes', sign: '✓', text: 'Есть', hint: 'подтверждено документом' },
  { mark: 'no', sign: '—', text: 'Нет', hint: 'отсутствие подтверждено' },
  { mark: 'unknown', sign: '?', text: 'Нет сведений', hint: 'в документации не найдено' },
  {
    mark: 'conditional',
    sign: '!',
    text: 'С условиями',
    hint: 'зависит от платформы или лицензии',
  },
] as const

/** Колонки на телефоне: не больше двух — первая и выбранная. */
export function phoneColumns(count: number, picked: number): number[] {
  if (count <= 2) return Array.from({ length: count }, (_, i) => i)
  return [0, picked > 0 && picked < count ? picked : 1]
}

// ── Карточка ──

/** Заголовок лицензии (UI 6.1): готовый от сервера, иначе семейство из полей ответа. */
export function offerTitle(o: GuideOffer | null | undefined): string {
  if (!o) return ''
  return o.display?.title || offerName(o).name || o.title
}

/** Подзаголовок: готовый от сервера, иначе срок и единица из ответа. */
export function offerSubtitle(o: GuideOffer | null | undefined): string {
  if (!o) return ''
  if (o.display) return o.display.subtitle
  return [offerName(o).detail, o.sale_unit || '', o.term ?? ''].filter(Boolean).join(' · ')
}

/** Цена с единицей для строки «При изменении условий»: готовая от сервера, иначе цена каталога. */
export function offerPriceText(o: GuideOffer | null | undefined): string {
  if (!o) return ''
  if (o.price_text) return o.price_text
  const p = offerPrice(o)
  const u = (o.unit ?? '').trim()
  return p.known && u ? `${p.unit} ${/^за\s/i.test(u) ? u : `за ${u}`}` : p.unit
}

export function offerById(
  r: Pick<GuideResponse, 'offers'> | null | undefined,
  id: string | null | undefined,
): GuideOffer | null {
  if (!id) return null
  return (r?.offers ?? []).find((o) => o.offer_id === id) ?? null
}

/**
 * Итоговая строка для выбранной позиции. Предвыбранная — готовая `summary`; другая (переключатель «Выбрано», строка
 * таблицы вендоров) — из полей той же карточки: итог и расчёт присылает сервер, фронт их не пересчитывает.
 */
export function summaryFor(
  screen: Pick<GuideScreen, 'summary'> | null | undefined,
  o: GuideOffer | null | undefined,
): GuideSummary | null {
  const base = screen?.summary ?? null
  if (!o) return base && base.action === 'choose' ? base : null
  if (base && base.offer_id === o.offer_id) return base
  const priced = offerPrice(o).known && isPositivePrice(o.total)
  return {
    offer_id: o.offer_id,
    label: base?.label || 'Стоимость закупки',
    total: priced ? o.total : null,
    total_text: priced ? formatPrice(o.total) : 'Цена по запросу',
    formula: priced ? (o.price_line ?? '') : '',
    action: priced ? 'add_to_spec' : 'request_quote',
    hint: '',
  }
}

/** Экран ответа — только от сервера; без `screen` фронт экран не собирает. */
export function screenOf(r: GuideResponse | null | undefined): GuideScreen | null {
  return r?.screen ?? null
}

/** Понятая задача: от сервера; иначе направление и вендор из условий. */
export function taskText(r: GuideResponse | null | undefined): string {
  const s = r?.screen?.task
  if (s) return s
  const conds = r?.conditions ?? []
  const parts = ['direction', 'vendor', 'object']
    .map((k) => conds.find((c) => c.key === k)?.label ?? '')
    .filter(Boolean)
  const t = parts.join(' · ')
  return t ? t[0]!.toUpperCase() + t.slice(1) : ''
}
