import { http, type RequestOptions } from './http'

/** GET /api/health — backend/out/a_base.go Health. */
export interface Health {
  status: 'ok'
  db: 'ok' | 'down'
  /** Время ответа сервера, RFC 3339 (UTC). */
  time: string
}

export const getHealth = (opts?: RequestOptions) => http.get<Health>('/health', opts)
