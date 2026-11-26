// Две выдачи рядом: «только здесь» и сдвиг позиции (src/lib/compare.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { compareColumn, compareSummary, shiftLabel } from '../src/lib/compare.ts'

test('compareColumn: общий товар со сдвигом, отсутствующий — только здесь', () => {
  const marks = compareColumn(['a', 'b', 'c'], ['b', 'x', 'a'])
  assert.deepEqual(marks[0], { position: 1, otherPosition: 3, onlyHere: false, shift: 2 })
  assert.equal(shiftLabel(marks[0]), 'выше на 2')
  assert.equal(marks[1].shift, -1)
  assert.equal(shiftLabel(marks[1]), 'ниже на 1')
  assert.equal(marks[2].onlyHere, true)
  assert.equal(shiftLabel(marks[2]), 'только здесь')
})

test('compareColumn: то же место', () => {
  const [m] = compareColumn(['a'], ['a'])
  assert.equal(shiftLabel(m), 'на том же месте')
})

test('compareSummary: общие и уникальные', () => {
  assert.deepEqual(compareSummary(['a', 'b', 'c'], ['b', 'd']), {
    common: 1,
    onlyLeft: 2,
    onlyRight: 1,
  })
  assert.deepEqual(compareSummary([], []), { common: 0, onlyLeft: 0, onlyRight: 0 })
})
