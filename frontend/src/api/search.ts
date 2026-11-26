import { http, type RequestOptions } from './http'
import type { SearchRequest, SearchResponse, SuggestResponse } from './types'

/** POST /api/search — выдача, строка понимания и трасса. */
export const postSearch = (body: SearchRequest, opts?: RequestOptions) =>
  http.post<SearchResponse>('/search', body, opts)

/** GET /api/suggest — подсказки, не больше 8 строк суммарно. */
export function getSuggest(q: string, clientId: string | null, opts?: RequestOptions) {
  const params = new URLSearchParams({ q })
  if (clientId) params.set('client_id', clientId)
  return http.get<SuggestResponse>(`/suggest?${params.toString()}`, opts)
}
