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

/** Новый идентификатор сессии вкладки: «s-» + 16 hex. */
export function newSessionId(random: () => number = Math.random): string {
  let hex = ''
  for (let i = 0; i < 16; i += 1) hex += Math.floor(random() * 16).toString(16)
  return `s-${hex}`
}
