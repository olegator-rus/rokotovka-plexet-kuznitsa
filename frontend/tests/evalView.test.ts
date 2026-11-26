// Представление прогонов «До / После» (src/lib/evalView.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { EvalMetrics, EvaluationView } from '../src/api/types.ts'
import {
  evalColumns,
  flattenMetrics,
  metricRows,
  regressionRows,
  reproducibility,
  shortHash,
} from '../src/lib/evalView.ts'

function metrics(over: Partial<EvalMetrics> = {}): EvalMetrics {
  return {
    queries: 10,
    empty_rate: null,
    empty_rate_weighted: null,
    no_relevant_top10: null,
    relevant_top10: null,
    ndcg10: null,
    mrr10: null,
    recall50: null,
    exact_accuracy: null,
    rescued_rate: null,
    false_rescue_rate: null,
    violation_rate: null,
    violation_items: 0,
    errors: 0,
    latency: { count: 0, p50: 0, p95: 0 },
    ...over,
  }
}

function view(
  config: EvaluationView['config'],
  over: Partial<EvaluationView> = {},
): EvaluationView {
  return {
    run_id: `run-${config}`,
    set: 'test',
    set_hash: 'abcdef0123456789',
    queries: 10,
    config,
    rules_version: 1,
    rules_override: false,
    catalog_version: 'mock-1',
    commit: '1234567890abcdef',
    repro: 'test@abcdef01 · правила v1',
    created_at: '2026-05-27T10:00:00Z',
    duration_ms: 100,
    metrics: metrics(),
    by_type: {},
    regressions: [],
    improvements: [],
    ...over,
  }
}

test('evalColumns: по лестнице B0→B4, у конфигурации — самый свежий прогон', () => {
  const cols = evalColumns({
    set: 'test',
    running: null,
    last_job: null,
    configs: [
      view('B3'),
      view('B0'),
      view('B1', { run_id: 'old', created_at: '2026-05-26T10:00:00Z' }),
      view('B1', { run_id: 'new', created_at: '2026-05-27T11:00:00Z' }),
    ],
  })
  assert.deepEqual(
    cols.map((c) => c.config),
    ['B0', 'B1', 'B3'],
  )
  assert.equal(cols[1].run_id, 'new')
  assert.deepEqual(evalColumns(null), [])
})

test('flattenMetrics: счётчики отброшены, задержка — только при замерах', () => {
  const f = flattenMetrics(metrics({ ndcg10: 0.5, latency: { count: 3, p50: 12, p95: 40 } }))
  assert.equal(f.ndcg10, 0.5)
  assert.equal(f.p50_ms, 12)
  assert.equal(f.p95_ms, 40)
  assert.equal('queries' in f, false)
  assert.equal('errors' in f, false)
  assert.equal('p50_ms' in flattenMetrics(metrics()), false)
})

test('metricRows: только посчитанные метрики, дельта к предыдущему столбцу', () => {
  const rows = metricRows([
    view('B0', { metrics: metrics({ ndcg10: 0.3, empty_rate: 0.4 }) }),
    view('B1', { metrics: metrics({ ndcg10: 0.6, empty_rate: 0.1 }) }),
  ])
  assert.deepEqual(
    rows.map((r) => r.key),
    ['ndcg10', 'empty_rate'],
  )
  const ndcg = rows[0]
  assert.equal(ndcg.cells[0].delta, null)
  assert.equal(ndcg.cells[1].delta?.verdict, 'better')
  assert.equal(ndcg.cells[1].prevConfig, 'B0')
  assert.equal(rows[1].cells[1].delta?.arrow, 'down')
  assert.equal(rows[1].cells[1].delta?.verdict, 'better')
})

test('reproducibility: один набор, версии без повторов', () => {
  const r = reproducibility([view('B0'), view('B1', { rules_version: 2 })])
  assert.ok(r)
  assert.equal(r.sameSet, true)
  assert.deepEqual(r.rulesVersions, [1, 2])
  assert.deepEqual(r.commits, ['1234567890abcdef'])
  assert.equal(reproducibility([]), null)
  const diff = reproducibility([view('B0'), view('B1', { set_hash: 'other' })])
  assert.equal(diff?.sameSet, false)
})

test('regressionRows: худшие по NDCG сверху, against из столбца, если сервер не прислал', () => {
  const change = (id: string, delta: number, against = '') => ({
    query_id: id,
    text: id,
    type: 'typo',
    freq: 1,
    against,
    ndcg_from: 0.9,
    ndcg_to: 0.9 + delta,
    ndcg_delta: delta,
    reasons: ['ndcg'],
  })
  const rows = regressionRows([
    view('B0'),
    view('B1', { regressions: [change('q1', -0.1), change('q2', -0.5, 'B0')] }),
  ])
  assert.deepEqual(
    rows.map((r) => r.query_id),
    ['q2', 'q1'],
  )
  assert.equal(rows[1].against, 'B0')
  assert.equal(rows[0].config, 'B1')
})

test('shortHash: длинный — 8 символов, короткий как есть, пусто — тире', () => {
  assert.equal(shortHash('abcdef0123456789'), 'abcdef01')
  assert.equal(shortHash('abc'), 'abc')
  assert.equal(shortHash(''), '—')
})

test('judgeView: порядок лестницы и пары, где судья расходится с разметкой', async () => {
  const { judgeView } = await import('../src/lib/evalView.ts')
  assert.equal(judgeView(null), null)
  const v = judgeView({
    by_config: [
      { config: 'B3', ndcg_labels: 0.869, ndcg_judge: 0.785 },
      { config: 'B0', ndcg_labels: 0.149, ndcg_judge: 0.142 },
      { config: 'B2', ndcg_labels: 0.82, ndcg_judge: 0.805 },
      { config: 'B1', ndcg_labels: 0.737, ndcg_judge: 0.708 },
    ],
  } as never)
  assert.deepEqual(
    v?.rows.map((r) => r.config),
    ['B0', 'B1', 'B2', 'B3'],
  )
  assert.deepEqual(v?.swapped, [['B2', 'B3']])
  assert.ok(Math.abs((v?.rows[3].diff ?? 0) + 0.084) < 1e-9)
})
