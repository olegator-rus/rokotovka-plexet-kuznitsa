// Подсказки строки поиска (src/lib/suggest.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { highlightParts, shouldSuggest, toSuggestGroups } from '../src/lib/suggest.ts'

test('toSuggestGroups: не больше 8 строк, пустые группы убраны, позиции сквозные', () => {
  const items = (n: number, p: string) =>
    Array.from({ length: n }, (_, i) => ({ text: `${p}${i}`, id: null }))
  const groups = toSuggestGroups({
    groups: [
      { type: 'query', label: 'Запросы', items: items(5, 'q') },
      { type: 'vendor', label: '', items: [] },
      { type: 'product', label: 'Товары', items: items(5, 'p') },
      { type: 'category', label: 'Категории', items: items(2, 'c') },
    ],
  })
  assert.deepEqual(
    groups.map((g) => [g.type, g.rows.length]),
    [
      ['query', 5],
      ['product', 3],
    ],
  )
  assert.deepEqual(
    groups.flatMap((g) => g.rows.map((r) => r.position)),
    [1, 2, 3, 4, 5, 6, 7, 8],
  )
  assert.equal(groups[1].rows[0].key, 'product:6')
  assert.deepEqual(toSuggestGroups(null), [])
})

test('toSuggestGroups: подпись группы по умолчанию', () => {
  const g = toSuggestGroups({
    groups: [{ type: 'vendor', label: '', items: [{ text: 'Microsoft', id: 'microsoft' }] }],
  })
  assert.equal(g[0].label, 'Вендоры')
})

test('highlightParts: выделение, края и неверный диапазон', () => {
  assert.deepEqual(highlightParts('windows 11 pro', [0, 3]), [
    { text: 'win', match: true },
    { text: 'dows 11 pro', match: false },
  ])
  assert.deepEqual(highlightParts('abc', [1, 2]), [
    { text: 'a', match: false },
    { text: 'b', match: true },
    { text: 'c', match: false },
  ])
  assert.deepEqual(highlightParts('abc', null), [{ text: 'abc', match: false }])
  assert.deepEqual(highlightParts('abc', [5, 9]), [{ text: 'abc', match: false }])
})

test('shouldSuggest: от двух символов', () => {
  assert.equal(shouldSuggest('w'), false)
  assert.equal(shouldSuggest(' wi '), true)
})
