// Отчёт SearchOps для маркетолога (src/lib/report.ts, контур фида): статус строки, ссылка на ленту, шаги строки.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { AgentStep } from '../src/api/types.ts'
import type { ReportRow } from '../src/api/typesReport.ts'
import { failLine, hasFeed, layerShort, rowStatus, rowSteps, runLink } from '../src/lib/report.ts'

function row(over: Partial<ReportRow> = {}): ReportRow {
  return {
    query: 'рутоен эцп',
    freq: 3,
    failures: 3,
    empty: 0,
    no_click: 3,
    reformulated: 0,
    layers: ['synthetic'],
    kind: 'pending',
    kind_label: 'агент ещё не разбирал',
    evidence: 'причина не доказана',
    ...over,
  }
}

function step(seq: number, over: Partial<AgentStep> = {}): AgentStep {
  return {
    id: `s${seq}`,
    run_id: 'r1',
    seq,
    ts: '2025-07-03T00:00:00Z',
    kind: 'tool_call',
    cluster_id: null,
    tool: null,
    title: `шаг ${seq}`,
    args: null,
    result: null,
    ok: true,
    hypothesis_no: null,
    rejected_reason: null,
    ms: null,
    ...over,
  }
}

test('статус строки: предложение главнее вывода кластера; без агента — «не разбирал»', () => {
  assert.equal(rowStatus(row()).label, 'агент не разбирал')
  const withAgent = row({ agent: { code: 'closed', label: 'закрыто: не решается правкой правил' } })
  assert.equal(rowStatus(withAgent).label, 'закрыто: не решается правкой правил')
  const pending = row({
    agent: { code: 'proposal', label: 'есть предложение агента', run_id: 'r1', cluster_id: 'c1' },
    proposal: {
      id: 'p1',
      status: 'pending',
      action: 'rules_patch',
      diagnosis: 'TYPO',
      run_id: 'r1',
    },
  })
  assert.deepEqual(rowStatus(pending), {
    label: 'правка ждёт утверждения',
    tone: 'status-pending',
  })
  const ticket = row({
    proposal: {
      id: 'p2',
      status: 'pending',
      action: 'ticket',
      diagnosis: 'ON_REQUEST_ONLY',
      run_id: 'r1',
    },
  })
  assert.equal(rowStatus(ticket).label, 'задача ждёт владельца')
  assert.equal(
    rowStatus(row({ agent: { code: 'unseen', label: 'не разобрано' } })).label,
    'агент не разбирал',
  )
})

test('ссылка на ленту: запуск и кластер; без кластера — запуск целиком; без запуска — нет', () => {
  const r = row({ agent: { code: 'proposal', label: '', run_id: 'r 1', cluster_id: 'c1' } })
  assert.equal(runLink(r), '/searchops/runs/r%201?cluster=c1')
  assert.equal(hasFeed(r), true)
  const noCluster = row({ agent: { code: 'closed', label: '', run_id: 'r1' } })
  assert.equal(runLink(noCluster), '/searchops/runs/r1')
  assert.equal(hasFeed(noCluster), false)
  assert.equal(runLink(row()), null)
})

test('шаги строки: память запуска один раз и шаги своего кластера по порядку', () => {
  const steps = [
    step(3, { cluster_id: 'c1', kind: 'tool_call', tool: 'search_catalog' }),
    step(1, { kind: 'memory_read' }),
    step(2, { kind: 'plan' }),
    step(4, { cluster_id: 'c2' }),
    step(5, { cluster_id: 'c1', kind: 'diagnosis' }),
    step(6, { kind: 'memory_read' }),
  ]
  const v = rowSteps(steps, 'c1')
  assert.deepEqual(
    v.map((x) => x.step.seq),
    [1, 3, 5],
  )
  assert.equal(v[1].toolLabel !== null, true)
})

test('подписи', () => {
  assert.equal(layerShort('customer'), 'заказчик')
  assert.equal(layerShort('site'), 'сайт')
  assert.equal(layerShort('synthetic'), 'синтетика')
  assert.equal(layerShort('x'), 'x')
  assert.equal(failLine({ failures: 12, freq: 16 }), '12 из 16')
})
