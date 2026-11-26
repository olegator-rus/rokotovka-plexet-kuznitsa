// Контур выдачи: панель поиска. Поля §11.2 приходят вместе с прежним
// `groups`; нет поля `suggestions` — сервер контура коммерции (флаг FEATURE_PANEL выключен), панель работает по-старому.
import type { SuggestGroup } from './types'
import type { Companion } from './typesP2'

export interface PanelCorrection {
  from: string
  to: string
  kind: 'layout' | 'typo' | string
}

/** Бейдж-уточнение под строкой: с `field` — структурированное условие, без — дописывает слово в строку. */
export interface PanelRefinement {
  label: string
  query: string
  field: string | null
  value: string | number | boolean | null
  op: 'eq' | 'gte' | 'lte' | 'range' | 'not' | null
  hits: number
}

export interface PanelSuggestion {
  text: string
  /** Диапазоны [начало, конец) набранной части в `text`; остальное — предсказанное продолжение. */
  highlight?: [number, number][] | null
  kind: 'query' | 'category' | string
  source?: 'successful_query' | 'catalog_template' | string
  hits?: number
  /** Подсказка категории: id и название категории («антивирус — в Антивирусы для организаций»). */
  category_id?: string | null
  category_name?: string | null
}

export interface PanelCategory {
  id: string
  name: string
  share?: number
}

export interface PanelVendor {
  id: string
  name: string
}

export interface PanelProduct {
  family_id: string
  family_name: string
  product_id: string
  title: string
  kind: 'software' | 'hardware' | string
  vendor: string
  category: string
  mpn: string
  license_line: string
  specs_line: string
  price_from_rub: number | null
  price_mode: string
  variants_count: number
  exact: boolean
  sku_exact: boolean
  /** Необязательные поля оборудования: срок поставки и фото. */
  delivery_line?: string
  image_url?: string
}

export interface PanelMeta {
  ms?: { total?: number; suggestions?: number; products?: number; refinements?: number }
  partial?: boolean
  products_cached?: boolean
  llm_calls?: number
  embed_calls?: number
  rerank_calls?: number
  index_version?: number
  catalog_version?: string
}

/** «Вместе с этим» (§16.8): дополнение к первому семейству панели из таблицы классов; дат нет. */
export interface PanelTogether {
  /** К какому семейству подсказки — первое в `products`. */
  anchor_family_id: string
  family_id: string
  family_name: string
  product_id: string
  title: string
  vendor: string
  price_rub: number | null
  /** Причина из таблицы классов; у A — плюс документ. */
  reason: string
  level: 'A' | 'B' | string
  kind: 'class_logic' | 'compat_document' | string
  /** Название класса дополнения («Антивирус для организаций»). */
  label: string
  offer_url?: string
  family_url?: string
  source_url?: string
}

/** GET /api/suggest (§11.2). Все поля контура выдачи необязательны: старый сервер отдаёт только `groups`. */
export interface PanelSuggestResponse {
  groups?: SuggestGroup[]
  q?: string
  normalized?: string
  mode?: string
  corrected?: PanelCorrection | null
  refinements?: PanelRefinement[]
  suggestions?: PanelSuggestion[]
  categories?: PanelCategory[]
  vendors?: PanelVendor[]
  products?: PanelProduct[]
  total_hint?: number | null
  meta?: PanelMeta
  /** §16.8: 0–2 дополнения к первому семейству `products`. */
  together?: PanelTogether[]
}

export type RecsBasis = 'spec' | 'exact_sku' | 'viewed' | 'category_popular'

/** GET /api/suggest/recommendations (§11.3); `null` — оснований нет, блок скрыт. */
export interface PanelRecs {
  basis: RecsBasis | string
  title: string
  why: string
  items: Companion[]
}

/** События панели (§11.1), все с `intent_id`. */
export type P3EventType =
  | 'search_opened'
  | 'suggest_impression'
  | 'suggestion_select'
  | 'instant_product_click'
  | 'category_select'
  | 'vendor_select'
  | 'refinement_apply'
  | 'refinement_remove'
  | 'search_commit'
  | 'search_abandon'
  | 'recs_impression'
  | 'recs_click'
  | 'recs_add'
  | 'history_clear'
