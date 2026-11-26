import { http, type RequestOptions } from './http'
import type {
  CommerceResponse,
  RelationRollbackResponse,
  RelationsResponse,
  RelationVersionsResponse,
} from './typesCommerce'

/** GET /api/relations?family=&status= — действующий справочник связей. */
export function getRelations(
  filter: { family?: string; status?: string } = {},
  opts?: RequestOptions,
) {
  const params = new URLSearchParams()
  if (filter.family) params.set('family', filter.family)
  if (filter.status) params.set('status', filter.status)
  const qs = params.toString()
  return http.get<RelationsResponse>(`/relations${qs ? `?${qs}` : ''}`, opts)
}

/** GET /api/relations/versions — версии справочника, с разницей. */
export const getRelationVersions = (opts?: RequestOptions) =>
  http.get<RelationVersionsResponse>('/relations/versions', opts)

/** POST /api/relations/rollback — новая версия = копия выбранной, история не стирается. */
export const rollbackRelations = (toVersion: number, comment = '') =>
  http.post<RelationRollbackResponse>('/relations/rollback', { to_version: toVersion, comment })

/** GET /api/evaluations/commerce?set=p2 — лестница C0–C3 и метрики блока. */
export const getCommerceEval = (opts?: RequestOptions, set = 'p2') =>
  http.get<CommerceResponse>(
    `/evaluations/commerce?${new URLSearchParams({ set }).toString()}`,
    opts,
  )
