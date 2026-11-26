// QA r2 SearchOps: честные подписи очереди, ленты и изменений.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { AgentRun, Proposal } from '../src/api/types.ts'
import type { RunBudget } from '../src/api/typesAgent.ts'
import { budgetRows, planLine } from '../src/lib/agentRuns.ts'
import { clusterOutcomes } from '../src/lib/agentFeed.ts'
import { episodeOutcome } from '../src/lib/agentMemory.ts'
import { REMOVED_LABEL, UNKNOWN_LABEL, causeView } from '../src/lib/causes.ts'
import { formatMoney, russianQuotes } from '../src/lib/format.ts'
import {
  diagnosisLabel,
  importSummary,
  isImportVersion,
  staleReason,
  ticketLines,
  zeroFreqCluster,
} from '../src/lib/proposals.ts'
import { queueOrderLabel } from '../src/lib/queue.ts'
import { proposalSubject, proposalSubjectDat } from '../src/lib/relationProposal.ts'

const NBSP = ' '

test('№15: деньги — два знака, меньше копейки «< 0,01 ₽»; кавычки модели — «ёлочки»', () => {
  assert.equal(formatMoney(0.1784), `0,18${NBSP}₽`)
  assert.equal(formatMoney(6.6613), `6,66${NBSP}₽`)
  assert.equal(formatMoney(0.004), `<${NBSP}0,01${NBSP}₽`)
  assert.equal(formatMoney(0), `0${NBSP}₽`)
  assert.equal(
    russianQuotes('Разбор "антивирус" и "касперский"'),
    'Разбор «антивирус» и «касперский»',
  )
  assert.equal(russianQuotes('дюйм 5"'), 'дюйм 5"')
  const run = { plan: { summary: 'Проверить "антивирус"', revision: 1 } } as unknown as AgentRun
  assert.equal(planLine(run), 'Проверить «антивирус»')
})

test('№8: упор в лимит на кластер — строка «Вызовы инструментов на кластер … 15 из 15»', () => {
  const budget = {
    used: { steps: 15, tokens: 65268, seconds: 50, cost_rub: 6.6613 },
    limits: { steps: 80, tokens: 600000, seconds: 180, cost_rub: 40 },
    exhausted: '',
  } as unknown as RunBudget
  const run = {
    limits: { tool_calls_per_cluster: 15 },
    episode: { clusters: [{ label: 'антивирус', outcome: 'limit', tool_calls: 15 }] },
  } as unknown as AgentRun
  const rows = budgetRows(budget, 'step_limit', run)
  assert.equal(rows[0]!.key, 'cluster_calls')
  assert.equal(rows[0]!.label, 'Вызовы инструментов на кластер «антивирус»')
  assert.equal(`${rows[0]!.used} из ${rows[0]!.limit}`, '15 из 15')
  assert.ok(rows[0]!.exhausted)
  assert.ok(!rows.find((r) => r.key === 'steps')!.exhausted)
  assert.equal(rows.find((r) => r.key === 'cost_rub')!.used, `6,66${NBSP}₽`)
  // Без упора в лимит кластера — только бюджет запуска.
  assert.equal(budgetRows(budget, null, run)[0]!.key, 'steps')
})

test('№9: исходы limit, solved, repeat — русскими словами в ленте и в памяти', () => {
  const run = {
    episode: {
      clusters: ['limit', 'solved', 'repeat'].map((outcome) => ({
        cluster_id: outcome,
        label: 'x',
        outcome,
      })),
    },
  } as unknown as AgentRun
  for (const r of clusterOutcomes(run)) assert.ok(!/^[a-z_]+$/.test(r.outcome), r.outcome)
  for (const o of ['limit', 'solved', 'repeat'])
    assert.ok(!/^[a-z_]+$/.test(episodeOutcome({ outcome: o, cause_label: '' } as never)))
})

test('№4, №12: UNKNOWN — «требует ручной проверки», убранные виды — не «шум»', () => {
  assert.equal(causeView({ diagnosis: 'UNKNOWN', cause: 'noise' }).label, UNKNOWN_LABEL)
  assert.equal(causeView({ diagnosis: 'NOISE' }).label, 'Шум или недостаточно данных')
  assert.equal(causeView({ diagnosis: 'BUNDLE_OPPORTUNITY' }).label, REMOVED_LABEL)
  assert.notEqual(diagnosisLabel('BUNDLE_OPPORTUNITY'), 'BUNDLE_OPPORTUNITY')
  const stale = { status: 'stale', decided_reason: 'removed_p4' } as Proposal
  assert.ok(!staleReason(stale)!.includes('removed_p4'))
})

test('№6: «чертилка сапр» — без «замены не нашёл», когда поиск показывает близкие; без «0 из 1»', () => {
  const p = {
    diagnosis: 'ASSORTMENT_GAP',
    cluster: {
      label: 'чертилка сапр',
      queries: [{ text: 'чертилка сапр', freq: 0 }],
      lost_demand: 0,
    },
    evidence: {
      substitutes: [],
      current_search: [
        {
          query: 'чертилка сапр',
          total: 2,
          empty: false,
          top: [
            { title: 'nanoCAD Плюс', family_id: 'nanocad', group: 'match' },
            { title: 'Autodesk AutoCAD', family_id: 'autodesk-autocad', group: 'match' },
          ],
        },
      ],
    },
  } as unknown as Proposal
  const lines = ticketLines(p).join('\n')
  assert.ok(!lines.includes('не нашёл'), lines)
  assert.ok(!lines.includes('0 из 1'), lines)
  assert.ok(lines.includes('nanoCAD Плюс'), lines)
  assert.ok(zeroFreqCluster(p))
  const empty = { ...p, evidence: { substitutes: [], current_search: [] } } as unknown as Proposal
  assert.ok(ticketLines(empty)[0]!.includes('замены в каталоге агент не нашёл'))
})

test('№7: у правки связей — «Предложение «…»», у остальных — «Кластер «…»»', () => {
  const rel = { action: 'relation_patch', cluster: { label: 'Связь rel-0065' } } as Proposal
  const rules = { action: 'rules_patch', cluster: { label: 'кис' } } as Proposal
  assert.equal(proposalSubject(rel), 'Предложение «Связь rel-0065»')
  assert.equal(proposalSubject(rules), 'Кластер «кис»')
  assert.equal(proposalSubjectDat(rel), 'по предложению «Связь rel-0065»')
})

test('№11: версия-импорт — «загружено N алиасов (было M)», без чисел — нейтрально', () => {
  assert.ok(isImportVersion({ source: 'import', description: '' }))
  assert.equal(
    importSummary({ aliases: 100 }, { aliases: 64 }),
    'Импорт словарей и правил из каталога: загружено 100 алиасов (было 64)',
  )
  assert.equal(importSummary({}, null), 'Импорт словарей и правил из каталога')
})

test('№16: у всех спрос 0 — подпись «новые сверху», а не «по убыванию спроса»', () => {
  const p = (lost: number, priority?: number) =>
    ({ cluster: { lost_demand: lost }, priority_score: priority }) as unknown as Proposal
  assert.match(queueOrderLabel([p(0), p(0)]), /новые сверху/)
  assert.match(queueOrderLabel([p(3), p(0)]), /по убыванию потерянного спроса/)
  assert.match(queueOrderLabel([p(0, 100), p(0)]), /по оценке приоритета/)
})
