// Загрузка данных раздела: статус, данные, ошибка, повтор. Прошлый запрос отменяется новым.
// Ошибка при уже загруженных данных их не стирает: блок показывает последние удачные и сообщение.
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import type { RequestOptions } from '@/api/http'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'

export type ResourceStatus = 'idle' | 'loading' | 'ready' | 'error'

export function useResource<T>(fetcher: (opts: RequestOptions) => Promise<T>) {
  const status = ref<ResourceStatus>('idle')
  const data = shallowRef<T | null>(null)
  const error = ref<ApiError | null>(null)
  let ctrl: AbortController | null = null

  async function load(opts: { quiet?: boolean } = {}) {
    ctrl?.abort()
    const c = new AbortController()
    ctrl = c
    if (!opts.quiet) status.value = 'loading'
    try {
      const value = await fetcher({ signal: c.signal })
      if (ctrl !== c) return
      data.value = value
      error.value = null
      status.value = 'ready'
    } catch (e) {
      if (ctrl !== c) return
      if (e instanceof DOMException && e.name === 'AbortError') return
      error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
      status.value = 'error'
    }
  }

  onBeforeUnmount(() => ctrl?.abort())
  return { status, data, error, load }
}
