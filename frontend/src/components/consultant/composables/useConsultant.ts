// Консультант контура консультанта: одно состояние на приложение (кнопка и окно монтируются один раз в оболочке, история
// переживает смену страницы). Окно открывается ТОЛЬКО вызовом toggle()/show() по нажатию покупателя.
// Ход: реплика → POST /api/consultant (или заглушка) → события в ленту; spec_event применяется к стору
// спецификации, неудача — ошибка в ленте, а не «Добавлено».
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { postConsultant } from '@/api/consultant'
import type {
  ConversationEvent,
  ProductReference,
  SpecEventAttachment,
} from '@/api/typesConsultant'
import {
  activeQuickReplies,
  applyEvent,
  type ChatMessage,
  CONSULTANT_KEY,
  consultantEnabled,
  newConversationId,
  pageContext,
  parseConversation,
  msgKey,
  resolveSpecEvent,
  SPEC_ERRORS,
  specPayload,
  turnFailed,
} from '@/lib/consultant'
import { mockStream } from '@/lib/consultantMock'
import { useAppStore } from '@/stores/app'
import { useSpecStore } from '@/stores/spec'

function storage(): Storage | null {
  try {
    return typeof window !== 'undefined' ? window.sessionStorage : null
  } catch {
    return null
  }
}

function readLocal(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

let flags: { enabled: boolean; mock: boolean } | null = null

/** Флаг считается один раз при запуске: переход по SPA теряет ?consultant=1 из адреса. */
export function consultantFlags(): { enabled: boolean; mock: boolean } {
  if (flags) return flags
  if (typeof window === 'undefined') return { enabled: false, mock: false }
  const search = window.location.search
  const mock =
    new URLSearchParams(search).get('consultant') === 'mock' || readLocal('consultant_mock') === '1'
  const enabled =
    mock ||
    consultantEnabled({
      env: import.meta.env.VITE_CONSULTANT as string | undefined,
      search,
      stored: readLocal('consultant'),
    })
  flags = { enabled, mock }
  return flags
}

function createState() {
  const saved = parseConversation(storage()?.getItem(CONSULTANT_KEY) ?? null)
  const conversationId = ref(saved.conversationId)
  const messages = ref<ChatMessage[]>(saved.messages)
  const draft = ref(saved.draft)
  const open = ref(saved.open)
  const expanded = ref(saved.expanded)
  const generating = ref(false)
  const step = ref<string | null>(null)
  const unread = ref(false)
  let turn = messages.value.reduce((m, x) => Math.max(m, x.turn), 0)
  let ctrl: AbortController | null = null

  watch(
    [conversationId, messages, draft, open, expanded],
    () => {
      try {
        storage()?.setItem(
          CONSULTANT_KEY,
          JSON.stringify({
            conversationId: conversationId.value,
            messages: messages.value,
            draft: draft.value,
            open: open.value,
            expanded: expanded.value,
          }),
        )
      } catch {
        // Хранилище недоступно или переполнено — разговор живёт до перезагрузки.
      }
    },
    { deep: true },
  )

  return {
    conversationId,
    messages,
    draft,
    open,
    expanded,
    generating,
    step,
    unread,
    nextTurn: () => ++turn,
    abort: () => {
      ctrl?.abort()
      ctrl = null
    },
    controller: () => {
      ctrl?.abort()
      ctrl = new AbortController()
      return ctrl
    },
  }
}

let state: ReturnType<typeof createState> | null = null

export function useConsultant() {
  if (!state) state = createState()
  const s = state
  const app = useAppStore()
  const spec = useSpecStore()
  const route = useRoute()

  const hasConversation = computed(() => s.messages.value.some((m) => m.role === 'user'))
  const quick = computed(() => (s.generating.value ? null : activeQuickReplies(s.messages.value)))
  const lastUserText = computed(() => {
    for (let i = s.messages.value.length - 1; i >= 0; i--) {
      const m = s.messages.value[i]
      if (m?.role === 'user') return m.text
    }
    return ''
  })

  /** Применить spec_event к стору; вернуть текст ошибки или undefined при успехе (проверяется по стору). */
  function applySpec(ev: SpecEventAttachment, query = ''): string | undefined {
    const r = resolveSpecEvent(spec.lines, ev, query)
    if (!r.ok) return r.error
    const op = r.op
    try {
      if (op.kind === 'add') {
        spec.add(op.line)
        return spec.has(op.line.product_id) ? undefined : SPEC_ERRORS.added
      }
      if (op.kind === 'remove') {
        spec.remove(op.offerId)
        return spec.has(op.offerId) ? SPEC_ERRORS.removed : undefined
      }
      spec.setQty(op.offerId, op.qty)
      const line = spec.lines.find((l) => l.product_id === op.offerId)
      return line && line.quantity === op.qty ? undefined : SPEC_ERRORS.updated
    } catch {
      return SPEC_ERRORS[ev.action] ?? SPEC_ERRORS.added
    }
  }

  function onEvent(ev: ConversationEvent, turn: number, query: string) {
    if (ev.type === 'status') {
      s.step.value = ev.step
      return
    }
    let failed: string | undefined
    if (ev.type === 'message.attachment' && ev.attachment.type === 'spec_event') {
      failed = applySpec(ev.attachment, query)
    }
    let next = applyEvent(s.messages.value, ev, turn)
    if (failed) {
      const key = msgKey(turn, ev.id)
      next = next.map((m) =>
        m.id === key
          ? {
              ...m,
              attachments: m.attachments.map((a, i, all) =>
                i === all.length - 1 ? { ...a, failed } : a,
              ),
            }
          : m,
      )
    }
    s.messages.value = next
  }

  async function run(text: string, turn: number) {
    const ctrl = s.controller()
    s.generating.value = true
    s.step.value = null
    const page = pageContext(
      typeof route.name === 'string' ? route.name : null,
      route.params as Record<string, unknown>,
      route.query as Record<string, unknown>,
      spec.open,
    )
    const query = page.query ?? ''
    try {
      if (!app.sessionId) await app.loadMeta().catch(() => undefined)
      const body = {
        session_id: app.sessionId,
        conversation_id: s.conversationId.value,
        text,
        page,
        spec: specPayload(spec.lines),
      }
      if (consultantFlags().mock) {
        for await (const ev of mockStream(body, ctrl.signal)) onEvent(ev, turn, query)
      } else {
        await postConsultant(body, (ev) => onEvent(ev, turn, query), ctrl.signal)
      }
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
    } finally {
      if (!ctrl.signal.aborted) {
        // Оборванные сообщения хода — завершены; ответа нет вовсе — ошибка с повтором.
        let next = s.messages.value.map((m) =>
          m.turn === turn && m.status === 'streaming' ? { ...m, status: 'complete' as const } : m,
        )
        if (turnFailed(next, turn)) {
          next = [
            ...next,
            {
              id: `${turn}:retry`,
              role: 'assistant',
              text: '',
              attachments: [],
              status: 'error',
              turn,
              retry: true,
            },
          ]
        }
        s.messages.value = next
        s.generating.value = false
        s.step.value = null
        if (!s.open.value) s.unread.value = true
      }
    }
  }

  function send(raw: string): boolean {
    const text = raw.trim()
    if (!text || s.generating.value) return false
    const turn = s.nextTurn()
    s.messages.value = [
      ...s.messages.value,
      { id: `${turn}:u`, role: 'user', text, attachments: [], status: 'complete', turn },
    ]
    void run(text, turn)
    return true
  }

  /** «Попробовать ещё раз»: тот же текст, без второго пузыря покупателя. */
  function retry() {
    if (s.generating.value) return
    const text = lastUserText.value
    if (!text) return
    s.messages.value = s.messages.value.filter((m) => !m.retry)
    void run(text, s.nextTurn())
  }

  /** «В спецификацию» на карточке — действие фронта и локальное событие «Добавлено». */
  function addProduct(p: ProductReference): boolean {
    const turn = s.messages.value.reduce((m, x) => Math.max(m, x.turn), 0)
    const ev = {
      type: 'spec_event' as const,
      action: 'added' as const,
      offerId: p.offerId,
      title: p.title,
      quantity: p.quantity,
      totalPriceMinor: p.priceStatus === 'known' ? p.totalPriceMinor : undefined,
      offer: p.offer,
    }
    const failed = applySpec(ev)
    const id = `${turn}:local-${Date.now()}`
    s.messages.value = [
      ...s.messages.value,
      {
        id,
        role: 'assistant',
        text: '',
        attachments: [{ key: `${id}:0`, data: ev, ...(failed ? { failed } : {}) }],
        status: 'complete',
        turn,
      },
    ]
    return !failed
  }

  function newConversation() {
    s.abort()
    s.generating.value = false
    s.step.value = null
    s.messages.value = []
    s.draft.value = ''
    s.conversationId.value = newConversationId()
  }

  function show() {
    s.open.value = true
    s.unread.value = false
  }
  function hide() {
    s.open.value = false
  }
  function toggle() {
    if (s.open.value) hide()
    else show()
  }

  return {
    ...s,
    hasConversation,
    quick,
    send,
    retry,
    addProduct,
    newConversation,
    show,
    hide,
    toggle,
  }
}
