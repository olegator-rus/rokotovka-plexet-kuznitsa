// Разложение скора товара на четыре сегмента полосы. Без DOM: tests/score.test.ts.
// Ширина сегмента — доля части в сумме положительных частей; подписи и числа — исходные значения
// сервера, без пересчёта.
import type { ScoreParts } from '@/api/types'

export type ScorePartKey = keyof ScoreParts

export interface ScoreSegment {
  key: ScorePartKey
  label: string
  value: number
  /** Доля ширины полосы, 0..100, сумма по сегментам — 100 (или 0, если все части нулевые). */
  percent: number
}

export const SCORE_PARTS: readonly { key: ScorePartKey; label: string }[] = [
  { key: 'relevance', label: 'Релевантность' },
  { key: 'popularity', label: 'Популярность' },
  { key: 'margin', label: 'Маржа' },
  { key: 'context', label: 'Контекст' },
]

function safe(n: unknown): number {
  return typeof n === 'number' && Number.isFinite(n) ? n : 0
}

export function scoreSegments(parts: Partial<ScoreParts> | null | undefined): ScoreSegment[] {
  const values = SCORE_PARTS.map((p) => safe(parts?.[p.key]))
  const total = values.reduce((sum, v) => sum + Math.max(v, 0), 0)
  return SCORE_PARTS.map((p, i) => ({
    key: p.key,
    label: p.label,
    value: values[i],
    percent: total > 0 ? (Math.max(values[i], 0) / total) * 100 : 0,
  }))
}

/** Сумма частей — для сверки с `final` в подписи («сумма частей 0,82»). */
export function scoreSum(parts: Partial<ScoreParts> | null | undefined): number {
  return SCORE_PARTS.reduce((sum, p) => sum + safe(parts?.[p.key]), 0)
}
