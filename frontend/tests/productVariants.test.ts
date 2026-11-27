// Варианты на странице товара: различающиеся колонки, порядок, свёрнутый список, скидка, подсказки помощнику
// (src/lib/productVariants.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { ProductLicense } from '../src/api/typesProduct.ts'
import {
  collapsedRows,
  discountView,
  guideQuestions,
  shortTitle,
  specQuantity,
  variantQuantity,
  titleTails,
  variantColumns,
  variantTable,
  type VariantSource,
} from '../src/lib/productVariants.ts'

const lic = (l: Partial<ProductLicense>): ProductLicense => ({
  type: 'subscription',
  metric: null,
  quantity: null,
  duration_months: null,
  edition: '',
  ...l,
})
const v = (
  id: string,
  l: Partial<ProductLicense>,
  price: number | null,
  title = 'PhotoDirector',
): VariantSource => ({
  product_id: id,
  title,
  license_line: '',
  license: lic(l),
  price_rub: price,
  price_on_request: price === null,
  price_mode: price === null ? 'quote_required' : 'fixed',
})

test('колонки — только различающиеся; одинаковые строки с разными названиями получают «Позиция»', () => {
  // Как у PhotoDirector: все «Подписка · для: госучреждение», различаются редакция, ступень и платформа в названии.
  const cur = v('win', {}, 12705, 'PhotoDirector, Windows')
  const others = [
    v('mac', {}, 12705, 'PhotoDirector, Mac'),
    v(
      'edu-60',
      {
        edition: 'Education',
        tier_min: 60,
        tier_max: 119,
        quantity: 60,
        org_type: 'government',
        org_label: 'Госучреждение',
      },
      5489,
    ),
    v(
      'edu-120',
      {
        edition: 'Education',
        tier_min: 120,
        tier_max: 250,
        quantity: 120,
        org_type: 'government',
        org_label: 'Госучреждение',
      },
      4940,
    ),
  ]
  const t = variantTable(cur, others)
  assert.deepEqual(
    t.columns.map((c) => c.key),
    ['title', 'edition', 'quantity', 'org'],
  )
  assert.equal(t.rows.length, 4)
  assert.equal(t.rows.filter((r) => r.current).length, 1)
  // Срок не задан ни у кого — колонки «Срок» нет, «бессрочная» не появляется.
  assert.ok(t.rows.every((r) => r.cells.term === '—'))
  const edu = t.rows.find((r) => r.productId === 'edu-60')!
  assert.equal(edu.cells.quantity, '60–119')
  assert.equal(edu.perPlace, 'за узел')
})

test('порядок: новая → продление, организация, срок, количество, цена', () => {
  const cur = v(
    'a',
    {
      kind: 'base',
      metric: 'device',
      tier_min: 100,
      tier_max: 149,
      duration_months: 12,
      org_type: 'commercial',
    },
    3710.7,
  )
  const others = [
    v(
      'r',
      {
        kind: 'renewal',
        type: 'renewal',
        metric: 'device',
        tier_min: 100,
        tier_max: 149,
        duration_months: 12,
        org_type: 'commercial',
      },
      2000,
    ),
    v(
      'b2',
      {
        kind: 'base',
        metric: 'device',
        tier_min: 100,
        tier_max: 149,
        duration_months: 24,
        org_type: 'commercial',
      },
      7000,
    ),
    v(
      'b150',
      {
        kind: 'base',
        metric: 'device',
        tier_min: 150,
        tier_max: 249,
        duration_months: 12,
        org_type: 'commercial',
      },
      3430.8,
    ),
    v(
      'edu',
      {
        kind: 'base',
        metric: 'device',
        tier_min: 100,
        tier_max: 149,
        duration_months: 12,
        org_type: 'academic',
        org_label: 'Образовательное учреждение',
      },
      900,
    ),
  ]
  const t = variantTable(cur, others)
  assert.deepEqual(
    t.rows.map((r) => r.productId),
    ['a', 'b150', 'b2', 'edu', 'r'],
  )
  const a = t.rows[0]!
  assert.equal(a.cells.kind, 'Подписка')
  assert.equal(a.cells.quantity, '100–149 устройств')
  assert.equal(a.cells.org, 'Коммерческая организация')
  assert.equal(t.rows.at(-1)!.cells.kind, 'Продление')
})

test('количество: организация, «от N», точное число', () => {
  assert.equal(variantQuantity(lic({ metric: 'org' })), 'На организацию')
  assert.equal(
    variantQuantity(lic({ metric: 'user', tier_min: 250, tier_max: null })),
    'от 250 пользователей',
  )
  assert.equal(variantQuantity(lic({ metric: 'device', quantity: 2 })), '2 устройства')
  assert.equal(variantQuantity(lic({ quantity: 11 })), '11')
  assert.equal(variantQuantity(lic({})), '')
})

test('свёрнутый список: немного — всё; много — та же группа вокруг текущей, текущая видна', () => {
  const cur = v('c', { kind: 'base', metric: 'device', quantity: 50 }, 50)
  const same = Array.from({ length: 30 }, (_, i) =>
    v(`s${i}`, { kind: 'base', metric: 'device', quantity: i + 1 }, i + 1),
  )
  const other = Array.from({ length: 20 }, (_, i) =>
    v(`r${i}`, { kind: 'renewal', metric: 'device', quantity: i + 1 }, i + 1),
  )
  const t = variantTable(cur, [...same, ...other])
  const shown = collapsedRows(t.rows)
  assert.equal(shown.length, 10)
  assert.ok(shown.some((r) => r.current))
  assert.ok(shown.every((r) => r.cells.kind === 'Подписка'))
  const small = variantTable(cur, same.slice(0, 5))
  assert.equal(collapsedRows(small.rows).length, 6)
})

test('скидка — только если старая цена выше; цена по запросу без скидки', () => {
  assert.deepEqual(discountView({ price_rub: 800, old_price_rub: 1000, price_on_request: false }), {
    old: '1\u00a0000\u00a0₽',
    badge: 'Скидка 20%',
  })
  assert.deepEqual(discountView({ price_rub: 800, old_price_rub: 800, price_on_request: false }), {
    old: '',
    badge: '',
  })
  assert.deepEqual(discountView({ price_rub: null, old_price_rub: 800, price_on_request: true }), {
    old: '',
    badge: '',
  })
})

test('подсказки помощнику: реальное число мест вместо «N», редакции — если их 2+', () => {
  const kes = v('k', { metric: 'device', tier_min: 100, tier_max: 149, quantity: 100 }, 3710.7)
  const q = guideQuestions(kes, [v('k2', { metric: 'device', quantity: 150 }, 3430.8)])
  assert.deepEqual(
    q.map((x) => x.label),
    ['Чем отличаются варианты?', 'Сколько стоит на 100 устройств?'],
  )
  const ed = guideQuestions(v('a', { edition: 'Стандартный', quantity: 1 }, null), [
    v('b', { edition: 'Профессиональный' }, null),
  ])
  assert.equal(ed[0]!.label, 'Чем отличаются редакции?')
  assert.equal(ed[1]!.label, 'Посчитать на моё число мест')
  assert.ok(!ed.some((x) => /\bN\b/.test(x.label)))
  assert.deepEqual(
    guideQuestions(v('solo', {}, 100), []).map((x) => x.label),
    [],
  )
  assert.deepEqual(
    guideQuestions(v('solo', { quantity: 1 }, 100), []).map((x) => x.label),
    ['Посчитать на моё число мест'],
  )
})

test('количество в спецификацию: нижняя граница ступени, иначе 1', () => {
  assert.equal(specQuantity(lic({ tier_min: 100, tier_max: 149 })), 100)
  assert.equal(specQuantity(lic({ quantity: 5 })), 1)
})

test('«Позиция»: без производителя в начале и хвостовой запятой', () => {
  assert.equal(
    shortTitle(
      'CyberLink Corporation Cyberlink PhotoDirector 365 (подписка Win/Mac),',
      'CyberLink Corporation',
    ),
    'Cyberlink PhotoDirector 365 (подписка Win/Mac)',
  )
  assert.equal(shortTitle('Astra Linux SE', 'Группа Астра'), 'Astra Linux SE')
})

test('колонки свёрнутого списка — по показанным строкам: одинаковый вид не повторяется', () => {
  const cur = v('c', { kind: 'base', type: 'perpetual', edition: 'Смоленск' }, 136300, 'ALSE диск')
  const others = [
    v('d', { kind: 'base', type: 'perpetual', edition: 'Смоленск' }, 130300, 'ALSE электронный'),
    v('r', { kind: 'support', duration_months: 12, edition: 'Смоленск' }, 294, 'ALSE поддержка'),
  ]
  const t = variantTable(cur, others)
  assert.deepEqual(
    t.columns.map((c) => c.key),
    ['title', 'kind', 'term'],
  )
  const same = t.rows.filter((r) => r.cells.kind === 'Бессрочная')
  assert.deepEqual(
    variantColumns(same).map((c) => c.key),
    ['title'],
  )
  assert.equal(same[0]!.cells.term, 'Бессрочная')
})

test('«Позиция»: общее начало названий от трёх слов — «…», различающийся хвост остаётся', () => {
  const t = variantTable(
    v('a', {}, null, 'МойОфис Стандартный 3 (корпоративная лицензия) Корпоративная на 1'),
    [v('b', {}, null, 'МойОфис Стандартный 3 (корпоративная лицензия) Корпоративная премиальная')],
  )
  const tails = titleTails(t.rows)
  assert.equal(tails.get('a'), '…на 1')
  assert.equal(tails.get('b'), '…премиальная')
  const short = variantTable(v('w', {}, 1, 'PhotoDirector, Windows'), [
    v('m', {}, 1, 'PhotoDirector, Mac'),
  ])
  assert.equal(titleTails(short.rows).get('m'), 'PhotoDirector, Mac')
})
