// Закупка и честное «нет» (src/lib/procurement.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Purchase, SearchItemBase, SearchResponse } from '../src/api/types.ts'
import {
  helpAnswerShown,
  helpHintShown,
  isPerUnitLine,
  parsePurchase,
  purchaseNote,
  reestrTone,
  selectedOffer,
  specPurchase,
  supplyShown,
} from '../src/lib/procurement.ts'

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

test('selectedOffer: без условий, без цены, не подходит — null', () => {
  assert.equal(selectedOffer(kes, null), null)
  assert.equal(selectedOffer(kes, { query: 'kes' }), null)
  assert.equal(selectedOffer({ ...kes, matches_query: false }, s2), null)
  assert.equal(selectedOffer({ ...kes, price_rub: null, price_on_request: true }, s2), null)
})

test('selectedOffer: пакет на 50 узлов не умножается', () => {
  const o = selectedOffer(
    { ...kes, license_line: 'Продление · 50 устройств · 1 год', price_rub: 117000 },
    s2,
  )
  assert.equal(o?.quantity, 1)
  assert.equal(o?.total, 117000)
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
