// Спецификация покупателя: позиции живут в localStorage, чтобы пережить
// перезагрузку; недоступное хранилище (приватный режим) — спецификация живёт до перезагрузки.
// Здесь же — «уже есть» по строкам условно обязательного и отправка запроса КП.
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { postEvent } from '@/api/events'
import { postQuote } from '@/api/quotes'
import type { EventType } from '@/api/types'
import type { QuoteResponse } from '@/api/typesP2'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { buildEvent, type EventFields } from '@/lib/events'
import {
  type SpecLine,
  addLine,
  parseStored,
  quoteRequest,
  removeLine,
  setQuantity,
  summarize,
} from '@/lib/spec'
import { GUEST_CLIENT_ID } from '@/lib/searchState'
import { useAppStore } from './app'

const SPEC_KEY = 'plexet.spec'

function load(): SpecLine[] {
  try {
    return parseStored(window.localStorage.getItem(SPEC_KEY))
  } catch {
    return []
  }
}

function save(lines: SpecLine[]) {
  try {
    window.localStorage.setItem(SPEC_KEY, JSON.stringify(lines))
  } catch {
    // Хранилище недоступно — позиции остаются в памяти вкладки.
  }
}

export const useSpecStore = defineStore('spec', () => {
  const app = useAppStore()
  const lines = ref<SpecLine[]>(load())
  const open = ref(false)
  /** Строки условно обязательного, отмеченные «уже есть» (по товару, на время вкладки). */
  const dismissed = ref<Set<string>>(new Set())
  const quoteStatus = ref<'idle' | 'sending' | 'sent' | 'error'>('idle')
  const quoteError = ref<ApiError | null>(null)
  const lastQuote = ref<QuoteResponse | null>(null)

  const count = computed(() => lines.value.length)
  const summary = computed(() => summarize(lines.value))
  const has = (productId: string) => lines.value.some((l) => l.product_id === productId)

  function commit(next: SpecLine[]) {
    lines.value = next
    save(next)
    if (quoteStatus.value !== 'sending') quoteStatus.value = 'idle'
  }

  function track(type: EventType, requestId: string | null, target: EventFields = {}) {
    const ev = buildEvent(
      type,
      { requestId, sessionId: app.sessionId, clientId: app.clientId || GUEST_CLIENT_ID },
      target,
    )
    // Служебное событие не мешает покупателю: ошибка записи не показывается.
    postEvent(ev).catch(() => undefined)
  }

  function add(line: SpecLine) {
    commit(addLine(lines.value, line))
  }

  function remove(productId: string, requestId: string | null = null) {
    const line = lines.value.find((l) => l.product_id === productId)
    if (!line) return
    commit(removeLine(lines.value, productId))
    track('spec_remove', requestId, { productId, payload: { origin: line.origin } })
  }

  function setQty(productId: string, qty: unknown) {
    commit(setQuantity(lines.value, productId, qty))
  }

  function clear() {
    commit([])
  }

  function setDismissed(productId: string, on: boolean) {
    const next = new Set(dismissed.value)
    if (on) next.add(productId)
    else next.delete(productId)
    dismissed.value = next
  }

  async function sendQuote(comment = '') {
    if (!lines.value.length || quoteStatus.value === 'sending') return
    quoteStatus.value = 'sending'
    quoteError.value = null
    if (!app.sessionId) await app.loadMeta()
    const body = quoteRequest(lines.value, {
      sessionId: app.sessionId,
      clientId: app.clientId || GUEST_CLIENT_ID,
      comment,
    })
    try {
      lastQuote.value = await postQuote(body)
      quoteStatus.value = 'sent'
      track('quote_request', null, {
        payload: { positions: body.items.length, on_request: summary.value.onRequest },
      })
    } catch (e) {
      // Позиции сохранены, кнопка «Повторить» («Состояния экранов»).
      quoteError.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
      quoteStatus.value = 'error'
    }
  }

  return {
    lines,
    open,
    dismissed,
    quoteStatus,
    quoteError,
    lastQuote,
    count,
    summary,
    has,
    add,
    remove,
    setQty,
    clear,
    setDismissed,
    sendQuote,
    track,
  }
})
