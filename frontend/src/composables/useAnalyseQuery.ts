// «Разобрать» (контур доводки S4, контур автономии кадр 3): запуск агента на одном запросе — POST /api/agent/runs {"query"}
// (триггер manual) и переход в ленту запуска. Тот же путь из «Запросов без ответа» и из трассы.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { startQueryRun } from '@/api/agent'
import { useToast } from '@/components/ui/useToast'
import { isApiError } from '@/lib/apiError'

/** Длина запроса для разбора — как на сервере. */
export const ANALYSE_MAX_LEN = 200

export function useAnalyseQuery() {
  const router = useRouter()
  const toast = useToast()
  /** Запрос, по которому сейчас стартует запуск (кнопка в строке показывает «Запускаем…»). */
  const busy = ref<string | null>(null)

  async function analyse(query: string) {
    const q = query.trim().slice(0, ANALYSE_MAX_LEN)
    if (!q || busy.value) return
    busy.value = q
    try {
      const { run_id } = await startQueryRun(q)
      await router.push(`/searchops/runs/${encodeURIComponent(run_id)}`)
    } catch (e) {
      toast.add({
        tone: 'error',
        title: `Разбор «${q}» не запустился`,
        detail: isApiError(e) ? e.message : undefined,
      })
    } finally {
      busy.value = null
    }
  }

  return { analyse, busy }
}
