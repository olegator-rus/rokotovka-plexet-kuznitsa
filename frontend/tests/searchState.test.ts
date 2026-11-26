// Состояние поиска ↔ адрес ↔ тело запроса (src/lib/searchState.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildSearchRequest,
  emptyState,
  parsePair,
  parseSearchQuery,
  sameRequest,
  serializeSearchState,
  withAsTyped,
  withChipRemoved,
  withChipsRestored,
  withClarify,
  withQuery,
} from '../src/lib/searchState.ts'

test('parseSearchQuery: всё из адреса', () => {
  const s = parseSearchQuery({
    q: '  касперский на 10 пк ',
    config: 'b2',
    client: 'client-a',
    as_typed: '1',
    rm: ['license_quantity:10', 'license_quantity:10', 'bad', 'vendor_id:kaspersky'],
    cl: 'license_metric:device',
    trace: '1',
  })
  assert.equal(s.q, 'касперский на 10 пк')
  assert.equal(s.config, 'B2')
  assert.equal(s.client, 'client-a')
  assert.equal(s.asTyped, true)
  assert.deepEqual(s.removed, [
    { field: 'license_quantity', value: '10' },
    { field: 'vendor_id', value: 'kaspersky' },
  ])
  assert.deepEqual(s.clarify, { license_metric: 'device' })
  assert.equal(s.trace, true)
})

test('parseSearchQuery: мусор и умолчания', () => {
  const s = parseSearchQuery({ config: 'B9', client: 'guest', as_typed: 'yes', q: ['x', 'y'] })
  assert.equal(s.q, 'x')
  assert.equal(s.config, null)
  assert.equal(s.client, null)
  assert.equal(s.asTyped, false)
  assert.deepEqual(parseSearchQuery({}), emptyState())
})

test('parsePair: значение с двоеточием, пустые части', () => {
  assert.deepEqual(parsePair('version:10:2'), { field: 'version', value: '10:2' })
  assert.equal(parsePair(':x'), null)
  assert.equal(parsePair('x:'), null)
  assert.equal(parsePair('x'), null)
})

test('serialize → parse: круг без потерь, умолчания не пишутся', () => {
  const s = withClarify(
    withChipRemoved(
      { ...emptyState(), q: 'офис', config: 'B3', client: 'client-a', trace: true },
      { field: 'origin', value: 'ru' },
    ),
    'license_metric',
    'user',
  )
  const q = serializeSearchState(s)
  assert.deepEqual(q, {
    q: 'офис',
    config: 'B3',
    client: 'client-a',
    rm: ['origin:ru'],
    cl: ['license_metric:user'],
    trace: '1',
  })
  assert.deepEqual(parseSearchQuery(q), s)
  assert.deepEqual(serializeSearchState(emptyState()), {})
})

test('withQuery сбрасывает уточнения; withChipRemoved не дублирует', () => {
  let s = withAsTyped({ ...emptyState(), q: 'касперкий' })
  s = withChipRemoved(s, { field: 'a', value: '1' })
  s = withChipRemoved(s, { field: 'a', value: '1' })
  assert.equal(s.removed.length, 1)
  s = withClarify(s, 'license_metric', 'any')
  const next = withQuery(s, ' dr.web ')
  assert.equal(next.q, 'dr.web')
  assert.equal(next.asTyped, false)
  assert.deepEqual(next.removed, [])
  assert.deepEqual(next.clarify, {})
  assert.deepEqual(withChipsRestored(s).removed, [])
})

test('sameRequest: трасса не меняет запрос', () => {
  const a = { ...emptyState(), q: 'windows', config: 'B3' as const }
  assert.equal(sameRequest(a, { ...a, trace: true }), true)
  assert.equal(sameRequest(a, { ...a, config: 'B1' }), false)
})

test('buildSearchRequest: тело по контракту', () => {
  const s = withClarify(
    withChipRemoved(
      { ...emptyState(), q: 'касперский на 10 пк', client: 'client-a' },
      {
        field: 'license_quantity',
        value: '10',
      },
    ),
    'license_metric',
    'device',
  )
  assert.deepEqual(buildSearchRequest(s, { sessionId: 's-1', defaultConfig: 'B3' }), {
    query: 'касперский на 10 пк',
    client_id: 'client-a',
    session_id: 's-1',
    config: 'B3',
    as_typed: false,
    removed_chips: [{ field: 'license_quantity', value: '10' }],
    clarify: { license_metric: 'device' },
    limit: 20,
  })
  const guest = buildSearchRequest(
    { ...emptyState(), q: 'x', config: 'B0' },
    {
      sessionId: 's-2',
      defaultConfig: 'B3',
      limit: 5,
    },
  )
  assert.equal(guest.client_id, 'guest')
  assert.equal(guest.config, 'B0')
  assert.equal(guest.limit, 5)
})
