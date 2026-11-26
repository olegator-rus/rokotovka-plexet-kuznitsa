import { http, type RequestOptions } from './http'
import type { AnalyticsResponse } from './types'

/** GET /api/analytics — плитки, воронка, проблемные запросы за период. */
export function getAnalytics(days: number, role: string, opts?: RequestOptions) {
  const params = new URLSearchParams({ days: String(days), role })
  return http.get<AnalyticsResponse>(`/analytics?${params.toString()}`, opts)
}
