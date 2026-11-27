// Режим подбора (src/lib/guide.ts): когда предлагать подбор, таблица сравнения, «только различия», подписи
// происхождения условий, цена оффера без «0 ₽», строка спецификации.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchResponse } from '../src/api/types.ts'
import type { GuideCondition, GuideOffer } from '../src/api/typesGuide.ts'
import {
  chipView,
  guideMode,
  isRefusal,
  lineFromOffer,
  offerPrice,
  offerQuantity,
  originLabel,
  offerName,
  conditionCount,
  offerRole,
  routeMode,
  selectionId,
  perUnit,
  specOffers,
  splitOffers,
  withSpecLine,
} from '../src/lib/guide.ts'

const sp = (s: string | undefined) => (s ?? '').replace(/ /g, ' ')

type Resp = Parameters<typeof guideMode>[1]
const resp = (over: Partial<SearchResponse> = {}): Resp =>
  ({
    items: [{ product_id: '1' }],
    chips: [],
    target: null,
    ...over,
  }) as unknown as Resp

test('guideMode: признак сервера главнее правил', () => {
  assert.equal(guideMode('kaspersky', resp({ guide: { mode: 'offer' } })), 'offer')
  assert.equal(guideMode('защита для офиса', resp({ guide: { mode: 'search' } })), 'search')
  // Неизвестное значение сервера — правила фронта.
  assert.equal(
    guideMode('криптопро csp 5.0', resp({ guide: { mode: 'x' }, target: fam })),
    'search',
  )
})

const fam = { type: 'family', url: '', title: 'КриптоПро CSP' }
const vendor = { type: 'vendor', url: '', title: 'Kaspersky' }

test('guideMode: таблица режимов SPEC §1', () => {
  assert.equal(guideMode('kaspersky', resp({ target: vendor })), 'offer')
  assert.equal(guideMode('криптопро csp 5.0', resp({ target: fam })), 'search')
  assert.equal(
    guideMode(
      'продление касперский на 100 пк на 2 года',
      resp({
        target: vendor,
        purchase: { query: '', deal: 'renewal', quantity: 100, duration_months: 24 },
      }),
    ),
    'search',
  )
  assert.equal(guideMode('kaspersky на 50 пк', resp({ target: fam })), 'offer')
  assert.equal(guideMode('защита для офиса', resp({ target: null })), 'offer')
  // Цель-семейство, но запрос задачей — предложение.
  assert.equal(guideMode('защита для компьютеров в офисе', resp({ target: fam })), 'offer')
  assert.equal(guideMode('чем расширенный отличается от стандартного', resp()), 'guide')
  assert.equal(guideMode('что взять для защиты 30 компьютеров на astra', resp()), 'guide')
  assert.equal(guideMode('подбери антивирус', null), 'guide')
  assert.equal(guideMode('самый дешёвый криптопро csp для сервера', resp({ target: fam })), 'guide')
})

test('guideMode: справка, сервис и пустая выдача — обычный поиск', () => {
  assert.equal(guideMode('оплата', resp({ target: { ...vendor, type: 'service' } })), 'search')
  assert.equal(guideMode('защита для офиса', resp({ items: [] })), 'search')
  assert.equal(
    guideMode(
      'как продлить',
      resp({
        help: { id: 'h', mode: 'answer', title: '', text: '', link_label: '', url: '' },
      }),
    ),
    'search',
  )
  assert.equal(guideMode('   ', resp()), 'search')
})

test('routeMode: режимы POST /api/guide/route', () => {
  assert.equal(routeMode({ mode: 'search_offer' }), 'offer')
  assert.equal(routeMode({ mode: 'search' }), 'search')
  assert.equal(routeMode({ mode: 'guide' }), 'guide')
  assert.equal(routeMode({ mode: '???' }), null)
  assert.equal(routeMode(null), null)
})

const offer = (over: Partial<GuideOffer> = {}): GuideOffer => ({
  offer_id: '4986',
  title: 'Kaspersky Endpoint Security для бизнеса Расширенный',
  sku: 'KL4867RAQFS',
  tier: '50–99',
  unit: 'компьютер',
  price_per_unit: 3940.2,
  total: 197010,
  price_state: 'fixed',
  offer_url: 'https://store.softline.ru/x',
  ...over,
})

test('offerPrice: цена за единицу и итог, «0 ₽» — никогда', () => {
  const p = offerPrice(offer())
  assert.equal(sp(p.unit), '3 940,20 ₽')
  assert.equal(sp(p.total), '197 010 ₽')
  assert.equal(p.known, true)
  for (const bad of [0, null, -1]) {
    const v = offerPrice(offer({ price_per_unit: bad, total: 0 }))
    assert.equal(v.unit, 'Цена по запросу')
    assert.equal(v.total, '')
    assert.ok(!sp(v.unit).includes('0 ₽'))
  }
  assert.equal(offerPrice(offer({ total: 0 })).total, '')
  assert.equal(offerPrice(offer({ price_state: 'quote_required' })).unit, 'Цена по запросу')
  assert.equal(offerPrice(offer({ price_state: 'on_request' })).unit, 'Цена по запросу')
  assert.equal(sp(offerPrice(offer({ price_state: 'priced' })).unit), '3 940,20 ₽')
  assert.equal(offerPrice(offer({ price_state: 'unavailable' })).unit, 'Нет в наличии')
  const from = offerPrice(offer({ price_state: 'from' }))
  assert.equal(sp(from.unit), 'от 3 940,20 ₽')
  assert.equal(from.state, 'цена от')
})

const conds: GuideCondition[] = [
  { key: 'count', label: '50 ПК', value: '50', origin: 'user' },
  { key: 'operation', label: 'новая лицензия', value: 'new', origin: 'default' },
  { key: 'term_months', label: '1 год', value: '12', origin: 'default' },
]

test('offerQuantity: поле сервера → итог / цена → условие → 1', () => {
  assert.equal(offerQuantity(offer({ quantity: 7 }), conds), 7)
  assert.equal(offerQuantity(offer(), []), 50)
  assert.equal(offerQuantity(offer({ total: null }), conds), 50)
  assert.equal(offerQuantity(offer({ total: null, price_per_unit: null }), []), 1)
})

test('lineFromOffer: строка спецификации с условиями закупки', () => {
  const l = lineFromOffer(offer(), conds, 'защита для компьютеров в офисе')
  assert.equal(l.product_id, '4986')
  assert.equal(l.quantity, 50)
  assert.equal(l.price_rub, 3940.2)
  assert.equal(l.price_mode, 'fixed')
  assert.equal(l.origin, 'search')
  assert.equal(l.license_line, 'арт. KL4867RAQFS · ступень 50–99 · за компьютер')
  assert.deepEqual(l.purchase, {
    query: 'защита для компьютеров в офисе',
    quantity: 50,
    deal: 'new',
    duration_months: 12,
  })
  const onReq = lineFromOffer(offer({ price_per_unit: 0, price_state: '' }), [], 'q')
  assert.equal(onReq.price_rub, null)
  assert.equal(onReq.price_mode, 'quote_required')
})

test('withSpecLine и selectionId: цена из черновика сервера', () => {
  const o = offer()
  const fresh = withSpecLine(o, {
    offer_id: '4986',
    title: '',
    sku: '',
    tier: '50–99',
    quantity: 50,
    price_per_unit: 3950,
    total: 197500,
    price_state: 'priced',
    offer_url: '',
  })
  assert.equal(fresh.price_per_unit, 3950)
  assert.equal(fresh.quantity, 50)
  assert.equal(fresh.sku, 'KL4867RAQFS')
  assert.equal(withSpecLine(o, null), o)
  assert.equal(withSpecLine(o, { ...fresh, offer_id: 'x', title: '' } as never), o)
  assert.equal(selectionId({ selection_id: 'sel-1' }), 'sel-1')
  assert.equal(selectionId(null), '')
  assert.equal(selectionId('x'), '')
  assert.equal(
    lineFromOffer(offer({ price_state: 'on_request' }), [], 'q').price_mode,
    'quote_required',
  )
})

test('specOffers: строки подбора D3 — все, иначе первый подходящий', () => {
  const o = (id: string, note = '', role = 'match') =>
    ({
      offer_id: id,
      title: id,
      sku: '',
      tier: '',
      unit: '',
      price_per_unit: 1,
      total: 1,
      price_state: 'fixed',
      offer_url: '',
      note,
      role,
    }) as GuideOffer
  const ws = o('ws', 'строка «рабочие станции»')
  const sr = o('sr', 'строка «серверы»')
  assert.deepEqual(
    specOffers([ws, sr]).map((x) => x.offer_id),
    ['ws', 'sr'],
  )
  // Три уровня одной строки («не знаю») — не строки подбора: первый.
  const lv = ['orel', 'vor', 'smol'].map((id) => o(id, `«${id}», строка «рабочие станции»`))
  assert.deepEqual(
    specOffers(lv).map((x) => x.offer_id),
    ['orel'],
  )
  assert.deepEqual(
    specOffers([o('a'), o('b')]).map((x) => x.offer_id),
    ['a'],
  )
  assert.deepEqual(
    specOffers([o('r', '', 'rejected'), o('alt', '', 'alternative')]).map((x) => x.offer_id),
    ['alt'],
  )
  assert.deepEqual(specOffers(null), [])
})

test('offerName: семейство и редакция вместо строки фида', () => {
  assert.deepEqual(
    offerName({
      title:
        'Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов',
      family_title: 'Kaspersky Endpoint Security для бизнеса Расширенный',
      term: '1 год',
    }),
    { name: 'Kaspersky Endpoint Security для бизнеса Расширенный', detail: 'лицензия Base' },
  )
  assert.deepEqual(
    offerName({
      title:
        'Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов',
      family_title: 'Kaspersky Endpoint Security для бизнеса Стандартный',
      term: '1 год',
    }).detail,
    'базовая лицензия',
  )
  // «Программа …» в семействе отбрасывается, отличие позиции остаётся.
  assert.deepEqual(
    offerName({
      title: 'КриптоПро CSP 5.0: серверная лицензия СКЗИ',
      family_title: 'Программа КриптоПро CSP 5.0',
      term: 'бессрочная',
    }),
    { name: 'КриптоПро CSP 5.0', detail: 'серверная лицензия СКЗИ' },
  )
  // Номер реестра в скобках после названия не нужен покупателю.
  assert.equal(
    offerName({
      title: 'КриптоПро CSP TLS (Номер ЕРРП: 4332), Лицензия на право использования',
      family_title: 'КриптоПро CSP TLS',
    }).detail,
    'Лицензия на право использования',
  )
  // Название семейства внутри строки — строка как есть; семейства нет — тоже.
  const raw = 'КриптоПро CSP (бессрочная лицензия СКЗИ КриптоПро CSP 5.0), на одном рабочем месте'
  assert.deepEqual(offerName({ title: raw, family_title: 'КриптоПро CSP 5.0' }), {
    name: raw,
    detail: '',
  })
  assert.deepEqual(offerName({ title: 'Astra Linux', family_title: '' }), {
    name: 'Astra Linux',
    detail: '',
  })
  // Семейство не совпадает со строкой фида: семейство — названием, строка — отличием.
  assert.deepEqual(
    offerName({
      title: 'Доктор Веб Комплексная защита + Центр управления на 1 год на 30 ПК',
      family_title: 'Dr.Web Desktop Security Suite',
      term: '1 год',
    }),
    {
      name: 'Dr.Web Desktop Security Suite',
      detail: 'Доктор Веб Комплексная защита + Центр управления на 30 ПК',
    },
  )
  // «лицензия» без уточнения — не отличие.
  assert.equal(
    offerName({
      title:
        'Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов',
      family_title: 'Kaspersky Endpoint Security Cloud',
      term: '1 год',
    }).detail,
    '',
  )
})

test('подписи и роли: происхождение условия, чип, отказ, роль оффера, число мест, единица', () => {
  assert.equal(originLabel('default'), 'по умолчанию')
  assert.equal(originLabel('inferred'), 'по запросу')
  assert.equal(originLabel('user'), '')
  assert.deepEqual(chipView({ label: '30', value: '30' }, 'count:workstation'), {
    label: '30',
    value: '30',
    key: 'count:workstation',
  })
  assert.deepEqual(chipView('10', 'count:server'), {
    label: '10',
    value: '10',
    key: 'count:server',
  })
  assert.equal(isRefusal({ type: 'no_data' }), true)
  assert.equal(isRefusal({ type: 'selection' }), false)
  assert.equal(offerRole({ role: 'rejected' }), 'rejected')
  assert.equal(offerRole({}), 'match')
  const parts = splitOffers([
    offer({ offer_id: 'a' }),
    offer({ offer_id: 'b', role: 'alternative' }),
  ])
  assert.deepEqual([parts.match.length, parts.alternative.length, parts.rejected.length], [1, 1, 0])
  assert.equal(
    conditionCount([{ key: 'count:workstation', label: '30 ПК', value: '30', origin: 'user' }]),
    30,
  )
  assert.equal(conditionCount([]), null)
  assert.equal(perUnit('компьютер'), 'за компьютер')
  assert.equal(perUnit('за сервер'), 'за сервер')
})
