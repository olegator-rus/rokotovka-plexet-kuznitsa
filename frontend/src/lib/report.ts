// Отчёт SearchOps для маркетолога (контур фида): подписи и раскрытие строки в ленту шагов
// агента. Без DOM: tests/report.test.ts.
import type { AgentStep } from '@/api/types'
import type { ReportGuideRow, ReportRow } from '@/api/typesReport'
import { rejectedHypotheses, stepView, type StepView } from './agentFeed.ts'

/** Происхождение запроса коротко — чип у строки. */
export const LAYER_SHORT: Record<string, string> = {
  customer: 'заказчик',
  site: 'сайт',
  synthetic: 'синтетика',
  live: 'живой поиск',
}

export const layerShort = (code: string) => LAYER_SHORT[code] ?? code

export interface RowStatus {
  label: string
  /** Класс тона бейджа (status-*), null — нейтральный. */
  tone: string | null
}

/** Что сделал агент по строке: предложение, задача, вывод кластера или «не разбирал». */
export function rowStatus(row: ReportRow): RowStatus {
  const p = row.proposal
  if (p) {
    const ticket = p.action === 'ticket'
    if (p.status === 'pending')
      return {
        label: ticket ? 'задача ждёт владельца' : 'правка ждёт утверждения',
        tone: 'status-pending',
      }
    if (p.status === 'accepted')
      return { label: ticket ? 'задача принята' : 'правка утверждена', tone: 'status-accepted' }
    if (p.status === 'rolled_back') return { label: 'правка откачена', tone: 'status-rejected' }
  }
  if (row.agent && row.agent.code !== 'unseen') return { label: row.agent.label, tone: null }
  return { label: 'агент не разбирал', tone: null }
}

/** Ход агента по строке есть: известен запуск и кластер. */
export function hasFeed(row: ReportRow): boolean {
  return !!(row.agent?.run_id && row.agent.cluster_id)
}

/** Адрес ленты запуска с прокруткой к кластеру строки. */
export function runLink(row: Pick<ReportRow, 'agent'>): string | null {
  const a = row.agent
  if (!a?.run_id) return null
  const q = a.cluster_id ? `?cluster=${encodeURIComponent(a.cluster_id)}` : ''
  return `/searchops/runs/${encodeURIComponent(a.run_id)}${q}`
}

/**
 * Лента строки: шаги кластера строки и шаг «читаю память» в начале запуска (кадр 2: агент прочитал
 * память, выбрал инструмент, получил ответ, выдвинул причину). Порядок — как в запуске.
 */
export function rowSteps(steps: AgentStep[], clusterId: string): StepView[] {
  const rejected = rejectedHypotheses(steps)
  const hypotheses = new Set<string>()
  for (const s of steps)
    if (s.kind === 'hypothesis' && s.cluster_id === clusterId && s.hypothesis_no != null)
      hypotheses.add(`${s.cluster_id}#${s.hypothesis_no}`)
  const picked = steps.filter(
    (s) => s.cluster_id === clusterId || (!s.cluster_id && s.kind === 'memory_read'),
  )
  picked.sort((a, b) => a.seq - b.seq)
  // Память читается один раз за запуск: в ленте строки — только первый такой шаг.
  let memory = false
  return picked
    .filter((s) => {
      if (s.cluster_id) return true
      if (memory) return false
      memory = true
      return true
    })
    .map((s) => stepView(s, rejected, hypotheses))
}

/** «12 неудачных из 16» — модельная частота. */
export function failLine(row: Pick<ReportRow, 'failures' | 'freq'>): string {
  return `${row.failures} из ${row.freq}`
}

/** Строка «вопрос подбору»: чего не хватило и что сделано. */
export function guideStatus(row: ReportGuideRow): RowStatus {
  const p = row.proposal
  if (p?.status === 'pending') return { label: 'задача ждёт владельца', tone: 'status-pending' }
  if (p?.status === 'accepted') return { label: 'задача принята', tone: 'status-accepted' }
  if (row.kb_missing) return { label: 'задачи ещё нет', tone: null }
  return { label: 'правкой базы не решается', tone: null }
}

/** Кнопка «Создать задачу каталогу»: только для причины kb_missing и только пока задачи нет. */
export const canCreateGapTask = (row: ReportGuideRow) => row.kb_missing && !row.proposal

/** Продукты вопроса одной строкой. */
export function guideProducts(row: ReportGuideRow): string {
  const t = row.families.map((f) => f.title || f.family_id).filter(Boolean)
  return t.length ? t.join(', ') : 'продукт не определён'
}
