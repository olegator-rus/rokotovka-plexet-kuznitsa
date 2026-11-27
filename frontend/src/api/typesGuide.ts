// Контур подбора (SPEC §6): режим подбора в поиске — POST /api/guide. Модель возвращает идентификаторы, код сервера
// собирает из них фразу, таблицу, офферы и условия; фронт только рисует.

export type GuideType =
  'comparison' | 'selection' | 'clarification' | 'no_data' | 'out_of_scope' | 'fallback'

/** Источник факта: страница вендора и фрагмент с неё. Дату в интерфейсе не показываем. */
export interface GuideSource {
  /** Подпись ссылки (контур экрана): «Документация вендора», «Правила лицензирования {вендор}». */
  label?: string
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
  // ── Поля экрана контура экрана: готовые тексты и числа, фронт не считает ──
  display?: GuideOfferDisplay | null
  /** Как продаётся: «за компьютер», «пакет на 20 компьютеров». */
  sale_unit?: string
  sale_unit_kind?: 'per_object' | 'package' | 'license' | string
  /** Цена одной единицы продажи из каталога. */
  unit_price?: number | null
  /** Сколько единиц продажи к оплате. */
  pay_quantity?: number
  /** Расчёт итога: «50 × 3 940,20 ₽»; '' — цены нет. */
  price_line?: string
  /** «3 940,20 ₽ за компьютер» или «Цена по запросу». */
  price_text?: string
  package?: GuideOfferPack | null
  reason?: GuideReason | null
}

/** Заголовок и подзаголовок лицензии (UI 6.1) и раскрытие «Сведения о лицензии». */
export interface GuideOfferDisplay {
  title: string
  subtitle: string
  details?: { label: string; value: string }[]
}

/** Пакет под задачу: text — «Пакет на 25 компьютеров: 5 мест в запасе». */
export interface GuideOfferPack {
  size: number
  packs: number
  seats: number
  spare: number
  text: string
}

/** Код причины и готовый текст («Не подходит для серверной ОС»). */
export interface GuideReason {
  code: string
  text: string
}

/** Текстовая ссылка на источник с дословным фрагментом (UI 6.6). */
export interface GuideScreenSource {
  label: string
  fragment: string
  url: string
}

/** only_valid · required_feature · lowest_total · vendor_named · none (UI 6.2). */
export type GuideBasis =
  'only_valid' | 'required_feature' | 'lowest_total' | 'vendor_named' | 'none'

export interface GuideScreenResult {
  basis: GuideBasis | string
  heading: string
  /** '' при basis = none. */
  offer_id: string
  reason: string
  source: GuideScreenSource | null
}

/** yes · no · unknown · conditional · value; text — готовый («Есть», «Нет сведений», «2 715,30 ₽»). */
export interface GuideScreenCell {
  mark: 'yes' | 'no' | 'unknown' | 'conditional' | 'value' | string
  text: string
  fact_id?: string
}

export interface GuideScreenRow {
  key: string
  label: string
  priority: number
  /** Функция из условий покупателя — видна всегда. */
  asked: boolean
  kind: 'feature' | 'commercial' | string
  cells: GuideScreenCell[]
}

export interface GuideScreenTable {
  title: string
  columns: { offer_id: string; family_id: string; title: string }[]
  rows: GuideScreenRow[]
  more_rows?: GuideScreenRow[]
  hidden_no_data?: number
  hidden_label?: string
  vendors_note?: string
  difference?: string
  source?: GuideScreenSource | null
}

export interface GuideTaskCostRow {
  offer_id: string
  title: string
  how_sold: string
  total: number | null
  total_text: string
  note: string
}

/** Полная стоимость одной задачи у нескольких вендоров (сценарий 4). */
export interface GuideTaskCost {
  title: string
  columns: string[]
  rows: GuideTaskCostRow[]
  note: string
  cheapest: string
}

export interface GuidePriceTier {
  tier: string
  unit_price: number
  text: string
}

export interface GuideChanged {
  text: string
  items?: string[]
  price_change?: {
    offer_id: string
    label: string
    from: GuidePriceTier
    to: GuidePriceTier
  } | null
}

/** Строка «При изменении условий». */
export interface GuideOther {
  offer_id: string
  title: string
  subtitle: string
  price_text: string
  reason: GuideReason | null
}

export interface GuideSummary {
  offer_id: string
  label: string
  total: number | null
  total_text: string
  formula: string
  /** add_to_spec · request_quote · choose. */
  action: 'add_to_spec' | 'request_quote' | 'choose' | string
  hint: string
}

/** Экран подбора контура экрана: всё, что показывает рабочая область, кроме полей карточек. */
export interface GuideScreen {
  title: string
  task: string
  result: GuideScreenResult | null
  /** Предвыбор для итоговой строки и «Выбрано»; '' — главной кнопки нет. */
  selected_offer_id: string
  options: string[]
  comparison: GuideScreenTable | null
  task_cost: GuideTaskCost | null
  changed: GuideChanged | null
  others: GuideOther[]
  summary: GuideSummary | null
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
  /** Документация вендора у ответа «нет сведений». */
  docs?: { title: string; url: string } | null
  /** Экран контура экрана; null — у out_of_scope, fallback и вопроса без вариантов. */
  screen?: GuideScreen | null
  /** Сообщение помощника блоками (контур помощника). */
  chat?: GuideChat | null
  meta?: GuideMeta
}

// ── Помощник по выбору (контур помощника): всё готово на сервере, фронт показывает ──

export type ChatStatus =
  | 'answer'
  | 'question'
  | 'no_offers'
  | 'no_data'
  | 'out_of_scope'
  | 'discount'
  | 'llm_failed'
  | 'failed'

export interface ChatCard {
  offer_id: string
  family_id: string
  badge: GuideReason | null
  title: string
  subtitle: string
  price_text: string
  total: number | null
  calc: string
  reason: string
  unsuitable: boolean
  price_state: string
  promo: boolean
  old_price_text: string
  note: string
  action: 'add_to_spec' | 'request_quote' | 'open_store' | string
  offer_url: string
  quantity: number
  details: { label: string; value: string }[]
}

export interface ChatGroup {
  key: 'match' | 'other' | 'rejected' | string
  title: string
  cards: ChatCard[]
}

export interface ChatTable {
  title: string
  columns: { offer_id: string; family_id: string; title: string }[]
  rows: { key: string; label: string; kind: string; cells: GuideScreenCell[] }[]
  source: GuideScreenSource | null
}

export interface ChatSuggestion {
  label: string
  message: string
  chip?: { label?: string; key: string; value: string }
}

export interface GuideChat {
  status: ChatStatus | string
  text: string[]
  warning: string
  comparison: ChatTable | null
  groups: ChatGroup[]
  more: { label: string; query: string; count: number } | null
  sources: GuideScreenSource[]
  complements: { title: string; reason: string; message: string }[]
  question: {
    key: string
    text: string
    chips: { label: string; key: string; value: string }[]
  } | null
  suggestions: ChatSuggestion[]
  conditions: { label: string; items: string[] }
  links: { kind: string; label: string; url: string }[]
  has_promo: boolean
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
  /** Раскрыть подбор сразу (контур экрана, UI §2). */
  open_now?: boolean
  open_reason?: 'ask' | 'compare' | 'use_limit' | ''
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
