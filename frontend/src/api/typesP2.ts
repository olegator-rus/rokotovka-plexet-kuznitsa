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
  /** §16.7 (фид): основание рекомендации — уровень A (документ вендора), B (каталог), C (не подтверждено). */
  evidence?: CompanionEvidence | null
  offer_url?: string | null
  family_url?: string | null
  promos?: Promo[] | null
}

export type EvidenceLevel = 'A' | 'B' | 'C'

export interface CompanionEvidence {
  level: EvidenceLevel | string
  kind: 'compat_document' | 'license_kind' | 'sku_platform' | 'vendor_ecosystem' | string
  title: string
  url: string
  doc_number: string
}

/** §16.7: действующая акция у позиции — «Акция: …, условия по ссылке»; срок не приходит и не выводится. */
export interface Promo {
  id: string
  title: string
  text: string
  url: string
}

export interface CompanionBlock {
  /** §16.7: заголовок блока (настройка UPSELL_BLOCK_TITLE); нет — «Дополните решение». */
  title?: string | null
  anchor: { product_id: string; family_id: string; confidence: 'high' | 'medium' | string }
  items: Companion[]
}

export interface FacetValue {
  value: string | number | boolean
  count: number
  /** Контур выдачи (§11.5): подпись значения лицензионного фасета («Продление», «1 год»). */
  label?: string
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
  /** Контур выдачи: страница результатов (§11.5). */
  results_v3?: boolean
}

/** Условие слота на позицию другого слота («ОС, если ноутбук без ОС»). */
export interface SlotCondition {
  slot: string
  key: string
  value: string
  /** Позиции слота `slot` (выбранная и альтернативы), при которых слот нужен. */
  product_ids: string[]
}

/** Слот решения: выбранная позиция, «заменить» и количество для каждой из них. */
export interface SolutionSlot {
  id: string
  label: string
  category_id: string
  query: string
  quantity_strategy: string
  /** Сколько штук выбранной позиции заказать (фасовка на N мест — ⌈N / мест в позиции⌉). */
  quantity: number
  item: SearchItemLite | null
  alternatives: SearchItemLite[]
  /** Количество для выбранной позиции и каждой альтернативы. */
  quantities: Record<string, number>
  /** Слот входит в решение при выбранных по умолчанию позициях. */
  active: boolean
  condition: SlotCondition | null
}

/** Позиция слота — поля позиции выдачи, нужные карточке и спецификации. */
export interface SearchItemLite {
  product_id: string
  family_id: string
  title: string
  vendor: string
  category: string
  license_line: string
  price_rub: number | null
  price_on_request: boolean
  kind?: ItemKind | string
  price_mode?: PriceMode | string
}

/** закупка под задачу по утверждённому шаблону. */
export interface Solution {
  template_id: string
  title: string
  /** Число мест из запроса (нет — 1). */
  seats: number
  /** «Собрано по шаблону Softline». */
  source: string
  slots: SolutionSlot[]
  /** Сумма по позициям по умолчанию — только если известны все цены. */
  total_rub: number | null
  on_request: number
}

/** «Старшая редакция» к первой позиции выдачи. */
export interface Upgrade extends Companion {
  for_product_id: string
  relation_id: string
}

/** GET /api/companions?product_id=&quantity= — «К этой позиции» в спецификации. */
export interface ProductCompanions {
  product_id: string
  required: Companion[]
  items: Companion[]
  upgrade: Upgrade | null
}

export type QuoteOrigin = 'search' | 'companion' | 'template'

export interface QuoteItem {
  product_id: string
  quantity: number
  origin: QuoteOrigin
  companion_class?: string
  /** Контур доводки (§12.6): исходный запрос и параметры закупки позиции. */
  purchase?: {
    query: string
    deal?: string
    quantity?: number
    metric?: string
    duration_months?: number
    org_type?: string
    edition?: string
  }
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
