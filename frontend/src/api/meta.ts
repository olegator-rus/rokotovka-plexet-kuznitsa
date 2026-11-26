import { http, type RequestOptions } from './http'
import type { Meta } from './types'

/** GET /api/meta — режим LLM, версии каталога и правил, конфигурации, клиенты для переключателя. */
export const getMeta = (opts?: RequestOptions) => http.get<Meta>('/meta', opts)
