// Панель поиска контура выдачи (src/lib/panel.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  bottomTogether,
  DESKTOP_LIMITS,
  MOBILE_LIMITS,
  RECENT_KEY,
  ResponseCache,
  Sequencer,
  buildOptions,
  cacheKey,
  loadRecent,
  moveActive,
  offeredRefinements,
  panelPrice,
  productLines,
  refinedQuery,
  refinementFilters,
  saveRecent,
  suggestionParts,
  toPanelView,
  togetherFor,
  togetherReason,
  withRecent,
  withoutRecent,
  type OptionsInput,
} from '../src/lib/panel.ts'
import type {
  PanelProduct,
  PanelRefinement,
  PanelSuggestResponse,
  PanelTogether,
} from '../src/api/typesPanel.ts'

const product = (i: number, over: Partial<PanelProduct> = {}): PanelProduct => ({
  family_id: `f${i}`,
  family_name: `Семейство ${i}`,
  product_id: `p${i}`,
  title: `Товар ${i}`,
  kind: 'software',
  vendor: 'Вендор',
  category: 'Антивирусы',
  mpn: '',
  license_line: 'Подписка · 1 год',
  specs_line: '',
  price_from_rub: 1000,
  price_mode: 'from',
  variants_count: 3,
  exact: false,
  sku_exact: false,
  ...over,
})

const ref = (label: string, over: Partial<PanelRefinement> = {}): PanelRefinement => ({
  label,
  query: `kaspersky ${label}`,
  field: null,
  value: null,
  op: null,
  hits: 5,
  ...over,
})

const wide: PanelSuggestResponse = {
  groups: [],
  suggestions: Array.from({ length: 7 }, (_, i) => ({ text: `антивирус ${i}`, kind: 'query' })),
  categories: [
    { id: 'c1', name: 'Антивирусы', share: 0.7 },
    { id: 'c2', name: 'Защита', share: 0.2 },
    { id: 'c3', name: 'Лишняя', share: 0.1 },
  ],
  vendors: [{ id: 'v1', name: 'Касперский' }],
  products: [
    product(1),
    product(2, { family_id: 'f1' }),
    ...[3, 4, 5, 6, 7, 8].map((i) => product(i)),
  ],
  refinements: Array.from({ length: 8 }, (_, i) => ref(`r${i}`)),
  total_hint: 37,
  meta: { partial: false, ms: { total: 12 } },
}

test('toPanelView: максимумы секций, семейства без дублей, без полей контура выдачи — legacy', () => {
  const v = toPanelView(wide, DESKTOP_LIMITS)
  assert.equal(v.legacy, false)
  assert.equal(v.suggestions.length, 5)
  assert.equal(v.categories.length, 2)
  assert.equal(v.refinements.length, 6)
  assert.equal(v.products.length, 6)
  assert.deepEqual(
    v.products.map((p) => p.family_id),
    ['f1', 'f3', 'f4', 'f5', 'f6', 'f7'],
  )
  assert.equal(v.totalHint, 37)
  assert.equal(v.serverMs, 12)
  const m = toPanelView(wide, MOBILE_LIMITS)
  assert.equal(m.products.length, 3)
  assert.equal(m.suggestions.length, 4)
  assert.equal(toPanelView({ groups: [] }).legacy, true)
  assert.equal(toPanelView(null).legacy, true)
})

test('toPanelView: точный артикул — одна карточка, без бейджей и категорий', () => {
  const v = toPanelView({
    ...wide,
    products: [product(1), product(9, { sku_exact: true, exact: true })],
  })
  assert.deepEqual(
    v.products.map((p) => p.product_id),
    ['p9'],
  )
  assert.equal(v.refinements.length, 0)
  assert.equal(v.categories.length, 0)
})

const base = (over: Partial<OptionsInput> = {}): OptionsInput => ({
  idle: false,
  recent: [],
  popular: [],
  view: toPanelView(wide),
  productsStale: false,
  recs: null,
  recsLimit: 3,
  mobile: false,
  withAll: true,
  ...over,
})

test('buildOptions: линейный порядок — левая колонка, товары, рекомендации, «Все результаты»', () => {
  const recs = {
    basis: 'category_popular',
    title: 'Вам может быть интересно',
    why: '',
    items: [{ product_id: 'r1', title: 'R1' }] as never[],
  }
  const kinds = buildOptions(base({ recs })).map((o) => o.kind)
  const firstProduct = kinds.indexOf('product')
  assert.ok(kinds.lastIndexOf('suggestion') < kinds.indexOf('category'))
  assert.ok(kinds.lastIndexOf('vendor') < firstProduct)
  assert.ok(kinds.lastIndexOf('product') < kinds.indexOf('rec'))
  assert.equal(kinds.at(-1), 'all')
  const ids = buildOptions(base()).map((o) => o.id)
  assert.equal(new Set(ids).size, ids.length)
})

test('buildOptions: мобильный порядок — товары сразу после подсказок, рекомендации последними', () => {
  const recs = {
    basis: 'spec',
    title: 'К вашей спецификации',
    why: '',
    items: [{ product_id: 'r1', title: 'R1' }] as never[],
  }
  const kinds = buildOptions(base({ mobile: true, recs })).map((o) => o.kind)
  assert.ok(kinds.indexOf('product') < kinds.indexOf('category'))
  assert.ok(kinds.indexOf('all') < kinds.indexOf('rec'))
  assert.equal(kinds.at(-1), 'rec_add')
})

test('buildOptions: товары прежнего запроса в обход не входят; точный SKU даёт «В спецификацию»', () => {
  assert.equal(
    buildOptions(base({ productsStale: true })).filter((o) => o.kind === 'product').length,
    0,
  )
  const exact = toPanelView({ ...wide, products: [product(9, { sku_exact: true })] })
  const kinds = buildOptions(base({ view: exact })).map((o) => o.kind)
  assert.deepEqual(
    kinds.filter((k) => k.startsWith('product')),
    ['product', 'product_add'],
  )
})

test('buildOptions: пустая строка — недавние, затем популярные, без «Все результаты»', () => {
  const opts = buildOptions(
    base({
      idle: true,
      withAll: false,
      recent: ['a', 'b'],
      popular: [{ text: 'p', kind: 'query' }],
    }),
  )
  assert.deepEqual(
    opts.map((o) => o.kind),
    ['recent', 'recent', 'popular'],
  )
})

test('moveActive: первый элемент не выделен, выход за край возвращает в строку', () => {
  assert.equal(moveActive(-1, 3, 1), 0)
  assert.equal(moveActive(-1, 3, -1), 2)
  assert.equal(moveActive(2, 3, 1), -1)
  assert.equal(moveActive(0, 3, -1), -1)
  assert.equal(moveActive(-1, 0, 1), -1)
})

test('suggestionParts: набранное обычным, продолжение выделено', () => {
  assert.deepEqual(suggestionParts('kaspersky endpoint', 'kasp', [[0, 4]]), [
    { text: 'kasp', continuation: false },
    { text: 'ersky endpoint', continuation: true },
  ])
  assert.deepEqual(suggestionParts('Kaspersky', 'kasp', null), [
    { text: 'Kasp', continuation: false },
    { text: 'ersky', continuation: true },
  ])
  assert.deepEqual(suggestionParts('каспер', 'rfcg', null), [
    { text: 'каспер', continuation: false },
  ])
})

test('недавние: localStorage, повтор наверх, не больше 5, сломанное хранилище — пусто', () => {
  const mem = new Map<string, string>()
  const storage = {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => void mem.set(k, v),
    removeItem: (k: string) => void mem.delete(k),
  }
  let list: string[] = []
  for (const q of ['a', 'b', 'c', 'd', 'e', 'f', ' B ']) list = withRecent(list, q)
  assert.deepEqual(list, ['B', 'f', 'e', 'd', 'c'])
  saveRecent(storage, list)
  assert.deepEqual(loadRecent(storage), list)
  assert.deepEqual(withoutRecent(list, 'f'), ['B', 'e', 'd', 'c'])
  saveRecent(storage, [])
  assert.equal(mem.has(RECENT_KEY), false)
  const broken = {
    getItem: () => {
      throw new Error('denied')
    },
    setItem: () => {
      throw new Error('denied')
    },
    removeItem: () => undefined,
  }
  assert.deepEqual(loadRecent(broken), [])
  assert.doesNotThrow(() => saveRecent(broken, ['x']))
  mem.set(RECENT_KEY, '{oops')
  assert.deepEqual(loadRecent(storage), [])
})

test('Sequencer и кэш: устаревший ответ не показывается, кэш вытесняет старое', () => {
  const s = new Sequencer()
  const a = s.next()
  const b = s.next()
  assert.equal(s.isLatest(a), false)
  assert.equal(s.isLatest(b), true)
  const c = new ResponseCache<number>(2)
  c.set('a', 1)
  c.set('b', 2)
  c.set('c', 3)
  assert.equal(c.get('a'), undefined)
  assert.equal(c.size, 2)
  assert.equal(cacheKey('  Kasp  Ersky '), 'kasp ersky')
})

test('уточнения: структурированное — условие без правки текста, применённые не повторяются', () => {
  const year = ref('+ 1 год', {
    query: 'kaspersky 1 год',
    field: 'duration_months',
    value: 12,
    op: 'eq',
  })
  const biz = ref('для бизнеса', {
    query: 'kaspersky 1 год для бизнеса',
    field: 'segment',
    value: 'b2b',
    op: 'eq',
  })
  assert.equal(refinedQuery('kaspersky', [year]), 'kaspersky 1 год')
  assert.equal(refinedQuery('kaspersky', [year, biz]), 'kaspersky 1 год для бизнеса')
  assert.equal(refinedQuery('kaspersky', [biz]), 'kaspersky для бизнеса')
  assert.deepEqual(refinementFilters([year]), [{ key: 'duration_months', op: 'eq', value: 12 }])
  assert.deepEqual(
    offeredRefinements([year, biz, ref('пусто', { hits: 0 })], [year]).map((r) => r.label),
    ['для бизнеса'],
  )
})

test('карточка: строки по типу и цена', () => {
  const sw = productLines(product(1, { variants_count: 7 }))
  assert.equal(sw.primary, 'Семейство 1')
  assert.equal(sw.count, '7 вариантов лицензии')
  const hw = productLines(
    product(2, { kind: 'hardware', mpn: 'R740-1', specs_line: '2×Xeon', variants_count: 2 }),
  )
  assert.equal(hw.secondary, 'R740-1')
  assert.equal(hw.count, '2 конфигурации')
  const fmt = (n: number) => `${n} ₽`
  assert.equal(panelPrice({ price_from_rub: 990, price_mode: 'from' }, fmt), 'от 990 ₽')
  assert.equal(panelPrice({ price_from_rub: 990, price_mode: 'fixed' }, fmt), '990 ₽')
  assert.equal(panelPrice({ price_from_rub: null, price_mode: 'fixed' }, fmt), 'Цена по запросу')
})

test('«Вместе с этим»: к первому семейству панели, без повторов, не больше двух; причина без дат', () => {
  const first = {
    family_id: 'astra',
    product_id: 'p1',
  } as unknown as Parameters<typeof togetherFor>[0]
  const t = (id: string, over: Partial<PanelTogether> = {}): PanelTogether => ({
    anchor_family_id: 'astra',
    family_id: `f-${id}`,
    family_name: `Семейство ${id}`,
    product_id: id,
    title: `Позиция ${id}`,
    vendor: 'v',
    price_rub: 100,
    reason: 'Защита рабочих мест',
    level: 'B',
    kind: 'class_logic',
    label: 'Антивирус',
    ...over,
  })
  assert.deepEqual(
    togetherFor(first, [
      t('a'),
      t('dup', { family_id: 'f-a' }),
      t('other', { anchor_family_id: 'kes' }),
      t('self', { family_id: 'astra' }),
      t('b'),
      t('c'),
    ]).map((x) => x.product_id),
    ['a', 'b'],
  )
  assert.deepEqual(togetherFor(undefined, [t('a')]), [])
  assert.deepEqual(togetherFor(first, undefined), [])
  assert.equal(
    togetherReason({
      reason: 'Совместимо с Astra Linux SE 1.7 — сертификат № 1 от 23.01.2025',
      label: 'x',
    }),
    'Совместимо с Astra Linux SE 1.7 — сертификат № 1',
  )
  assert.equal(togetherReason({ reason: '', label: 'Хранилище' }), 'Хранилище')
})

test('«Вместе с этим» в обходе стрелками — в нижнем блоке, после товаров', () => {
  const view = toPanelView({
    suggestions: [],
    products: [
      { family_id: 'astra', product_id: 'p1', title: 'Astra' },
      { family_id: 'alt', product_id: 'p2', title: 'Alt' },
    ] as unknown as PanelSuggestResponse['products'],
    together: [
      {
        anchor_family_id: 'astra',
        family_id: 'kes',
        family_name: 'KES',
        product_id: 'k1',
        title: 'KES 1',
        vendor: 'K',
        price_rub: 3307.77,
        reason: 'Защита рабочих мест',
        level: 'A',
        kind: 'compat_document',
        label: 'Антивирус',
      },
    ],
  })
  const ids = buildOptions({
    idle: false,
    recent: [],
    popular: [],
    view,
    productsStale: false,
    recs: null,
    recsLimit: 3,
    mobile: false,
    withAll: true,
  }).map((o) => o.id)
  assert.deepEqual(ids, [
    'panel-opt-product-0',
    'panel-opt-product-1',
    'panel-opt-together-0',
    'panel-opt-all-0',
  ])
})

test('нижний блок: проверенные дополнения важнее «Вместе с этим», подборка по категории — уступает', () => {
  const together = [{ family_id: 'kes' }] as unknown as Parameters<typeof bottomTogether>[0]
  const recs = (basis: string) =>
    ({ basis, items: [{}] }) as unknown as Parameters<typeof bottomTogether>[1]
  assert.equal(bottomTogether(together, null, 3).length, 1)
  assert.equal(bottomTogether(together, recs('category_popular'), 3).length, 1)
  assert.equal(bottomTogether(together, recs('spec'), 3).length, 0)
  assert.equal(bottomTogether(together, recs('exact_sku'), 3).length, 0)
})
