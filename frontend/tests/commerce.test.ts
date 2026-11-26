// Коммерческая лестница и карточка правки связей (src/lib/commerce.ts, src/lib/relationProposal.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Proposal } from '../src/api/types.ts'
import {
  companionsEvalLines,
  economicStatus,
  ladderColumns,
  ladderReady,
  ladderRegressions,
  metricDelta,
  metricValue,
  singleChangeDeltas,
  singleChangeLabel,
  sortLadder,
  technicalStatus,
} from '../src/lib/commerce.ts'
import { relationIds, relationProposalView } from '../src/lib/relationProposal.ts'
import { diagnosisLabel } from '../src/lib/proposals.ts'
import { TOOL_LABELS } from '../src/lib/agentFeed.ts'

test('значения метрик: доли в процентах, нарушения целым, индекс — три знака, null — нет', () => {
  assert.equal(metricValue('required_recall', 0.95), '95 %')
  assert.equal(metricValue('compat_violations', 0), '0')
  assert.equal(metricValue('ndcg10', 0.8904), '0,890')
  assert.equal(metricValue('coverage', null), null)
})

test('дельта: нарушения — меньше лучше, доли в п.п.', () => {
  assert.deepEqual(metricDelta('compat_violations', 0, 3), { text: '−3', tone: 'better' })
  assert.deepEqual(metricDelta('required_recall', 0.9, 0.95), { text: '−5,0 п.п.', tone: 'worse' })
  assert.equal(metricDelta('ndcg10', 0.89, 0.89)?.tone, 'same')
  assert.equal(metricDelta('ndcg10', null, 0.89), null)
})

test('лестница: порядок C0→C3, столбцы только с числами', () => {
  const rows = sortLadder([
    { config: 'C2', metrics: { coverage: 0.7 } },
    { config: 'C0', metrics: { ndcg10: 0.89, coverage: null } },
  ])
  assert.deepEqual(
    rows.map((r) => r.config),
    ['C0', 'C2'],
  )
  assert.deepEqual(
    ladderColumns(rows).map((c) => c.key),
    ['ndcg10', 'coverage'],
  )
})

test('один компонент: улучшения и ухудшения раздельно', () => {
  const ladder = [{ config: 'C1', metrics: { ndcg10: 0.89, coverage: 0, compat_violations: 0 } }]
  const d = singleChangeDeltas(
    {
      component: 'companions',
      base: 'C1',
      metrics: { ndcg10: 0.888, coverage: 0.6, compat_violations: 0 },
    },
    ladder,
  )
  assert.deepEqual(
    d.better.map((x) => x.def.key),
    ['coverage'],
  )
  assert.deepEqual(
    d.worse.map((x) => x.def.key),
    ['ndcg10'],
  )
  assert.deepEqual(
    d.same.map((x) => x.def.key),
    ['compat_violations'],
  )
})

test('два раздельных статуса', () => {
  assert.equal(technicalStatus('verified')?.label, 'Техническая корректность: подтверждена')
  assert.equal(technicalStatus(null), null)
  assert.equal(economicStatus(null).label, 'Эффект: неизвестен, нужен замер')
  assert.deepEqual(
    companionsEvalLines({ required_recall: 0.5 }, { required_recall: 1, compat_violations: 0 }),
    ['Полнота обязательных: было 50 %, стало 100 %', 'Нарушения совместимости: 0'],
  )
})

test('карточка relation_patch: было/стало, основание, прогон, статусы, номера связей', () => {
  const old = {
    id: 'rel-9',
    from_family: 'windows-server-2025',
    to_family: 'windows-server-cal-2022',
    type: 'complement',
    requirement: 'conditional_required',
  }
  const p = {
    id: 'p1',
    action: 'relation_patch',
    diagnosis: 'BROKEN_RELATION',
    patch: { update: [{ ...old, to_family: 'windows-server-cal-2025' }], add: [], remove: [] },
    evidence: {
      relation_before: old,
      finding_text: 'CAL 2022 привязан к серверу 2025',
      license_rules: [
        {
          id: 'lr-ws-cal',
          reason: 'CAL не ниже версии сервера',
          source_url: 'https://example.org/doc',
        },
      ],
      technical_note: 'проверено кодом',
    },
    evidence_level: 'E2',
    technical_status: 'verified',
    economic_status: 'unknown',
    evaluation: {
      resolved: true,
      findings_before: 1,
      findings_after: 0,
      block: { pairs: 40, before: { required_recall: 0.5 }, after: { required_recall: 1 } },
      regression: { total: 120, worse: 0, better: 2 },
      passed: true,
    },
  } as unknown as Proposal
  const v = relationProposalView(p)
  assert.equal(v.diff.length, 1)
  assert.equal(
    v.diff[0].title,
    'windows-server-2025 → windows-server-cal-2022  ⇒  windows-server-cal-2025',
  )
  assert.equal(v.evidenceLevel, 'E2 — документ вендора')
  assert.equal(v.strength, 'сильное')
  assert.deepEqual(
    v.basis.map((b) => b.text),
    ['CAL 2022 привязан к серверу 2025', 'Правило вендора lr-ws-cal: CAL не ниже версии сервера'],
  )
  assert.equal(v.technical?.ok, true)
  assert.equal(v.economic.label, 'Эффект: неизвестен, нужен замер')
  assert.deepEqual(v.notes, ['проверено кодом'])
  assert.deepEqual(v.evalLines, [
    'Ошибки справочника: было 1, стало 0 — находка снята',
    'Полнота обязательных: было 50\u00a0%, стало 100\u00a0% (на 40 размеченных парах)',
    'Регрессия поиска: ухудшилось 0 запросов из 120, улучшилось 2',
  ])
  assert.equal(v.passed, true)
  assert.deepEqual(relationIds(p), ['rel-9'])
  assert.deepEqual(
    relationIds({
      patch: { remove: [old], update: [{ before: old, after: old }] },
    } as unknown as Proposal),
    ['rel-9', 'rel-9'],
  )
})

test('лестница бэкенда: готовность ступеней, подпись одного изменения, регрессии', () => {
  assert.equal(ladderReady({ config: 'C2', metrics: {} }, { companions: false }), false)
  assert.equal(ladderReady({ config: 'C1', metrics: {} }, { hardware: true }), true)
  assert.equal(
    singleChangeLabel({ config: 'C0+companions', changed: '+companions', metrics: {} }),
    '+ Блок допродажи',
  )
  assert.equal(
    singleChangeLabel({ config: 'C3-hardware', changed: '-hardware', metrics: {} }),
    '− Оборудование',
  )
  assert.deepEqual(
    ladderRegressions([
      {
        config: 'C2',
        metrics: {},
        regressions: [{ text: 'q', against: 'C1', ndcg_from: 0.9, ndcg_to: 0.8 }],
      },
    ]),
    [{ query: 'q', from: 'C1', to: 'C2', before: 0.9, after: 0.8 }],
  )
})

test('новые диагнозы и инструменты контура коммерции подписаны', () => {
  assert.equal(diagnosisLabel('BROKEN_RELATION'), 'Сломанная связь')
  assert.equal(diagnosisLabel('MISSING_REQUIRED'), 'Нет обязательного дополнения')
  assert.equal(TOOL_LABELS.check_relation_integrity, 'Проверка целостности связей')
})
