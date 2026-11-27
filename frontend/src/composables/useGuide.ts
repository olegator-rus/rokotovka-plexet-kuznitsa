// Ход подбора (контур подбора, SPEC §2, §5; контур экрана, UI §3–§4): открытие, ответ на вопрос, правка условия, повтор, скрытие.
// Одно актуальное состояние, без ленты сообщений: новый ответ заменяет прежний, пока ход идёт — прежний остаётся
// с пометкой «Обновляем подбор». Состояние подбора хранит сервер; фронт возвращает его следующим ходом как есть.
// Скрытие и сворачивание состояние не сбрасывают.
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import { postGuide, postGuideSpec } from '@/api/guide'
import type {
  GuideChipAnswer,
  GuideConditionEdit,
  GuideOffer,
  GuideRequest,
  GuideResponse,
} from '@/api/typesGuide'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { offerQuantity, selectionId, withSpecLine } from '@/lib/guide'
import { useAppStore } from '@/stores/app'

export type GuideStatus = 'idle' | 'loading' | 'ready' | 'error'

export function useGuide() {
  const app = useAppStore()
  /** Рабочая область показана. */
  const open = ref(false)
  /** Свёрнута в одну строку (телефон). */
  const collapsed = ref(false)
  const status = ref<GuideStatus>('idle')
  const response = shallowRef<GuideResponse | null>(null)
  const error = ref<ApiError | null>(null)
  /** Запрос строки поиска, к которому относится подбор. */
  const query = ref('')
  /** Позиция, добавленная в спецификацию из показанного ответа: «Добавлено в спецификацию» в итоговой строке. */
  const added = ref<string | null>(null)
  /** Последняя фраза покупателя, на которую отвечает показанное состояние: «Вы: …» одной строкой. */
  const said = ref('')
  let lastSaid = ''
  let last: Omit<GuideRequest, 'session_id'> | null = null
  let ctrl: AbortController | null = null

  async function send(body: Omit<GuideRequest, 'session_id'>, phrase = '') {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    last = body
    lastSaid = phrase
    open.value = true
    collapsed.value = false
    status.value = 'loading'
    error.value = null
    try {
      if (!app.sessionId) await app.loadMeta()
      if (ctrl !== c) return
      const resp = await postGuide(
        { session_id: app.sessionId, ...body },
        { signal: c.signal, ignoreUnauthorized: true },
      )
      if (ctrl !== c) return
      response.value = resp
      said.value = phrase
      added.value = null
      status.value = 'ready'
    } catch (e) {
      if (ctrl !== c) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
      status.value = 'error'
    }
  }

  /**
   * Подбор по запросу строки (кнопка «Помочь выбрать», автораскрытие, вход со страницы семейства). Тот же запрос
   * без реплики, если подбор уже есть, — просто показать его снова: условия сохраняются.
   */
  function start(q: string, message?: string) {
    if (!message && q === query.value && response.value && status.value !== 'error') {
      open.value = true
      collapsed.value = false
      return
    }
    query.value = q
    response.value = null
    added.value = null
    said.value = ''
    void send({ query: q, ...(message ? { message } : {}) }, message ?? '')
  }

  const prevState = () => response.value?.state ?? undefined

  /** Ответ на вопрос подбора вариантом. */
  function answer(chip: GuideChipAnswer) {
    void send({ query: query.value, chip, state: prevState() })
  }

  /** Реплика покупателя (поле подбора или строка поиска): уточнение текущего подбора. */
  function say(message: string) {
    const m = message.trim()
    if (!m) return
    void send({ query: query.value, message: m, state: prevState() }, m)
  }

  /** Изменить (value — новое значение) или снять (null) условие; результат пересчитывается сразу. */
  function editCondition(edit: GuideConditionEdit) {
    void send({ query: query.value, condition: edit, state: prevState() })
  }

  /** Версия черновика спецификации подбора, которую видел покупатель (SPEC §4, update_spec). */
  let specVersion = 0
  // Черновик версионный: две строки подряд (рабочие станции и серверы) идут по очереди, иначе вторая получает 409.
  let specQueue: Promise<unknown> = Promise.resolve()
  /**
   * Строка в черновик подбора на сервере: цена перечитывается из каталога, ключ операции не даёт задвоить строку.
   * Сервер недоступен или не ответил за 3 с — оффер как есть (цена из ответа хода, тоже из каталога).
   */
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
    if (last) void send(last, lastSaid)
  }

  /** Скрыть рабочую область: условия и результат сохраняются, «Помочь выбрать» вернёт их. */
  function hide() {
    if (status.value === 'loading') {
      ctrl?.abort()
      ctrl = null
      status.value = response.value ? 'ready' : 'idle'
    }
    open.value = false
  }

  /** Сбросить подбор целиком: строка поиска очищена. */
  function reset() {
    ctrl?.abort()
    ctrl = null
    open.value = false
    collapsed.value = false
    status.value = 'idle'
    response.value = null
    error.value = null
    added.value = null
    said.value = ''
    last = null
    query.value = ''
  }

  onBeforeUnmount(() => ctrl?.abort())

  return {
    open,
    collapsed,
    status,
    response,
    error,
    query,
    added,
    said,
    start,
    answer,
    say,
    editCondition,
    toSpec,
    retry,
    hide,
    reset,
  }
}

export type GuideSession = ReturnType<typeof useGuide>
