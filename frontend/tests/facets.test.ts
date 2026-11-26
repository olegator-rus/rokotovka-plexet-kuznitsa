// Фасеты оборудования: адрес ↔ фильтр ↔ ползунок, подписи чипов (src/lib/facets.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Facet } from '../src/api/typesP2.ts'
import {
  chipLabel,
  parseFilter,
  parseFilters,
  rangeFilter,
  rangeOf,
  rangeStep,
  serializeFilter,
  usableFacet,
  withFilter,
} from '../src/lib/facets.ts'
import {
  buildSearchRequest,
  emptyState,
  parseSearchQuery,
  serializeSearchState,
} from '../src/lib/searchState.ts'

const ram: Facet = {
  key: 'ram_gb',
  label: 'Память',
  unit: 'ГБ',
  type: 'number',
  min: 8,
  max: 64,
  values: [],
}

test('фильтр в адресе туда и обратно', () => {
  for (const raw of [
    'ram_gb:gte:32',
    'ram_gb:range:16:64',
    'os:eq:none',
    'vendor:not:Dell',
    'ssd:eq:true',
  ]) {
    const f = parseFilter(raw)
    assert.ok(f, raw)
    assert.equal(serializeFilter(f), raw)
  }
  assert.deepEqual(parseFilter('ram_gb:range:64:16'), {
    key: 'ram_gb',
    op: 'range',
    value: 16,
    value2: 64,
  })
  assert.equal(parseFilter('ram_gb:like:3'), null)
  assert.equal(parseFilter('ram_gb:range:a:3'), null)
  assert.equal(parseFilter('ram_gb'), null)
  assert.equal(parseFilters(['a:eq:1', 'a:eq:2', 'b:gte:3']).length, 2)
})

test('ползунок: весь диапазон — без фильтра, край — gte/lte, середина — range', () => {
  assert.equal(rangeFilter(ram, 8, 64), null)
  assert.deepEqual(rangeFilter(ram, 32, 64), { key: 'ram_gb', op: 'gte', value: 32 })
  assert.deepEqual(rangeFilter(ram, 8, 32), { key: 'ram_gb', op: 'lte', value: 32 })
  assert.deepEqual(rangeFilter(ram, 16, 32), { key: 'ram_gb', op: 'range', value: 16, value2: 32 })
  assert.deepEqual(rangeOf(ram, { key: 'ram_gb', op: 'gte', value: 32 }), [32, 64])
  assert.deepEqual(rangeOf(ram, undefined), [8, 64])
  assert.equal(rangeStep(ram), 1)
  assert.equal(rangeStep({ ...ram, min: 13.3, max: 17.3 }), 0.1)
})

test('фасет без выбора не показывается', () => {
  assert.equal(usableFacet(ram), true)
  assert.equal(usableFacet({ ...ram, max: 8 }), false)
  assert.equal(usableFacet({ ...ram, type: 'enum', values: [] }), false)
  assert.equal(usableFacet({ ...ram, type: 'enum', values: [{ value: 'none', count: 3 }] }), true)
})

test('withFilter заменяет по ключу и снимает', () => {
  const a = withFilter([], 'ram_gb', { key: 'ram_gb', op: 'gte', value: 16 })
  const b = withFilter(a, 'ram_gb', { key: 'ram_gb', op: 'gte', value: 32 })
  assert.deepEqual(b, [{ key: 'ram_gb', op: 'gte', value: 32 }])
  assert.deepEqual(withFilter(b, 'ram_gb', null), [])
})

test('фильтры в состоянии поиска: адрес и тело запроса; без фильтров тело как в контуре поиска', () => {
  const s = parseSearchQuery({ q: 'ноутбук', f: ['ram_gb:gte:16'] })
  assert.deepEqual(s.filters, [{ key: 'ram_gb', op: 'gte', value: 16 }])
  assert.deepEqual(serializeSearchState(s).f, ['ram_gb:gte:16'])
  const body = buildSearchRequest(s, { sessionId: 's', defaultConfig: 'live' })
  assert.deepEqual(body.filters, [{ key: 'ram_gb', op: 'gte', value: 16 }])
  const plain = buildSearchRequest(
    { ...emptyState(), q: 'x' },
    { sessionId: 's', defaultConfig: 'live' },
  )
  assert.equal('filters' in plain, false)
})

test('подпись чипа: парт-номер, готовая подпись, сборка из полей', () => {
  assert.equal(
    chipLabel({ field: 'mpn', value: 'HMA82GR7CJR8N-XN', label: '' }),
    'Парт-номер: HMA82GR7CJR8N-XN',
  )
  assert.equal(chipLabel({ field: 'mpn', value: 'X', label: 'Парт-номер X' }), 'Парт-номер X')
  assert.equal(
    chipLabel({ field: 'ram_gb', value: '32', label: 'Память ≥ 32 ГБ', op: 'gte', unit: 'ГБ' }),
    'Память ≥ 32 ГБ',
  )
  assert.equal(
    chipLabel({ field: 'ram_gb', value: '32', label: '', op: 'gte', unit: 'ГБ' }),
    'ram_gb ≥ 32 ГБ',
  )
})
