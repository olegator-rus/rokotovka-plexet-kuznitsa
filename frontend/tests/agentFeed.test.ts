// Лента запуска агента (src/lib/agentFeed.ts): редьюсер SSE, отклонённые гипотезы, сегменты кластеров.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { AgentRun, AgentStep } from '../src/api/types.ts'
import {
  EMPTY_FEED,
  currentClusterCalls,
  feedReducer,
  formatDuration,
  parseSse,
  rejectedHypotheses,
  runOutcomeLine,
  segmentByCluster,
  stepTitle,
  stepView,
  stopReasonLabel,
} from '../src/lib/agentFeed.ts'

function run(over: Partial<AgentRun> = {}): AgentRun {
  return {
    id: 'r1',
    status: 'running',
    trigger: 'manual',
    started_at: '2026-05-27T10:00:00Z',
    finished_at: null,
    clusters_total: 5,
    clusters_done: 1,
    proposals_created: 1,
    tool_calls: 21,
    input_tokens: 1000,
    output_tokens: 200,
    cost_rub: 0.42,
    stop_reason: null,
    summary: null,
    current_cluster: { id: 'c2', label: 'акад' },
    ...over,
  }
}

function step(seq: number, over: Partial<AgentStep> = {}): AgentStep {
  return {
    id: `s${seq}`,
    run_id: 'r1',
    seq,
    ts: '2026-05-27T10:00:00Z',
    kind: 'tool_call',
    cluster_id: 'c1',
    tool: 'search_catalog',
    title: `шаг ${seq}`,
    args: {},
    result: {},
    ok: true,
    hypothesis_no: null,
    rejected_reason: null,
    ms: 10,
    ...over,
  }
}

test('feedReducer: снимок, затем шаги SSE без дублей и по seq', () => {
  let s = feedReducer(EMPTY_FEED, {
    type: 'snapshot',
    detail: { run: run(), steps: [step(1), step(2)] },
  })
  assert.equal(s.done, false)
  s = feedReducer(s, { type: 'step', step: step(2, { title: 'повтор' }) })
  s = feedReducer(s, { type: 'step', step: step(4) })
  s = feedReducer(s, { type: 'step', step: step(3) })
  assert.deepEqual(
    s.steps.map((x) => x.seq),
    [1, 2, 3, 4],
  )
  assert.equal(s.steps[1].title, 'повтор')
})

test('feedReducer: шаг чужого запуска игнорируется', () => {
  const s = feedReducer(EMPTY_FEED, { type: 'snapshot', detail: { run: run(), steps: [] } })
  const s2 = feedReducer(s, { type: 'step', step: step(1, { run_id: 'other' }) })
  assert.equal(s2.steps.length, 0)
})

test('feedReducer: статус не running и done завершают ленту', () => {
  let s = feedReducer(EMPTY_FEED, { type: 'snapshot', detail: { run: run(), steps: [] } })
  s = feedReducer(s, { type: 'status', run: run({ status: 'running', tool_calls: 30 }) })
  assert.equal(s.done, false)
  assert.equal(s.run?.tool_calls, 30)
  s = feedReducer(s, { type: 'status', run: run({ status: 'stopped' }) })
  assert.equal(s.done, true)
  const d = feedReducer(EMPTY_FEED, { type: 'done' })
  assert.equal(d.done, true)
  const finished = feedReducer(EMPTY_FEED, {
    type: 'snapshot',
    detail: { run: run({ status: 'done' }), steps: [] },
  })
  assert.equal(finished.done, true)
})

test('parseSse: битый JSON — null', () => {
  assert.deepEqual(parseSse('{"a":1}'), { a: 1 })
  assert.equal(parseSse('не json'), null)
  assert.equal(parseSse('42'), null)
})

test('отклонённая гипотеза: причина из шага проверки зачёркивает гипотезу того же номера', () => {
  const steps = [
    step(1, { kind: 'hypothesis', tool: null, hypothesis_no: 1, title: 'алиас «кис» → kaspersky' }),
    step(2, {
      kind: 'check',
      tool: 'run_evaluation',
      hypothesis_no: 1,
      ok: false,
      rejected_reason: 'слишком широко: 14 запросов ухудшились',
    }),
    step(3, { kind: 'hypothesis', tool: null, hypothesis_no: 2 }),
    step(4, { kind: 'check', tool: 'run_evaluation', hypothesis_no: 2, ok: true }),
    step(5, { kind: 'hypothesis', tool: null, hypothesis_no: 1, cluster_id: 'c2' }),
  ]
  const rej = rejectedHypotheses(steps)
  assert.equal(rej.size, 1)
  const v1 = stepView(steps[0], rej)
  assert.equal(v1.rejected, 'слишком широко: 14 запросов ухудшились')
  assert.equal(stepView(steps[1], rej).passed, false)
  assert.equal(stepView(steps[2], rej).rejected, null)
  assert.equal(stepView(steps[3], rej).passed, true)
  // Причина пишется один раз — у гипотезы; у проверки — только если гипотезы в ленте нет.
  const hyps = new Set(['c1#1', 'c1#2'])
  assert.equal(stepView(steps[0], rej, hyps).showReason, true)
  assert.equal(stepView(steps[1], rej, hyps).showReason, false)
  assert.equal(stepView(steps[1], rej).showReason, true)
  // Гипотеза №1 другого кластера не задета.
  assert.equal(stepView(steps[4], rej).rejected, null)
})

test('лестница контура коммерции: пройденные ступени отклонённой гипотезы остаются пройденными', () => {
  const steps = [
    step(1, {
      kind: 'hypothesis',
      tool: null,
      hypothesis_no: 1,
      ok: false,
      rejected_reason: 'ступень 5',
    }),
    step(2, { kind: 'check', tool: null, hypothesis_no: 1, ok: true }),
    step(3, { kind: 'check', tool: null, hypothesis_no: 1, ok: false }),
  ]
  const rej = rejectedHypotheses(steps)
  assert.equal(stepView(steps[1], rej).passed, true)
  assert.equal(stepView(steps[1], rej).rejected, null)
  assert.equal(stepView(steps[2], rej).passed, false)
  assert.equal(stepView(steps[2], rej).rejected, 'ступень 5')
})

test('stepView: раскрывается только при непустых аргументах или ответе; подпись инструмента', () => {
  const rej = new Map<string, string>()
  assert.equal(stepView(step(1, { args: {}, result: {} }), rej).expandable, false)
  assert.equal(stepView(step(1, { args: { q: 'кис' }, result: null }), rej).expandable, true)
  assert.equal(stepView(step(1), rej).toolLabel, 'Поиск по каталогу')
  assert.equal(stepView(step(1, { tool: 'new_tool' }), rej).toolLabel, 'new_tool')
  assert.equal(stepView(step(1, { kind: 'memory', tool: null }), rej).kind.group, 'memory')
})

test('segmentByCluster: подряд идущие шаги кластера — один сегмент с подписью', () => {
  const steps = [
    step(1, { cluster_id: null, tool: 'get_failed_queries' }),
    step(2, { tool: 'get_query_cluster', result: { label: 'кис' } }),
    step(3),
    step(4, { cluster_id: 'c2' }),
  ]
  const segs = segmentByCluster(steps, run())
  assert.deepEqual(
    segs.map((s) => [s.clusterId, s.label, s.steps.length]),
    [
      [null, 'Подготовка запуска', 1],
      ['c1', 'кис', 2],
      ['c2', 'акад', 1],
    ],
  )
})

test('колонка состояния: вызовы в текущем кластере, итог, причина остановки, длительность', () => {
  const steps = [
    step(1, { cluster_id: 'c2' }),
    step(2, { cluster_id: 'c2', kind: 'thought' }),
    step(3),
  ]
  assert.equal(currentClusterCalls(run(), steps), 1)
  assert.equal(currentClusterCalls(run({ current_cluster: null }), steps), 0)
  assert.equal(
    runOutcomeLine(run({ clusters_done: 3, proposals_created: 2, tool_calls: 41 })),
    'Разобрано 3 из 5 кластеров · 2 предложения · 41 вызов инструментов',
  )
  assert.equal(stopReasonLabel('step_limit'), 'достигнут лимит вызовов инструментов на кластер')
  assert.equal(stopReasonLabel('что-то своё'), 'что-то своё')
  assert.equal(stopReasonLabel(null), null)
  assert.equal(formatDuration('2026-05-27T10:00:00Z', '2026-05-27T10:02:14Z'), '2 мин 14 с')
  assert.equal(formatDuration('2026-05-27T10:00:00Z', '2026-05-27T10:00:48Z'), '48 с')
})

test('живая лента: подпись кластера из шага «Кластер «…»», рассуждение без вставок ```', () => {
  const steps = [
    step(1, {
      kind: 'thought',
      tool: null,
      cluster_id: 'k1',
      title: 'Кластер «акад»: 6 запросов, потерянный спрос 37 поисков',
      result: { terms: ['акад'], queries: [] },
    }),
    step(2, {
      kind: 'thought',
      tool: null,
      cluster_id: 'k1',
      title: 'Проверим трассу «акад». ```{"name":"get_search_trace"}```',
    }),
  ]
  const segs = segmentByCluster(steps, run({ current_cluster: null }))
  assert.equal(segs[0].label, 'акад')
  assert.equal(segs[0].steps[1].title, 'Проверим трассу «акад».')
  assert.equal(stepTitle('Сначала проверим: ```json {} ```'), 'Сначала проверим')
  assert.equal(stepTitle('```json {}```'), '```json {}```')
})

test('clusterOutcomes: итог кластеров словами, предложение только когда есть id', async () => {
  const { clusterOutcomes } = await import('../src/lib/agentFeed.ts')
  const rows = clusterOutcomes(
    run({
      episode: {
        clusters: [
          {
            cluster_id: 'k1',
            label: 'акад',
            outcome: 'proposal',
            diagnosis: 'ALIAS_GAP',
            hypotheses: 3,
            rejected: 2,
            tool_calls: 13,
            proposal_id: 'p1',
          },
          {
            cluster_id: 'k2',
            label: 'кис',
            outcome: 'closed',
            diagnosis: 'ALIAS_GAP',
            hypotheses: 1,
            rejected: 1,
            tool_calls: 11,
            proposal_id: '',
          },
        ],
      },
    }),
  )
  assert.equal(rows[0].outcome, 'правка правил — в очереди')
  assert.equal(rows[0].proposalId, 'p1')
  assert.equal(rows[1].outcome, 'правкой правил не решается')
  assert.equal(rows[1].proposalId, null)
  assert.deepEqual(clusterOutcomes(null), [])
})
