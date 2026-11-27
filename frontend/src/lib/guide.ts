// Режим подбора в поиске (контур подбора, SPEC §1): когда предлагать подбор, подписи происхождения условий, цена оффера,
// строка спецификации. Экран рабочей области — lib/guideView.ts (контур экрана). Без DOM: tests/guide.test.ts.
import type { SearchResponse } from '@/api/types'
import type {
  GuideChip,
  GuideCondition,
  GuideOffer,
  GuideResponse,
  GuideRoute,
  GuideSpecLine,
} from '@/api/typesGuide'
import { formatPrice, isPositivePrice } from './format.ts'
import type { SpecPurchase } from './procurement.ts'
import { clampQty, type SpecLine } from './spec.ts'

// ── Три состояния строки (SPEC §1) ──

export type GuideMode = 'search' | 'offer' | 'guide'

/**
 * Вопрос, на который отвечает подбор: «чем отличается…», «что взять для…», «подбери», «самый дешёвый …» (проверяемое
 * утверждение пересчитывает код среди подходящих, SPEC §7).
 */
const QUESTION =
  /(чем\s+(\S+\s+){0,3}отлича|в\s+ч[её]м\s+разниц|разниц[аы]\s+между|что\s+(лучше\s+)?(взять|выбрать|купить)\s+(для|на|под)|подбер|помоги(те)?\s+(выбрать|подобрать)|какую\s+(редакцию|лицензию)\s+(взять|выбрать)|^сравни|сам(ый|ая|ое|ую)\s+(дешёв|дешев|недорог))/i

/** Запрос задачей, а не названием: «защита для офиса», «решение по безопасности». */
const TASK = /(защит|безопасност|для\s+офис|для\s+компани|для\s+предприят|решени[ея]\s+(для|по))/i

/** Число мест в тексте: «на 50 пк», «30 компьютеров», «для трёх админов» не считается. */
const COUNT = /\b\d+\s*(пк|компьютер|мест|пользовател|сотрудник|рабоч|устройств|узл|сервер|лиценз)/i

const KNOWN_MODES: readonly string[] = ['search', 'offer', 'guide']

/**
 * Режим строки для текущей выдачи. Есть признак сервера (`response.guide.mode`) — он главный. Иначе по запросу
 * и цели поиска:
 * - вопрос («чем отличается…», «что взять для…», «подбери») — сразу подбор;
 * - ответ справки, сервисная цель, пустая выдача — обычный поиск;
 * - полные условия (тип покупки, количество и срок разобрал нормализатор) — обычный поиск;
 * - цель — артикул или семейство без числа мест и без задачи — обычный поиск («криптопро csp 5.0»);
 * - иначе — выдача и предложение «Помочь выбрать» (категория, производитель, задача, число мест без продукта).
 */
export function guideMode(
  query: string,
  resp: Pick<SearchResponse, 'guide' | 'target' | 'purchase' | 'help' | 'items' | 'chips'> | null,
): GuideMode {
  const server = resp?.guide?.mode
  if (server && KNOWN_MODES.includes(server)) return server as GuideMode
  const q = query.trim()
  if (!q) return 'search'
  if (QUESTION.test(q)) return 'guide'
  if (!resp) return 'search'
  if (resp.help?.mode === 'answer') return 'search'
  if (resp.target?.type === 'service') return 'search'
  if (!resp.items?.length) return 'search'
  // Выдача — оборудование («Комплектующие», «подобрать комп»): лицензию подбирать нечему.
  if (resp.items[0]?.kind === 'hardware') return 'search'
  const p = resp.purchase
  if (p?.deal && p.quantity && p.duration_months) return 'search'
  const counted =
    Boolean(p?.quantity) ||
    (resp.chips ?? []).some((c) => c.field === 'license_quantity') ||
    COUNT.test(q)
  const task = TASK.test(q)
  const t = resp.target?.type
  if ((t === 'sku' || t === 'family') && !counted && !task) return 'search'
  return 'offer'
}

/** Режим из POST /api/guide/route: search_offer — «Поиск + предложение»; неизвестное — null (решают правила). */
export function routeMode(
  r: Pick<GuideRoute, 'mode' | 'open_now'> | null | undefined,
): GuideMode | null {
  if (r?.open_now) return 'guide'
  if (r?.mode === 'search_offer') return 'offer'
  if (r?.mode === 'search' || r?.mode === 'guide') return r.mode
  return null
}

// ── Чипы и условия ──

export interface ChipView {
  label: string
  value: string
  /** Какое условие задаёт чип; нет у чипа — ключ вопроса. */
  key: string
}

export function chipView(c: GuideChip, questionKey = ''): ChipView {
  if (typeof c === 'string') return { label: c, value: c, key: questionKey }
  return { label: c.label, value: c.value ?? c.label, key: c.key || questionKey }
}

/** Подпись происхождения условия: введённое покупателем не подписывается. */
export function originLabel(origin: string): string {
  if (origin === 'default') return 'по умолчанию'
  if (origin === 'inferred') return 'по запросу'
  return ''
}

// ── Цена и оффер (SPEC §8, «0 ₽» — никогда) ──

export interface OfferPriceView {
  /** «3 940,20 ₽», «от 1 850 ₽» или «Цена по запросу». */
  unit: string
  /** «197 010 ₽» или '' — итога нет. */
  total: string
  /** Цена известна и складывается в итог. */
  known: boolean
  /** Пометка состояния цены: '' у обычной цены каталога. */
  state: string
}

/** Состояния цены «по запросу»: ядро пишет on_request, выдача — quote_required. */
const ON_REQUEST = new Set(['quote_required', 'on_request', 'customer_specific'])

export function offerPrice(
  o: Pick<GuideOffer, 'price_per_unit' | 'total' | 'price_state'>,
): OfferPriceView {
  const st = o.price_state
  if (st === 'unavailable')
    return { unit: 'Нет в наличии', total: '', known: false, state: 'нет в наличии' }
  const priced = isPositivePrice(o.price_per_unit) && !ON_REQUEST.has(st)
  if (!priced) return { unit: 'Цена по запросу', total: '', known: false, state: 'цена по запросу' }
  const from = st === 'from'
  const unit = `${from ? 'от ' : ''}${formatPrice(o.price_per_unit)}`
  const total = isPositivePrice(o.total) ? `${from ? 'от ' : ''}${formatPrice(o.total)}` : ''
  return { unit, total, known: true, state: from ? 'цена от' : '' }
}

/** Число из условия «сколько» (count, seats, quantity…). */
export function conditionCount(conds: readonly GuideCondition[] | null | undefined): number | null {
  // Ядро пишет «count:workstation», «count:server»: первое такое условие.
  const c = (conds ?? []).find((x) =>
    /^(count|seats|quantity|objects|workstations)(:|$)/.test(x.key),
  )
  const n = c ? Number(String(c.value).replace(/[^\d]/g, '')) : NaN
  return Number.isFinite(n) && n > 0 ? n : null
}

/** Сколько единиц оффера класть в спецификацию: поле сервера → итог / цена → условие «сколько» → 1. */
export function offerQuantity(o: GuideOffer, conds: readonly GuideCondition[] = []): number {
  if (typeof o.quantity === 'number' && o.quantity > 0) return clampQty(o.quantity)
  if (isPositivePrice(o.total) && isPositivePrice(o.price_per_unit)) {
    const n = o.total / o.price_per_unit
    if (Math.abs(n - Math.round(n)) < 0.01) return clampQty(Math.round(n))
  }
  return clampQty(conditionCount(conds) ?? 1)
}

/** Параметры закупки из условий подбора — уходят в заявку КП вместе со строкой. */
export function purchaseFromConditions(
  query: string,
  conds: readonly GuideCondition[] = [],
): SpecPurchase {
  const out: SpecPurchase = { query }
  const n = conditionCount(conds)
  if (n) out.quantity = n
  for (const c of conds) {
    const v = String(c.value ?? '')
    if (c.key === 'operation' && v) out.deal = v
    if (c.key === 'org_type' && v) out.org_type = v
    if (c.key === 'edition' && v) out.edition = v
    if (c.key === 'term_months') {
      const m = Number(v.replace(/[^\d]/g, ''))
      if (m > 0) out.duration_months = m
    }
  }
  return out
}

/** Подпись к цене: «за компьютер», «за лицензию»; падеж берётся у сервера, предлог не задваивается. */
export function perUnit(unit: string | null | undefined): string {
  const u = (unit ?? '').trim()
  if (!u) return ''
  return /^за\s/i.test(u) ? u : `за ${u}`
}

export type OfferRole = 'match' | 'alternative' | 'rejected'

/** Роль оффера: нет поля или неизвестное значение — подходящий. */
export function offerRole(o: Pick<GuideOffer, 'role'>): OfferRole {
  return o.role === 'alternative' || o.role === 'rejected' ? o.role : 'match'
}

/** Офферы ответа по ролям: подходящие, альтернативы, «Не подходит». */
export function splitOffers(offers: readonly GuideOffer[] | null | undefined) {
  const out: Record<OfferRole, GuideOffer[]> = { match: [], alternative: [], rejected: [] }
  for (const o of offers ?? []) out[offerRole(o)].push(o)
  return out
}

/** Строка подбора, к которой относится оффер («строка «серверы»»); "" — подбор в одну строку. */
function lineOf(o: Pick<GuideOffer, 'note'>): string {
  return /строка «[^»]+»/.exec(o.note ?? '')?.[0] ?? ''
}

/**
 * Что кладёт в спецификацию общая кнопка подбора: если подходящие офферы — разные строки одного подбора (D3:
 * рабочие станции и серверы), то все они; иначе первый подходящий или альтернатива.
 */
export function specOffers(offers: readonly GuideOffer[] | null | undefined): GuideOffer[] {
  const { match, alternative } = splitOffers(offers)
  const lines = new Set(match.map(lineOf))
  if (match.length > 1 && !lines.has('') && lines.size === match.length) return [...match]
  const first = match[0] ?? alternative[0]
  return first ? [first] : []
}

/** Строка спецификации из подобранного оффера: та же спецификация и тот же КП, что у выдачи. */
export function lineFromOffer(
  o: GuideOffer,
  conds: readonly GuideCondition[],
  query: string,
): SpecLine {
  const st = o.price_state
  const mode =
    st === 'from' || st === 'unavailable'
      ? st
      : ON_REQUEST.has(st) || !isPositivePrice(o.price_per_unit)
        ? 'quote_required'
        : 'fixed'
  const parts = [o.sku ? `арт. ${o.sku}` : '', o.tier ? `ступень ${o.tier}` : '', perUnit(o.unit)]
  return {
    product_id: o.offer_id,
    title: o.title,
    vendor: '',
    category: '',
    license_line: parts.filter(Boolean).join(' · '),
    price_rub: isPositivePrice(o.price_per_unit) ? o.price_per_unit : null,
    price_mode: mode,
    quantity: offerQuantity(o, conds),
    origin: 'search',
    purchase: purchaseFromConditions(query, conds),
  }
}

/** Оффер с ценой и количеством, перечитанными из каталога ответом POST /api/guide/spec. */
export function withSpecLine(o: GuideOffer, line: GuideSpecLine | null | undefined): GuideOffer {
  if (!line || line.offer_id !== o.offer_id) return o
  return {
    ...o,
    price_per_unit: line.price_per_unit,
    total: line.total,
    price_state: line.price_state || o.price_state,
    quantity: line.quantity > 0 ? line.quantity : o.quantity,
    sku: line.sku || o.sku,
    tier: line.tier || o.tier,
  }
}

/** Идентификатор подбора из непрозрачного состояния сервера. */
export function selectionId(state: unknown): string {
  if (state && typeof state === 'object') {
    const id = (state as Record<string, unknown>).selection_id
    if (typeof id === 'string') return id
  }
  return ''
}

// ── Ответ целиком ──

/** Отказ подбора: ниже обычная выдача и чипы. */
export function isRefusal(r: Pick<GuideResponse, 'type'> | null | undefined): boolean {
  return r?.type === 'no_data' || r?.type === 'out_of_scope'
}

// ── Название лицензии (запасное, если сервер не прислал display) ──

export interface OfferNameView {
  /** Семейство и редакция: «Kaspersky Endpoint Security для бизнеса Расширенный». */
  name: string
  /** Чем позиция отличается внутри семейства: «серверная лицензия СКЗИ»; '' — нечего добавить. */
  detail: string
}

const FAMILY_PREFIX = /^(программа|программное обеспечение)\s+/i
/** Хвосты названий фида, которые уже показаны ступенью, сроком и единицей. */
const DETAIL_NOISE = [
  /[.,]?\s*количество узлов\.?/gi,
  /\s*по количеству (узлов|рабочих станций\/файловых серверов|рабочих станций|пользователей)/gi,
]
const DETAIL_TERM = /\s*на\s+\d+\s+(год|года|лет|месяц|месяца|месяцев)(?![а-яё])/gi

function squash(s: string): string {
  return s.replace(/\s+/g, ' ').trim()
}

function tidyDetail(raw: string, hasTerm: boolean): string {
  let s = raw.replace(/^[\s:,.;–—-]+/, '')
  while (/^\([^)]*\)/.test(s)) s = s.replace(/^\([^)]*\)[\s:,.;–—-]*/, '')
  for (const re of DETAIL_NOISE) s = s.replace(re, '')
  if (hasTerm) s = s.replace(DETAIL_TERM, '')
  s = squash(s).replace(/[\s:,.;–—-]+$/, '')
  return /^лицензия$/i.test(s) ? '' : s
}

/**
 * Понятное название позиции из полей ответа: семейство с редакцией вместо сырой строки фида и короткое отличие
 * позиции. Сервер пока не присылает редакцию и вид лицензии отдельно (см. отчёт) — собираем из title и family_title.
 */
export function offerName(o: Pick<GuideOffer, 'title' | 'family_title' | 'term'>): OfferNameView {
  const title = squash(o.title ?? '')
  const fam = squash(o.family_title ?? '').replace(FAMILY_PREFIX, '')
  if (!fam) return { name: title, detail: '' }
  const at = title.toLowerCase().indexOf(fam.toLowerCase())
  if (at > 0) return { name: title, detail: '' }
  const rest = at === 0 ? title.slice(fam.length) : title
  return { name: fam, detail: tidyDetail(rest, Boolean(o.term)) }
}
