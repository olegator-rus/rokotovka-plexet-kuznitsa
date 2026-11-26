// Роли и права во фронте. Сервер проверяет права на каждое решение; здесь — только что прятать,
// что выключать и куда вести после входа. Чистые функции без DOM и vue-router: их проверяют node-тесты
// (tests/auth.test.ts), а роутер и компоненты только зовут.

/** Роли бэк-офиса: набор блоков «Аналитики» и зона решений. */
export type StaffRole = 'ecom' | 'product' | 'content' | 'head'
/** Кто смотрит: гость, покупатель, сотрудник; system — скрипт по токену или бэк-офис без входа (development). */
export type ViewerRole = 'guest' | 'buyer' | StaffRole | 'system'

export const LOGIN_PATH = '/login'

export const ROLE_LABELS: Record<ViewerRole, string> = {
  guest: 'Гость',
  buyer: 'Покупатель',
  ecom: 'Менеджер e-com',
  product: 'Продукт-менеджер',
  content: 'Контент-менеджер',
  head: 'Руководитель e-com',
  system: 'Служебный доступ',
}

export const roleLabel = (role: string | null | undefined): string =>
  role && role in ROLE_LABELS ? ROLE_LABELS[role as ViewerRole] : (role ?? '')

/** Сотрудник магазина: видит бэк-офис. */
export function isStaff(role: string | null | undefined): boolean {
  return (
    role === 'ecom' ||
    role === 'product' ||
    role === 'content' ||
    role === 'head' ||
    role === 'system'
  )
}

/** Зоны решений вне очереди предложений — чья это зона (как на сервере). */
export const AREA = {
  /** Откат правил поиска. */
  rules: 'ecom',
  /** Откат справочника связей. */
  relations: 'product',
  /** Применение и откат политики маржи. */
  margin: 'ecom',
} as const satisfies Record<string, StaffRole>

/** Может ли actor решать в зоне роли owner: руководитель и служебный доступ — всё, остальные — только своё. */
export function canDecide(
  actor: string | null | undefined,
  owner: string | null | undefined,
): boolean {
  if (actor === 'head' || actor === 'system') return true
  if (actor === 'ecom' || actor === 'product' || actor === 'content') return actor === owner
  return false
}

/** Подсказка у неактивной кнопки — та же фраза, что 403 сервера. */
export function denyMessage(owner: string | null | undefined): string {
  if (owner === 'ecom' || owner === 'product' || owner === 'content') {
    const l = ROLE_LABELS[owner]
    return `Это решение принимает ${l.charAt(0).toLowerCase()}${l.slice(1)}`
  }
  return 'Это решение принимает руководитель e-com'
}

/** Может ли роль смотреть «Аналитику» глазами другой роли («Смотреть как»). */
export const canViewAs = (role: string | null | undefined) => role === 'head' || role === 'system'

/** Стартовый экран роли: покупатель и гость — витрина, продукт-менеджер — очередь агента своей роли, остальные —
 * «Аналитика» своей роли. */
export function homeFor(role: string | null | undefined): string {
  switch (role) {
    case 'product':
      return '/searchops?tab=queue&role=product'
    case 'ecom':
    case 'content':
    case 'head':
    case 'system':
      return '/analytics'
    default:
      return '/search'
  }
}

/** Разделы бэк-офиса: путь → нужна роль сотрудника. */
const BACKOFFICE_PREFIXES = ['/analytics', '/searchops', '/changes', '/costs', '/margin']

export function isBackofficePath(path: string): boolean {
  const p = path.split(/[?#]/)[0] ?? ''
  return BACKOFFICE_PREFIXES.some((b) => p === b || p.startsWith(`${b}/`))
}

/**
 * Безопасный returnTo: только путь своего сайта. Схема, «//host», «/\host», управляющие символы, сам экран входа и
 * слишком длинная строка — null (открытый редирект через ссылку на вход).
 */
export function safeReturnTo(raw: unknown): string | null {
  if (typeof raw !== 'string' || raw.length === 0 || raw.length > 512) return null
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return null
  if (/[\u0000-\u001f\u007f\\]/.test(raw)) return null
  const path = raw.split(/[?#]/)[0] ?? ''
  if (path === LOGIN_PATH || path.startsWith(`${LOGIN_PATH}/`)) return null
  return raw
}

/** Куда вести после входа: returnTo, если он есть и роли туда можно, иначе стартовый экран роли. */
export function afterLogin(role: string | null | undefined, returnTo: unknown): string {
  const target = safeReturnTo(returnTo)
  if (target && (isStaff(role) || !isBackofficePath(target))) return target
  return homeFor(role)
}

export interface GuardInput {
  /** Полный путь цели с query (для returnTo). */
  fullPath: string
  /** Цель — экран входа. */
  isLogin: boolean
  /** Раздел бэк-офиса (meta.backoffice). */
  backoffice: boolean
  /** Кто смотрит. */
  role: ViewerRole
  /** returnTo в адресе экрана входа. */
  returnTo?: unknown
}

export type GuardResult = { kind: 'allow' } | { kind: 'redirect'; to: string }

/** Решение охраны маршрута: бэк-офис без роли сотрудника → вход с returnTo; вошедший на /login → его экран. */
export function decideGuard(i: GuardInput): GuardResult {
  if (i.isLogin) {
    // Вошедший открыл /login: на свой экран. Исключение — покупатель, которого вернули из бэк-офиса: он может
    // войти сотрудником.
    if (i.role === 'guest' || i.role === 'system') return { kind: 'allow' }
    const target = safeReturnTo(i.returnTo)
    if (i.role === 'buyer' && target && isBackofficePath(target)) return { kind: 'allow' }
    return { kind: 'redirect', to: afterLogin(i.role, i.returnTo) }
  }
  if (i.backoffice && !isStaff(i.role)) {
    return { kind: 'redirect', to: `${LOGIN_PATH}?returnTo=${encodeURIComponent(i.fullPath)}` }
  }
  return { kind: 'allow' }
}

/** Retry-After (секунды или HTTP-дата) → секунды ожидания; нет или мусор — null. */
export function retryAfterSeconds(raw: string | null | undefined, now = Date.now()): number | null {
  if (!raw) return null
  const s = raw.trim()
  if (/^\d+$/.test(s)) return Math.max(1, Number(s))
  const t = Date.parse(s)
  if (Number.isNaN(t)) return null
  return Math.max(1, Math.ceil((t - now) / 1000))
}

/** Контакт гостя для ответа на КП: почта или телефон; null — годится, иначе — одна фраза, что не так. */
export function contactProblem(raw: string): string | null {
  const v = raw.trim()
  if (!v) return 'Оставьте почту или телефон — менеджер пришлёт КП'
  if (v.length > 120) return 'Слишком длинный контакт'
  const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)
  const phone = (v.match(/\d/g) ?? []).length >= 7 && /^[+\d\s()-]+$/.test(v)
  return email || phone ? null : 'Нужна почта (name@company.ru) или телефон'
}
