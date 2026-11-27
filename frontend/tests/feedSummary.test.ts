// Сводка «На данных заказчика» (src/lib/feedSummary.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { FeedClosedDetail, FeedSummary } from '../src/api/typesFeed.ts'
import {
  closedCells,
  closedExamples,
  closedFacts,
  closedHeldOut,
  closedKind,
  closedModelShare,
  closedPending,
  configLabel,
  hasFeedRuns,
  headlineCost,
  isMockCatalog,
  modeLabel,
  outOf,
  setLabel,
  shareOrNull,
} from '../src/lib/feedSummary.ts'

const NBSP = '\u00a0'

function summary(over: Partial<FeedSummary> = {}): FeedSummary {
  return {
    catalog: { source: 'feed', label: 'фид заказчика', offers: 136040 },
    etalon: null,
    closed: [],
    four: null,
    cost: null,
    table: [],
    configs: [
      { id: 'BM25', label: 'BM25', note: '' },
      { id: 'B2', label: 'Гибрид', note: '' },
      { id: 'live', label: 'Полный продукт', note: '' },
    ],
    mock_runs: true,
    ...over,
  }
}

test('закрытая часть: до прогона — три пустые ячейки и признак «не запускался»', () => {
  const s = summary()
  assert.equal(closedPending(s), true)
  const cells = closedCells(s)
  assert.deepEqual(
    cells.map((c) => [c.config, c.label, c.score]),
    [
      ['BM25', 'BM25', null],
      ['B2', 'Гибрид', null],
      ['live', 'Полный продукт', null],
    ],
  )
})

test('закрытая часть: числа по своим конфигурациям, порядок BM25 → гибрид → продукт', () => {
  const sc = (config: string, matched: number) => ({
    set: 'closed',
    config,
    matched,
    total: 80,
    success1: matched / 80,
    top4: null,
    run_tail: 'abc123',
  })
  const s = summary({ closed: [sc('live', 70), sc('BM25', 30)] })
  assert.equal(closedPending(s), false)
  const cells = closedCells(s)
  assert.equal(cells[0]!.score?.matched, 30)
  assert.equal(cells[1]!.score, null)
  assert.equal(cells[2]!.score?.matched, 70)
})

test('отложенный набор: подписи видов и примеры — по ключу, неизвестный вид как есть', () => {
  assert.deepEqual(closedKind('раскладка'), {
    label: 'Чужая раскладка',
    examples: ['fcnhf kbyerc', 'ьшскщыщае 365'],
  })
  assert.equal(
    closedKind('по запросу → альтернатива').label,
    'Продукт без цены, рядом альтернатива',
  )
  assert.equal(closedExamples('склонение'), '«астру линукс», «сканеру-вс»')
  assert.equal(closedExamples('сервисный'), '«как оплатить заказ»')
  assert.deepEqual(closedKind('новый вид'), { label: 'новый вид', examples: [] })
  assert.equal(closedExamples('новый вид'), '')
  assert.deepEqual(closedKind('constructor'), { label: 'constructor', examples: [] })
})

test('отложенный набор: факты и фразы — только из чисел сервера, слова согласованы с числом', () => {
  const d: FeedClosedDetail = {
    total: 1200,
    bm25: 300,
    live: 900,
    top4: 1100,
    model_searches: 21,
    p50_ms: 120.5,
    kinds: [],
    spelling: { total: 40, bm25: 5, live: 31, top4: 38 },
    product: { total: 61, bm25: 20, live: 50 },
  }
  assert.deepEqual(closedFacts(d), [
    {
      value: `900 из 1${NBSP}200`,
      text: 'целевая страница первым ответом. У текстового поиска — 300',
    },
    { value: `1${NBSP}100 из 1${NBSP}200`, text: 'нужная страница в первых четырёх ответах' },
    {
      value: '38 из 40',
      text: 'запросы с искажённым написанием: нужная страница в первых четырёх. Первым ответом — 31, у текстового поиска — 5',
    },
    {
      value: '50 против 20',
      text: 'из 61 запроса, где нужно найти страницу товара (без вопросов и мусора)',
    },
  ])
  assert.equal(
    closedFacts({ ...d, product: { total: 69, bm25: 2, live: 3 } })[3]!.text.slice(0, 14),
    'из 69 запросов',
  )
  assert.equal(closedHeldOut(83), '83 запроса были отложены')
  assert.equal(closedHeldOut(80), '80 запросов были отложены')
  assert.equal(closedHeldOut(81), '81 запрос был отложен')
  assert.equal(closedModelShare(d), `на 21 запросе из 1${NBSP}200`)
  assert.equal(closedModelShare({ ...d, model_searches: 12, total: 83 }), 'на 12 запросах из 83')
})

test('подписи и доли', () => {
  assert.equal(outOf(10, 10), '10 из 10')
  assert.equal(outOf(1234, 2000), `1${NBSP}234 из 2${NBSP}000`)
  assert.equal(shareOrNull(0.95), `95${NBSP}%`)
  assert.equal(shareOrNull(null), null)
  assert.equal(setLabel('closed'), 'Отложенный набор (закрытая часть)')
  assert.equal(setLabel('x'), 'x')
  assert.equal(configLabel(summary(), 'B2'), 'Гибрид')
  assert.equal(configLabel(null, 'B1'), 'B1')
  assert.equal(modeLabel('cold'), 'холодный')
  assert.equal(modeLabel('warm'), 'тёплый')
  assert.equal(modeLabel('journal'), 'журнал витрины')
})

test('стоимость для плитки — тёплый прогон, без него холодный; прогонов нет — пусто', () => {
  const run = (mode: string, rub: number) => ({
    mode,
    sets: ['etalon'],
    searches: 10,
    route_d_label: 0.2,
    route_d_no_model: 1,
    p: 0.1,
    h: mode === 'warm' ? 1 : 0,
    c_llm: 0.15,
    llm_calls: 1,
    input_tokens: 800,
    output_tokens: 100,
    llm_rub: 0.1,
    p50_ms: 80,
    p95_ms: 300,
    llm_per_request_rub: 0.01,
    fixed_per_request_rub: 0.57,
    per_request_rub: rub,
    month_rub: rub * 15000,
    infra_share: 0.9,
    agent_share: 0.08,
    support_share: 0,
    llm_share: 0.02,
  })
  const cost = {
    formula: 'C',
    requests: 15000,
    params: [],
    cold: run('cold', 0.6),
    warm: null,
    journal: null,
    notes: [],
  }
  assert.equal(headlineCost(summary({ cost }))?.mode, 'cold')
  assert.equal(headlineCost(summary({ cost: { ...cost, warm: run('warm', 0.58) } }))?.mode, 'warm')
  assert.equal(headlineCost(summary()), null)
  assert.equal(hasFeedRuns(summary()), false)
  assert.equal(hasFeedRuns(summary({ cost })), true)
})

test('mock-каталог по версии', () => {
  assert.equal(isMockCatalog('mock-1'), true)
  assert.equal(isMockCatalog('softline-x'), false)
  assert.equal(isMockCatalog(null), false)
})

test('номер прогона и каталог — без дат', async () => {
  const { runTail, catalogLabel } = await import('../src/lib/feedSummary.ts')
  assert.equal(runTail('20260528T120920-ece6d7'), '…ece6d7')
  assert.equal(runTail(''), '—')
  assert.equal(catalogLabel('mock-1'), 'mock-каталог, до данных заказчика')
  assert.equal(catalogLabel('softline-feed'), 'фид заказчика')
  assert.equal(catalogLabel('fixture-1'), 'fixture-1')
})
