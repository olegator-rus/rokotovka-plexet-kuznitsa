// Контур фида: цель запроса, группы выдачи по ступени лестницы, цена карточки. Без DOM:
// tests/matchGroups.test.ts. Поля приходят по мере готовности сервера; без них экран работает как в контуре выдачи.
import type { MatchType, SearchItem, SearchResponse, SearchTarget } from '@/api/types'
import { formatPrice, isPositivePrice } from './format.ts'
import type { PositionedItem } from './results.ts'

/** Порядок ступеней на экране. */
export const MATCH_ORDER: readonly MatchType[] = [
  'exact',
  'variant',
  'related',
  'alternative',
  'fallback',
]

/** Заголовки групп — из */
export const MATCH_TITLES: Record<MatchType, string> = {
  exact: 'Точное совпадение',
  variant: 'Другие варианты лицензии',
  related: 'Похожие',
  alternative: 'Можно купить сейчас',
  fallback: 'Популярное в наличии',
}

/** Подпись у альтернативы: без «замена» и «аналог». */
export const ALTERNATIVE_NOTE =
  'Тот же класс решения, можно купить сейчас. Совместимость с вашими приложениями нужно проверить.'

/** Пояснение под заголовком блока fallback: это не результат поиска. */
export const FALLBACK_NOTE =
  'Это не результат поиска по запросу — популярные товары, которые можно купить сразу.'

export interface MatchGroup {
  type: MatchType
  /** Уникальный ключ группы на экране: тип, у продолжения группы после «Можно купить сейчас» — `<тип>-rest`. */
  key: string
  title: string
  items: PositionedItem[]
}

function isMatchType(v: unknown): v is MatchType {
  return typeof v === 'string' && (MATCH_ORDER as readonly string[]).includes(v)
}

/** Сервер уже размечает ступени: хотя бы у одной карточки известный match_type. */
export function hasMatchTypes(items: readonly Pick<SearchItem, 'match_type'>[] | null | undefined) {
  return (items ?? []).some((it) => isMatchType(it.match_type))
}

/**
 * Карточки по группам в порядке лестницы; внутри группы — порядок сервера. Карточка без ступени или с незнакомой
 * при размеченном ответе — в «Похожие»: ни выше точных, ни в блоке «не результат поиска». Пустые группы не выводятся.
 */
export function groupByMatchType(items: readonly PositionedItem[]): MatchGroup[] {
  const buckets = new Map<MatchType, PositionedItem[]>()
  for (const p of items) {
    const t = isMatchType(p.item.match_type) ? p.item.match_type : 'related'
    const list = buckets.get(t) ?? []
    list.push(p)
    buckets.set(t, list)
  }
  const groups: MatchGroup[] = MATCH_ORDER.filter((t) => buckets.has(t)).map((t) => ({
    type: t,
    key: t,
    title: MATCH_TITLES[t],
    items: buckets.get(t) ?? [],
  }))
  // : первая карточка «по запросу» — «Можно купить сейчас» сразу под ней, как в ответе сервера;
  // остальные карточки её группы — после блока, отдельной группой с тем же заголовком.
  const alt = groups.find((g) => g.type === 'alternative')
  const head = groups[0]
  if (!alt || !head || head === alt || !isOnRequest(head.items[0]?.item)) return groups
  const rest = groups.filter((g) => g !== head && g !== alt)
  const out: MatchGroup[] = [{ ...head, items: head.items.slice(0, 1) }, alt]
  if (head.items.length > 1)
    out.push({ ...head, key: `${head.type}-rest`, items: head.items.slice(1) })
  return [...out, ...rest]
}

/** Карточка без цены: «Цена по запросу» (состояние сервера, иначе нет положительной цены). */
function isOnRequest(item: SearchItem | undefined): boolean {
  if (!item) return false
  if (item.price_state) return item.price_state === 'on_request'
  return !isPositivePrice(item.price_from) && !isPositivePrice(item.price_rub)
}

/** Пояснение над «Популярное в наличии»: своё у сервера, иначе сообщение пустого результата. */
export function fallbackLead(
  resp: Pick<SearchResponse, 'fallback_note' | 'empty'> | null | undefined,
): string {
  return (resp?.fallback_note || resp?.empty?.message || '').trim()
}

// ── Цель ──

const TARGET_KINDS: Record<string, string> = {
  sku: 'Товар',
  family: 'Продукт',
  vendor: 'Производитель',
  vendor_category: 'Производитель и категория',
  category: 'Категория',
  service: 'Раздел магазина',
}

export interface TargetView {
  kind: string
  label: string
  url: string
  reason: string
  /**
   * link — ссылка на store.softline.ru; manager — сервисная цель без страницы («Помощь менеджера»): кнопка открывает
   * менеджера; none — категория без страницы на сайте (сервер присылает url пустым): заголовок и чипы производителей
   * под ним (`clarifications`) — они и есть кликабельная часть цели.
   */
  action: 'link' | 'manager' | 'none'
}

/** Только http(s): ссылка уходит в новую вкладку, `javascript:` и прочие схемы не пропускаются. */
export function safeExternalUrl(url: string | null | undefined): string {
  if (!url) return ''
  try {
    const u = new URL(url)
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : ''
  } catch {
    return ''
  }
}

/**
 * Подпись цели: семейство — его название; вендор — «Все продукты МТС Линк»; вендор + категория — как прислал
 * сервер («Киберпротект — резервное копирование»). Без названия — цели нет (null). Без ссылки цель остаётся только у
 * категории (§16.6: url пусто, чипы вендоров) и сервисного запроса (справки нет — менеджер); у товара, семейства и
 * вендора сервер ссылку присылает всегда, без неё цель не выводится.
 */
export function targetView(t: SearchTarget | null | undefined): TargetView | null {
  if (!t) return null
  const url = safeExternalUrl(t.url)
  const title = (t.title ?? '').trim()
  if (!title) return null
  let action: TargetView['action'] = 'link'
  if (!url) {
    if (t.type === 'category') action = 'none'
    else if (t.type === 'service') action = 'manager'
    else return null
  }
  const label =
    t.type === 'vendor' && !/^все\s+продукты/i.test(title) ? `Все продукты ${title}` : title
  return {
    kind: TARGET_KINDS[t.type] ?? 'Раздел магазина',
    label,
    url,
    reason: (t.reason ?? '').trim(),
    action,
  }
}

/** Ссылка «на сайте» у карточки: страница семейства, иначе оффера. */
export function storeLink(
  item: Pick<SearchItem, 'family_url' | 'offer_url'>,
): { url: string; kind: 'family' | 'offer' } | null {
  const fam = safeExternalUrl(item.family_url)
  if (fam) return { url: fam, kind: 'family' }
  const off = safeExternalUrl(item.offer_url)
  return off ? { url: off, kind: 'offer' } : null
}

// ── Цена карточки ──

export interface CardPrice {
  state: 'priced' | 'on_request'
  /** «от 155 ₽», «79 250 ₽» или «Цена по запросу». */
  label: string
  /** Зачёркнутая старая цена — только когда показана цена самого оффера. */
  old: string
  /** «Скидка 20%» — без даты: даты в интерфейсе не пишем. */
  discount: string
}

/** «Скидка N%» из процента; текст сервера сводится к тому же виду — дата и прочие хвосты отбрасываются. */
function discountLabel(pct: number | null | undefined): string {
  return typeof pct === 'number' && Number.isFinite(pct) && pct > 0
    ? `Скидка ${Math.round(pct)}%`
    : 'Скидка'
}

function percentIn(text: string): number | null {
  const m = /(\d+(?:[.,]\d+)?)\s*%/.exec(text)
  return m ? Number(m[1]?.replace(',', '.')) : null
}

function badgeText(item: SearchItem): string {
  for (const b of item.badges ?? []) {
    if (typeof b === 'string') {
      if (/скидк/i.test(b)) return discountLabel(percentIn(b))
      continue
    }
    const kind = (b.type ?? b.kind ?? '').toLowerCase()
    const text = b.label || b.text || ''
    if (kind ? kind !== 'discount' : !/скидк/i.test(text)) continue
    return discountLabel(b.percent ?? b.discount_pct ?? percentIn(text))
  }
  return ''
}

/**
 * Цена карточки: состояние сервера (`price_state`) главнее; без него — есть положительная цена или нет. Значение —
 * `price_from` сервера, у семейства с вариантами — `price_from_rub`, иначе цена позиции. 0 и null — «Цена по запросу».
 */
export function cardPrice(item: SearchItem, variantsCount: number): CardPrice {
  const none: CardPrice = { state: 'on_request', label: 'Цена по запросу', old: '', discount: '' }
  if (item.price_state === 'on_request') return none
  const own = !item.price_on_request && isPositivePrice(item.price_rub) ? item.price_rub : null
  let value: number | null = null
  let from = false
  if (isPositivePrice(item.price_from)) {
    value = item.price_from
    from = true
  } else if (variantsCount > 0 && isPositivePrice(item.price_from_rub)) {
    value = item.price_from_rub
    from = true
  } else if (own !== null) {
    value = own
  }
  if (value === null) return none
  const label = `${from ? 'от ' : ''}${formatPrice(value)}`
  const old = item.old_price_rub
  const showsOwn = own !== null && value === own
  const struck = showsOwn && isPositivePrice(old) && old > own ? formatPrice(old) : ''
  let discount = badgeText(item)
  if (!discount && struck && own !== null && isPositivePrice(old)) {
    const pct = Math.round(((old - own) / old) * 100)
    if (pct > 0) discount = discountLabel(pct)
  }
  return { state: 'priced', label, old: struck, discount }
}
