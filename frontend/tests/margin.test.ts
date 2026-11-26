// Раздел «Маржа» (src/lib/margin.ts): упор рычага, язык владельца, «было / стало», геометрия кривой.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { MarginSimPoint } from '../src/api/typesCommerce.ts'
import {
  affectedList,
  swapExample,
  estimateNote,
  guardIndicators,
  hasMarginData,
  isLocked,
  pointFor,
  positionIndex,
  qualityLossText,
  rankColumns,
  stopIndex,
} from '../src/lib/margin.ts'

const POS = [0, 0.03, 0.05, 0.08, 0.12, 0.15]
const pt = (weight: number, delta: number, index: number | null): MarginSimPoint => ({
  weight,
  ndcg_delta_mean: delta,
  ndcg_delta_p10: delta * 3,
  share_over_eps: 0,
  margin_index: index,
  margin_index_relevant: index,
  top1_changed: 0,
  top3_changed: 0,
  guards_fired: 0,
  affected: [],
})

test('положение и упор: ближайшее положение, упор по stop_weight, без упора — последнее', () => {
  assert.equal(positionIndex(POS, 0.05), 2)
  assert.equal(positionIndex(POS, 0.07), 3)
  assert.equal(stopIndex(POS, 0.08), 3)
  assert.equal(stopIndex(POS, null), 5)
  assert.equal(stopIndex([], 0.1), -1)
})

test('точка симуляции по весу и наличие маржи', () => {
  const sim = { items: [pt(0, 0, 1), pt(0.05, -0.002, 1.2)] }
  assert.equal(pointFor(sim, 0.05)?.margin_index, 1.2)
  assert.equal(pointFor(sim, 0.08), null)
  assert.equal(hasMarginData(sim), true)
  assert.equal(hasMarginData({ items: [pt(0, 0, null)] }), false)
  assert.equal(hasMarginData(null), false)
})

test('потеря качества — языком владельца, не NDCG', () => {
  assert.equal(
    qualityLossText(-0.004),
    'В среднем на 0,4 % меньше лучших по соответствию товаров в первой десятке, чем при весе 0,05',
  )
  assert.match(qualityLossText(0.002, 0.05), /^В среднем на 0,2 % больше/)
  assert.match(qualityLossText(0), /как при весе 0,05/)
  assert.match(qualityLossText(undefined), /не посчитано/)
  assert.doesNotMatch(qualityLossText(-0.01), /NDCG/)
})

test('подпись природы оценки и защиты', () => {
  assert.equal(
    estimateNote(120, 'tune'),
    'Оценка на 120 размеченных запросах (набор tune). Маржа по показам в первой десятке. Не прогноз продаж.',
  )
  assert.equal(guardIndicators({ max_shift: 2 })[1], 'Сдвиг не больше 2 позиций')
  assert.equal(guardIndicators(null).length, 3)
})

test('затронутые: топ-1 раньше топ-3 и худших, без повторов', () => {
  const list = affectedList({
    ...pt(0.08, -0.003, 1.3),
    affected: [
      { query: 'c', before: [], after: [], reason: 'worst', ndcg_delta: -0.02 },
      { query: 'a', before: [], after: [], reason: 'top1' },
      { query: 'b', before: [], after: [], reason: 'top3' },
      { query: 'a', before: [], after: [], reason: 'worst' },
    ],
  })
  assert.deepEqual(
    list.map((x) => x.query),
    ['a', 'b', 'c'],
  )
})

test('было / стало: стрелки у переставленных позиций, новая в десятке', () => {
  const { before, after } = rankColumns({
    query: 'q',
    before: ['p1', 'p2', { product_id: 'p3', title: 'Товар 3' }],
    after: ['p2', 'p1', 'p4'],
  })
  assert.equal(before[2].title, 'Товар 3')
  assert.deepEqual(
    after.map((r) => [r.id, r.move, r.otherPos]),
    [
      ['p2', 'up', 2],
      ['p1', 'down', 1],
      ['p4', 'new', null],
    ],
  )
})

test('упор: available=false, выше stop_weight, ниже min_weight', () => {
  const sim = { items: [{ weight: 0.12, available: false }], stop_weight: 0.15, min_weight: 0.03 }
  assert.equal(isLocked(0.12, sim), true)
  assert.equal(isLocked(0, sim), true)
  assert.equal(isLocked(0.05, sim), false)
  assert.equal(isLocked(0.2, { stop_weight: 0.15 }), true)
  assert.equal(isLocked(0.2, null), false)
})

test('затронутые: причина из флагов сервера', () => {
  const l = affectedList({
    ...pt(0.08, 0, 1),
    affected: [
      { query: 'x', before: [], after: [], top3_changed: true },
      { query: 'y', before: [], after: [], top1_changed: true, top3_changed: true },
    ],
  })
  assert.deepEqual(
    l.map((a) => [a.query, a.reason]),
    [
      ['y', 'top1'],
      ['x', 'top3'],
    ],
  )
})

test('пример политики: обмен двух соседей в первой тройке важнее прочих перестановок', () => {
  const shuffle = {
    query: 'антивирус',
    before: ['a', 'b', 'c'],
    after: ['c', 'a', 'b'],
  }
  const swap = { query: 'офис', before: ['x', 'y', 'z'], after: ['y', 'x', 'z'] }
  assert.equal(swapExample([shuffle, swap])?.query, 'офис')
  assert.equal(swapExample([shuffle])?.query, 'антивирус')
  assert.equal(swapExample([]), null)
  // Первая тройка не изменилась — не пример: иначе экран писал бы о перестановке, которой нет.
  const still = { query: 'sql', before: ['a', 'b', 'c', 'd'], after: ['a', 'b', 'c', 'e'] }
  assert.equal(swapExample([still]), null)
  assert.equal(swapExample([still, swap])?.query, 'офис')
})
