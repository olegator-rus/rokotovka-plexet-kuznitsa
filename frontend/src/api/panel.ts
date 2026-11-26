import { http, type RequestOptions } from './http'
import type { PanelRecs, PanelSuggestResponse } from './typesPanel'

export interface PanelSuggestParams {
  q: string
  intentId: string
  sessionId: string
  clientId: string | null
  limit?: number
}

/** GET /api/suggest: подсказки, уточнения, категории, производители, товары панели. */
export function getPanelSuggest(p: PanelSuggestParams, opts?: RequestOptions) {
  const params = new URLSearchParams({ q: p.q })
  if (p.intentId) params.set('intent_id', p.intentId)
  if (p.sessionId) params.set('session_id', p.sessionId)
  if (p.clientId) params.set('client_id', p.clientId)
  if (p.limit) params.set('limit', String(p.limit))
  return http.get<PanelSuggestResponse>(`/suggest?${params.toString()}`, opts)
}

export interface PanelRecsParams {
  sessionId: string
  intentId: string
  spec: string[]
  /** Точный SKU в текущем вводе (основание exact_sku, §11.3 — аддитивно). */
  exact?: string | null
  viewed?: string | null
  category?: string | null
}

/** GET /api/suggest/recommendations: блок рекомендаций на сессию; null — блок скрыт. */
export function getPanelRecs(p: PanelRecsParams, opts?: RequestOptions) {
  const params = new URLSearchParams()
  if (p.sessionId) params.set('session_id', p.sessionId)
  if (p.intentId) params.set('intent_id', p.intentId)
  if (p.spec.length) params.set('spec', p.spec.join(','))
  if (p.exact) params.set('exact', p.exact)
  if (p.viewed) params.set('viewed', p.viewed)
  if (p.category) params.set('category', p.category)
  return http.get<PanelRecs | null>(`/suggest/recommendations?${params.toString()}`, opts)
}
