// Живой запуск агента: снимок GET /api/agent/runs/{id}, затем лента по SSE (step, status, done).
// Сервер закрывает поток каждые ≤25 с — EventSource переподключается сам с Last-Event-ID и получает
// только новые шаги. Поток не поднимается несколько раз подряд — опрос GET раз в 2 с, пока запуск идёт.
// Завершённый запуск — только снимок.
import { onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue'
import { getRun, runStreamUrl } from '@/api/agent'
import type { AgentRun, AgentStep } from '@/api/types'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { EMPTY_FEED, type FeedEvent, type FeedState, feedReducer, parseSse } from '@/lib/agentFeed'

export const POLL_MS = 2000
/** Сколько ошибок потока подряд (без единого события между ними) терпим до перехода на опрос. */
export const SSE_MAX_ERRORS = 3

export type FeedTransport = 'none' | 'sse' | 'poll'

export function useAgentRun(runId: Ref<string>) {
  const feed = shallowRef<FeedState>(EMPTY_FEED)
  const status = ref<'loading' | 'ready' | 'error'>('loading')
  const error = ref<ApiError | null>(null)
  const transport = ref<FeedTransport>('none')

  let es: EventSource | null = null
  let timer: number | null = null
  let ctrl: AbortController | null = null
  let disposed = false

  const apply = (ev: FeedEvent) => {
    feed.value = feedReducer(feed.value, ev)
    if (feed.value.done) finishLive()
  }

  function closeStream() {
    es?.close()
    es = null
  }

  function stopPolling() {
    if (timer !== null) window.clearTimeout(timer)
    timer = null
  }

  function finishLive() {
    const wasLive = transport.value !== 'none'
    closeStream()
    stopPolling()
    transport.value = 'none'
    // После done догружаем итог (стоимость, итоговая строка) одним снимком.
    if (wasLive) void snapshot(true)
  }

  async function snapshot(quiet = false): Promise<boolean> {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    if (!quiet) status.value = 'loading'
    try {
      const detail = await getRun(runId.value, { signal: c.signal })
      if (ctrl !== c || disposed) return false
      feed.value = feedReducer(feed.value, { type: 'snapshot', detail })
      status.value = 'ready'
      error.value = null
      return true
    } catch (e) {
      if (ctrl !== c || disposed) return false
      if (e instanceof DOMException && e.name === 'AbortError') return false
      error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
      // Ошибка опроса при уже показанной ленте не стирает её.
      if (!feed.value.run) status.value = 'error'
      return false
    }
  }

  function poll() {
    stopPolling()
    transport.value = 'poll'
    const tick = async () => {
      await snapshot(true)
      if (disposed || feed.value.done) {
        if (feed.value.done) transport.value = 'none'
        return
      }
      timer = window.setTimeout(tick, POLL_MS)
    }
    timer = window.setTimeout(tick, POLL_MS)
  }

  function stream() {
    if (typeof EventSource === 'undefined') return poll()
    closeStream()
    const source = new EventSource(runStreamUrl(runId.value))
    es = source
    transport.value = 'sse'
    let errors = 0
    source.onopen = () => (errors = 0)
    source.addEventListener('step', (e) => {
      errors = 0
      const step = parseSse<AgentStep>((e as MessageEvent<string>).data)
      if (step) apply({ type: 'step', step })
    })
    source.addEventListener('status', (e) => {
      errors = 0
      const run = parseSse<AgentRun>((e as MessageEvent<string>).data)
      if (run) apply({ type: 'status', run })
    })
    source.addEventListener('done', () => apply({ type: 'done' }))
    source.onerror = () => {
      if (es !== source) return
      errors += 1
      // Плановое закрытие потока сервером: браузер переподключится сам (readyState CONNECTING).
      if (source.readyState === EventSource.CONNECTING && errors < SSE_MAX_ERRORS) return
      // Поток закрыт или не поднимается — дальше опросом, лента не теряется.
      closeStream()
      if (!feed.value.done && !disposed) poll()
    }
  }

  async function open() {
    closeStream()
    stopPolling()
    transport.value = 'none'
    feed.value = EMPTY_FEED
    const ok = await snapshot()
    if (ok && !feed.value.done && !disposed) stream()
  }

  watch(runId, () => void open(), { immediate: true })

  onBeforeUnmount(() => {
    disposed = true
    ctrl?.abort()
    closeStream()
    stopPolling()
  })

  return { feed, status, error, transport, reload: open, refresh: () => snapshot(true) }
}
