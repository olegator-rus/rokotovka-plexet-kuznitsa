// Контур подбора (SPEC §6): режим подбора в поиске — POST /api/guide. Модель возвращает идентификаторы, код сервера
// собирает из них фразу, таблицу, офферы и условия; фронт только рисует.

export type GuideType = 'comparison' | 'selection' | 'clarification' | 'no_data' | 'out_of_scope'

/** Источник факта: страница вендора и фрагмент с неё. Дату в интерфейсе не показываем. */
export interface GuideSource {
  title: string
  url: string
  fragment: string
}

/** Утверждение о свойстве продукта; status — состояние проверки факта («verified» — проверен человеком). */
export interface GuideClaim {
  fact_id: string
  text: string
  source: GuideSource | null
  status: string
}

export interface GuideColumn {
  family_id: string
  title: string
  family_url: string
}

/** yes — есть · no — нет по данным вендора · value — значение · unknown — нет сведений. */
export type GuideCellKind = 'yes' | 'no' | 'value' | 'unknown'

export interface GuideCell {
  kind: GuideCellKind | string
  value?: string | number | null
  fact_id?: string | null
}

export interface GuideRow {
  key: string
  label: string
  /** feature — строка о функции (со значком источника), commercial — цена, единица, срок. */
  kind?: 'feature' | 'commercial' | string
  cells: GuideCell[]
  /** Пояснение к строке («цена за год не считается для бессрочной»). */
  note?: string
}

export interface GuideComparison {
  columns: GuideColumn[]
  rows: GuideRow[]
}

export interface GuideOffer {
  offer_id: string
  title: string
  sku: string
  /** Ступень количества («50–99»). */
  tier: string
  /** Единица продажи («компьютер», «пакет на 5 мест»). */
  unit: string
  price_per_unit: number | null
  total: number | null
  /** fixed | from | quote_required | unavailable — как price_mode выдачи. */
  price_state: string
  offer_url: string
  /** Сколько единиц в итоге; если сервер не прислал — из total / price_per_unit или условия «сколько». */
  quantity?: number | null
  family_id?: string
  family_title?: string
  /** Срок лицензии («1 год», «бессрочная»). */
  term?: string
  /** match — подходит, alternative — альтернатива, rejected — «Не подходит» (причина в note). */
  role?: 'match' | 'alternative' | 'rejected' | string
  /** Пояснение сервера к офферу (почему подходит или нет). */
  note?: string
  rule_id?: string
  rule_source?: string
  /** Продаётся пакетом: price_per_unit — цена за место, package_price — цена пакета на package_size. */
  package_price?: number | null
  package_size?: number | null
}

/** Рекомендованная карточка: в offers идёт первой; reason — причина с числом из каталога. */
export interface GuideRecommendation {
  offer_id: string
  reason: string
  /** feature — есть нужная функция, named — названная редакция, price_per_seat — дешевле за место в год. */
  basis: 'feature' | 'named' | 'price_per_seat' | string
}

/** Чип вопроса: объект ядра {label, key, value} или строка. */
export type GuideChip = string | { label: string; key?: string; value?: string }

export interface GuideClarification {
  /** Какое условие выясняет вопрос (count, object, level…). */
  key?: string
  question: string
  chips: GuideChip[]
  /** Вопрос о праве использования: без ответа результат предварительный (SPEC §3). */
  required?: boolean
}

export type GuideOrigin = 'user' | 'default' | 'inferred'

export interface GuideCondition {
  key: string
  label: string
  value: string
  origin: GuideOrigin | string
}

export interface GuideChange {
  condition: string
  effect: string
}

export interface GuideTraceStep {
  step: string
  tool?: string
  ms?: number
  /** Итог шага («семейств: 2», «подходят: 2»). */
  result?: string
}

export interface GuideResponse {
  type: GuideType | string
  lead: string
  claims: GuideClaim[]
  comparison: GuideComparison | null
  offers: GuideOffer[]
  clarification: GuideClarification | null
  conditions: GuideCondition[]
  changes: GuideChange[]
  next: string[]
  trace: GuideTraceStep[]
  /** Состояние подбора — непрозрачно для фронта (кроме selection_id), возвращается серверу следующим ходом. */
  state: unknown
  /** Результат предварительный: не отвечен вопрос о праве использования. */
  preliminary?: boolean
  /** Служебная пометка сервера для покупателя (отказ, предел хода). */
  notice?: string
  gaps?: string[]
  /** Рекомендация среди подходящих; null — одна карточка или нет цен. */
  recommendation?: GuideRecommendation | null
  meta?: GuideMeta
}

/** Телеметрия хода (SPEC §13); на экране не показывается. */
export interface GuideMeta {
  turn_id?: string
  llm_calls?: number
  ms_first?: number
  ms_checked?: number
  ms_total?: number
  fallback?: string
  removed?: number
}

/** POST /api/guide/route — состояние строки (SPEC §1). */
export interface GuideRoute {
  mode: 'search' | 'search_offer' | 'guide' | string
  reason?: string
  /** Текст строки-предложения над выдачей. */
  offer?: string
}

/** POST /api/guide/spec — строка подбора в черновик; цена перечитана из каталога. */
export interface GuideSpecRequest {
  session_id: string
  selection_id: string
  version: number
  offer_id: string
  quantity: number
  op_key: string
}

export interface GuideSpecLine {
  offer_id: string
  family_id?: string
  title: string
  sku: string
  tier: string
  quantity: number
  price_per_unit: number | null
  total: number | null
  price_state: string
  offer_url: string
}

export interface GuideSpec {
  version: number
  lines: GuideSpecLine[]
  total_rub: number | null
  on_request: number
  unresolved: string[]
  duplicate: boolean
}

/** Изменение условия чипом: value null — убрать. Расширение контракта, см. отчёт. */
export interface GuideConditionEdit {
  key: string
  value: string | null
}

/** Ответ чипом: какое условие и какое значение. */
export interface GuideChipAnswer {
  key: string
  value: string
}

export interface GuideRequest {
  session_id: string
  query: string
  message?: string
  chip?: GuideChipAnswer
  condition?: GuideConditionEdit
  state?: unknown
}
