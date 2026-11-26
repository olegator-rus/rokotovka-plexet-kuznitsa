// Закупка и честное «нет» (src/lib/procurement.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Purchase, SearchItemBase, SearchResponse } from '../src/api/types.ts'
import {
  helpAnswerShown,
  helpHintShown,
  isPerUnitLine,
  parsePurchase,
  purchaseQuantity,
  purchaseNote,
  reestrTone,
  selectedOffer,
  specPurchase,
  supplyShown,
} from '../src/lib/procurement.ts'

const NBSP = '\u00a0'

const kes: SearchItemBase = {
  product_id: 'kes-std-ren24',
  family_id: 'kaspersky-endpoint-security',
  title: 'Kaspersky Endpoint Security для бизнеса Стандартный, продление на 2 года, за 1 узел',
  vendor: 'Kaspersky',
  category: 'Антивирусная защита для бизнеса',
  license_line: 'Продление · 1 устройство · 2 года',
  price_rub: 4290,
  price_on_request: false,
  group: 'exact',
  matches_query: true,
  purchased_before: false,
  final: 0.9,
  attrs: {
    edition: 'Стандартный',
    license_type: 'renewal',
    license_metric: 'device',
    license_quantity: 1,
    duration_months: 24,
    delivery: '',
    in_reestr: true,
  },
}
const s2: Purchase = {
  query: 'продлить касперский на 100 пк на 2 года',
  deal: 'renewal',
  deal_label: 'Продление',
  quantity: 100,
  metric: 'device',
  duration_months: 24,
}

test('selectedOffer: продление на 2 года × 100 узлов', () => {
  const o = selectedOffer(kes, s2)
  assert.ok(o)
  assert.equal(o.label, 'Продление · 2 года · Стандартный')
  assert.equal(o.quantity, 100)
  assert.equal(o.total, 429000)
  assert.match(o.sum, /× 100 устройств =/)
})

test('selectedOffer: ступень 100–149 — цена за узел × 100, а не один пакет (S3)', () => {
  const tiered: SearchItemBase = {
    ...kes,
    product_id: '413720',
    license_line: 'Продление · 100 устройств · 2 года',
    price_rub: 3070.8,
    attrs: { ...kes.attrs!, edition: '', license_quantity: 100 },
    tier_min: 100,
    tier_max: 149,
  }
  const o = selectedOffer(tiered, s2)
  assert.ok(o)
  assert.equal(o.quantity, 100)
  assert.equal(o.total, 307080)
  assert.equal(o.label, 'Продление · 2 года · ступень 100–149 устройств')
  assert.equal(o.sum, `3${NBSP}070,80${NBSP}₽ за узел × 100 устройств = 307${NBSP}080${NBSP}₽`)
  // Без ступени тот же пакет «на 100 устройств» — один пакет.
  assert.equal(
    selectedOffer({ ...tiered, tier_min: undefined, tier_max: undefined }, s2)?.total,
    3070.8,
  )
  assert.equal(purchaseQuantity(tiered, 120, 'device'), 120)
})

test('selectedOffer: без условий, без цены, не подходит — null', () => {
  assert.equal(selectedOffer(kes, null), null)
  assert.equal(selectedOffer(kes, { query: 'kes' }), null)
  assert.equal(selectedOffer({ ...kes, matches_query: false }, s2), null)
  assert.equal(selectedOffer({ ...kes, price_rub: null, price_on_request: true }, s2), null)
})

test('selectedOffer: пакет на 50 узлов при 30 ПК — один пакет', () => {
  const pack50 = {
    ...kes,
    license_line: 'Продление · 50 устройств · 1 год',
    price_rub: 117000,
    attrs: { ...kes.attrs!, license_quantity: 50 },
  }
  const o = selectedOffer(pack50, { ...s2, quantity: 30 })
  assert.equal(o?.quantity, 1)
  assert.equal(o?.total, 117000)
  // 100 ПК — два пакета по 50, а не один на 50 мест.
  assert.equal(selectedOffer(pack50, s2)?.quantity, 2)
})

test('selectedOffer: CAL пакетами по 5 при 50 пользователях — 10 пакетов (QA r2 №2)', () => {
  const cal: SearchItemBase = {
    ...kes,
    product_id: 'wscal25-u5',
    title: 'Microsoft Windows Server 2025 CAL, 5 клиентских лицензий на пользователя',
    license_line: 'Бессрочная · 5 пользователей',
    price_rub: 27490,
    attrs: {
      edition: 'User CAL',
      license_type: 'perpetual',
      license_metric: 'user',
      license_quantity: 5,
      duration_months: null,
      delivery: '',
      in_reestr: false,
    },
  }
  const p: Purchase = {
    query: 'windows server 2025 standard на 50 пользователей',
    quantity: 50,
    metric: 'user',
  }
  const o = selectedOffer(cal, p)
  assert.ok(o)
  assert.equal(o.quantity, 10)
  assert.equal(o.total, 274900)
  assert.equal(
    o.sum.replace(/\u00a0/g, ' '),
    '27 490 ₽ × 10 пакетов по 5 пользователей = 274 900 ₽',
  )
  // 52 пользователя — 11 пакетов (55 лицензий), меньше не хватит.
  assert.equal(selectedOffer(cal, { ...p, quantity: 52 })?.quantity, 11)
})

test('purchaseQuantity: штуки, пакеты, ядра, другая метрика', () => {
  const at = (metric: string, q: number | null) => ({
    edition: '',
    license_type: 'perpetual',
    license_metric: metric,
    license_quantity: q,
    duration_months: null,
    delivery: '' as const,
    in_reestr: false,
  })
  // Поштучная — само число.
  assert.equal(purchaseQuantity({ license_line: '', attrs: at('device', 1) }, 100, 'device'), 100)
  // Пакет 10 ПК при 10 ПК — один.
  assert.equal(purchaseQuantity({ license_line: '', attrs: at('device', 10) }, 10, 'device'), 1)
  // Ядра — не места: 1.
  assert.equal(
    purchaseQuantity({ license_line: 'Бессрочная · 16 ядер', attrs: at('core', 16) }, 50, 'user'),
    1,
  )
  // Пакет на устройства при запросе на пользователей — не делим, пакет не поштучный: 1.
  assert.equal(
    purchaseQuantity(
      { license_line: 'Бессрочная · 5 устройств', attrs: at('device', 5) },
      50,
      'user',
    ),
    1,
  )
  // Без атрибутов — по строке лицензии, как раньше.
  assert.equal(purchaseQuantity({ license_line: 'Подписка · 1 пользователь' }, 7), 7)
  assert.equal(purchaseQuantity({ license_line: 'Подписка · 1 пользователь' }, 0), 1)
})

test('isPerUnitLine', () => {
  assert.ok(isPerUnitLine('Продление · 1 устройство · 2 года'))
  assert.ok(!isPerUnitLine('Подписка · 10 устройств · 1 год'))
})

test('specPurchase и purchaseNote: параметры из запроса в заявку', () => {
  const p = specPurchase({ ...s2, edition: 'Стандартный' })
  assert.deepEqual(p, {
    query: s2.query,
    deal: 'renewal',
    quantity: 100,
    metric: 'device',
    duration_months: 24,
    edition: 'Стандартный',
  })
  assert.equal(
    purchaseNote(p ?? undefined),
    'продление · редакция Стандартный · 2 года · по запросу «продлить касперский на 100 пк на 2 года»',
  )
  assert.equal(specPurchase(null), null)
})

test('parsePurchase: битое отбрасывается', () => {
  assert.equal(parsePurchase(null), undefined)
  assert.equal(parsePurchase({ deal: 'renewal' }), undefined)
  assert.deepEqual(parsePurchase({ query: 'q', quantity: -1, deal: 5, edition: 'Std' }), {
    query: 'q',
    edition: 'Std',
  })
})

test('reestrTone', () => {
  assert.equal(reestrTone({ status: 'confirmed', label: 'x' }), 'ok')
  assert.equal(reestrTone({ status: 'unknown', label: 'x' }), 'muted')
  assert.equal(reestrTone(null), null)
})

test('справка и поставка', () => {
  const help = {
    id: 'invoice',
    mode: 'answer',
    title: 't',
    text: 'x',
    link_label: 'l',
    url: '/help#invoice',
  }
  const base = { items: [], empty: null } as unknown as SearchResponse
  assert.ok(helpAnswerShown({ ...base, help }))
  assert.ok(!helpHintShown({ ...base, help }))
  assert.ok(helpHintShown({ items: [kes as never], help: { ...help, mode: 'hint' } }))
  assert.ok(supplyShown({ empty: { message: 'm', show_manager: true, supply: true } }))
  assert.ok(!supplyShown({ empty: { message: 'm', show_manager: true, supply: true }, help }))
  assert.ok(!supplyShown({ empty: { message: 'm', show_manager: true } }))
})
