// Строка лицензии и порядок вариантов (src/lib/license.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItem, SearchItemBase } from '../src/api/types.ts'
import {
  isPurchasable,
  joinLicenseParts,
  kindFromLine,
  licenseKindLabel,
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

test('licenseKindLabel: вид из license_kind, а не из типа атрибутов (Cross-grade — не «Новая»)', () => {
  const attrs = (license_type: string) => ({ license_type })
  assert.equal(
    licenseKindLabel({ license_kind: 'migration', attrs: attrs('upgrade') }),
    'Переход с другого продукта',
  )
  assert.equal(licenseKindLabel({ license_kind: 'upgrade', attrs: attrs('upgrade') }), 'Обновление')
  assert.equal(
    licenseKindLabel({ license_kind: 'support', attrs: attrs('perpetual') }),
    'Техподдержка',
  )
  assert.equal(licenseKindLabel({ license_kind: 'renewal', attrs: attrs('renewal') }), 'Продление')
  assert.equal(licenseKindLabel({ license_kind: 'module', attrs: attrs('subscription') }), 'Модуль')
  assert.equal(licenseKindLabel({ license_kind: 'course', attrs: attrs('perpetual') }), 'Курс')
  assert.equal(
    licenseKindLabel({ license_kind: 'base', attrs: attrs('subscription') }),
    'Новая · подписка',
  )
  // Без вида (mock, страница товара) — по началу license_line, затем по типу.
  assert.equal(
    licenseKindLabel({ license_line: 'Техподдержка · 1 год', attrs: attrs('subscription') }),
    'Техподдержка',
  )
  assert.equal(
    licenseKindLabel({ license_line: 'Бессрочная', attrs: attrs('perpetual') }),
    'Новая · бессрочная',
  )
  assert.equal(licenseKindLabel({}), '')
  assert.equal(kindFromLine('Переход с другого продукта · 2 года'), 'Переход с другого продукта')
  assert.equal(kindFromLine('Подписка · 1 год'), '')
})
