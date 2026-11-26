// Типы страницы результатов контура выдачи Поля приходят только при
// FEATURE_RESULTS_V3; без них страница работает как в контуре коммерции (типы допускают undefined).

export type ResultsSort = 'recommended' | 'relevance' | 'price_asc' | 'price_desc' | 'delivery'
export type ResultsGroup = 'family' | 'none'

/** Предложенное уточнение (бейдж панели §11.2 и чип «+» над выдачей). */
export interface Refinement {
  label: string
  query: string
  field: string | null
  value: string | null
  op: string | null
  hits: number
}

export interface ResultCounts {
  families: number
  /** Контур выдачи; в контракте контура фида вместо него `skus`. */
  variants?: number
  /** Контур фида. */
  skus?: number
  by_match_type?: Record<string, number>
}

export interface ResultSortInfo {
  value: ResultsSort | string
  note: string
}

/** Атрибуты карточки: одинаковый набор у карточек категории, нет значения — прочерк. */
export interface ItemAttrs {
  edition: string
  license_type: string
  license_metric: string
  license_quantity: number | null
  duration_months: number | null
  delivery: 'electronic' | 'oem' | 'physical' | ''
  in_reestr: boolean
}

export type ResultsEventType =
  | 'variant_expand'
  | 'filter_apply'
  | 'filter_remove'
  | 'filters_reset'
  | 'sort_change'
  | 'group_change'
  | 'refinement_apply'
  | 'refinement_remove'
