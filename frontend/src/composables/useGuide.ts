// Ход подбора (контур подбора, SPEC §2, §5): открытие, ответ чипом или текстом, правка условия, повтор, закрытие.
// Состояние подбора хранит сервер; фронт возвращает его следующим ходом как есть. Прошлый ход отменяется новым.
import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'
import { postGuide, postGuideSpec } from '@/api/guide'
import type {
  GuideChipAnswer,
  GuideConditionEdit,
  GuideOffer,
  GuideRequest,
  GuideResponse,
} from '@/api/typesGuide'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { offerQuantity, pendingStep, selectionId, withSpecLine } from '@/lib/guide'
import { useAppStore } from '@/stores/app'

export type GuideStatus = 'idle' | 'loading' | 'ready' | 'error'

export function useGuide() {
  const app = useAppStore()
  const open = ref(false)
  const collapsed = ref(false)
  const status = ref<GuideStatus>('idle')
  const response = shallowRef<GuideResponse | null>(null)
  const error = ref<ApiError | null>(null)
  /** Запрос строки поиска, к которому относится подбор. */
  const query = ref('')
  const elapsed = ref(0)
  let last: Omit<GuideRequest, 'session_id'> | null = null
  let ctrl: AbortController | null = null
  let timer: ReturnType<typeof setInterval> | null = null

  function stopTimer() {
    if (timer) clearInterval(timer)
    timer = null
  }

  async function send(body: Omit<GuideRequest, 'session_id'>) {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    last = body
    open.value = true
    collapsed.value = false
    status.value = 'loading'
    error.value = null
    elapsed.value = 0
    const started = Date.now()
    stopTimer()
    timer = setInterval(() => (elapsed.value = Date.now() - started), 250)
    try {
      if (!app.sessionId) await app.loadMeta()
      if (ctrl !== c) return
      const resp = await postGuide(
        { session_id: app.sessionId, ...body },
        { signal: c.signal, ignoreUnauthorized: true },
      )
      if (ctrl !== c) return
      response.value = resp
      status.value = 'ready'
    } catch (e) {
      if (ctrl !== c) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
      status.value = 'error'
    } finally {
      if (ctrl === c) stopTimer()
    }
  }

  /** Новый подбор по запросу строки (кнопка «Помочь выбрать», вопрос в строке, вход со страницы семейства). */
  function start(q: string, message?: string) {
    query.value = q
    response.value = null
    void send({ query: q, ...(message ? { message } : {}) })
  }

  const prevState = () => response.value?.state ?? undefined

  /** Ответ чипом на вопрос подбора. */
  function answer(chip: GuideChipAnswer) {
    void send({ query: query.value, chip, state: prevState() })
  }

  /** Ответ или вопрос текстом в поле подбора. */
  function say(message: string) {
    const m = message.trim()
    if (!m) return
    void send({ query: query.value, message: m, state: prevState() })
  }

  /**
   * Убрать (value null) или изменить условие. message — тот же смысл текстом: ядро, которое ещё не принимает
   * condition, разбирает правку как реплику покупателя.
   */
  function editCondition(edit: GuideConditionEdit, message: string) {
    void send({ query: query.value, condition: edit, message, state: prevState() })
  }

  /** Версия черновика спецификации подбора, которую видел покупатель (SPEC §4, update_spec). */
  let specVersion = 0
  /**
   * Строка в черновик подбора на сервере: цена перечитывается из каталога, ключ операции не даёт задвоить строку.
   * Сервер недоступен или не ответил за 3 с — оффер как есть (цена из ответа хода, тоже из каталога).
   */
  // Черновик версионный: две строки подряд (рабочие станции и серверы) идут по очереди, иначе вторая получает 409.
  let specQueue: Promise<unknown> = Promise.resolve()
  function toSpec(o: GuideOffer): Promise<GuideOffer> {
    const next = specQueue.then(() => toSpecNow(o))
    specQueue = next.catch(() => undefined)
    return next
  }

  async function toSpecNow(o: GuideOffer): Promise<GuideOffer> {
    const r = response.value
    const sel = selectionId(r?.state)
    if (!sel) return o
    const quantity = offerQuantity(o, r?.conditions ?? [])
    const c = new AbortController()
    const t = setTimeout(() => c.abort(), 3000)
    try {
      const spec = await postGuideSpec(
        {
          session_id: app.sessionId,
          selection_id: sel,
          version: specVersion,
          offer_id: o.offer_id,
          quantity,
          op_key: `${sel}:${o.offer_id}:${quantity}`,
        },
        { signal: c.signal, ignoreUnauthorized: true },
      )
      specVersion = spec.version
      return withSpecLine(
        o,
        spec.lines.find((l) => l.offer_id === o.offer_id),
      )
    } catch {
      return o
    } finally {
      clearTimeout(t)
    }
  }

  function retry() {
    if (last) void send(last)
  }

  /** Закрыть: выдача и спецификация остаются (SPEC §1). */
  function close() {
    ctrl?.abort()
    ctrl = null
    stopTimer()
    open.value = false
    status.value = 'idle'
    response.value = null
    error.value = null
    last = null
  }

  onBeforeUnmount(() => {
    ctrl?.abort()
    stopTimer()
  })

  const pendingText = computed(() => pendingStep(elapsed.value))

  return {
    open,
    collapsed,
    status,
    response,
    error,
    query,
    pendingText,
    start,
    answer,
    say,
    editCondition,
    toSpec,
    retry,
    close,
  }
}

export type GuideSession = ReturnType<typeof useGuide>
