// Страница товара: ссылка из выдачи, разбор rid/pos из адреса, выбор «назад». Без DOM и роутера —
// tests/product.test.ts.
import { plural } from './format.ts'
import { kindFromLine } from './license.ts'

/** Номер поиска в адресе: буквы, цифры, «-» и «_», до 64 знаков (ULID сервера и подобные). */
const RID_RE = /^[A-Za-z0-9_-]{1,64}$/
/** Позиция в выдаче — как в POST /api/events: целое 1..10000. */
const POS_MAX = 10000

/** Откуда пришёл покупатель: номер поиска и позиция карточки; null — не известно (прямая ссылка). */
export interface ProductRef {
  requestId: string | null
  position: number | null
}

/** Ссылка на страницу товара из выдачи: /product/{id}?rid={request_id}&pos={position}. */
export function productHref(
  productId: string,
  requestId?: string | null,
  position?: number | null,
): string {
  const path = `/product/${encodeURIComponent(productId)}`
  const q = new URLSearchParams()
  if (requestId && RID_RE.test(requestId)) q.set('rid', requestId)
  if (validPosition(position)) q.set('pos', String(position))
  const qs = q.toString()
  return qs ? `${path}?${qs}` : path
}

function validPosition(v: unknown): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= 1 && v <= POS_MAX
}

function first(v: unknown): unknown {
  return Array.isArray(v) ? v[0] : v
}

/** rid и pos из адреса страницы; мусор → null. */
export function parseProductRef(query: Record<string, unknown>): ProductRef {
  const rid = first(query.rid)
  const rawPos = first(query.pos)
  const pos = typeof rawPos === 'string' && /^\d{1,5}$/.test(rawPos) ? Number(rawPos) : null
  return {
    requestId: typeof rid === 'string' && RID_RE.test(rid) ? rid : null,
    position: validPosition(pos) ? pos : null,
  }
}

/** Путь страницы поиска (/search, /search?…, /search#…), а не /searchops. */
export function isSearchPath(path: unknown): boolean {
  return typeof path === 'string' && /^\/search(?:[?#]|$)/.test(path)
}

/**
 * «← К результатам»: пришли со страницы поиска (history.state.back) — шаг назад, выдача, запрос, фильтры и прокрутка
 * на месте (состояние поиска живёт в адресе); иначе (прямая ссылка) — ссылка на /search.
 */
export function backTarget(historyBack: unknown): 'back' | '/search' {
  return isSearchPath(historyBack) ? 'back' : '/search'
}

const LICENSE_TYPES: Record<string, string> = {
  subscription: 'Подписка',
  perpetual: 'Бессрочная',
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

const METRIC_ONE: Record<string, string> = {
  device: 'На устройство',
  user: 'На пользователя',
  server: 'На сервер',
  core: 'На ядро',
  org: 'На организацию',
}

/** Условия лицензии (ProductPage.license) — строки «подпись — значение»; неизвестное пропускается. */
export interface LicenseTerms {
  type: string | null
  metric: string | null
  quantity: number | null
  duration_months: number | null
  edition: string
}

/** line — `license_line` позиции: вид из её начала («Переход с другого продукта») главнее типа атрибутов. */
export function licenseRows(
  l: LicenseTerms,
  line?: string | null,
): { label: string; value: string }[] {
  const rows: { label: string; value: string }[] = []
  const kind = kindFromLine(line)
  if (kind) rows.push({ label: 'Тип', value: kind })
  else if (l.type) rows.push({ label: 'Тип', value: LICENSE_TYPES[l.type] ?? l.type })
  const forms = l.metric ? METRIC_FORMS[l.metric] : undefined
  if (l.metric === 'org') rows.push({ label: 'Метрика', value: METRIC_ONE.org })
  else if (forms && l.quantity !== null && l.quantity > 0)
    rows.push({ label: 'Количество', value: `${l.quantity} ${plural(l.quantity, forms)}` })
  else if (l.metric) rows.push({ label: 'Метрика', value: METRIC_ONE[l.metric] ?? l.metric })
  rows.push({ label: 'Срок', value: durationText(l.duration_months) })
  if (l.edition) rows.push({ label: 'Редакция', value: l.edition })
  return rows
}

/** Срок лицензии: null или 0 — «бессрочная», кратно 12 — годы, иначе месяцы. */
export function durationText(months: number | null): string {
  if (months === null || months <= 0) return 'бессрочная'
  if (months % 12 === 0) {
    const y = months / 12
    return `${y} ${plural(y, ['год', 'года', 'лет'])}`
  }
  return `${months} ${plural(months, ['месяц', 'месяца', 'месяцев'])}`
}

const COMPAT_STATES: Record<string, string> = {
  verified: 'проверена',
  conditional: 'с условием',
  incompatible: 'несовместима',
  unknown: 'неизвестно',
}

/** Состояние правила совместимости словами; незнакомое — «неизвестно». */
export function compatStateText(state: string): string {
  return COMPAT_STATES[state] ?? COMPAT_STATES.unknown
}

/** Происхождение позиции: «Отечественное» / «Зарубежное»; иное — пусто (не показывается). */
export function originText(origin: string): string {
  if (origin === 'ru') return 'Отечественное'
  if (origin === 'foreign') return 'Зарубежное'
  return ''
}

/** Значение характеристики с единицей: 64 ГБ, 2 U, 27″ (дюймы без пробела). */
export function specText(value: number, unit: string): string {
  const v = String(value).replace('.', ',')
  if (!unit) return v
  return unit === '″' ? `${v}${unit}` : `${v} ${unit}`
}
