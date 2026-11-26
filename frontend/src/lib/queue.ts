// Порядок очереди SearchOps: по оценке приоритета, затем по потерянному спросу. Фронт ничего не
// считает сам: только порядок и подпись числа сервера. Проверка — tests/queue.test.ts.
import type { Proposal } from '@/api/types'
import { formatRub } from './format.ts'

type WithPriority = Proposal & { priority_score?: number | null }

const num = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)

/** Сначала предложения с priority_score по убыванию, затем по потерянному спросу, при равенстве — новые выше. */
export function sortQueue<T extends WithPriority>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const pa = num(a.priority_score) ? a.priority_score : null
    const pb = num(b.priority_score) ? b.priority_score : null
    if (pa !== null && pb !== null && pa !== pb) return pb - pa
    if (pa !== null && pb === null) return -1
    if (pa === null && pb !== null) return 1
    return (
      (b.cluster?.lost_demand ?? 0) - (a.cluster?.lost_demand ?? 0) ||
      String(b.created_at).localeCompare(String(a.created_at))
    )
  })
}

/**
 * Подпись порядка очереди: по чему она на самом деле упорядочена. Если ни у кого нет ни оценки, ни потерянного
 * спроса — «по убыванию спроса» ничего не значит: порядок по времени, новые сверху.
 */
export function queueOrderLabel(items: WithPriority[]): string {
  if (items.some((p) => num(p.priority_score)))
    return 'по оценке приоритета (не прогноз), затем по потерянному спросу'
  if (items.length > 1 && items.every((p) => !(p.cluster?.lost_demand ?? 0)))
    return 'новые сверху: потерянного спроса ни у одного предложения нет'
  return 'по убыванию потерянного спроса, при равенстве — новые сверху'
}

export const PRIORITY_CAPTION = 'оценка приоритета, не прогноз'

/** «69 942 ₽» — число оценки приоритета; нет — null. */
export const priorityValue = (p: Pick<WithPriority, 'priority_score'>) =>
  num(p.priority_score) ? formatRub(p.priority_score) : null
