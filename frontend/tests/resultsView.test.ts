// Страница результатов контура выдачи: заголовок, применённые фильтры, уточнения, строка атрибутов, порядок фасетов,
// сортировка и группировка в адресе (src/lib/resultsView.ts, src/lib/searchState.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Facet } from '../src/api/typesP2.ts'
import type { ItemAttrs } from '../src/api/typesResults.ts'
import { formatPrice } from '../src/lib/format.ts'
import {
  attrCells,
  cardAction,
  familyPriceLabel,
  filterChipLabel,
  filterFacetValues,
  namedAttrs,
  refinementFilter,
  resultsHeading,
  stableFacetOrder,
} from '../src/lib/resultsView.ts'
import {
  buildSearchRequest,
  emptyState,
  parseSearchQuery,
  serializeSearchState,
  withQuery,
} from '../src/lib/searchState.ts'

const facet = (key: string, values: [string, string][] = []): Facet => ({
  key,
  label: key === 'duration_months' ? 'Срок' : key,
  unit: '',
  type: 'enum',
  min: null,
  max: null,
  values: values.map(([value, label]) => ({ value, label, count: 1 })),
})

test('заголовок «N продуктов · M вариантов»; без counts — контур коммерции', () => {
  assert.equal(
    resultsHeading({ query: 'антивирус', counts: { families: 7, variants: 32 } }),
    'По запросу «антивирус» — 7 продуктов · 32 варианта',
  )
  assert.equal(
    resultsHeading({ query: 'x', counts: { families: 1, variants: 1 } }),
    'По запросу «x» — 1 продукт · 1 вариант',
  )
  assert.equal(resultsHeading({ query: 'x' }), null)
})

test('применённый фильтр — название и подпись значения', () => {
  const facets = [
    facet('duration_months', [['12', '1 год']]),
    facet('license_type', [['renewal', 'Продление']]),
  ]
  facets[1]!.label = 'Тип лицензии'
  assert.equal(
    filterChipLabel({ key: 'duration_months', op: 'eq', value: 12 }, facets),
    'Срок: 1 год',
  )
  assert.equal(
    filterChipLabel({ key: 'license_type', op: 'eq', value: 'renewal' }, facets),
    'Тип лицензии: Продление',
  )
  assert.equal(
    filterChipLabel({ key: 'price_rub', op: 'gte', value: 5000 }, []),
    `Цена: от ${formatPrice(5000)}`,
  )
})

test('уточнение → фильтр с типом значения; без поля — текст', () => {
  const base = { label: '1 год', query: 'антивирус 1 год', op: 'eq', hits: 3 }
  assert.deepEqual(refinementFilter({ ...base, field: 'duration_months', value: '12' }), {
    key: 'duration_months',
    op: 'eq',
    value: 12,
  })
  assert.deepEqual(refinementFilter({ ...base, field: 'in_reestr', value: 'true' }), {
    key: 'in_reestr',
    op: 'eq',
    value: true,
  })
  assert.deepEqual(
    refinementFilter({ ...base, field: 'license_type', value: 'renewal' })?.value,
    'renewal',
  )
  assert.equal(refinementFilter({ ...base, field: null, value: null }), null)
})

test('строка атрибутов: одинаковый набор, прочерк, выделение названного в запросе', () => {
  const attrs: ItemAttrs = {
    edition: '',
    license_type: 'subscription',
    license_metric: 'device',
    license_quantity: 100,
    duration_months: 12,
    delivery: '',
    in_reestr: true,
  }
  const cells = attrCells(attrs, namedAttrs([{ field: 'license_quantity' }]))
  assert.deepEqual(
    cells.map((c) => c.key),
    ['edition', 'type', 'quantity', 'duration', 'delivery', 'reestr'],
  )
  assert.equal(cells[0]!.value, '—')
  assert.equal(cells[2]!.value, '100 устройств')
  assert.equal(cells[2]!.highlight, true)
  assert.equal(cells[3]!.value, '1 год')
  assert.equal(cells[3]!.highlight, false)
  const perpetual = attrCells(
    { ...attrs, license_type: 'perpetual', duration_months: null },
    new Set(),
  )
  assert.equal(perpetual[3]!.value, 'Бессрочно')
})

test('действие карточки по состоянию и цена «от»', () => {
  const item = { price_rub: 1000, price_on_request: false, price_from_rub: 900 } as never
  assert.equal(cardAction(item, 3), 'choose')
  assert.equal(cardAction(item, 0), 'spec')
  assert.equal(cardAction({ price_rub: null, price_on_request: true } as never, 0), 'price_request')
  assert.equal(familyPriceLabel(item, 3), `от ${formatPrice(900)}`)
  assert.equal(
    familyPriceLabel({ price_rub: null, price_on_request: true } as never, 0),
    'Цена по запросу',
  )
})

test('порядок фасетов устойчив: прежние на местах, новые в конце', () => {
  const next = [facet('license_type'), facet('vendor'), facet('price_rub')]
  assert.deepEqual(stableFacetOrder([], next), ['license_type', 'vendor', 'price_rub'])
  assert.deepEqual(stableFacetOrder(['vendor', 'delivery', 'license_type'], next), [
    'vendor',
    'license_type',
    'price_rub',
  ])
})

test('поиск внутри фасета по подписи', () => {
  const vs = [
    { value: 'drweb', label: 'Доктор Веб' },
    { value: 'kaspersky', label: 'Kaspersky' },
  ]
  assert.deepEqual(
    filterFacetValues(vs, 'док').map((v) => v.value),
    ['drweb'],
  )
  assert.equal(filterFacetValues(vs, ' ').length, 2)
})

test('сортировка и группировка: адрес ↔ состояние ↔ тело; по умолчанию не пишутся', () => {
  const s = parseSearchQuery({ q: 'антивирус', sort: 'price_asc', group: 'none' })
  assert.equal(s.sort, 'price_asc')
  assert.equal(s.group, 'none')
  assert.deepEqual(serializeSearchState(s), { q: 'антивирус', sort: 'price_asc', group: 'none' })
  const body = buildSearchRequest(s, { sessionId: 's', defaultConfig: 'live' })
  assert.equal(body.sort, 'price_asc')
  assert.equal(body.group, 'none')
  const def = parseSearchQuery({ q: 'x', sort: 'recommended', group: 'family' })
  assert.equal(def.sort, undefined)
  assert.equal(def.group, undefined)
  const plain = buildSearchRequest(
    { ...emptyState(), q: 'x' },
    { sessionId: 's', defaultConfig: 'live' },
  )
  assert.equal('sort' in plain, false)
  assert.equal('group' in plain, false)
  // «Сбросить фильтры» — не новый запрос; новый запрос сбрасывает фильтры, но не сортировку.
  const next = withQuery(
    { ...s, filters: [{ key: 'vendor', op: 'eq', value: 'kaspersky' }] },
    'офис',
  )
  assert.deepEqual(next.filters, [])
  assert.equal(next.sort, 'price_asc')
})
