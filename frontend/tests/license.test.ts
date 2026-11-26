// Строка лицензии и порядок вариантов (src/lib/license.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItem, SearchItemBase } from '../src/api/types.ts'
import {
  isPurchasable,
  joinLicenseParts,
  otherVariants,
  priceLabel,
  sortVariants,
  splitLicenseLine,
} from '../src/lib/license.ts'

function base(id: string, over: Partial<SearchItemBase> = {}): SearchItemBase {
  return {
    product_id: id,
    family_id: 'kis',
    title: id,
    vendor: 'Kaspersky',
    category: 'Антивирусы',
    license_line: 'Подписка · 2 устройства · 1 год',
    price_rub: 1000,
    price_on_request: false,
    group: 'match',
    matches_query: false,
    purchased_before: false,
    final: 0.5,
    ...over,
  }
}

test('splitLicenseLine: разделители ·, •, | и пустые части', () => {
  assert.deepEqual(splitLicenseLine('Подписка · 10 устройств · 1 год'), [
    'Подписка',
    '10 устройств',
    '1 год',
  ])
  assert.deepEqual(splitLicenseLine('Бессрочная | на сервер ·  '), ['Бессрочная', 'на сервер'])
  assert.deepEqual(splitLicenseLine(''), [])
  assert.deepEqual(splitLicenseLine(null), [])
  assert.equal(joinLicenseParts(['Подписка', '1 год']), 'Подписка · 1 год')
})

test('priceLabel и isPurchasable: цена по запросу и null', () => {
  assert.equal(priceLabel({ price_rub: null, price_on_request: true }), 'Цена по запросу')
  assert.equal(priceLabel({ price_rub: null, price_on_request: false }), 'Цена по запросу')
  assert.equal(priceLabel({ price_rub: 2990, price_on_request: false }), '2 990 ₽')
  assert.equal(isPurchasable({ price_rub: 2990, price_on_request: false }), true)
  assert.equal(isPurchasable({ price_rub: 2990, price_on_request: true }), false)
})

test('sortVariants: подходящие первыми, затем по цене, по запросу — в конце', () => {
  const list = [
    base('a', { price_rub: 3000 }),
    base('b', { price_rub: null, price_on_request: true }),
    base('c', { price_rub: 5000, matches_query: true }),
    base('d', { price_rub: 1000 }),
    base('e', { price_rub: 1000 }),
  ]
  assert.deepEqual(
    sortVariants(list).map((v) => v.product_id),
    ['c', 'd', 'e', 'a', 'b'],
  )
})

test('otherVariants: без дубля основной позиции и без variants', () => {
  const item: SearchItem = {
    ...base('main'),
    variants: [base('main'), base('x', { matches_query: true }), base('y')],
  }
  assert.deepEqual(
    otherVariants(item).map((v) => v.product_id),
    ['x', 'y'],
  )
  const bare = { ...base('solo') } as SearchItem
  assert.deepEqual(otherVariants(bare), [])
})
