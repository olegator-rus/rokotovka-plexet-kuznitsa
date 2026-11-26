import { http, type RequestOptions } from './http'
import type { CostsResponse } from './types'

export const COSTS_PAGE = 20

/** GET /api/costs — сводка стоимости, маршруты, журнал вызовов модели (страницами). */
export function getCosts(offset = 0, opts?: RequestOptions) {
  const params = new URLSearchParams({ limit: String(COSTS_PAGE), offset: String(offset) })
  return http.get<CostsResponse>(`/costs?${params.toString()}`, opts)
}

/** GET /api/costs/summary — открытая сводка (гость): маршруты, средняя цена, доля без LLM, «Месяц»; журнала нет. */
export function getCostsSummary(opts?: RequestOptions) {
  return http.get<CostsResponse>('/costs/summary', opts)
}
