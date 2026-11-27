// Метрики «До / После»: дельты, направление, формат, перцентили (src/lib/metrics.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  formatMetric,
  metricDef,
  metricDelta,
  orderMetricKeys,
  percentile,
} from '../src/lib/metrics.ts'

const NBSP = '\u00a0'

test('metricDelta: доля растёт и это лучше — стрелка вверх, п.п. со знаком', () => {
  const d = metricDelta(0.62, 0.5, metricDef('ndcg10'))
  assert.ok(d)
  assert.equal(d.arrow, 'up')
  assert.equal(d.verdict, 'better')
  assert.equal(d.text, '+0,120')
})

test('metricDelta: доля пустых падает — стрелка вниз, но это лучше', () => {
  const d = metricDelta(0.05, 0.182, metricDef('empty_rate'))
  assert.ok(d)
  assert.equal(d.arrow, 'down')
  assert.equal(d.verdict, 'better')
  assert.equal(d.text, `−13,2${NBSP}п.п.`)
  assert.equal(d.label, `лучше на 13,2${NBSP}п.п.`)
})

test('metricDelta: рост времени P95 — хуже', () => {
  const d = metricDelta(48, 21, metricDef('p95_ms'))
  assert.ok(d)
  assert.equal(d.verdict, 'worse')
  assert.equal(d.text, `+27${NBSP}мс`)
})

test('metricDelta: изменение меньше видимого шага — без изменений', () => {
  const d = metricDelta(0.5002, 0.5, metricDef('recall50'))
  assert.ok(d)
  assert.equal(d.arrow, 'flat')
  assert.equal(d.verdict, 'same')
  assert.equal(d.label, 'без изменений')
})

test('metricDelta: нет одного из значений — null', () => {
  assert.equal(metricDelta(null, 0.4, metricDef('ndcg10')), null)
  assert.equal(metricDelta(0.4, undefined, metricDef('ndcg10')), null)
})

test('formatMetric: доля в %, скор тремя знаками, мс, пусто — тире', () => {
  assert.equal(formatMetric(0.1234, 'share'), `12,3${NBSP}%`)
  assert.equal(formatMetric(1, 'share'), `100${NBSP}%`)
  assert.equal(formatMetric(0.81234, 'score'), '0,812')
  assert.equal(formatMetric(12.4, 'ms'), `12${NBSP}мс`)
  assert.equal(formatMetric(null, 'share'), '—')
})

test('orderMetricKeys: известные по порядку (семейные — первыми), неизвестные в конце по алфавиту', () => {
  assert.deepEqual(orderMetricKeys(['zeta', 'empty_rate', 'ndcg10', 'ndcg10_family', 'alpha']), [
    'ndcg10_family',
    'empty_rate',
    'ndcg10',
    'alpha',
    'zeta',
  ])
})

test('metricDef: неизвестный ключ с _ms — время, меньше лучше', () => {
  const d = metricDef('rerank_ms')
  assert.equal(d.format, 'ms')
  assert.equal(d.better, 'down')
})

test('percentile: линейная интерполяция, P50 и P95', () => {
  const xs = [10, 20, 30, 40, 50, 60, 70, 80, 90, 100]
  assert.equal(percentile(xs, 50), 55)
  assert.ok(Math.abs((percentile(xs, 95) ?? 0) - 95.5) < 1e-9)
  assert.equal(percentile([7], 95), 7)
  assert.equal(percentile([], 50), null)
  assert.equal(percentile([3, 1, 2], 0), 1)
  assert.equal(percentile([3, 1, 2], 100), 3)
})
