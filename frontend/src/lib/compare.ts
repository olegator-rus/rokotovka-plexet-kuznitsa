// Вкладка «Один запрос»: две выдачи рядом. Товар только в одной колонке помечен,
// у общего — сдвиг позиции относительно соседней колонки. Без DOM: tests/compare.test.ts.

export interface CompareMark {
  /** Позиция в этой колонке, с 1. */
  position: number
  /** Позиция того же товара в соседней колонке; null — его там нет. */
  otherPosition: number | null
  /** true — товар есть только в этой колонке. */
  onlyHere: boolean
  /** На сколько позиций здесь выше, чем в соседней: +2 — выше на 2, −1 — ниже на 1, 0 — на том же месте. */
  shift: number | null
}

/** Разметка колонки `ids` относительно соседней `otherIds` (порядок — позиции в выдаче). */
export function compareColumn(ids: readonly string[], otherIds: readonly string[]): CompareMark[] {
  const other = new Map<string, number>()
  otherIds.forEach((id, i) => {
    if (!other.has(id)) other.set(id, i + 1)
  })
  return ids.map((id, i) => {
    const position = i + 1
    const otherPosition = other.get(id) ?? null
    return {
      position,
      otherPosition,
      onlyHere: otherPosition === null,
      shift: otherPosition === null ? null : otherPosition - position,
    }
  })
}

/** Подпись сдвига: «выше на 2», «ниже на 1», «на том же месте». */
export function shiftLabel(mark: CompareMark): string {
  if (mark.onlyHere || mark.shift === null) return 'только здесь'
  if (mark.shift === 0) return 'на том же месте'
  return mark.shift > 0 ? `выше на ${mark.shift}` : `ниже на ${Math.abs(mark.shift)}`
}

/** Сводка пары колонок: сколько общих и сколько только в каждой. */
export function compareSummary(left: readonly string[], right: readonly string[]) {
  const l = new Set(left)
  const r = new Set(right)
  let common = 0
  for (const id of l) if (r.has(id)) common += 1
  return { common, onlyLeft: l.size - common, onlyRight: r.size - common }
}
