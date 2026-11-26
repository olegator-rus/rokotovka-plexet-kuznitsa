// Блок «Месяц» в «Экономике» (src/lib/costMonth.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { monthScenario, parseRubInput } from '../src/lib/costMonth.ts'

test('monthScenario: сервер + LLM × запросы + агент × исследования', () => {
  const r = monthScenario({
    requests: 20_000,
    serverRub: 7500,
    llmPerRequestRub: 0.0168,
    agentRunRub: 2.6,
    agentRuns: 100,
  })
  assert.equal(r.server, 7500)
  assert.ok(Math.abs((r.llm ?? 0) - 336) < 1e-9)
  assert.ok(Math.abs((r.agent ?? 0) - 260) < 1e-9)
  assert.ok(Math.abs(r.total - 8096) < 1e-9)
  assert.ok(Math.abs((r.perRequest ?? 0) - 0.4048) < 1e-9)
  assert.equal(r.partial, false)
})

test('monthScenario: нет замера агента — строка пустая, итог неполный', () => {
  const r = monthScenario({
    requests: 200_000,
    serverRub: 15000,
    llmPerRequestRub: 0,
    agentRunRub: null,
    agentRuns: 1000,
  })
  assert.equal(r.agent, null)
  assert.equal(r.llm, 0)
  assert.equal(r.total, 15000)
  assert.equal(r.partial, true)
})

test('monthScenario: отрицательные и мусорные значения — ноль', () => {
  const r = monthScenario({
    requests: 0,
    serverRub: -5,
    llmPerRequestRub: 1,
    agentRunRub: 1,
    agentRuns: Number.NaN,
  })
  assert.equal(r.server, 0)
  assert.equal(r.perRequest, null)
})

test('parseRubInput: пробелы, запятая, ₽; мусор — null', () => {
  assert.equal(parseRubInput('7 500'), 7500)
  assert.equal(parseRubInput('7 500,5 ₽'), 7500.5)
  assert.equal(parseRubInput(''), null)
  assert.equal(parseRubInput('abc'), null)
  assert.equal(parseRubInput('-1'), null)
})

test('agentRunCost: среднее по полным запускам, иначе среднее сервера', async () => {
  const { agentRunCost } = await import('../src/lib/costMonth.ts')
  const runs = [
    { status: 'done', clusters_done: 8, cost_rub: 30 },
    { status: 'done', clusters_done: 8, cost_rub: 32 },
    { status: 'failed', clusters_done: 0, cost_rub: 2 },
    { status: 'done', clusters_done: 0, cost_rub: 0 },
  ]
  assert.deepEqual(agentRunCost(runs, 19.98), { rub: 31, full: 2 })
  assert.deepEqual(agentRunCost([], 19.98), { rub: 19.98, full: 0 })
})
