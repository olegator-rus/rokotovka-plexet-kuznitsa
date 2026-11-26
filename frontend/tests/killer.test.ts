// Киллер-фичи контура коммерции (src/lib/killer.ts): порядок очереди, доказательства, комплект, предложение по марже и
// сверка с песочницей, входы модели эффекта, плитки «Допродаж».
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Proposal } from '../src/api/types.ts'
import {
  bundleView,
  evidenceView,
  fieldToInput,
  identityText,
  inputToField,
  isCopurchase,
  marginProposalView,
  orderEffectScenarios,
  proposalMatch,
  sortQueue,
  termRows,
  termText,
  tileValue,
} from '../src/lib/killer.ts'

const NB = ' '
const prop = (over: Record<string, unknown>) =>
  ({
    id: 'p',
    status: 'pending',
    action: 'rules_patch',
    diagnosis: 'X',
    cluster: { id: 'c', label: 'c', queries: [], lost_demand: 0 },
    created_at: '2026-05-27T10:00:00Z',
    patch: null,
    evaluation: null,
    evidence: null,
    ...over,
  }) as unknown as Proposal & { priority_score?: number | null }

test('sortQueue: оценка приоритета по убыванию, затем потерянный спрос, затем новые', () => {
  const items = [
    prop({ id: 'a', cluster: { lost_demand: 50 } }),
    prop({ id: 'b', priority_score: 69942 }),
    prop({ id: 'c', priority_score: 281918.4 }),
    prop({ id: 'd', cluster: { lost_demand: 7 } }),
    prop({
      id: 'e',
      priority_score: null,
      cluster: { lost_demand: 50 },
      created_at: '2026-05-28T00:00:00Z',
    }),
  ]
  assert.deepEqual(
    sortQueue(items).map((p) => p.id),
    ['c', 'b', 'e', 'a', 'd'],
  )
})

test('evidenceView: таблица, сила, ступени с пропуском и провалом, судья, оценка, отклонённые', () => {
  const v = evidenceView(
    prop({
      priority_score: 69942,
      evidence: {
        table: [
          { label: 'Аккаунтов', value: '6 из 41' },
          { label: '', value: 'x' },
        ],
        strength: 'сильное',
        ladder: [
          { no: 2, name: 'Совместимость', ok: false, text: 'нет' },
          { no: 1, name: 'Структура', ok: true, text: 'да' },
          { no: 6, name: 'Регрессия', ok: true, skipped: true, text: 'не нужна' },
        ],
        judge: { available: true, objection: false, reason: 'возражений нет', model: 'qwen' },
        priority: { formula: 'покупки × разрыв × маржа' },
        hypotheses: [
          { no: 1, passed: true },
          { no: 2, passed: false, hypothesis: 'Гипотеза 2: A → B', rejected_reason: 'ступень 1' },
        ],
      },
    }),
  )
  assert.deepEqual(v.table, [{ label: 'Аккаунтов', value: '6 из 41' }])
  assert.equal(v.strong, true)
  assert.deepEqual(
    v.ladder.map((r) => [r.no, r.state, r.stateLabel]),
    [
      [1, 'ok', 'пройдена'],
      [2, 'failed', 'не пройдена'],
      [6, 'skipped', 'не применялась'],
    ],
  )
  assert.equal(v.judge?.text, 'возражений нет (qwen)')
  assert.equal(v.priority?.value, `69${NB}942${NB}₽`)
  assert.equal(v.priority?.caption, 'оценка приоритета, не прогноз')
  assert.deepEqual(v.rejected, [{ no: 2, text: 'Гипотеза 2: A → B', reason: 'ступень 1' }])

  const weak = evidenceView(prop({ evidence: { judge: { available: false, objection: false } } }))
  assert.match(weak.judge?.text ?? '', /не ответил/)
  assert.equal(weak.priority, null)
})

test('isCopurchase: комплект, MISSING_RELATION, связь из co_purchase; справочник — нет', () => {
  assert.equal(isCopurchase(prop({ action: 'bundle' })), true)
  assert.equal(
    isCopurchase(prop({ action: 'relation_patch', diagnosis: 'MISSING_RELATION' })),
    true,
  )
  assert.equal(
    isCopurchase(prop({ action: 'relation_patch', evidence: { source: 'co_purchase' } })),
    true,
  )
  assert.equal(
    isCopurchase(prop({ action: 'relation_patch', diagnosis: 'BROKEN_RELATION' })),
    false,
  )
})

test('bundleView: позиции с классом, совместимостью, суммой; цена по запросу — без суммы', () => {
  const v = bundleView({
    anchor: {
      product_id: 's',
      title: 'Сервер',
      price_rub: 1149000,
      price_mode: 'from',
      margin: 0.071,
    },
    items: [
      {
        product_id: 'u',
        title: 'ИБП',
        class: 'complement',
        compatibility: 'conditional',
        condition: 'мощность ≥',
        quantity: 1,
        price_rub: 98900,
        margin: 0.17,
        attach_rate: 0.7,
        q_value: 0.0017,
        repeated: true,
        relation_status: 'proposed',
      },
    ],
    totals: { price_rub: 1437500, margin_rub: 128565, margin_pct: 0.0894, priced_all: true },
  })
  assert.ok(v)
  assert.equal(v.anchor.price, `от 1${NB}149${NB}000${NB}₽`)
  assert.equal(v.rows[0].cls, 'дополнение')
  assert.equal(v.rows[0].compat, 'совместимо при условии')
  assert.equal(v.rows[0].relation, 'связь предложена агентом')
  assert.match(v.rows[0].stats ?? '', /повторилось в позднем периоде/)
  assert.equal(v.total, `1${NB}437${NB}500${NB}₽`)
  assert.equal(v.marginTotal, `8,9${NB}% · 128${NB}565${NB}₽`)
  const q = bundleView({
    anchor: { product_id: 's', title: 'S', price_rub: null },
    items: [{ product_id: 'a', title: 'A', price_rub: null }],
    totals: { priced_all: false },
  })
  assert.equal(q?.total, '2 позиции · сумма по запросу')
  assert.equal(bundleView({ anchor: { product_id: 's', title: 'S' }, items: [] }), null)
})

const marginProp = prop({
  id: 'mp1',
  action: 'margin_policy',
  patch: { category_id: 'ide', category: 'Среды разработки', weight_before: 0.05, weight: 0.08 },
  evaluation: {
    set: 'tune',
    judged: 125,
    scope: {
      category_id: 'ide',
      category: 'Среды разработки',
      queries_in_scope: 12,
      judged_in_scope: 12,
    },
    before: { weight: 0.05, ndcg10: 0.9483, margin_index: 0.4791 },
    after: { weight: 0.08, ndcg10: 0.9483, margin_index: 0.4793, queries_with_shifts: 2 },
    margin_index_delta: 0.0002,
    synthetic: true,
  },
})

test('marginProposalView: пара чисел экрана «Маржа», проверено запросов, адрес песочницы', () => {
  const v = marginProposalView(marginProp)
  assert.deepEqual(v.ndcg, { before: '0,9483', after: '0,9483' })
  assert.deepEqual(v.index, { before: '0,4791', after: '0,4793', delta: '+0,0002' })
  assert.match(v.checked ?? '', /125 размеченных запросах набора tune, позиции категории — в 12/)
  assert.equal(v.shifted, 'переставлено в 2 запросах')
  assert.equal(v.synthetic, true)
  assert.deepEqual(v.link, { path: '/margin', query: { w: '0.08', p: 'mp1', cat: 'ide' } })
})

test('proposalMatch: те же числа — совпало; другая базовая выдача — расхождение с обоими значениями', () => {
  const ev = marginProp.evaluation as never
  const same = proposalMatch(
    ev,
    { weight: 0.05, margin_index: 0.4791, ndcg10: 0.9483 },
    {
      weight: 0.08,
      margin_index: 0.47929,
      ndcg10: 0.9483,
    },
  )
  assert.equal(same?.same, true)
  const diff = proposalMatch(
    ev,
    { weight: 0.05, margin_index: 0.4786 },
    { weight: 0.08, margin_index: 0.4788 },
  )
  assert.deepEqual(diff, { same: false, proposal: '0,4791 → 0,4793', now: '0,4786 → 0,4788' })
  assert.equal(proposalMatch(ev, null, null), null)
})

test('входы эффекта: доля — проценты 0..100, рубли ≥ 0, сумма допроданного > 0', () => {
  const share = { key: 'incrementality', unit: 'доля' }
  const rub = { key: 'mix_gp_per_order', unit: '₽' }
  const sum = { key: 'companion_sum_rub', unit: '₽' }
  assert.equal(inputToField(share, 0.25), '25')
  assert.equal(inputToField(share, 0.1184), '11,84')
  assert.equal(fieldToInput(share, '50'), 0.5)
  assert.equal(fieldToInput(share, '1,9 %'), 0.019)
  assert.equal(fieldToInput(share, '0'), 0)
  assert.equal(fieldToInput(share, '150'), null)
  assert.equal(fieldToInput(share, 'abc'), null)
  assert.equal(fieldToInput(rub, '0'), 0)
  assert.equal(fieldToInput(rub, '12 500'), 12500)
  assert.equal(fieldToInput(sum, '0'), null)
})

test('сценарии и члены: порядок, строки член за членом, знак, тождество словами', () => {
  const term = (key: string, gp: number | null, kind = 'mechanism') => ({
    key,
    label: key,
    kind,
    gp_rub_month: gp,
    formula: 'f',
  })
  const scen = (id: string, rescued: number) => ({
    id,
    label: id,
    rule: '',
    terms: [term('rescued', rescued), term('anchor_loss', -1000, 'deduction')],
    delta_gp_rub_month: rescued - 1000,
    complete: true,
    uplift: 0.1,
    check: {
      o0: 1,
      g0: 1,
      delta_o: 1,
      delta_g: 1,
      do_g0: 900,
      o0_dg: 100,
      do_dg: -10,
      sum: 990,
      direct: 990,
      terms: 990,
      ok: true,
      epsilon: 0.5,
    },
  })
  const ordered = orderEffectScenarios([
    scen('optimistic', 3000),
    scen('base', 2000),
    scen('conservative', 1000),
  ])
  assert.deepEqual(
    ordered.map((s) => s.id),
    ['conservative', 'base', 'optimistic'],
  )
  const rows = termRows(ordered)
  assert.deepEqual(
    rows.map((r) => r.key),
    ['rescued', 'anchor_loss'],
  )
  assert.deepEqual(rows[1].cells, [`−1${NB}000${NB}₽`, `−1${NB}000${NB}₽`, `−1${NB}000${NB}₽`])
  assert.equal(termText({ gp_rub_month: null }), 'не посчитано')
  assert.equal(termText({ gp_rub_month: 0.2 }), `0${NB}₽`)
  assert.equal(
    identityText(ordered[0]),
    `+900${NB}₽ + +100${NB}₽ + −10,00${NB}₽ = +990${NB}₽; напрямую +990${NB}₽; сумма членов +990${NB}₽`,
  )
})

test('tileValue: доля, рубли, число; нет значения — null', () => {
  assert.equal(tileValue({ value: 1, unit: 'share' }), `100${NB}%`)
  assert.equal(tileValue({ value: 1950.98, unit: 'rub' }), `1${NB}951${NB}₽`)
  assert.equal(tileValue({ value: 12, unit: 'count' }), '12')
  assert.equal(tileValue({ value: null, unit: 'share' }), null)
})
