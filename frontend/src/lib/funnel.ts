// Воронка поиска: поиски → клики → корзины → заказы с долей перехода.
// Числа шагов считает сервер; здесь только доли между ними. Без DOM: tests/funnel.test.ts.

export interface FunnelInput {
  key: string
  label: string
  count: number
}

export interface FunnelStep extends FunnelInput {
  /** Доля от предыдущего шага; у первого — null. */
  fromPrev: number | null
  /** Доля от первого шага (ширина полосы). */
  fromFirst: number
}

export function funnelSteps(steps: readonly FunnelInput[]): FunnelStep[] {
  const first = steps[0]?.count ?? 0
  return steps.map((s, i) => {
    const prev = i > 0 ? steps[i - 1].count : null
    return {
      ...s,
      fromPrev: prev === null ? null : prev > 0 ? s.count / prev : 0,
      fromFirst: first > 0 ? s.count / first : 0,
    }
  })
}

/** Доля числителя: делитель 0 — null (честное «нет данных», а не 0 %). */
export function safeShare(numerator: number, denominator: number): number | null {
  if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator <= 0) return null
  return numerator / denominator
}
