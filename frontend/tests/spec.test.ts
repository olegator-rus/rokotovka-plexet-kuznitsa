// Спецификация: итог, происхождение, количество, тело запроса КП, хранение (src/lib/spec.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItemBase } from '../src/api/types.ts'
import {
  addLine,
  clampQty,
  lineFromCompanion,
  lineFromItem,
  anchorOf,
  originLabel,
  parseStored,
  priceModeLabel,
  quantityForItem,
  quoteRequest,
  removeLine,
  setQuantity,
  specText,
  summarize,
} from '../src/lib/spec.ts'
import { companion } from './p2fixtures.ts'

/** Неразрывные пробелы форматтера — в обычные, чтобы ожидания читались. */
const sp = (s: string | undefined) => (s ?? '').replace(/\u00a0/g, ' ')

const item = (id: string, over: Partial<SearchItemBase> = {}): SearchItemBase => ({
  product_id: id,
  family_id: 'ws',
  title: id,
  vendor: 'Microsoft',
  category: 'Серверные ОС',
  license_line: 'Бессрочная · 16 ядер',
  price_rub: 100000,
  price_on_request: false,
  group: 'exact',
  matches_query: true,
  purchased_before: false,
  final: 0.9,
  ...over,
})

test('позиция из выдачи: режим цены по полю контура коммерции или по старому флагу', () => {
  assert.equal(lineFromItem(item('a')).price_mode, 'fixed')
  assert.equal(lineFromItem(item('a', { price_on_request: true })).price_mode, 'quote_required')
  assert.equal(lineFromItem(item('a', { price_mode: 'from' })).price_mode, 'from')
  assert.equal(lineFromItem(item('a'), 50).quantity, 50)
})

test('повторное добавление не дублирует строку, а берёт новое количество', () => {
  let lines = addLine([], lineFromItem(item('a'), 1))
  lines = addLine(lines, lineFromItem(item('b'), 2))
  lines = addLine(lines, lineFromItem(item('a'), 5))
  assert.deepEqual(
    lines.map((l) => [l.product_id, l.quantity]),
    [
      ['a', 5],
      ['b', 2],
    ],
  )
  assert.deepEqual(
    removeLine(lines, 'a').map((l) => l.product_id),
    ['b'],
  )
  assert.equal(setQuantity(lines, 'b', '7')[1]?.quantity, 7)
})

test('количество: целое от 1, мусор — 1', () => {
  assert.equal(clampQty(0), 1)
  assert.equal(clampQty(-3), 1)
  assert.equal(clampQty('12'), 12)
  assert.equal(clampQty(2.6), 3)
  assert.equal(clampQty('abc'), 1)
})

test('итог: все цены известны — сумма; иначе «у M цена по запросу · известная часть от …»', () => {
  const a = lineFromItem(item('a', { price_rub: 100000 }), 1)
  const cal = lineFromCompanion(companion('cal', { price_rub: 3000 }), 50, 'Windows Server')
  const all = summarize([a, cal])
  assert.equal(all.onRequest, 0)
  assert.equal(all.knownRub, 250000)
  assert.equal(all.exact, true)
  assert.equal(sp(all.text), '2 позиции · итого 250 000 ₽')

  const svc = lineFromCompanion(
    companion('svc', { price_rub: null, price_mode: 'quote_required', class: 'complement' }),
    1,
  )
  const back = lineFromItem(item('bk', { price_on_request: true, price_rub: null }))
  const part = summarize([a, cal, svc, back])
  assert.equal(part.onRequest, 2)
  assert.equal(sp(part.text), '4 позиции · у 2 цена по запросу · известная часть от 250 000 ₽')

  const none = summarize([svc])
  assert.equal(none.knownRub, null)
  assert.equal(sp(none.text), '1 позиция · у 1 цена по запросу')
  assert.equal(sp(summarize([]).text), 'Позиций нет')
})

test('цена «от» входит в сумму с пометкой «от»', () => {
  const s = summarize([lineFromItem(item('a', { price_mode: 'from', price_rub: 1000 }), 2)])
  assert.equal(sp(s.text), '1 позиция · итого от 2 000 ₽')
  assert.equal(s.exact, false)
  assert.equal(sp(priceModeLabel({ price_rub: 1000, price_mode: 'from' })), 'от 1 000 ₽')
  assert.equal(sp(priceModeLabel({ price_rub: 1000, price_mode: 'unavailable' })), 'Нет в наличии')
  assert.equal(sp(priceModeLabel({ price_rub: null, price_mode: 'fixed' })), 'Цена по запросу')
})

test('пометка происхождения', () => {
  assert.equal(originLabel({ origin: 'search' }), 'из поиска')
  assert.equal(
    originLabel({ origin: 'companion', companion_class: 'conditional_required' }),
    'условно обязательное',
  )
  assert.equal(originLabel({ origin: 'companion', companion_class: 'complement' }), 'дополнение')
  // Старшая редакция — своя пометка и без ссылки на базовую позицию, которой может не быть (QA r2 №12).
  assert.equal(originLabel({ origin: 'companion', companion_class: 'upgrade' }), 'старшая редакция')
  assert.equal(anchorOf({ anchor: 'Windows Server 2025 Standard', companion_class: 'upgrade' }), '')
  assert.equal(anchorOf({ anchor: 'Сервер', companion_class: 'complement' }), 'Сервер')
  assert.equal(anchorOf({ companion_class: 'complement' }), '')
})

test('текст для копирования: у дополнения — к какой позиции (QA r2 №15)', () => {
  const text = sp(
    specText([
      lineFromItem(item('Сервер', { license_line: '', price_rub: 689000 }), 1),
      lineFromCompanion(
        companion('Память', { class: 'complement', license_line: '' }),
        1,
        'Сервер',
      ),
      lineFromCompanion(
        companion('Datacenter', { class: 'upgrade', license_line: '' }),
        1,
        'Standard',
      ),
    ]),
  )
  assert.match(text, /\n2\. Память × 1 — 3 000 ₽ — дополнение к «Сервер»\n/)
  assert.match(text, /\n3\. Datacenter × 1 — 3 000 ₽ — старшая редакция\n/)
})

test('тело POST /api/quotes', () => {
  const lines = [
    lineFromItem(item('ws'), 1),
    lineFromCompanion(companion('cal'), 50, 'Windows Server'),
  ]
  assert.deepEqual(quoteRequest(lines, { sessionId: 's-1', clientId: 'client-a', comment: ' ' }), {
    session_id: 's-1',
    client_id: 'client-a',
    items: [
      { product_id: 'ws', quantity: 1, origin: 'search' },
      {
        product_id: 'cal',
        quantity: 50,
        origin: 'companion',
        companion_class: 'conditional_required',
      },
    ],
  })
})

test('хранилище: битое и чужое отбрасывается, повторы — тоже', () => {
  assert.deepEqual(parseStored(null), [])
  assert.deepEqual(parseStored('{'), [])
  assert.deepEqual(parseStored('{"a":1}'), [])
  const lines = [lineFromItem(item('a'), 3)]
  const raw = JSON.stringify([...lines, lines[0], { title: 'без id' }, 42])
  assert.deepEqual(parseStored(raw), lines)
  const odd = parseStored(
    JSON.stringify([{ product_id: 'x', title: 'X', quantity: -5, origin: 'evil' }]),
  )
  assert.equal(odd[0]?.quantity, 1)
  assert.equal(odd[0]?.origin, 'search')
  assert.equal(odd[0]?.price_mode, 'quote_required')
  // Строки из снятых шаблонов решений, сохранённые раньше, не падают и считаются «из поиска».
  const legacy = parseStored(
    JSON.stringify([{ product_id: 't', title: 'T', quantity: 2, origin: 'template' }]),
  )
  assert.equal(legacy[0]?.origin, 'search')
  assert.equal(legacy[0]?.quantity, 2)
  assert.equal(originLabel(legacy[0]!), 'из поиска')
})

test('текст для копирования: строка на позицию и итог', () => {
  const text = sp(specText([lineFromCompanion(companion('CAL 2025'), 50)]))
  assert.match(
    text,
    /^Спецификация\n1\. CAL 2025 \(Бессрочная · 1 пользователь\) × 50 — 3 000 ₽ — условно обязательное\n1 позиция · итого 150 000 ₽$/,
  )
})

test('количество из запроса — только для поштучной лицензии', () => {
  const chips = [{ field: 'license_quantity', value: '50' }]
  assert.equal(quantityForItem({ license_line: 'Подписка · 1 пользователь · 1 год' }, chips), 50)
  assert.equal(quantityForItem({ license_line: 'Бессрочная · 1 устройство' }, chips), 50)
  assert.equal(quantityForItem({ license_line: 'Бессрочная · 16 ядер' }, chips), 1)
  assert.equal(quantityForItem({ license_line: 'Подписка · 10 устройств' }, chips), 1)
  assert.equal(quantityForItem({ license_line: 'Подписка · 1 пользователь' }, []), 1)
})

test('количество из запроса для пакета — ceil(число / размер пакета) (QA r2 №2)', () => {
  const chips = [
    { field: 'license_quantity', value: '50' },
    { field: 'license_metric', value: 'user' },
  ]
  const cal = {
    license_line: 'Бессрочная · 5 пользователей',
    attrs: {
      edition: 'User CAL',
      license_type: 'perpetual',
      license_metric: 'user',
      license_quantity: 5,
      duration_months: null,
      delivery: '' as const,
      in_reestr: false,
    },
  }
  assert.equal(quantityForItem(cal, chips), 10)
  const cores = {
    ...cal,
    license_line: 'Бессрочная · 16 ядер',
    attrs: { ...cal.attrs, license_metric: 'core', license_quantity: 16 },
  }
  assert.equal(quantityForItem(cores, chips), 1)
})
