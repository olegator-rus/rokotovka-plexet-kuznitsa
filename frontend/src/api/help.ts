import { http, type RequestOptions } from './http'
import type { HelpAnswer } from './types'

/** GET /api/help — статьи справки магазина (контур доводки). */
export const getHelp = (opts?: RequestOptions) => http.get<{ items: HelpAnswer[] }>('/help', opts)
