// Помощник по выбору (контур помощника, CHAT.md): лента разговора, ход к серверу, «В спецификацию», хранение в
// sessionStorage. Панель открывается ТОЛЬКО вызовом open() по нажатию покупателя; восстановленный разговор её не
// открывает. Сообщения помощника строит сервер — здесь только порядок ленты и состояние ожидания.
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { postGuide, postGuideSpec } from '@/api/guide'
import type { ChatCard, ChatSuggestion, GuideChipAnswer, GuideOffer } from '@/api/typesGuide'
import {
  addedLine,
  type AssistantEntry,
  type ChatEntry,
  lastAssistantId,
  loadAssistant,
  type OpenSource,
  openPlan,
  saveAssistant,
  turnBody,
} from '@/lib/assistant'
import { lineFromOffer, selectionId, withSpecLine } from '@/lib/guide'
import { useAppStore } from '@/stores/app'
import { useSpecStore } from '@/stores/spec'

const storage = (): Storage | null => {
  try {
    return typeof window !== 'undefined' ? window.sessionStorage : null
  } catch {
    return null
  }
}

/** Тексты ожидания: сразу и через полторы секунды. */
export const WAIT_FIRST = 'Уточняю условия лицензирования…'
export const WAIT_SECOND = 'Проверяю подходящие варианты…'

export function useAssistant() {
  const app = useAppStore()
  const spec = useSpecStore()
  const saved = loadAssistant(storage())

  const open = ref(false)
  const entries = ref<ChatEntry[]>(saved.entries)
  const serverState = shallowRef<unknown>(saved.state)
  /** Тема со страницы продукта: «О продукте: Astra Linux Special Edition». */
  const product = ref(saved.product)
  /** Запрос строки поиска, с которым открыли: стартовые подсказки по нему. */
  const context = ref('')
  const draft = ref('')
  const pending = ref(false)
  const waitText = ref(WAIT_FIRST)
  /** Есть новый ответ, пока панель закрыта: точка на кнопке. */
  const unread = ref(false)
  /** Карточки, которые сейчас добавляются, и уже добавленные из помощника. */
  const adding = ref<Set<string>>(new Set())
  const failedAdd = ref<Set<string>>(new Set())
  /** Запрос, с которого начат текущий подбор (для заявки КП). */
  let startQuery = (() => {
    for (let i = saved.entries.length - 1; i >= 0; i--) {
      const e = saved.entries[i]
      if (e && e.kind === 'assistant') return e.query
    }
    return ''
  })()
  let nextId = entries.value.reduce((m, e) => Math.max(m, e.id), 0) + 1
  let ctrl: AbortController | null = null
  let waitTimer: ReturnType<typeof setTimeout> | null = null
  let lastInput: { text?: string; chip?: GuideChipAnswer; bubble: string } | null = null
  let specVersion = 0

  const lastId = computed(() => lastAssistantId(entries.value))
  const lastChat = computed(() => {
    const e = entries.value.find((x) => x.id === lastId.value)
    return e && e.kind === 'assistant' ? e.chat : null
  })
  /** «Учитываю: …» — после первого ответа и пока подбор не сброшен. */
  const conditionsLabel = computed(() =>
    serverState.value ? (lastChat.value?.conditions.label ?? '') : '',
  )
  const hasConversation = computed(() => entries.value.length > 0)

  watch(
    [entries, serverState, product],
    () =>
      saveAssistant(storage(), {
        entries: entries.value,
        state: serverState.value,
        product: product.value,
      }),
    { deep: true },
  )

  function push(e: NewEntry) {
    entries.value = [...entries.value, { ...e, id: nextId++ } as ChatEntry]
  }

  /** Открыть панель по нажатию: как поступить с запросом поиска — CHAT.md §1. */
  function openPanel(source: OpenSource, query = '', productTitle = '') {
    open.value = true
    unread.value = false
    context.value = productTitle || query
    if (source === 'product') product.value = productTitle
    const plan = openPlan(source, query, hasConversation.value)
    if (plan.send) void send(plan.send)
    else if (plan.prefill && !draft.value) draft.value = plan.prefill
  }

  function close() {
    open.value = false
  }

  function stopWait() {
    pending.value = false
    if (waitTimer) clearTimeout(waitTimer)
    waitTimer = null
  }

  async function turn(input: { text?: string; chip?: GuideChipAnswer }, bubble: string) {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    lastInput = { ...input, bubble }
    pending.value = true
    waitText.value = WAIT_FIRST
    if (waitTimer) clearTimeout(waitTimer)
    waitTimer = setTimeout(() => (waitText.value = WAIT_SECOND), 1500)
    if (!serverState.value) startQuery = input.text ?? ''
    const body = turnBody(input, serverState.value, startQuery)
    try {
      if (!app.sessionId) await app.loadMeta()
      if (ctrl !== c) return
      const resp = await postGuide(
        { session_id: app.sessionId, ...body },
        { signal: c.signal, ignoreUnauthorized: true },
      )
      if (ctrl !== c) return
      stopWait()
      // Сервер начал новый подбор (реплика о другом вендоре) — исходный запрос теперь эта реплика.
      const prevSel = selectionId(serverState.value)
      if (prevSel && selectionId(resp.state) !== prevSel && input.text) startQuery = input.text
      serverState.value = resp.state ?? null
      if (resp.chat) {
        const entry: Omit<AssistantEntry, 'id'> = {
          kind: 'assistant',
          chat: resp.chat,
          selectionId: selectionId(resp.state),
          conditions: resp.conditions ?? [],
          offers: resp.offers ?? [],
          query: startQuery,
        }
        push(entry)
      } else {
        push({ kind: 'error', text: 'Не удалось получить товары.' })
      }
      if (!open.value) unread.value = true
    } catch (e) {
      if (ctrl !== c) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      stopWait()
      // Старое не выдаём за новое: в ленте — сбой с повтором.
      push({ kind: 'error', text: 'Не удалось получить товары.' })
    }
  }

  /** Реплика покупателя: пузырь сразу, поле очищается. */
  async function send(text: string) {
    const t = text.trim()
    if (!t || pending.value) return
    draft.value = ''
    push({ kind: 'user', text: t })
    await turn({ text: t }, t)
  }

  /** Подсказка следующего шага или кнопка вопроса: подпись — пузырь покупателя, на сервер — чип или реплика. */
  async function suggest(s: ChatSuggestion | { label: string; key: string; value: string }) {
    if (pending.value) return
    if ('key' in s && s.key) {
      push({ kind: 'user', text: s.label })
      await turn({ chip: { key: s.key, value: s.value } }, s.label)
      return
    }
    const sg = s as ChatSuggestion
    push({ kind: 'user', text: sg.label })
    if (sg.chip && serverState.value)
      await turn({ chip: { key: sg.chip.key, value: sg.chip.value } }, sg.label)
    else await turn({ text: sg.message || sg.label }, sg.label)
  }

  function retry() {
    if (!lastInput || pending.value) return
    entries.value = entries.value.filter(
      (e, i) => !(e.kind === 'error' && i === entries.value.length - 1),
    )
    void turn(lastInput, lastInput.bubble)
  }

  /** «Сбросить»: условия очищаются, история и спецификация остаются; следующая реплика — новый подбор. */
  function resetConditions() {
    serverState.value = null
    startQuery = ''
  }

  /** «Начать новый разговор» (очистить ленту). */
  function clearAll() {
    ctrl?.abort()
    stopWait()
    entries.value = []
    serverState.value = null
    product.value = ''
    draft.value = ''
  }

  /** «В спецификацию»: цена перечитывается сервером; без ответа за 3 с — числа карточки (тоже из каталога). */
  async function addToSpec(entry: AssistantEntry, card: ChatCard): Promise<boolean> {
    const id = card.offer_id
    if (adding.value.has(id) || spec.has(id)) return false
    adding.value = new Set([...adding.value, id])
    const nf = new Set(failedAdd.value)
    nf.delete(id)
    failedAdd.value = nf
    const base: GuideOffer | undefined = entry.offers.find((o) => o.offer_id === id)
    try {
      if (!base) throw new Error('нет позиции')
      let offer: GuideOffer = { ...base, quantity: card.quantity }
      if (entry.selectionId) {
        const c = new AbortController()
        const t = setTimeout(() => c.abort(), 3000)
        try {
          const res = await postGuideSpec(
            {
              session_id: app.sessionId,
              selection_id: entry.selectionId,
              version: specVersion,
              offer_id: id,
              quantity: card.quantity,
              op_key: `${entry.selectionId}:${id}:${card.quantity}`,
            },
            { signal: c.signal, ignoreUnauthorized: true },
          )
          specVersion = res.version
          offer = withSpecLine(
            offer,
            res.lines.find((l) => l.offer_id === id),
          )
        } catch {
          // подбор истёк или сервер не ответил — числа из ответа хода
        } finally {
          clearTimeout(t)
        }
      }
      spec.add(lineFromOffer(offer, entry.conditions, entry.query))
      const sum = spec.summary
      push({ kind: 'system', text: addedLine(sum.positions, sum.knownRub), openSpec: true })
      return true
    } catch {
      failedAdd.value = new Set([...failedAdd.value, id])
      return false
    } finally {
      const na = new Set(adding.value)
      na.delete(id)
      adding.value = na
    }
  }

  onBeforeUnmount(() => {
    ctrl?.abort()
    stopWait()
  })

  return {
    open,
    entries,
    product,
    context,
    draft,
    pending,
    waitText,
    unread,
    adding,
    failedAdd,
    lastId,
    conditionsLabel,
    hasConversation,
    openPanel,
    close,
    send,
    suggest,
    retry,
    resetConditions,
    clearAll,
    addToSpec,
  }
}

/** Запись ленты без номера (номер даёт push). */
type NewEntry = ChatEntry extends infer E ? (E extends ChatEntry ? Omit<E, 'id'> : never) : never

export type AssistantSession = ReturnType<typeof useAssistant>
