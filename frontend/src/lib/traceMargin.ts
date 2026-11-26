// Шаг трассы `margin`: сдвиги «позиция 4 → 3: маржа» и сработавшие защиты.
// Числа маржи приходят только в трассе бэк-офиса; на витрине шага нет. Без DOM: tests/traceMargin.test.ts.
import type { TraceStep } from '@/api/types'
import { formatDecimal, formatInt, plural } from './format.ts'
import { guardLabel } from './margin.ts'

export interface MarginShift {
  product_id: string
  from: number
  to: number
}

function shiftsOf(step: TraceStep): MarginShift[] {
  const raw = Array.isArray(step.shifts) ? step.shifts : []
  return raw.filter(
    (s): s is MarginShift =>
      !!s &&
      typeof s === 'object' &&
      typeof (s as MarginShift).from === 'number' &&
      typeof (s as MarginShift).to === 'number',
  )
}

function guardsOf(step: TraceStep): string[] {
  return Array.isArray(step.guards)
    ? step.guards.filter((g): g is string => typeof g === 'string')
    : []
}

/** Итог шага одной строкой: «2 сдвига · 1 защита · вес 0,05». */
export function marginSummary(step: TraceStep): string {
  const n = shiftsOf(step).length
  const g = guardsOf(step).length
  const parts = [n ? `${formatInt(n)} ${plural(n, ['сдвиг', 'сдвига', 'сдвигов'])}` : 'без сдвигов']
  if (g) parts.push(`${formatInt(g)} ${plural(g, ['защита', 'защиты', 'защит'])}`)
  if (typeof step.weight === 'number') parts.push(`вес ${formatDecimal(step.weight, 2)}`)
  return parts.join(' · ')
}

/** Строки сдвигов: «позиция 4 → 3: маржа» с названием товара. */
export function marginShiftLines(
  step: TraceStep,
  titles: Record<string, string>,
): { title: string; text: string; up: boolean }[] {
  return shiftsOf(step).map((s) => ({
    title: titles[s.product_id] ?? s.product_id,
    text: `позиция ${s.from} → ${s.to}: маржа`,
    up: s.to < s.from,
  }))
}

export const marginGuardLines = (step: TraceStep) => guardsOf(step).map(guardLabel)

/** Версия политики: «политика v3». */
export const marginPolicyLabel = (step: TraceStep) =>
  typeof step.policy_version === 'number' ? `политика v${step.policy_version}` : null

/** Поля шага margin, которые показывает свой блок (не уходят в «ключ: значение»). */
export const MARGIN_STEP_FIELDS = ['policy_version', 'weight', 'shifts', 'guards']
