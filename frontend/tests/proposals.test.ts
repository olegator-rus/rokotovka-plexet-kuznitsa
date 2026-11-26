// Предложения агента (src/lib/proposals.ts): разница правил словами, строки прогона, очередь, ссылка.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Proposal } from '../src/api/types.ts'
import {
  clusterFreq,
  diffLines,
  evaluationLines,
  isTicket,
  pendingCount,
  proposalStatus,
  searchCheckUrl,
  sortByLostDemand,
  sortVersions,
} from '../src/lib/proposals.ts'

const NBSP = '\u00a0'

function proposal(over: Partial<Proposal> = {}): Proposal {
  return {
    id: 'p1',
    run_id: 'r1',
    cluster: {
      id: 'c1',
      label: 'кис',
      queries: [
        { text: 'кис на 2 устройства', freq: 6 },
        { text: 'кис', freq: 10 },
      ],
      lost_demand: 16,
    },
    diagnosis: 'ALIAS_GAP',
    diagnosis_text: '',
    role: 'ecom',
    action: 'rules_patch',
    patch: { add: [], remove: [] },
    evaluation: null,
    hypotheses_tried: 1,
    status: 'pending',
    created_at: '2026-05-27T10:00:00Z',
    decided_at: null,
    decided_reason: null,
    config_version: null,
    ...over,
  }
}

test('diffLines: алиас словами, удалённое отдельно, незнакомая форма — пары, не JSON', () => {
  const lines = diffLines({
    add: [
      {
        kind: 'alias',
        alias: 'кис',
        canonical: 'kaspersky internet security',
        target: 'family',
        target_id: 'kis',
      },
    ],
    remove: [
      { kind: 'typo', from: 'касперкий', to: 'kaspersky' },
      { kind: 'boost', field: 'vendor' },
    ],
  })
  assert.deepEqual(lines[0], {
    op: 'add',
    text: '«кис» → «kaspersky internet security»',
    note: 'алиас, односторонний · линейка kis',
  })
  assert.equal(lines[1].op, 'remove')
  assert.equal(lines[1].text, '«касперкий» → «kaspersky»')
  assert.equal(lines[1].note, 'исправление')
  assert.equal(lines[2].text, 'field: vendor')
  assert.deepEqual(diffLines(null), [])
  const card = diffLines({
    add: [
      { kind: 'card_field', field: 'keywords', before: 'антивирус', after: 'антивирус, для офиса' },
    ],
    remove: [],
  })
  assert.equal(card[0].text, 'было «антивирус» → предлагается «антивирус, для офиса»')
  assert.equal(card[0].note, 'правка карточки · поле keywords')
})

test('evaluationLines: две строки «находилось X из N, стало Y из N» и «ухудшилось K»', () => {
  const l = evaluationLines({
    target: { before: { found: 0, total: 12 }, after: { found: 11, total: 12 } },
    regression: { worse: 0, better: 2, total: 148 },
    exact_worse: 0,
    constraint_violations_new: 0,
    passed: true,
  })
  assert.ok(l)
  assert.equal(l.target, 'Целевой кластер: находилось 0 из 12, стало 11 из 12')
  assert.equal(l.regression, 'Регрессионный набор: ухудшилось 0 запросов из 148, улучшилось 2')
  assert.deepEqual(l.extra, [])
  assert.equal(l.passed, true)

  const bad = evaluationLines({
    target: { before: { found: 0, total: 12 }, after: { found: 12, total: 12 } },
    regression: { worse: 14, better: 0, total: 1500 },
    exact_worse: 1,
    constraint_violations_new: 2,
    passed: false,
  })
  assert.equal(bad?.regression, `Регрессионный набор: ухудшилось 14 запросов из 1${NBSP}500`)
  assert.deepEqual(bad?.extra, [
    'Точные запросы: ухудшилось 1 запрос',
    'Новых нарушений условий: 2',
  ])
  assert.equal(evaluationLines(null), null)
})

test('очередь: по убыванию потерянного спроса, счётчик ждущих', () => {
  const items = [
    proposal({ id: 'a', cluster: { ...proposal().cluster, lost_demand: 5 } }),
    proposal({ id: 'b', cluster: { ...proposal().cluster, lost_demand: 30 }, status: 'accepted' }),
    proposal({ id: 'c', cluster: { ...proposal().cluster, lost_demand: 12 } }),
  ]
  assert.deepEqual(
    sortByLostDemand(items).map((p) => p.id),
    ['b', 'c', 'a'],
  )
  assert.equal(pendingCount(items), 2)
})

test('«Проверить в поиске»: самый частый запрос кластера на B4', () => {
  assert.equal(searchCheckUrl(proposal()), '/search?q=%D0%BA%D0%B8%D1%81&config=B4')
  assert.equal(searchCheckUrl(proposal({ cluster: { ...proposal().cluster, queries: [] } })), null)
  assert.equal(clusterFreq(proposal()), 16)
})

test('статусы, задача вместо правки, версии новые сверху', () => {
  assert.equal(proposalStatus('rolled_back').label, 'Откачено')
  assert.equal(proposalStatus('pending').tone, 'status-pending')
  assert.equal(isTicket(proposal({ action: 'ticket' })), true)
  assert.equal(isTicket(proposal()), false)
  const v = sortVersions([
    {
      version: 1,
      created_at: '',
      author: 'system',
      description: '',
      proposal_id: null,
      diff: null,
      effect: null,
    },
    {
      version: 3,
      created_at: '',
      author: 'human',
      description: '',
      proposal_id: null,
      diff: null,
      effect: null,
    },
  ])
  assert.deepEqual(
    v.map((x) => x.version),
    [3, 1],
  )
})

test('proposalsByQuery: запрос кластера → предложение, ждущее решения важнее отклонённого', async () => {
  const { proposalsByQuery, findProposalForQuery } = await import('../src/lib/proposals.ts')
  const mk = (id: string, status: string, texts: string[]) =>
    ({
      id,
      status,
      created_at: '2026-05-27T00:00:00Z',
      cluster: {
        id: 'c' + id,
        label: texts[0],
        queries: texts.map((t) => ({ text: t, freq: 1 })),
        lost_demand: 1,
      },
    }) as unknown as Proposal
  const idx = proposalsByQuery([
    mk('a', 'rejected', ['Figma']),
    mk('b', 'pending', ['figma', 'зуп']),
  ])
  assert.equal(findProposalForQuery(idx, ' FIGMA ')?.id, 'b')
  assert.equal(findProposalForQuery(idx, 'зуп')?.id, 'b')
  assert.equal(findProposalForQuery(idx, 'кис'), null)
})
