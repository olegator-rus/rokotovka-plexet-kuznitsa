import { http, type RequestOptions } from './http'
import type { AnalyticsResponse, EffectResponse, ProblemsNowResponse } from './types'

/** GET /api/analytics — плитки, воронка, проблемные запросы за период (у head — ещё вилка эффекта). */
export function getAnalytics(days: number, role: string, opts?: RequestOptions) {
  const params = new URLSearchParams({ days: String(days), role })
  return http.get<AnalyticsResponse>(`/analytics?${params.toString()}`, opts)
}

/** GET /api/analytics/current — что отвечает действующий поиск на первые limit проблемных запросов сейчас. */
export function getProblemsNow(days: number, limit: number, opts?: RequestOptions) {
  const params = new URLSearchParams({ days: String(days), limit: String(limit) })
  return http.get<ProblemsNowResponse>(`/analytics/current?${params.toString()}`, opts)
}

/** GET /api/effect — вилка эффекта; params — изменённые допущения (ключ → значение). */
export function getEffect(days: number, params: Record<string, number>, opts?: RequestOptions) {
  const q = new URLSearchParams({ days: String(days) })
  for (const [k, v] of Object.entries(params)) q.set(k, String(v))
  return http.get<EffectResponse>(`/effect?${q.toString()}`, opts)
}
