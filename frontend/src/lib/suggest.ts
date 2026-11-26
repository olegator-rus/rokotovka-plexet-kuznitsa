// Подсказки строки поиска. Без DOM: tests/suggest.test.ts.
import type { SuggestGroup, SuggestItem, SuggestResponse } from '@/api/types'

export const SUGGEST_MIN_CHARS = 2
export const SUGGEST_MAX_ROWS = 8

export interface SuggestRow extends SuggestItem {
  /** Уникальный ключ строки в списке: тип группы + порядковый номер. */
  key: string
  type: SuggestGroup['type']
  /** Позиция строки в списке подсказок, с 1 — уходит в событие suggest_pick. */
  position: number
}

export interface SuggestRowGroup {
  type: SuggestGroup['type']
  label: string
  rows: SuggestRow[]
}

const DEFAULT_LABELS: Record<SuggestGroup['type'], string> = {
  query: 'Запросы',
  product: 'Товары',
  vendor: 'Вендоры',
  category: 'Категории',
}

/** Группы ответа → строки для списка: пустые группы убраны, всего не больше max строк. */
export function toSuggestGroups(
  resp: SuggestResponse | null | undefined,
  max = SUGGEST_MAX_ROWS,
): SuggestRowGroup[] {
  const out: SuggestRowGroup[] = []
  let position = 0
  for (const g of resp?.groups ?? []) {
    if (position >= max) break
    const rows: SuggestRow[] = []
    for (const item of g.items ?? []) {
      if (position >= max) break
      if (!item?.text) continue
      position += 1
      rows.push({ ...item, key: `${g.type}:${position}`, type: g.type, position })
    }
    if (rows.length) out.push({ type: g.type, label: g.label || DEFAULT_LABELS[g.type], rows })
  }
  return out
}

export interface TextPart {
  text: string
  match: boolean
}

/** Текст подсказки на части: совпавшая выделяется начертанием (не цветом). */
export function highlightParts(text: string, range: SuggestItem['highlight']): TextPart[] {
  if (!range || range.length !== 2) return [{ text, match: false }]
  const start = Math.max(0, Math.min(text.length, range[0]))
  const end = Math.max(start, Math.min(text.length, range[1]))
  if (start === end) return [{ text, match: false }]
  return [
    { text: text.slice(0, start), match: false },
    { text: text.slice(start, end), match: true },
    { text: text.slice(end), match: false },
  ].filter((p) => p.text.length > 0)
}

/** Просить ли подсказки: от двух значимых символов. */
export function shouldSuggest(q: string): boolean {
  return q.trim().length >= SUGGEST_MIN_CHARS
}
