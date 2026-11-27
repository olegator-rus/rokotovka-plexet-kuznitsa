// Сводка «На данных заказчика» (): подписи и строки из GET /api/evaluations/feed. Ничего не
// пересчитывает — только выбирает и подписывает числа сервера. Без DOM: проверяет tests/feedSummary.test.ts.
import type { FeedCostRun, FeedScore, FeedSummary } from '@/api/typesFeed'
import { formatInt, formatPercent } from './format.ts'

/** Конфигурации закрытой части на экране S6: BM25, гибрид, полный продукт. */
export const CLOSED_CONFIGS = ['BM25', 'B2', 'live'] as const

const SET_LABELS: Record<string, string> = {
  etalon: 'Эталоны заказчика',
  tz: 'Примеры из ТЗ',
  neighbors: 'Соседи эталонов (настройка)',
  closed: 'Отложенный набор (закрытая часть)',
  understanding: 'Набор понимания (настройка)',
  feed_regression: 'Регрессия агента',
  feed_tune: 'Настройка агента',
}

/** Подпись набора. */
export function setLabel(set: string): string {
  return SET_LABELS[set] ?? set
}

/** Подпись конфигурации из сводки; неизвестная — её id. */
export function configLabel(s: FeedSummary | null, id: string): string {
  return s?.configs.find((c) => c.id === id)?.label ?? id
}

/** «10 из 10». */
export function outOf(n: number, of: number): string {
  return `${formatInt(n)} из ${formatInt(of)}`
}

/** Доля как «95 %» или null, если считать не из чего. */
export function shareOrNull(v: number | null | undefined): string | null {
  return typeof v === 'number' && Number.isFinite(v) ? formatPercent(v, 0) : null
}

export interface ClosedCell {
  config: string
  label: string
  /** null — у конфигурации прогона закрытой части нет. */
  score: FeedScore | null
}

/** Три числа закрытой части по порядку BM25 → гибрид → полный продукт. */
export function closedCells(s: FeedSummary | null): ClosedCell[] {
  return CLOSED_CONFIGS.map((id) => ({
    config: id,
    label: configLabel(s, id),
    score: s?.closed.find((c) => c.config === id) ?? null,
  }))
}

/** Закрытая часть ещё не прогонялась ни в одной конфигурации. */
export function closedPending(s: FeedSummary | null): boolean {
  return !s || s.closed.length === 0
}

/** Прогон стоимости для плитки: тёплый (как у покупателя), иначе холодный. */
export function headlineCost(s: FeedSummary | null): FeedCostRun | null {
  return s?.cost?.warm ?? s?.cost?.cold ?? null
}

/** «холодный» | «тёплый». */
export function modeLabel(mode: string | undefined): string {
  if (mode === 'cold') return 'холодный'
  if (mode === 'warm') return 'тёплый'
  if (mode === 'journal') return 'журнал витрины'
  return mode ?? ''
}

/** Прогоны на фиде есть: эталоны, таблица или стоимость. */
export function hasFeedRuns(s: FeedSummary | null): boolean {
  return !!s && (s.table.length > 0 || s.etalon !== null || s.cost !== null)
}

/** Каталог — mock (прогоны до данных заказчика), по версии каталога строки прогона. */
export function isMockCatalog(version: string | null | undefined): boolean {
  return !!version && version.toLowerCase().startsWith('mock')
}

/** Подпись mock-прогонов на экранах прежних метрик. */
export const MOCK_RUN_LABEL = 'mock-каталог, до данных заказчика'

/** Короткий номер прогона без даты: «…abc123» (в run_id — время запуска). */
export function runTail(id: string | null | undefined): string {
  if (!id) return '—'
  return id.length > 6 ? `…${id.slice(-6)}` : id
}

/** Каталог прогона для подписи: mock — «mock-каталог, до данных заказчика», версия фида — «фид заказчика». */
export function catalogLabel(version: string | null | undefined): string {
  if (!version) return '—'
  if (isMockCatalog(version)) return MOCK_RUN_LABEL
  if (version.startsWith('softline-')) return 'фид заказчика'
  return version
}
