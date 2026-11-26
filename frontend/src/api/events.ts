import { http } from './http'
import type { SearchEvent } from './types'

/** POST /api/events → 204. Событие `search` пишет сам POST /api/search. */
export const postEvent = (event: SearchEvent) => http.post<void>('/events', event)
