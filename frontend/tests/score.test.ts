// Сегменты полосы скора (src/lib/score.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { scoreSegments, scoreSum } from '../src/lib/score.ts'

test('scoreSegments: четыре сегмента в порядке, доли в сумме 100', () => {
  const segs = scoreSegments({ relevance: 0.74, popularity: 0.06, margin: 0.01, context: 0.01 })
  assert.deepEqual(
    segs.map((s) => s.key),
    ['relevance', 'popularity', 'margin', 'context'],
  )
  assert.deepEqual(
    segs.map((s) => s.label),
    ['Релевантность', 'Популярность', 'Маржа', 'Контекст'],
  )
  const total = segs.reduce((s, x) => s + x.percent, 0)
  assert.ok(Math.abs(total - 100) < 1e-9)
  assert.ok(Math.abs(segs[0].percent - (0.74 / 0.82) * 100) < 1e-9)
  assert.equal(segs[0].value, 0.74)
})

test('scoreSegments: нули, отрицательные и отсутствующие части', () => {
  assert.deepEqual(
    scoreSegments(null).map((s) => s.percent),
    [0, 0, 0, 0],
  )
  const segs = scoreSegments({ relevance: 0.5, popularity: -0.1 })
  assert.equal(segs[0].percent, 100)
  assert.equal(segs[1].percent, 0)
  assert.equal(segs[1].value, -0.1)
  assert.equal(segs[3].value, 0)
})

test('scoreSum: сумма частей с учётом знака', () => {
  assert.ok(
    Math.abs(scoreSum({ relevance: 0.74, popularity: 0.06, margin: 0.01, context: 0.01 }) - 0.82) <
      1e-9,
  )
  assert.equal(scoreSum(undefined), 0)
})
