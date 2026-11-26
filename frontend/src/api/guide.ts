import { http, type RequestOptions } from './http'
import type {
  GuideRequest,
  GuideResponse,
  GuideRoute,
  GuideSpec,
  GuideSpecRequest,
} from './typesGuide'

/** POST /api/guide — один ход подбора (контур подбора, SPEC §5–§6). */
export const postGuide = (body: GuideRequest, opts?: RequestOptions) =>
  http.post<GuideResponse>('/guide', body, opts)

/** POST /api/guide/route — режим строки для запроса: search | search_offer | guide (SPEC §1). */
export const postGuideRoute = (query: string, opts?: RequestOptions) =>
  http.post<GuideRoute>('/guide/route', { query }, opts)

/** POST /api/guide/spec — строка подбора в черновик с перечитанной ценой и ключом операции (SPEC §4). */
export const postGuideSpec = (body: GuideSpecRequest, opts?: RequestOptions) =>
  http.post<GuideSpec>('/guide/spec', body, opts)
