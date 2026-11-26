import { http, type RequestOptions } from './http'
import type { AnalyticsResponse } from './types'

/** GET /api/analytics — неудачные запросы за период (вкладка SearchOps «Запросы без ответа»). */
export function getAnalytics(days: number, opts?: RequestOptions) {
  const params = new URLSearchParams({ days: String(days) })
  return http.get<AnalyticsResponse>(`/analytics?${params.toString()}`, opts)
}
