// Вкладка «Панель» в «До / После» (src/lib/evalPanel.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { PanelMetrics } from '../src/api/typesEvalPanel.ts'
import {
  blockingChecks,
  formatPanelMs,
  PANEL_METRICS,
  panelDelta,
  panelValue,
  rankLabel,
  rateSub,
  rateValue,
  splitChanges,
} from '../src/lib/evalPanel.ts'

const base: PanelMetrics = {
  prefixes: 10,
  errors: 0,
  suggest_n: 8,
  suggest_mrr5: 0.5,
  suggest_success5: 0.75,
  product_n: 6,
  product_mrr5: null,
  product_success5: null,
  saved_targets: 4,
  saved_keystrokes: 6.25,
  saved_share: 0.4,
  empty_suggest: 1,
  empty_suggest_rate: 0.125,
  product_prefixes: 5,
  family_dup_prefixes: 0,
  family_dup_rate: 0,
  distinct_families: 4,
  exact_checks: 3,
  exact_displaced: 1,
  suggestions_checked: 20,
  suggestions_to_empty: 0,
  to_empty_examples: [],
  p50_ms: 0.034,
  p95_ms: 12.4,
  p50_ms_cached: null,
  p95_ms_cached: null,
  products_p95_ms: null,
  partial: 0,
  llm_calls: 0,
  embed_calls: 0,
  rerank_calls: 0,
}
const def = (k: string) => PANEL_METRICS.find((d) => d.key === k)!

test('значения: индекс три знака, доли в процентах, мс с сотыми до 10, null — нет', () => {
  assert.equal(panelValue(def('suggest_mrr5'), base), '0,500')
  assert.equal(panelValue(def('suggest_success5'), base), '75 %')
  assert.equal(panelValue(def('product_mrr5'), base), null)
  assert.equal(panelValue(def('p50_ms'), base), '0,03 мс')
  assert.equal(formatPanelMs(12.4), '12 мс')
  assert.equal(panelValue(def('saved_keystrokes'), base), '6,3 симв.')
})

test('изменение: доля пустых и задержка — меньше лучше', () => {
  const after = { ...base, empty_suggest_rate: 0.3, p95_ms: 5, suggest_mrr5: 0.5 }
  assert.equal(panelDelta(def('empty_suggest_rate'), base, after), 'worse')
  assert.equal(panelDelta(def('p95_ms'), base, after), 'better')
  assert.equal(panelDelta(def('suggest_mrr5'), base, after), 'same')
  assert.equal(panelDelta(def('product_mrr5'), base, after), null)
})

test('блокирующие проверки и доли намерений', () => {
  const c = blockingChecks(base)
  assert.deepEqual(
    c.map((x) => [x.key, x.ok]),
    [
      ['exact', false],
      ['dups', true],
      ['empty', true],
      ['models', true],
    ],
  )
  assert.equal(rateValue({ num: 3, den: 8, value: 0.375 }), '37,5 %')
  assert.equal(rateValue({ num: 0, den: 0, value: null }), null)
  assert.equal(
    rateSub({ num: 3, den: 21, value: 0.1 }, ['намерение', 'намерения', 'намерений']),
    '3 из 21 намерение',
  )
  assert.equal(rankLabel(0), 'нет в первых 5')
  const s = splitChanges([
    {
      id: 'a',
      prefix: 'x',
      type: 'broad',
      target: 't',
      section: 'suggestions',
      old_rank: 1,
      new_rank: 0,
      kind: 'regression',
    },
    {
      id: 'b',
      prefix: 'y',
      type: 'broad',
      target: 't',
      section: 'products',
      old_rank: 0,
      new_rank: 1,
      kind: 'improvement',
    },
  ])
  assert.equal(s.worse.length, 1)
  assert.equal(s.better.length, 1)
})
