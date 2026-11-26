// Единственный HTTP-клиент фронта (AGENTS.md: все вызовы через него). Тот же origin, что и SPA:
// /api проксирует Vite в dev и Caddy в проде. Любой отказ приходит вызывающему как ApiError
// (lib/apiError.ts), никогда как сырой Response или TypeError.
import { networkError, parseApiError } from '@/lib/apiError'

const BASE = '/api'

export interface RequestOptions {
  signal?: AbortSignal
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts: RequestOptions = {},
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  let resp: Response
  try {
    resp = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
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
  if (!resp.ok) throw parseApiError(resp.status, data)
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
