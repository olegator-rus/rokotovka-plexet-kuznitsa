// Раскладка выдачи по блокам и сборка событий (src/lib/results.ts, src/lib/events.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItem, SearchResponse } from '../src/api/types.ts'
import { emptyTitle, splitResults } from '../src/lib/results.ts'
import { buildEvent, newSessionId } from '../src/lib/events.ts'

const item = (id: string, group: SearchItem['group']): SearchItem => ({
  product_id: id,
  family_id: id,
  title: id,
  vendor: 'v',
  category: 'c',
  license_line: '',
  price_rub: 1,
  price_on_request: false,
  group,
  matches_query: true,
  purchased_before: false,
  final: 0.5,
  variants: [],
})

test('splitResults: основная выдача и аналоги, позиции из ответа', () => {
  const resp = {
    query: 'фотошоп',
    items: [item('a', 'exact'), item('b', 'analog'), item('c', 'match')],
    empty: null,
  } as unknown as SearchResponse
  const r = splitResults(resp)
  assert.deepEqual(
    r.main.map((p) => [p.item.product_id, p.position]),
    [
      ['a', 1],
      ['c', 3],
    ],
  )
  assert.deepEqual(
    r.analogs.map((p) => [p.item.product_id, p.position]),
    [['b', 2]],
  )
  assert.equal(r.absent, false)
  assert.equal(r.nothing, false)
})

test('splitResults: товара нет и пустой ответ', () => {
  const absent = {
    query: 'adobe photoshop',
    items: [item('x', 'analog')],
    empty: { message: 'Товара «adobe photoshop» нет в каталоге', show_manager: true },
  } as unknown as SearchResponse
  const r = splitResults(absent)
  assert.equal(r.absent, true)
  assert.equal(r.analogs.length, 1)
  assert.equal(emptyTitle(absent), 'Товара «adobe photoshop» нет в каталоге')
  assert.equal(emptyTitle({ query: 'zzz', empty: null }), 'Товара «zzz» нет в каталоге')
  assert.equal(splitResults(null).nothing, true)
})

test('buildEvent: тело по контракту, гость по умолчанию', () => {
  const e = buildEvent(
    'add_to_cart',
    { requestId: 'r1', sessionId: 's-1', clientId: 'client-a' },
    { productId: 'kis', position: 2 },
  )
  assert.deepEqual(e, {
    type: 'add_to_cart',
    request_id: 'r1',
    session_id: 's-1',
    client_id: 'client-a',
    product_id: 'kis',
    position: 2,
    payload: {},
  })
  const m = buildEvent(
    'manager_request',
    { requestId: null, sessionId: 's', clientId: null },
    {
      payload: { query: 'adobe' },
    },
  )
  assert.equal(m.client_id, 'guest')
  assert.equal(m.product_id, null)
  assert.deepEqual(m.payload, { query: 'adobe' })
})

test('newSessionId: формат s- + 16 hex', () => {
  assert.match(newSessionId(), /^s-[0-9a-f]{16}$/)
  assert.equal(
    newSessionId(() => 0),
    's-0000000000000000',
  )
})
