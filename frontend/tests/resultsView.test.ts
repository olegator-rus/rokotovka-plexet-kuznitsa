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
  showMoreLabel,
  stableFacetOrder,
} from '../src/lib/resultsView.ts'
import {
  SEARCH_FLAT_LIMIT,
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
  const cells = attrCells(attrs, namedAttrs([{ field: 'license_quantity', value: '100' }]))
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
  // Новый запрос начинается с настроек по умолчанию: фильтры, сортировка и группировка сбрасываются (QA r2 №13).
  const next = withQuery(
    { ...s, filters: [{ key: 'vendor', op: 'eq', value: 'kaspersky' }] },
    'офис',
  )
  assert.deepEqual(next.filters, [])
  assert.equal(next.sort, undefined)
  assert.equal(next.group, undefined)
  assert.deepEqual(serializeSearchState(next), { q: 'офис' })
})

test('подсветка «названо в запросе» — только при совпадающем значении (QA r2 №9)', () => {
  const attrs: ItemAttrs = {
    edition: '',
    license_type: 'subscription',
    license_metric: 'user',
    license_quantity: 1,
    duration_months: 12,
    delivery: '',
    in_reestr: false,
  }
  const hl = (chips: { field: string; value: string }[]) =>
    attrCells(attrs, namedAttrs(chips))
      .filter((c) => c.highlight)
      .map((c) => c.key)
  // «на 3 года» — срок 1 год не выделяется; «на 1 год» — выделяется.
  assert.deepEqual(hl([{ field: 'license_duration_months', value: '36' }]), [])
  assert.deepEqual(hl([{ field: 'license_duration_months', value: '12' }]), ['duration'])
  // «на 10 рабочих мест»: метрика совпала, количество нет — ячейка количества не выделяется.
  const ten = [
    { field: 'license_quantity', value: '10' },
    { field: 'license_metric', value: 'user' },
  ]
  assert.deepEqual(hl(ten), [])
  assert.deepEqual(hl([{ field: 'license_metric', value: 'user' }]), ['quantity'])
  // «бессрочно» при подписке — тип не выделяется; продление — только у продления.
  assert.deepEqual(hl([{ field: 'license_type', value: 'perpetual' }]), [])
  assert.deepEqual(hl([{ field: 'license_type', value: 'subscription' }]), ['type'])
  assert.deepEqual(hl([{ field: 'renewal', value: 'true' }]), [])
  // Набор полей «Подходит под запрос» не зависит от значений.
  assert.equal(namedAttrs(ten).size, 1)
})

test('«Без группировки»: честный заголовок и «Показать ещё» (QA r2 витрина №3)', () => {
  const resp = { query: 'касперский', counts: { families: 6, variants: 33 } }
  assert.equal(
    resultsHeading(resp, 20),
    'По запросу «касперский» — 6 продуктов · 33 варианта · показаны 20',
  )
  assert.equal(resultsHeading(resp, 33), 'По запросу «касперский» — 6 продуктов · 33 варианта')
  assert.equal(resultsHeading(resp), 'По запросу «касперский» — 6 продуктов · 33 варианта')
  assert.equal(showMoreLabel(13), 'Показать ещё 13 вариантов')
  assert.equal(showMoreLabel(40), 'Показать ещё 20 вариантов')
  assert.equal(showMoreLabel(1), 'Показать ещё 1 вариант')
  const flat = buildSearchRequest(
    { ...emptyState(), q: 'x', group: 'none' },
    { sessionId: 's', defaultConfig: 'live' },
  )
  assert.equal(flat.limit, SEARCH_FLAT_LIMIT)
  const fam = buildSearchRequest(
    { ...emptyState(), q: 'x' },
    { sessionId: 's', defaultConfig: 'live' },
  )
  assert.equal(fam.limit, 20)
})
