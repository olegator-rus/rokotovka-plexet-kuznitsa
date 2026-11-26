// Разбор ответа об ошибке API в один вид. Конверт сервера:
// { "error": { "code", "message", "traceId", "details"? } } («Соглашения API»).
// Чистая функция без DOM и fetch: её проверяют node-тесты, а http.ts только зовёт.

export interface FieldProblem {
  field: string
  code?: string
  message: string
}

export interface ApiError {
  /** HTTP-статус; 0 — ответа не было (сеть). */
  status: number
  /** Машинный код: из конверта или по статусу (NETWORK — нет соединения). */
  code: string
  /** Текст для человека: из конверта или общий по ситуации. */
  message: string
  traceId?: string
  fields?: FieldProblem[]
}

export const MSG_NETWORK = 'Нет соединения. Проверьте интернет и повторите'
export const MSG_SERVER = 'Что-то пошло не так на сервере. Повторите позже'
export const MSG_UNKNOWN = 'Запрос не выполнен. Повторите'
/** 401 на стенде: экраны менеджера за входом (basic_auth в Caddy, BACKOFFICE_TOKEN на сервере). */
export const MSG_AUTH =
  'Нужен вход менеджера: обновите страницу — браузер спросит логин и пароль бэк-офиса'

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function fieldProblems(raw: unknown): FieldProblem[] | undefined {
  if (!Array.isArray(raw)) return undefined
  const list = raw.filter(
    (p): p is FieldProblem =>
      isObject(p) && typeof p.field === 'string' && typeof p.message === 'string',
  )
  return list.length > 0 ? list : undefined
}

export function networkError(): ApiError {
  return { status: 0, code: 'NETWORK', message: MSG_NETWORK }
}

/** Ответ с ошибкой → ApiError. Тело не по контракту (прокси, HTML) — общий текст по статусу. */
export function parseApiError(status: number, body: unknown): ApiError {
  const env = isObject(body) && isObject(body.error) ? body.error : undefined
  // Текст сервера про токен — для скриптов; человеку — что сделать в браузере.
  if (status === 401)
    return {
      status,
      code: 'UNAUTHORIZED',
      message: MSG_AUTH,
      traceId: typeof env?.traceId === 'string' ? env.traceId : undefined,
    }
  const fallback = status >= 500 ? MSG_SERVER : MSG_UNKNOWN
  return {
    status,
    code: typeof env?.code === 'string' ? env.code : `HTTP_${status}`,
    message: typeof env?.message === 'string' && env.message ? env.message : fallback,
    traceId: typeof env?.traceId === 'string' ? env.traceId : undefined,
    fields: fieldProblems(env?.details),
  }
}

export function isApiError(e: unknown): e is ApiError {
  return isObject(e) && typeof e.status === 'number' && typeof e.message === 'string'
}

/** Нужен вход менеджера (401 от API бэк-офиса). */
export const isAuthError = (e: ApiError | null | undefined) => e?.status === 401
