// Сборка тела POST /api/events. Без DOM: tests/events.test.ts.
// Каждое событие несёт идентификатор поиска, сессии и клиента («События»).
import type { EventType, SearchEvent } from '@/api/types'
import { GUEST_CLIENT_ID } from './searchState.ts'

export interface EventContext {
  requestId: string | null
  sessionId: string
  clientId: string | null
}

export interface EventFields {
  productId?: string | null
  position?: number | null
  payload?: Record<string, unknown>
}

export function buildEvent(
  type: EventType,
  ctx: EventContext,
  target: EventFields = {},
): SearchEvent {
  return {
    type,
    request_id: ctx.requestId,
    session_id: ctx.sessionId,
    client_id: ctx.clientId ?? GUEST_CLIENT_ID,
    product_id: target.productId ?? null,
    position: target.position ?? null,
    payload: { ...(target.payload ?? {}) },
  }
}

/**
 * Токен сессии, выданный сервером в GET /api/meta: «s-» + 16 hex + «.» + подпись (32 hex). Самодельный номер
 * сервер в журнал не пишет, поэтому фронт свой не придумывает.
 */
export function isSessionToken(v: unknown): v is string {
  return typeof v === 'string' && /^s-[0-9a-f]{16}\.[0-9a-f]{32}$/.test(v)
}
