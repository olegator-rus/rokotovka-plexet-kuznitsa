import { http, type RequestOptions } from './http'
import type { Meta } from './types'

/**
 * GET /api/meta — режим LLM, версии каталога и правил, конфигурации, клиенты для переключателя и токен сессии.
 * session — действующий токен: сервер его продлевает (X-Session), иначе выдаёт новый.
 */
export const getMeta = (session?: string, opts?: RequestOptions) =>
  http.get<Meta>('/meta', session ? { ...opts, headers: { 'X-Session': session } } : opts)
