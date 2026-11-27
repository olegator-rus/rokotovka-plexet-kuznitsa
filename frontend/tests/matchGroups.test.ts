// Контур фида: группы по match_type, подпись цели, цена карточки (src/lib/matchGroups.ts), нулевая цена в
// formatPrice / priceLabel / cardAction, флаг узкой витрины (src/lib/showcase.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItem, SearchResponse } from '../src/api/types.ts'
import { formatPrice, isPositivePrice } from '../src/lib/format.ts'
import { isPurchasable, priceLabel } from '../src/lib/license.ts'
import {
  ALTERNATIVE_NOTE,
  cardPrice,
  fallbackLead,
  groupByMatchType,
  hasMatchTypes,
  MATCH_TITLES,
  safeExternalUrl,
  storeLink,
  targetView,
} from '../src/lib/matchGroups.ts'
import { splitResults } from '../src/lib/results.ts'
import { cardAction, familyPriceLabel, resultsHeading } from '../src/lib/resultsView.ts'
import { showcaseParts } from '../src/lib/showcase.ts'

const NBSP = '\u00a0'

const item = (id: string, extra: Partial<SearchItem> = {}): SearchItem => ({
  product_id: id,
  family_id: id,
  title: id,
  vendor: 'v',
  category: 'c',
  license_line: '',
  price_rub: 100,
  price_on_request: false,
  group: 'exact',
  matches_query: true,
  purchased_before: false,
  final: 0.5,
  variants: [],
  ...extra,
})

const positioned = (items: SearchItem[]) => items.map((it, i) => ({ item: it, position: i + 1 }))

test('groupByMatchType: порядок лестницы, подписи, позиции из ответа', () => {
  const groups = groupByMatchType(
    positioned([
      item('f1', { match_type: 'fallback' }),
      item('e1', { match_type: 'exact' }),
      item('a1', { match_type: 'alternative' }),
      item('e2', { match_type: 'exact' }),
      item('v1', { match_type: 'variant' }),
      item('r1', { match_type: 'related' }),
    ]),
  )
  assert.deepEqual(
    groups.map((g) => g.type),
    ['exact', 'variant', 'related', 'alternative', 'fallback'],
  )
  assert.deepEqual(
    groups.map((g) => g.title),
    [
      'Точное совпадение',
      'Другие варианты лицензии',
      'Похожие',
      'Можно купить сейчас',
      'Популярное в наличии',
    ],
  )
  assert.deepEqual(
    groups[0]?.items.map((p) => [p.item.product_id, p.position]),
    [
      ['e1', 2],
      ['e2', 4],
    ],
  )
})

test('groupByMatchType: первая карточка «по запросу» — «Можно купить сейчас» сразу под ней (S4)', () => {
  const groups = groupByMatchType(
    positioned([
      item('ws', { match_type: 'exact', price_state: 'on_request', price_rub: null }),
      item('astra', { match_type: 'alternative' }),
      item('alt', { match_type: 'alternative' }),
      item('ws2', { match_type: 'exact', price_state: 'on_request', price_rub: null }),
      item('cal', { match_type: 'related' }),
    ]),
  )
  assert.deepEqual(
    groups.map((g) => [g.key, g.items.map((p) => p.item.product_id)]),
    [
      ['exact', ['ws']],
      ['alternative', ['astra', 'alt']],
      ['exact-rest', ['ws2']],
      ['related', ['cal']],
    ],
  )
  // Первая с ценой — порядок лестницы прежний.
  const priced = groupByMatchType(
    positioned([
      item('e', { match_type: 'exact', price_state: 'priced' }),
      item('a', { match_type: 'alternative' }),
      item('r', { match_type: 'related' }),
    ]),
  )
  assert.deepEqual(
    priced.map((g) => g.key),
    ['exact', 'related', 'alternative'],
  )
})

test('groupByMatchType: без ступени или с незнакомой — в «Похожие», пустых групп нет', () => {
  const groups = groupByMatchType(
    positioned([item('e', { match_type: 'exact' }), item('x'), item('y', { match_type: 'odd' })]),
  )
  assert.deepEqual(
    groups.map((g) => [g.type, g.items.length]),
    [
      ['exact', 1],
      ['related', 2],
    ],
  )
  assert.equal(MATCH_TITLES.related, 'Похожие')
})

test('hasMatchTypes: старый ответ без поля — раскладка контура выдачи', () => {
  assert.equal(hasMatchTypes([item('a'), item('b')]), false)
  assert.equal(hasMatchTypes([item('a', { match_type: null })]), false)
  assert.equal(hasMatchTypes([item('a', { match_type: 'exact' })]), true)
  assert.equal(hasMatchTypes(undefined), false)
})

test('группы + splitResults: аналог контура доводки с match_type попадает в свою ступень', () => {
  const resp = {
    query: 'q',
    items: [
      item('a', { match_type: 'exact' }),
      item('b', { group: 'analog', match_type: 'alternative' }),
    ],
    empty: null,
  } as unknown as SearchResponse
  const b = splitResults(resp)
  const groups = groupByMatchType([...b.main, ...b.analogs])
  assert.deepEqual(
    groups.map((g) => g.type),
    ['exact', 'alternative'],
  )
})

test('fallbackLead: своё пояснение сервера, иначе сообщение пустого результата, иначе пусто', () => {
  assert.equal(
    fallbackLead({ fallback_note: 'Не нашли «asdfgh». Возможно, вы искали…', empty: null }),
    'Не нашли «asdfgh». Возможно, вы искали…',
  )
  assert.equal(
    fallbackLead({ empty: { message: 'Не нашли «x»', show_manager: true } }),
    'Не нашли «x»',
  )
  assert.equal(fallbackLead({}), '')
  assert.equal(fallbackLead(null), '')
})

test('targetView: семейство, вендор, вендор + категория', () => {
  assert.deepEqual(
    targetView({
      type: 'family',
      url: 'https://store.softline.ru/astra/astra-linux-special-edition/',
      title: 'Astra Linux Special Edition',
      reason: 'одно явное базовое семейство',
    }),
    {
      kind: 'Продукт',
      label: 'Astra Linux Special Edition',
      url: 'https://store.softline.ru/astra/astra-linux-special-edition/',
      reason: 'одно явное базовое семейство',
      action: 'link',
    },
  )
  assert.equal(
    targetView({ type: 'vendor', url: 'https://store.softline.ru/webinar/', title: 'МТС Линк' })
      ?.label,
    'Все продукты МТС Линк',
  )
  assert.equal(
    targetView({
      type: 'vendor',
      url: 'https://store.softline.ru/webinar/',
      title: 'Все продукты МТС Линк',
    })?.label,
    'Все продукты МТС Линк',
  )
  const vc = targetView({
    type: 'vendor_category',
    url: 'https://store.softline.ru/cyberprotect/?filter[category][]=1033',
    title: 'Киберпротект — резервное копирование',
    reason: 'у вендора несколько равных семейств в одной категории',
  })
  assert.equal(vc?.label, 'Киберпротект — резервное копирование')
  assert.equal(vc?.kind, 'Производитель и категория')
  assert.equal(vc?.reason, 'у вендора несколько равных семейств в одной категории')
})

test('targetView: без цели, без ссылки, не http(s) — цели нет', () => {
  assert.equal(targetView(null), null)
  assert.equal(targetView(undefined), null)
  assert.equal(targetView({ type: 'family', url: '', title: 'X' }), null)
  assert.equal(targetView({ type: 'vendor', url: '', title: 'X' }), null)
  assert.equal(targetView({ type: 'family', url: 'javascript:alert(1)', title: 'X' }), null)
  assert.equal(targetView({ type: 'family', url: 'https://store.softline.ru/', title: ' ' }), null)
  assert.equal(
    targetView({ type: 'service', url: 'https://store.softline.ru/delivery/', title: 'Доставка' })
      ?.kind,
    'Раздел магазина',
  )
})

test('targetView: категория и сервис без ссылки — цель без перехода на сайт (живой ответ «сервер», «доставка»)', () => {
  const cat = targetView({
    type: 'category',
    url: '',
    title: 'Rack-серверы',
    reason: 'общее слово совпадает с категорией',
  })
  assert.equal(cat?.action, 'none')
  assert.equal(cat?.kind, 'Категория')
  assert.equal(cat?.label, 'Rack-серверы')
  const svc = targetView({
    type: 'service',
    url: '',
    title: 'Помощь менеджера',
    reason: 'запрос не о товаре',
  })
  assert.equal(svc?.action, 'manager')
  assert.equal(svc?.kind, 'Раздел магазина')
  assert.equal(targetView({ type: 'category', url: '', title: ' ' }), null)
})

test('safeExternalUrl и storeLink: страница семейства выше оффера', () => {
  assert.equal(safeExternalUrl('data:text/html,x'), '')
  assert.equal(safeExternalUrl('не ссылка'), '')
  assert.deepEqual(
    storeLink({
      family_url: 'https://store.softline.ru/f/',
      offer_url: 'https://store.softline.ru/o/',
    }),
    { url: 'https://store.softline.ru/f/', kind: 'family' },
  )
  assert.deepEqual(storeLink({ family_url: null, offer_url: 'https://store.softline.ru/o/' }), {
    url: 'https://store.softline.ru/o/',
    kind: 'offer',
  })
  assert.equal(storeLink({}), null)
})

test('formatPrice: 0, null, отрицательное — пусто, «0 ₽» не выводится', () => {
  assert.equal(formatPrice(0), '')
  assert.equal(formatPrice(null), '')
  assert.equal(formatPrice(undefined), '')
  assert.equal(formatPrice(-5), '')
  assert.equal(formatPrice(155), `155${NBSP}₽`)
  assert.equal(formatPrice(3070.8), `3${NBSP}070,80${NBSP}₽`)
  assert.equal(isPositivePrice(0), false)
  assert.equal(isPositivePrice(Number.NaN), false)
})

test('priceLabel, isPurchasable, cardAction: цена 0 — «Цена по запросу» и «Запросить цену»', () => {
  const zero = item('z', { price_rub: 0 })
  assert.equal(priceLabel(zero), 'Цена по запросу')
  assert.equal(isPurchasable(zero), false)
  assert.equal(cardAction(zero, 0), 'price_request')
  assert.equal(familyPriceLabel(zero, 0), 'Цена по запросу')
  assert.equal(
    familyPriceLabel(item('z2', { price_rub: 0, price_from_rub: 0 }), 3),
    'Цена по запросу',
  )
  const nul = item('n', { price_rub: null, price_on_request: true })
  assert.equal(priceLabel(nul), 'Цена по запросу')
  assert.equal(cardAction(nul, 0), 'price_request')
  // price_state сервера главнее: on_request при числе в price_rub и с вариантами — «Запросить цену».
  const onReq = item('o', { price_rub: 500, price_state: 'on_request' })
  assert.equal(isPurchasable(onReq), false)
  assert.equal(cardAction(onReq, 4), 'price_request')
  assert.equal(cardAction(item('p'), 0), 'spec')
  assert.equal(cardAction(item('p'), 2), 'choose')
})

test('cardPrice: priced «от N ₽», on_request, 0 и null', () => {
  assert.deepEqual(cardPrice(item('a', { price_state: 'priced', price_from: 155 }), 0), {
    state: 'priced',
    label: `от 155${NBSP}₽`,
    old: '',
    discount: '',
  })
  assert.equal(cardPrice(item('b', { price_from_rub: 4 }), 5).label, `от 4${NBSP}₽`)
  assert.equal(cardPrice(item('c', { price_rub: 79250 }), 0).label, `79${NBSP}250${NBSP}₽`)
  assert.equal(
    cardPrice(item('d', { price_state: 'on_request', price_from: 900 }), 0).label,
    'Цена по запросу',
  )
  const zero = cardPrice(item('e', { price_rub: 0, price_from: 0, price_from_rub: 0 }), 3)
  assert.deepEqual(zero, { state: 'on_request', label: 'Цена по запросу', old: '', discount: '' })
  assert.equal(
    cardPrice(item('f', { price_rub: null, price_on_request: true }), 0).state,
    'on_request',
  )
  // priced без положительной цены — всё равно «по запросу», не «0 ₽».
  assert.equal(
    cardPrice(item('g', { price_state: 'priced', price_rub: 0 }), 0).label,
    'Цена по запросу',
  )
})

test('cardPrice: скидка по old_price_rub — старая цена зачёркнута, бейдж «Скидка N%» без даты', () => {
  const p = cardPrice(item('a', { price_rub: 800, old_price_rub: 1000 }), 0)
  assert.equal(p.old, `1${NBSP}000${NBSP}₽`)
  assert.equal(p.label, `800${NBSP}₽`)
  assert.equal(p.discount, 'Скидка 20%')
  // Показана цена «от» семейства, а не самого оффера — старую цену не зачёркиваем.
  const fam = cardPrice(item('b', { price_rub: 800, old_price_rub: 1000, price_from_rub: 4 }), 3)
  assert.equal(fam.old, '')
  assert.equal(fam.discount, '')
  // Старая цена не выше текущей — не скидка.
  assert.equal(cardPrice(item('c', { price_rub: 800, old_price_rub: 800 }), 0).discount, '')
})

test('cardPrice: бейдж сервера строкой или объектом — «Скидка N%», дата отбрасывается', () => {
  assert.equal(
    cardPrice(item('a', { badges: ['Скидка 15%, цены на 27.05'] }), 0).discount,
    'Скидка 15%',
  )
  assert.equal(
    cardPrice(item('b', { badges: [{ type: 'discount', percent: 12.4 }] }), 0).discount,
    'Скидка 12%',
  )
  assert.equal(
    cardPrice(item('c', { badges: [{ type: 'discount', label: 'Скидка 30% до 01.06' }] }), 0)
      .discount,
    'Скидка 30%',
  )
  assert.equal(cardPrice(item('d', { badges: ['Скидка'] }), 0).discount, 'Скидка')
  assert.equal(
    cardPrice(item('e', { badges: [{ type: 'new', label: 'Новинка' }] }), 0).discount,
    '',
  )
})

test('тексты цены и подписи альтернативы без дат', () => {
  const texts = [
    cardPrice(item('a', { price_rub: 800, old_price_rub: 1000 }), 0).discount,
    cardPrice(item('b', { badges: ['Скидка 15%, цены на 27.05'] }), 0).discount,
    ALTERNATIVE_NOTE,
  ]
  for (const t of texts) assert.doesNotMatch(t, /\d{1,2}\.\d{2}|цены на/)
})

test('resultsHeading: counts контура фида со skus вместо variants', () => {
  assert.equal(
    resultsHeading({ query: 'кибер бэкап', counts: { families: 4, skus: 4 } }),
    'По запросу «кибер бэкап» — 4 продукта · 4 варианта',
  )
  assert.equal(
    resultsHeading({ query: 'q', counts: { families: 1 } }),
    'По запросу «q» — 1 продукт',
  )
})

test('подпись альтернативы — без «замена» и «аналог»', () => {
  assert.match(ALTERNATIVE_NOTE, /Совместимость с вашими приложениями нужно проверить/)
  assert.doesNotMatch(ALTERNATIVE_NOTE, /замен|аналог/i)
})

test('showcaseParts: персонализация включена всегда, шаблоны — только на полной витрине', () => {
  assert.deepEqual(showcaseParts(false), { personalization: true, solutionTemplates: false })
  assert.deepEqual(showcaseParts(true), { personalization: true, solutionTemplates: true })
})
