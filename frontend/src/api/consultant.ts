// POST /api/consultant: ответ — поток application/x-ndjson, его читает
// response.body.getReader(). http.ts поток не отдаёт, поэтому здесь свой fetch с тем же адресом, cookie и
// разбором конверта ошибки; ошибка до потока приходит вызывающему как ApiError.
import { networkError, parseApiError } from '@/lib/apiError'
import { readNdjson } from '@/lib/consultant'
import type { ConsultantRequest, ConversationEvent } from './typesConsultant'

export async function postConsultant(
  body: ConsultantRequest,
  onEvent: (e: ConversationEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  let resp: Response
  try {
    resp = await fetch('/api/consultant', {
      method: 'POST',
      headers: { Accept: 'application/x-ndjson', 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'include',
      cache: 'no-store',
      signal,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e
    throw networkError()
  }
  if (!resp.ok || !resp.body) {
    let data: unknown = undefined
    try {
      data = await resp.json()
    } catch {
      data = undefined
    }
    throw parseApiError(resp.status, data, null, false)
  }
  await readNdjson(resp.body, onEvent)
}
