// Подписи и сводки трассы (src/lib/trace.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Trace, TraceStep } from '../src/api/types.ts'
import {
  parseSourceLabel,
  rankLabel,
  rejectedLabel,
  routeLabel,
  stepExtraFields,
  stepHasDetails,
  stepLabel,
  stepSummary,
  traceItemFor,
} from '../src/lib/trace.ts'

const step = (s: Partial<TraceStep> & { name: string }): TraceStep => ({ ms: 0, ...s })

test('routeLabel и parseSourceLabel', () => {
  assert.equal(routeLabel('D'), 'D · с LLM')
  assert.equal(routeLabel('X'), 'маршрут не указан')
  assert.equal(parseSourceLabel('cache'), 'кэш')
  assert.equal(parseSourceLabel('new'), 'new')
  assert.equal(parseSourceLabel(undefined), 'не указан')
})

test('stepSummary: пропущенный, нормализация, атрибуты, кандидаты', () => {
  assert.equal(stepSummary(step({ name: 'intent', skipped: true })), 'пропущен')
  assert.equal(
    stepSummary(step({ name: 'rerank', skipped: true, reason: 'not_in_config' })),
    'пропущен · не входит в конфигурацию',
  )
  assert.equal(stepSummary(step({ name: 'x', skipped: true, reason: 'odd' })), 'пропущен · odd')
  assert.equal(stepSummary(step({ name: 'normalize', changes: [] })), 'без изменений')
  assert.equal(
    stepSummary(
      step({
        name: 'normalize',
        changes: [
          { kind: 'typo', from: 'касперкий', to: 'касперский' },
          { kind: 'alias', from: 'касперский', to: 'kaspersky' },
        ],
      }),
    ),
    '2 замены',
  )
  assert.equal(
    stepSummary(
      step({
        name: 'attributes',
        found: [{ field: 'q', value: '10', strict: true, source: 'rules' }],
        unresolved: ['license_metric'],
      }),
    ),
    '1 условие, 1 не разрешено',
  )
  assert.equal(
    stepSummary(step({ name: 'bm25', top: [{ product_id: 'a', rank: 1, score: 2 }] })),
    '1 кандидат',
  )
  assert.equal(
    stepSummary(step({ name: 'route', route: 'B', signals: ['alias_hit'] })),
    'B · сленг или алиас',
  )
  assert.equal(stepSummary(step({ name: 'fusion', k: 60 })), 'k = 60')
  assert.equal(
    stepSummary(step({ name: 'business', weights: { relevance: 0.8, context: 0.05 } })),
    'релевантность 0,80 · контекст 0,05',
  )
  assert.equal(stepSummary(step({ name: 'relaxation', ladder: [] })), 'не понадобилось')
  assert.equal(stepSummary(step({ name: 'rerank', error: 'timeout' })), 'ошибка, запасной путь')
})

test('stepLabel, stepExtraFields, stepHasDetails', () => {
  assert.equal(stepLabel('bm25'), 'Текстовый поиск BM25')
  assert.equal(stepLabel('custom'), 'custom')
  const s = step({ name: 'intent', ms: 3, model: 'flash', tokens_in: 120, skipped: false })
  assert.deepEqual(stepExtraFields(s), [
    ['model', 'flash'],
    ['tokens_in', 120],
  ])
  assert.equal(stepHasDetails(s), true)
  assert.equal(stepHasDetails(step({ name: 'fusion', k: 60 })), false)
})

test('traceItemFor, rankLabel, rejectedLabel', () => {
  const trace = {
    items: [
      {
        product_id: 'p1',
        final: 0.8,
        group: 'exact',
        parts: { relevance: 0.7, popularity: 0.1, margin: 0, context: 0 },
        bm25_rank: 1,
        dense_rank: null,
      },
    ],
  } as unknown as Trace
  assert.equal(traceItemFor(trace, 'p1')?.final, 0.8)
  assert.equal(traceItemFor(trace, 'p2'), null)
  assert.equal(traceItemFor(null, 'p1'), null)
  assert.equal(rankLabel(3), '№ 3')
  assert.equal(rankLabel(null), 'нет в списке')
  assert.equal(
    rejectedLabel({
      product_id: 'x',
      reason: 'license_metric_mismatch',
      detail: 'на пользователя',
    }),
    'не та метрика лицензии: на пользователя',
  )
  assert.equal(rejectedLabel({ product_id: 'x', reason: 'odd' }), 'odd')
})

test('шаг intent: источник разбора и причина отката на правила', async () => {
  const { intentFallback } = await import('../src/lib/trace.ts')
  assert.equal(stepSummary(step({ name: 'intent', source: 'llm' })), 'LLM')
  assert.equal(
    stepSummary(step({ name: 'intent', source: 'rules', reason: 'budget_exceeded' })),
    'правила · дневной бюджет LLM исчерпан',
  )
  assert.equal(
    stepSummary(step({ name: 'intent', source: 'rules', reason: 'ip_quota' })),
    'правила · суточная квота LLM для адреса исчерпана',
  )
  const trace = { steps: [step({ name: 'intent', source: 'rules', reason: 'ip_quota' })] }
  assert.equal(intentFallback(trace), 'суточная квота LLM для адреса исчерпана')
  assert.equal(
    intentFallback({ steps: [step({ name: 'intent', skipped: true, reason: 'not_route_d' })] }),
    null,
  )
  assert.equal(intentFallback(null), null)
})
