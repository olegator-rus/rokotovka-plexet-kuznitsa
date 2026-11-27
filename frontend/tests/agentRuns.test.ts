// Контур автономии: экраны «Запуски», «Память», плашка «Очереди» (src/lib/agentRuns.ts, src/lib/agentMemory.ts) и новые виды
// шагов ленты (src/lib/agentFeed.ts). Данные — по форме, без выдуманных чисел из стенда.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { AgentRun, AgentStep, Proposal } from '../src/api/types.ts'
import type { MemorySuppress } from '../src/api/typesAgent.ts'
import {
  budgetRows,
  matchesOutcome,
  originPhrase,
  outcomeLine,
  planLine,
  queueBannerLine,
  queueOrigins,
  runWhen,
  triggerDetail,
  triggerLabel,
} from '../src/lib/agentRuns.ts'
import { conditionNow, episodeOutcome } from '../src/lib/agentMemory.ts'
import { effectVerdict, segmentByCluster, stepKind, stopReasonLabel } from '../src/lib/agentFeed.ts'

const NOW = new Date(2026, 4, 27, 15, 0)
const at = (d: number, h: number, m: number) => new Date(2026, 4, d + 2, h, m).toISOString()

function run(over: Partial<AgentRun> = {}): AgentRun {
  return {
    id: 'r1',
    status: 'done',
    trigger: 'schedule',
    started_at: at(25, 3, 0),
    finished_at: at(25, 3, 4),
    clusters_total: 7,
    clusters_done: 7,
    proposals_created: 4,
    tool_calls: 40,
    input_tokens: 0,
    output_tokens: 0,
    cost_rub: 11,
    stop_reason: '',
    summary: '',
    current_cluster: null,
    plan: { revision: 1, source: 'model', summary: 'Разобрать 7: ворд, офис 365 бесплатно' },
    budget: {
      limits: { steps: 80, tokens: 600000, cost_rub: 40, seconds: 1500 },
      used: { steps: 40, tokens: 120000, cost_rub: 11, seconds: 240 },
    },
    outcome: {
      proposals: 4,
      tasks: 2,
      rollbacks: 0,
      drafts: 0,
      skipped_memory: 1,
      effect_checks: 1,
      helped: 1,
      no_effect: 0,
      harmed: 0,
      checked: 0,
      stale: 0,
    },
    ...over,
  }
}

test('триггер: короткая подпись и подробная из input', () => {
  assert.equal(triggerLabel('schedule'), 'расписание')
  assert.equal(triggerLabel('spike'), 'всплеск')
  assert.equal(triggerLabel('catalog_change'), 'каталог')
  assert.equal(triggerLabel('manual'), 'вручную')
  assert.equal(
    triggerDetail(run({ trigger: 'spike', input: { spike: { fired: ['empty_share'] } } })),
    'всплеск пустых выдач',
  )
  assert.equal(triggerDetail(run({ trigger: 'manual', query: 'ворд' })), 'разбор запроса')
  assert.equal(triggerDetail(run({ trigger: 'catalog_change' })), 'обновление каталога')
})

test('время запуска: сегодня — 03:00, вчера — «вчера 18:05», раньше — «3 дня назад, 09:07», без даты', () => {
  assert.equal(runWhen(at(25, 3, 0), NOW), '03:00')
  assert.equal(runWhen(at(24, 18, 5), NOW), 'вчера 18:05')
  assert.equal(runWhen(at(22, 9, 7), NOW), '3\u00a0дня назад, 09:07')
})

test('итог одной строкой: N предложений, M задач, K пропущено по памяти + проверки эффекта', () => {
  assert.equal(
    outcomeLine(run()),
    '4 предложения, 2 задачи, 1 пропущено по памяти, 1 проверка эффекта (помогло 1)',
  )
  // Запуск до контура автономии: без плана и бюджета — предложения из proposals_created.
  const legacy = run({ plan: null, budget: null, outcome: null, proposals_created: 2 })
  assert.equal(outcomeLine(legacy), '2 предложения')
  assert.equal(planLine(legacy), '')
  assert.equal(planLine(run({ plan: null, budget: null, query: 'ворд' })), 'Разбор запроса «ворд»')
})

test('фильтр по итогу', () => {
  assert.ok(matchesOutcome(run(), 'memory'))
  assert.ok(matchesOutcome(run(), 'effect'))
  assert.ok(!matchesOutcome(run(), 'rollbacks'))
  assert.ok(!matchesOutcome(run(), 'nothing'))
  const empty = run({
    outcome: { ...run().outcome!, proposals: 0, tasks: 0, skipped_memory: 0, effect_checks: 0 },
  })
  assert.ok(matchesOutcome(empty, 'nothing'))
})

test('бюджет: израсходовано из лимита, исчерпанный ресурс отмечен', () => {
  const rows = budgetRows(run().budget, 'cost_budget')
  assert.deepEqual(
    rows.map((r) => r.key),
    ['steps', 'tokens', 'cost_rub', 'seconds'],
  )
  assert.equal(rows[0]!.used, '40')
  assert.equal(rows[0]!.limit, '80')
  assert.equal(rows[0]!.share, 0.5)
  assert.equal(rows[3]!.used, '4 мин 0 с')
  assert.ok(rows[2]!.exhausted)
  assert.deepEqual(budgetRows(null), [])
})

test('плашка очереди: «4 предложения от ночного запуска 03:00 · 1 от всплеска 14:20»', () => {
  const p = (id: string, run_id: string, trigger: string, started: string, status = 'pending') =>
    ({ id, run_id, trigger, run_started_at: started, status }) as unknown as Proposal
  const items = [
    p('a', 'spike1', 'spike', at(25, 14, 20)),
    ...['b', 'c', 'd', 'e'].map((id) => p(id, 'night', 'schedule', at(25, 3, 0))),
    p('f', 'night', 'schedule', at(25, 3, 0), 'accepted'),
  ]
  const origins = queueOrigins(items, NOW)
  assert.equal(
    queueBannerLine(origins),
    '4 предложения от ночного запуска 03:00 · 1 от всплеска 14:20',
  )
  assert.equal(originPhrase('catalog_change', at(24, 18, 5), NOW), 'проверки каталога вчера 18:05')
  assert.equal(queueBannerLine(queueOrigins([], NOW)), '')
})

test('память: текущее значение условия возврата и итог эпизода', () => {
  const s = {
    status: 'active',
    return_failures: 6,
    current_failures: 3,
    checked_at: null,
  } as unknown as MemorySuppress
  assert.equal(conditionNow(s), 'неудачных поисков 3 из 6 нужных · агент ещё не проверял')
  assert.equal(
    conditionNow({ ...s, status: 'removed', removed_reason: 'начали продавать' }),
    'снято: «начали продавать»',
  )
  assert.equal(
    episodeOutcome({ outcome: 'ticket', cause_label: 'Товара нет в каталоге' }),
    'задача владельцу · Товара нет в каталоге',
  )
})

test('лента: шаги памяти, плана и проверки эффекта — свои виды; эффект по verdict', () => {
  assert.equal(stepKind('memory_skip').group, 'memory')
  assert.equal(stepKind('memory_read').group, 'memory')
  assert.equal(stepKind('effect_check').group, 'effect')
  assert.equal(stepKind('plan').group, 'plan')
  const step = (over: Partial<AgentStep>): AgentStep => ({
    id: String(over.seq),
    run_id: 'r1',
    seq: 1,
    ts: at(25, 3, 0),
    kind: 'thought',
    cluster_id: null,
    tool: null,
    title: '',
    args: null,
    result: null,
    ok: true,
    hypothesis_no: null,
    rejected_reason: null,
    ms: null,
    ...over,
  })
  assert.equal(
    effectVerdict(step({ kind: 'effect_check', result: { verdict: 'helped' } })),
    'helped',
  )
  assert.equal(effectVerdict(step({ kind: 'effect_check', ok: false })), 'harmed')
  assert.equal(effectVerdict(step({ kind: 'thought' })), null)
  const segs = segmentByCluster(
    [
      step({ seq: 1, kind: 'plan' }),
      step({ seq: 2, kind: 'effect_check' }),
      step({ seq: 3, kind: 'memory_skip', cluster_id: 'c1', title: 'Читаю память → пропускаю' }),
      step({ seq: 4, kind: 'budget' }),
    ],
    null,
  )
  assert.equal(segs[0]!.label, 'План, проверка эффекта прошлых правок')
  assert.equal(segs[2]!.label, 'Бюджет запуска')
  assert.equal(stopReasonLabel('already_solved'), 'поиск уже отвечает верно — разбирать нечего')
})
