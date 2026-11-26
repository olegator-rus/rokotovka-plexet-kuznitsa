// Типы контура коммерции витрины Поля ответа контура коммерции
// сервер присылает всегда, но фронт переживает и старый ответ без них (типы допускают undefined).

export type ItemKind = 'software' | 'hardware'
export type PriceMode = 'fixed' | 'from' | 'quote_required' | 'customer_specific' | 'unavailable'
export type CompatState = 'verified' | 'conditional' | 'incompatible' | 'unknown'

export type CompanionClass =
  'conditional_required' | 'compatible' | 'complement' | 'upgrade' | 'renewal'

export type CompanionSource =
  'vendor_documentation' | 'catalog_rule' | 'expert' | 'co_purchase' | 'agent_proposal'

/** Совместимость позиции с целевой моделью при запросе «X для Y». */
export interface ItemCompatibility {
  target_model: string
  state: CompatState | string
  condition: string
}

/** Позиция блока «Дополните решение» или строки «условно обязательное» в карточке якоря. */
export interface Companion {
  product_id: string
  family_id: string
  title: string
  vendor: string
  category: string
  license_line: string
  price_rub: number | null
  price_mode: PriceMode | string
  class: CompanionClass | string
  compatibility: 'verified' | 'conditional' | 'unknown' | string
  /** Одна строка «почему вместе». */
  reason: string
  conditions: string[]
  evidence_level: string
  source: CompanionSource | string
  source_url: string
  /** Количество из запроса («на 50 пользователей» → 50); null — числа в запросе нет. */
  suggested_quantity: number | null
  /** «Проверьте необходимость», «Совместимо с …», «Дополняет решение», «Старшая редакция», «Продление и поддержка». */
  label: string
  upgrade_adds: string[]
}

export interface CompanionBlock {
  anchor: { product_id: string; family_id: string; confidence: 'high' | 'medium' | string }
  items: Companion[]
}

export interface FacetValue {
  value: string | number | boolean
  count: number
}

export interface Facet {
  key: string
  label: string
  unit: string
  type: 'number' | 'enum' | 'bool' | string
  min: number | null
  max: number | null
  values: FacetValue[]
}

export type FilterOp = 'eq' | 'gte' | 'lte' | 'range' | 'not'

/** Фасет в запросе POST /api/search (`filters`, до 20). */
export interface SearchFilter {
  key: string
  op: FilterOp
  value: string | number | boolean
  value2?: number | null
}

/** GET /api/meta → features: что включено флагами сервера. */
export interface Features {
  hardware: boolean
  companions: boolean
  margin_policy: boolean
  solutions: boolean
}

/** шаблон решения (фронт витрины пока не рисует). */
export interface Solution {
  template_id: string
  title: string
  slots: unknown[]
}

export type QuoteOrigin = 'search' | 'companion' | 'template'

export interface QuoteItem {
  product_id: string
  quantity: number
  origin: QuoteOrigin
  companion_class?: string
}

/** POST /api/quotes — запрос КП по спецификации. */
export interface QuoteRequest {
  session_id: string
  client_id: string | null
  items: QuoteItem[]
  comment?: string
}

export interface QuoteResponse {
  id: string
  status: string
  positions: number
  /** Только если известны все цены. */
  total_rub: number | null
  on_request: number
}

/** События витрины контура коммерции. */
export type P2EventType =
  | 'companion_impression'
  | 'companion_click'
  | 'companion_add'
  | 'companion_dismiss'
  | 'spec_remove'
  | 'template_apply'
  | 'quote_request'
  | 'facet_apply'
  // бэк-офис (p2-front-backoffice): рычаг маржи и решения по связям
  | 'margin_simulate'
  | 'margin_apply'
  | 'relation_decision'
