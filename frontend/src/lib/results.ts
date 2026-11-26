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
  /**
   * Дописанные фильтром категории или производителя из панели (§11.8, `filter_fill`): поиск их не нашёл — отдельный
   * блок «Ещё в категории …» под найденным, чтобы слабое не выдавалось за найденное.
   */
  fill: PositionedItem[]
  /** Аналоги — отдельным блоком под выдачей (или «Похожие по назначению» при пустом результате). */
  analogs: PositionedItem[]
  /** Точного товара нет: заголовок «Товара “…” нет в каталоге». */
  absent: boolean
  /** Ничего не найдено совсем: ни товаров, ни аналогов. */
  nothing: boolean
}

export function splitResults(resp: SearchResponse | null | undefined): ResultBlocks {
  const items = resp?.items ?? []
  const positioned = items.map((item, i) => ({ item, position: i + 1 }))
  const main = positioned.filter((p) => p.item.group !== 'analog' && !p.item.filter_fill)
  const fill = positioned.filter((p) => p.item.group !== 'analog' && p.item.filter_fill)
  const analogs = positioned.filter((p) => p.item.group === 'analog')
  return {
    main,
    fill,
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

/** Заголовок блока дописанных фильтром: производитель — «Ещё у производителя …», иначе категория позиций. */
export function fillTitle(fill: PositionedItem[], byVendor: boolean): string {
  const pick = (f: (it: SearchItem) => string) => {
    const vals = new Set(fill.map((p) => f(p.item)).filter(Boolean))
    return vals.size === 1 ? [...vals][0] : ''
  }
  if (byVendor) {
    const v = pick((it) => it.vendor)
    return v ? `Ещё у производителя «${v}»` : 'Ещё у выбранного производителя'
  }
  const c = pick((it) => it.category)
  return c ? `Ещё в категории «${c}»` : 'Ещё в выбранной категории'
}
