// Блок «Месяц» в «Экономике» (src/lib/costMonth.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  AGENT_RUNS_MAX,
  SERVER_RUB_MAX,
  monthScenario,
  parseAssumption,
  parseRubInput,
  runsInHint,
} from '../src/lib/costMonth.ts'

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

test('parseRubInput: hex, экспонента, Infinity, знак, несколько разделителей, сверх предела — null', () => {
  for (const bad of [
    '0x10',
    '1e3',
    '1e400',
    'Infinity',
    '-Infinity',
    'NaN',
    '-5000',
    '+5',
    '1,5,5',
    '1.5.5',
    '7 500,',
    ',5',
    '99999999999999999999',
    String(SERVER_RUB_MAX + 1),
  ])
    assert.equal(parseRubInput(bad), null, bad)
  assert.equal(parseRubInput(String(SERVER_RUB_MAX)), SERVER_RUB_MAX)
  assert.equal(parseRubInput('0'), 0)
  assert.equal(parseRubInput('7\u00a0500.25'), 7500.25)
})

test('parseAssumption: текст ошибки для поля; запуски — целое до предела', () => {
  const runs = { max: AGENT_RUNS_MAX, integer: true }
  assert.deepEqual(parseAssumption('30', runs), { ok: true, value: 30 })
  assert.deepEqual(parseAssumption('1 000', runs), { ok: true, value: 1000 })
  const err = (raw: string) => {
    const r = parseAssumption(raw, runs)
    assert.equal(r.ok, false, raw)
    return r.ok ? '' : r.error
  }
  assert.equal(err(''), 'Введите число')
  assert.equal(err('   '), 'Введите число')
  assert.equal(err('-3'), 'Не меньше 0')
  assert.equal(err('2.5'), 'Целое число')
  assert.equal(err('2,5'), 'Целое число')
  assert.match(err('abc'), /Только цифры/)
  assert.match(err('0x10'), /Только цифры/)
  assert.match(err('1e3'), /Только цифры/)
  assert.equal(err('100000000'), 'Не больше 10\u00a0000')
  const rub = parseAssumption('20 000 000', { max: SERVER_RUB_MAX, unit: '₽' })
  assert.deepEqual(rub, { ok: false, error: 'Не больше 10\u00a0000\u00a0000 ₽' })
})

test('runsInHint: число запусков из подсказки сервера', () => {
  assert.equal(runsInHint('Замер — средняя стоимость завершённых запусков агента (11)'), 11)
  assert.equal(runsInHint('Замер — без числа'), null)
  assert.equal(runsInHint(undefined), null)
})
