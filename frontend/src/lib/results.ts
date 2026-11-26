// Раскладка ответа поиска по блокам экрана. Без DOM: tests/results.test.ts.
// Позиция товара — его место в ответе сервера (с 1); она же уходит в события click/add_to_cart.
import type { SearchItem, SearchResponse } from '@/api/types'

export interface PositionedItem {
  item: SearchItem
  position: number
}

export interface ResultBlocks {
  /** Точные и подходящие товары. */
  main: PositionedItem[]
  /** Аналоги — отдельным блоком под выдачей (или «Похожие по назначению» при пустом результате). */
  analogs: PositionedItem[]
  /** Точного товара нет (F4): заголовок «Товара “…” нет в каталоге». */
  absent: boolean
  /** Ничего не найдено совсем: ни товаров, ни аналогов. */
  nothing: boolean
}

export function splitResults(resp: SearchResponse | null | undefined): ResultBlocks {
  const items = resp?.items ?? []
  const positioned = items.map((item, i) => ({ item, position: i + 1 }))
  const main = positioned.filter((p) => p.item.group !== 'analog')
  const analogs = positioned.filter((p) => p.item.group === 'analog')
  return {
    main,
    analogs,
    absent: Boolean(resp?.empty),
    nothing: items.length === 0,
  }
}

/** Заголовок пустого результата: текст сервера или общий по запросу. */
export function emptyTitle(resp: Pick<SearchResponse, 'empty' | 'query'>): string {
  if (resp.empty?.message) return resp.empty.message
  return `Товара «${resp.query}» нет в каталоге`
}
