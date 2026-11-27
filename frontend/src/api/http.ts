// Единственный HTTP-клиент фронта (AGENTS.md: все вызовы через него). Тот же origin, что и SPA:
// /api проксирует Vite в dev и Caddy в проде. Любой отказ приходит вызывающему как ApiError
// (lib/apiError.ts), никогда как сырой Response или TypeError.
import { networkError, parseApiError } from '@/lib/apiError'
import { retryAfterSeconds } from '@/lib/auth'

/** 401 посреди работы (сессия истекла или вышли в другой вкладке): стор входа узнаёт и ведёт на /login. */
type UnauthorizedHandler = (path: string) => void
let unauthorized: UnauthorizedHandler | null = null
export function onUnauthorized(fn: UnauthorizedHandler) {
  unauthorized = fn
}

const BASE = '/api'

export interface RequestOptions {
  signal?: AbortSignal
  /** Дополнительные заголовки (X-Session в GET /api/meta). */
  headers?: Record<string, string>
  /** 401 не означает конец сессии (необязательный вызов, у которого пути может не быть): вход не сбрасывается. */
  ignoreUnauthorized?: boolean
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts: RequestOptions = {},
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json', ...opts.headers }
  // FormData (загрузка файла) — заголовок с границей частей ставит сам браузер.
  const form = typeof FormData !== 'undefined' && body instanceof FormData
  if (body !== undefined && !form) headers['Content-Type'] = 'application/json'
  let resp: Response
  try {
    resp = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : form ? body : JSON.stringify(body),
      credentials: 'include',
      cache: 'no-store',
      signal: opts.signal,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e
    throw networkError()
  }
  if (resp.status === 204) return undefined as T
  const text = await resp.text()
  let data: unknown = undefined
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = undefined
    }
  }
  if (!resp.ok) {
    const authCall = path.startsWith('/auth/')
    if (resp.status === 401 && !authCall && !opts.ignoreUnauthorized) unauthorized?.(path)
    throw parseApiError(
      resp.status,
      data,
      retryAfterSeconds(resp.headers.get('Retry-After')),
      authCall,
    )
  }
  return data as T
}

export const http = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, undefined, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('POST', path, body ?? {}, opts),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PUT', path, body ?? {}, opts),
  del: <T = void>(path: string, opts?: RequestOptions) =>
    request<T>('DELETE', path, undefined, opts),
}
