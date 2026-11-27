// Режим подбора (src/lib/guide.ts): когда предлагать подбор, таблица сравнения, «только различия», подписи
// происхождения условий, цена оффера без «0 ₽», строка спецификации.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchResponse } from '../src/api/types.ts'
import type { GuideComparison, GuideCondition, GuideOffer } from '../src/api/typesGuide.ts'
import {
  buildTable,
  cellView,
  chipView,
  claimIndex,
  claimMark,
  conditionAria,
  conditionMessage,
  guideMode,
  isOtherChip,
  isRefusal,
  lineFromOffer,
  offerPrice,
  offerQuantity,
  originLabel,
  pendingStep,
  pendingIndex,
  offerName,
  priceMath,
  packText,
  rejectedTitle,
  sharedNote,
  turnAnswer,
  conditionAsk,
  conditionText,
  conditionOptions,
  compareView,
  columnOffer,
  keepNumbers,
  conditionCount,
  offerRole,
  routeMode,
  rowDiffers,
  selectionId,
  sourceName,
  perUnit,
  priceWithUnit,
  specOffers,
  splitOffers,
  unitText,
  visibleColumns,
  withSpecLine,
} from '../src/lib/guide.ts'

const sp = (s: string | undefined) => (s ?? '').replace(/ /g, ' ')

type Resp = Parameters<typeof guideMode>[1]
const resp = (over: Partial<SearchResponse> = {}): Resp =>
  ({
    items: [{ product_id: '1' }],
    chips: [],
    target: null,
    ...over,
  }) as unknown as Resp

test('guideMode: признак сервера главнее правил', () => {
  assert.equal(guideMode('kaspersky', resp({ guide: { mode: 'offer' } })), 'offer')
  assert.equal(guideMode('защита для офиса', resp({ guide: { mode: 'search' } })), 'search')
  // Неизвестное значение сервера — правила фронта.
  assert.equal(
    guideMode('криптопро csp 5.0', resp({ guide: { mode: 'x' }, target: fam })),
    'search',
  )
})

const fam = { type: 'family', url: '', title: 'КриптоПро CSP' }
const vendor = { type: 'vendor', url: '', title: 'Kaspersky' }

test('guideMode: таблица режимов SPEC §1', () => {
  assert.equal(guideMode('kaspersky', resp({ target: vendor })), 'offer')
  assert.equal(guideMode('криптопро csp 5.0', resp({ target: fam })), 'search')
  assert.equal(
    guideMode(
      'продление касперский на 100 пк на 2 года',
      resp({
        target: vendor,
        purchase: { query: '', deal: 'renewal', quantity: 100, duration_months: 24 },
      }),
    ),
    'search',
  )
  assert.equal(guideMode('kaspersky на 50 пк', resp({ target: fam })), 'offer')
  assert.equal(guideMode('защита для офиса', resp({ target: null })), 'offer')
  // Цель-семейство, но запрос задачей — предложение.
  assert.equal(guideMode('защита для компьютеров в офисе', resp({ target: fam })), 'offer')
  assert.equal(guideMode('чем расширенный отличается от стандартного', resp()), 'guide')
  assert.equal(guideMode('что взять для защиты 30 компьютеров на astra', resp()), 'guide')
  assert.equal(guideMode('подбери антивирус', null), 'guide')
  assert.equal(guideMode('самый дешёвый криптопро csp для сервера', resp({ target: fam })), 'guide')
})

test('guideMode: справка, сервис и пустая выдача — обычный поиск', () => {
  assert.equal(guideMode('оплата', resp({ target: { ...vendor, type: 'service' } })), 'search')
  assert.equal(guideMode('защита для офиса', resp({ items: [] })), 'search')
  assert.equal(
    guideMode(
      'как продлить',
      resp({
        help: { id: 'h', mode: 'answer', title: '', text: '', link_label: '', url: '' },
      }),
    ),
    'search',
  )
  assert.equal(guideMode('   ', resp()), 'search')
})

test('cellView: четыре вида ячейки, пустое значение — нет сведений', () => {
  assert.deepEqual(cellView({ kind: 'yes', fact_id: 'f1' }), {
    tone: 'yes',
    text: 'есть',
    factId: 'f1',
  })
  assert.equal(cellView({ kind: 'no' }).text, 'нет по данным вендора')
  assert.equal(cellView({ kind: 'unknown' }).text, 'нет сведений')
  assert.equal(cellView({ kind: 'value', value: '2 813,40 ₽' }).text, '2 813,40 ₽')
  assert.equal(cellView({ kind: 'value', value: '' }).tone, 'unknown')
  assert.equal(cellView({ kind: 'value', value: null }).tone, 'unknown')
  assert.equal(cellView(undefined).text, 'нет сведений')
  assert.equal(cellView({ kind: 'странное' }).tone, 'unknown')
})

const cmp: GuideComparison = {
  columns: [
    { family_id: '3935', title: 'Стандартный', family_url: 'https://x/std' },
    { family_id: '3953', title: 'Расширенный', family_url: 'https://x/adv' },
    { family_id: '4000', title: 'Total', family_url: '' },
  ],
  rows: [
    {
      key: 'encryption',
      label: 'Шифрование данных',
      cells: [
        { kind: 'no', fact_id: 'kb_std_enc' },
        { kind: 'yes', fact_id: 'kb_adv_enc' },
        { kind: 'yes' },
      ],
    },
    {
      key: 'console',
      label: 'Единая консоль управления',
      cells: [{ kind: 'yes' }, { kind: 'yes' }, { kind: 'yes' }],
    },
    {
      key: 'certs',
      label: 'Сертификаты',
      cells: [{ kind: 'unknown' }, { kind: 'value', value: '' }, { kind: 'unknown' }],
    },
    {
      key: 'price',
      label: 'Цена за компьютер в год',
      cells: [
        { kind: 'value', value: '2 813,40 ₽' },
        { kind: 'value', value: '4 081,50 ₽' },
        { kind: 'value', value: '4 081,50 ₽' },
      ],
    },
  ],
}

test('buildTable: строки, колонки и различия', () => {
  const t = buildTable(cmp)
  assert.deepEqual(
    t.columns.map((c) => c.title),
    ['Стандартный', 'Расширенный', 'Total'],
  )
  assert.equal(t.total, 4)
  assert.equal(t.differing, 2)
  assert.deepEqual(
    t.rows.map((r) => r.differs),
    [true, false, false, true],
  )
  assert.equal(t.rows[0]!.cells[0]!.factId, 'kb_std_enc')
})

test('buildTable: «только различия» и различие по показанным колонкам', () => {
  const only = buildTable(cmp, { onlyDiff: true })
  assert.deepEqual(
    only.rows.map((r) => r.key),
    ['encryption', 'price'],
  )
  assert.equal(only.total, 4)
  // Расширенный против Total: шифрование одинаково, цена одинакова — различий нет.
  const pair = buildTable(cmp, { onlyDiff: true, columns: [1, 2] })
  assert.equal(pair.rows.length, 0)
  assert.equal(pair.differing, 0)
  // Чужие индексы отбрасываются.
  assert.equal(buildTable(cmp, { columns: [0, 9] }).columns.length, 1)
  assert.equal(buildTable(null).rows.length, 0)
})

test('rowDiffers: «нет сведений» везде — не различие, одна колонка — не различие', () => {
  assert.equal(rowDiffers([cellView({ kind: 'unknown' }), cellView({ kind: 'unknown' })]), false)
  assert.equal(rowDiffers([cellView({ kind: 'unknown' }), cellView({ kind: 'yes' })]), true)
  assert.equal(rowDiffers([cellView({ kind: 'yes' })]), false)
  assert.equal(
    rowDiffers([
      cellView({ kind: 'value', value: 'Да' }),
      cellView({ kind: 'value', value: 'да' }),
    ]),
    false,
  )
})

test('visibleColumns: на телефоне две колонки, вторая — выбранная', () => {
  assert.deepEqual(visibleColumns(3, false, 2), [0, 1, 2])
  assert.deepEqual(visibleColumns(2, true, 1), [0, 1])
  assert.deepEqual(visibleColumns(4, true, 3), [0, 3])
  assert.deepEqual(visibleColumns(4, true, 0), [0, 1])
  assert.deepEqual(visibleColumns(4, true, 7), [0, 1])
})

test('claimIndex и sourceName: источник по факту, имя без даты', () => {
  const m = claimIndex([
    {
      fact_id: 'f1',
      text: 'Шифрование',
      source: { title: '', url: 'https://www.kaspersky.ru/a', fragment: '…' },
      status: 'verified',
    },
    { fact_id: '', text: '', source: null, status: '' },
  ])
  assert.equal(m.size, 1)
  assert.equal(sourceName(m.get('f1')!.source), 'kaspersky.ru')
  assert.equal(sourceName({ title: ' Справка Kaspersky ', url: '' }), 'Справка Kaspersky')
  assert.equal(sourceName({ title: '', url: 'not a url' }), 'источник')
  assert.equal(sourceName(null), '')
  assert.equal(claimMark({ status: 'verified' }), '')
  assert.equal(claimMark({ status: 'verified_agent' }), 'проверено автоматически')
  assert.equal(claimMark({ status: 'draft' }), 'не проверено')
})

test('originLabel: подписи происхождения условий', () => {
  assert.equal(originLabel('default'), 'по умолчанию')
  assert.equal(originLabel('inferred'), 'по запросу')
  assert.equal(originLabel('user'), '')
  assert.equal(originLabel('что-то'), '')
  const c: GuideCondition = { key: 'term_months', label: '1 год', value: '12', origin: 'default' }
  assert.equal(conditionAria(c), '1 год — по умолчанию')
  assert.equal(conditionAria({ ...c, origin: 'user' }), '1 год')
})

test('chipView и isOtherChip: ключ чипа или ключ вопроса', () => {
  assert.deepEqual(chipView('30', 'count'), { label: '30', value: '30', key: 'count' })
  assert.deepEqual(chipView({ label: 'Воронеж' }, 'level'), {
    label: 'Воронеж',
    value: 'Воронеж',
    key: 'level',
  })
  assert.deepEqual(chipView({ label: '1 год', key: 'term_months', value: '12' }, 'x'), {
    label: '1 год',
    value: '12',
    key: 'term_months',
  })
  assert.equal(isOtherChip(chipView('другое')), true)
  assert.equal(isOtherChip(chipView('Другое')), true)
  assert.equal(isOtherChip(chipView('100')), false)
})

test('routeMode: режимы POST /api/guide/route', () => {
  assert.equal(routeMode({ mode: 'search_offer' }), 'offer')
  assert.equal(routeMode({ mode: 'search' }), 'search')
  assert.equal(routeMode({ mode: 'guide' }), 'guide')
  assert.equal(routeMode({ mode: '???' }), null)
  assert.equal(routeMode(null), null)
})

test('conditionMessage: правка условия текстом', () => {
  const c = { label: '1 год', value: '12' }
  assert.equal(conditionMessage(c, null), 'без условия «1 год»')
  assert.equal(conditionMessage(c, '2 года'), '2 года')
})

const offer = (over: Partial<GuideOffer> = {}): GuideOffer => ({
  offer_id: '4986',
  title: 'Kaspersky Endpoint Security для бизнеса Расширенный',
  sku: 'KL4867RAQFS',
  tier: '50–99',
  unit: 'компьютер',
  price_per_unit: 3940.2,
  total: 197010,
  price_state: 'fixed',
  offer_url: 'https://store.softline.ru/x',
  ...over,
})

test('offerPrice: цена за единицу и итог, «0 ₽» — никогда', () => {
  const p = offerPrice(offer())
  assert.equal(sp(p.unit), '3 940,20 ₽')
  assert.equal(sp(p.total), '197 010 ₽')
  assert.equal(p.known, true)
  for (const bad of [0, null, -1]) {
    const v = offerPrice(offer({ price_per_unit: bad, total: 0 }))
    assert.equal(v.unit, 'Цена по запросу')
    assert.equal(v.total, '')
    assert.ok(!sp(v.unit).includes('0 ₽'))
  }
  assert.equal(offerPrice(offer({ total: 0 })).total, '')
  assert.equal(offerPrice(offer({ price_state: 'quote_required' })).unit, 'Цена по запросу')
  assert.equal(offerPrice(offer({ price_state: 'on_request' })).unit, 'Цена по запросу')
  assert.equal(sp(offerPrice(offer({ price_state: 'priced' })).unit), '3 940,20 ₽')
  assert.equal(offerPrice(offer({ price_state: 'unavailable' })).unit, 'Нет в наличии')
  const from = offerPrice(offer({ price_state: 'from' }))
  assert.equal(sp(from.unit), 'от 3 940,20 ₽')
  assert.equal(from.state, 'цена от')
})

const conds: GuideCondition[] = [
  { key: 'count', label: '50 ПК', value: '50', origin: 'user' },
  { key: 'operation', label: 'новая лицензия', value: 'new', origin: 'default' },
  { key: 'term_months', label: '1 год', value: '12', origin: 'default' },
]

test('offerQuantity: поле сервера → итог / цена → условие → 1', () => {
  assert.equal(offerQuantity(offer({ quantity: 7 }), conds), 7)
  assert.equal(offerQuantity(offer(), []), 50)
  assert.equal(offerQuantity(offer({ total: null }), conds), 50)
  assert.equal(offerQuantity(offer({ total: null, price_per_unit: null }), []), 1)
})

test('lineFromOffer: строка спецификации с условиями закупки', () => {
  const l = lineFromOffer(offer(), conds, 'защита для компьютеров в офисе')
  assert.equal(l.product_id, '4986')
  assert.equal(l.quantity, 50)
  assert.equal(l.price_rub, 3940.2)
  assert.equal(l.price_mode, 'fixed')
  assert.equal(l.origin, 'search')
  assert.equal(l.license_line, 'арт. KL4867RAQFS · ступень 50–99 · за компьютер')
  assert.deepEqual(l.purchase, {
    query: 'защита для компьютеров в офисе',
    quantity: 50,
    deal: 'new',
    duration_months: 12,
  })
  const onReq = lineFromOffer(offer({ price_per_unit: 0, price_state: '' }), [], 'q')
  assert.equal(onReq.price_rub, null)
  assert.equal(onReq.price_mode, 'quote_required')
})

test('withSpecLine и selectionId: цена из черновика сервера', () => {
  const o = offer()
  const fresh = withSpecLine(o, {
    offer_id: '4986',
    title: '',
    sku: '',
    tier: '50–99',
    quantity: 50,
    price_per_unit: 3950,
    total: 197500,
    price_state: 'priced',
    offer_url: '',
  })
  assert.equal(fresh.price_per_unit, 3950)
  assert.equal(fresh.quantity, 50)
  assert.equal(fresh.sku, 'KL4867RAQFS')
  assert.equal(withSpecLine(o, null), o)
  assert.equal(withSpecLine(o, { ...fresh, offer_id: 'x', title: '' } as never), o)
  assert.equal(selectionId({ selection_id: 'sel-1' }), 'sel-1')
  assert.equal(selectionId(null), '')
  assert.equal(selectionId('x'), '')
  assert.equal(
    lineFromOffer(offer({ price_state: 'on_request' }), [], 'q').price_mode,
    'quote_required',
  )
})

test('isRefusal и pendingStep', () => {
  assert.equal(isRefusal({ type: 'no_data' }), true)
  assert.equal(isRefusal({ type: 'out_of_scope' }), true)
  assert.equal(isRefusal({ type: 'comparison' }), false)
  assert.equal(isRefusal(null), false)
  assert.equal(pendingStep(0), 'Ищу редакции')
  assert.equal(pendingStep(1300), 'Сверяю правила')
  assert.equal(pendingStep(99999), 'Считаю цену')
  assert.equal(pendingIndex(-5), 0)
  assert.equal(pendingIndex(2500), 2)
})

test('cellView: число в денежной строке — цена, «0 ₽» никогда', () => {
  assert.equal(
    sp(cellView({ kind: 'value', value: 2813.4 }, 'price_per_seat_year').text),
    '2 813,40 ₽',
  )
  assert.equal(sp(cellView({ kind: 'value', value: 84402 }, 'total').text), '84 402 ₽')
  assert.equal(cellView({ kind: 'value', value: 0 }, 'total').tone, 'unknown')
  assert.equal(sp(cellView({ kind: 'value', value: 1500 }, 'seats').text), '1 500')
  assert.equal(
    sp(
      buildTable({
        columns: cmp.columns.slice(0, 1),
        rows: [{ key: 'total', label: 'Итого', cells: [{ kind: 'value', value: 197010 }] }],
      }).rows[0]!.cells[0]!.text,
    ),
    '197 010 ₽',
  )
})

test('unitText, offerRole, splitOffers и условие count:workstation', () => {
  assert.equal(unitText('за компьютер'), 'компьютер')
  assert.equal(unitText('компьютер'), 'компьютер')
  assert.equal(unitText(undefined), '')
  assert.equal(perUnit('за лицензию'), 'за лицензию')
  assert.equal(perUnit('компьютер'), 'за компьютер')
  assert.equal(perUnit(''), '')
  assert.equal(
    sp(priceWithUnit(offer({ price_per_unit: 1850, unit: 'за рабочее место' }))),
    '1 850 ₽ за рабочее место',
  )
  assert.equal(priceWithUnit(offer({ price_per_unit: 0 })), 'Цена по запросу')
  assert.equal(offerRole({ role: 'rejected' }), 'rejected')
  assert.equal(offerRole({ role: 'alternative' }), 'alternative')
  assert.equal(offerRole({}), 'match')
  assert.equal(offerRole({ role: '???' }), 'match')
  const s = splitOffers([
    offer({ offer_id: 'a', role: 'match' }),
    offer({ offer_id: 'b', role: 'rejected' }),
    offer({ offer_id: 'c', role: 'alternative' }),
    offer({ offer_id: 'd' }),
  ])
  assert.deepEqual(
    s.match.map((o) => o.offer_id),
    ['a', 'd'],
  )
  assert.deepEqual(
    s.alternative.map((o) => o.offer_id),
    ['c'],
  )
  assert.deepEqual(
    s.rejected.map((o) => o.offer_id),
    ['b'],
  )
  assert.equal(
    conditionCount([{ key: 'count:workstation', label: '50 ПК', value: '50', origin: 'user' }]),
    50,
  )
  assert.equal(conditionCount([{ key: 'counter', label: '', value: '5', origin: 'user' }]), null)
  assert.equal(
    lineFromOffer(offer({ unit: 'за компьютер' }), [], 'q').license_line,
    'арт. KL4867RAQFS · ступень 50–99 · за компьютер',
  )
})

test('specOffers: строки подбора D3 — все, иначе первый подходящий', () => {
  const o = (id: string, note = '', role = 'match') =>
    ({
      offer_id: id,
      title: id,
      sku: '',
      tier: '',
      unit: '',
      price_per_unit: 1,
      total: 1,
      price_state: 'fixed',
      offer_url: '',
      note,
      role,
    }) as GuideOffer
  const ws = o('ws', 'строка «рабочие станции»')
  const sr = o('sr', 'строка «серверы»')
  assert.deepEqual(
    specOffers([ws, sr]).map((x) => x.offer_id),
    ['ws', 'sr'],
  )
  // Три уровня одной строки («не знаю») — не строки подбора: первый.
  const lv = ['orel', 'vor', 'smol'].map((id) => o(id, `«${id}», строка «рабочие станции»`))
  assert.deepEqual(
    specOffers(lv).map((x) => x.offer_id),
    ['orel'],
  )
  assert.deepEqual(
    specOffers([o('a'), o('b')]).map((x) => x.offer_id),
    ['a'],
  )
  assert.deepEqual(
    specOffers([o('r', '', 'rejected'), o('alt', '', 'alternative')]).map((x) => x.offer_id),
    ['alt'],
  )
  assert.deepEqual(specOffers(null), [])
})

test('offerName: семейство и редакция вместо строки фида', () => {
  assert.deepEqual(
    offerName({
      title:
        'Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов',
      family_title: 'Kaspersky Endpoint Security для бизнеса Расширенный',
      term: '1 год',
    }),
    { name: 'Kaspersky Endpoint Security для бизнеса Расширенный', detail: 'лицензия Base' },
  )
  assert.deepEqual(
    offerName({
      title:
        'Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов',
      family_title: 'Kaspersky Endpoint Security для бизнеса Стандартный',
      term: '1 год',
    }).detail,
    'базовая лицензия',
  )
  // «Программа …» в семействе отбрасывается, отличие позиции остаётся.
  assert.deepEqual(
    offerName({
      title: 'КриптоПро CSP 5.0: серверная лицензия СКЗИ',
      family_title: 'Программа КриптоПро CSP 5.0',
      term: 'бессрочная',
    }),
    { name: 'КриптоПро CSP 5.0', detail: 'серверная лицензия СКЗИ' },
  )
  // Номер реестра в скобках после названия не нужен покупателю.
  assert.equal(
    offerName({
      title: 'КриптоПро CSP TLS (Номер ЕРРП: 4332), Лицензия на право использования',
      family_title: 'КриптоПро CSP TLS',
    }).detail,
    'Лицензия на право использования',
  )
  // Название семейства внутри строки — строка как есть; семейства нет — тоже.
  const raw = 'КриптоПро CSP (бессрочная лицензия СКЗИ КриптоПро CSP 5.0), на одном рабочем месте'
  assert.deepEqual(offerName({ title: raw, family_title: 'КриптоПро CSP 5.0' }), {
    name: raw,
    detail: '',
  })
  assert.deepEqual(offerName({ title: 'Astra Linux', family_title: '' }), {
    name: 'Astra Linux',
    detail: '',
  })
  // Семейство не совпадает со строкой фида: семейство — названием, строка — отличием.
  assert.deepEqual(
    offerName({
      title: 'Доктор Веб Комплексная защита + Центр управления на 1 год на 30 ПК',
      family_title: 'Dr.Web Desktop Security Suite',
      term: '1 год',
    }),
    {
      name: 'Dr.Web Desktop Security Suite',
      detail: 'Доктор Веб Комплексная защита + Центр управления на 30 ПК',
    },
  )
  // «лицензия» без уточнения — не отличие.
  assert.equal(
    offerName({
      title:
        'Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов',
      family_title: 'Kaspersky Endpoint Security Cloud',
      term: '1 год',
    }).detail,
    '',
  )
})

test('priceMath и packText: цена × количество, пакеты, «0 ₽» — никогда', () => {
  const m = priceMath({
    price_per_unit: 3940.2,
    total: 197010,
    price_state: 'priced',
    unit: 'за компьютер',
  })
  assert.equal(sp(m.unit), '3 940,20 ₽')
  assert.equal(m.per, 'за компьютер')
  assert.equal(m.times, 50)
  assert.equal(sp(m.total), '197 010 ₽')
  assert.equal(m.pack, null)
  // Одна единица — без «× 1».
  const one = priceMath({
    price_per_unit: 70000,
    total: 70000,
    price_state: 'priced',
    unit: 'за сервер',
  })
  assert.equal(one.times, null)
  // Пакет: цена за место, комплекты из итога.
  const pack = priceMath({
    price_per_unit: 1347.84,
    total: 26956.8,
    price_state: 'priced',
    unit: 'за место, пакетами по 5',
    package_price: 6739.2,
    package_size: 5,
  })
  assert.equal(pack.per, 'за место')
  assert.equal(pack.times, 20)
  assert.deepEqual(pack.pack && { ...pack.pack, price: sp(pack.pack.price) }, {
    count: 4,
    size: 5,
    price: '6 739,20 ₽',
  })
  assert.equal(sp(packText(pack.pack)), '4 комплекта на 5 мест по 6 739,20 ₽')
  assert.equal(
    sp(packText({ count: 1, size: 20, price: '36 368,28 ₽' })),
    'комплект на 20 мест — 36 368,28 ₽',
  )
  assert.equal(packText(null), '')
  const none = priceMath({ price_per_unit: 0, total: 0, price_state: 'priced', unit: 'за место' })
  assert.equal(none.known, false)
  assert.equal(none.unit, 'Цена по запросу')
  assert.equal(none.per, '')
  assert.equal(none.total, '')
})

test('rejectedTitle, sharedNote, turnAnswer, conditionAsk, conditionText', () => {
  const c = (key: string, label: string, value = label): GuideCondition => ({
    key,
    label,
    value,
    origin: 'user',
  })
  assert.equal(rejectedTitle([c('object', 'для сервера', 'server')]), 'Не подходит для сервера')
  assert.equal(rejectedTitle([]), 'Не подходит под ваши условия')
  assert.equal(
    sharedNote([{ note: 'нужна серверная' }, { note: 'нужна  серверная' }]),
    'нужна серверная',
  )
  assert.equal(sharedNote([{ note: 'а' }, { note: 'б' }]), '')
  const base = { type: 'selection', lead: '', offers: [], clarification: null } as never
  assert.equal(turnAnswer(null), '')
  assert.equal(
    turnAnswer({ ...(base as object), lead: ' Подходит «Расширенный». ' } as never),
    'Подходит «Расширенный».',
  )
  assert.equal(
    turnAnswer({
      ...(base as object),
      clarification: { question: 'Сколько компьютеров?', chips: [] },
    } as never),
    'Сколько компьютеров?',
  )
  assert.equal(
    turnAnswer({ ...(base as object), offers: [{ role: 'match' }, { role: 'match' }] } as never),
    '2 варианта под условия',
  )
  assert.equal(conditionAsk(c('term_months', '1 год', '12'), null), 'Без условия «1 год»')
  assert.equal(conditionAsk(c('count:workstation', '30 ПК', '30'), '40'), '30 ПК → 40')
  assert.equal(
    conditionAsk(c('org_type', 'коммерческая организация', 'commercial'), 'home'),
    'коммерческая организация → для дома',
  )
  assert.equal(
    conditionText(c('direction', 'антивирус для рабочих мест', 'endpoint')),
    'Понял: антивирус для рабочих мест',
  )
  assert.equal(conditionText(c('operation', 'новая лицензия', 'new')), 'новая лицензия')
  assert.ok(conditionOptions('org_type')?.some((o) => o.value === 'home'))
  assert.equal(conditionOptions('count:workstation'), null)
  assert.equal(
    keepNumbers('среди подходящих: 1 347,84 ₽'),
    'среди подходящих: 1\u00a0347,84\u00a0₽',
  )
})

test('compareView: функции с «только различия», деньги одной строкой', () => {
  const v = (kind: string, value?: number | string) => ({ kind, value })
  const cmp: GuideComparison = {
    columns: [
      { family_id: 'a', title: 'Стандартный', family_url: '' },
      { family_id: 'b', title: 'Расширенный', family_url: '' },
    ],
    rows: [
      { key: 'encryption', label: 'Шифрование', kind: 'feature', cells: [v('no'), v('yes')] },
      { key: 'console', label: 'Консоль', kind: 'feature', cells: [v('yes'), v('yes')] },
      {
        key: 'license_unit',
        label: 'Единица',
        kind: 'commercial',
        cells: [v('value', 'за место'), v('value', 'за компьютер')],
      },
      {
        key: 'price_per_seat_year',
        label: 'Цена за компьютер в год, 25–49 мест',
        kind: 'commercial',
        cells: [v('value', 2813.4), v('value', 4081.5)],
      },
      {
        key: 'total',
        label: 'Итого на 30 ПК',
        kind: 'commercial',
        cells: [v('value', 84402), v('value', 122445)],
      },
    ],
  }
  const all = compareView(cmp)
  assert.deepEqual(
    all.features.map((r) => r.key),
    ['encryption', 'console', 'license_unit'],
  )
  assert.equal(all.total, 3)
  assert.equal(all.differing, 2)
  assert.equal(all.prices.length, 1)
  const p = all.prices[0]!
  assert.equal(p.key, 'total')
  assert.equal(p.note, 'цена за компьютер в год, 25–49 мест')
  assert.equal(sp(p.cells[1]!.total.text), '122 445 ₽')
  assert.equal(sp(p.cells[1]!.unit!.text), '4 081,50 ₽')
  const diff = compareView(cmp, { onlyDiff: true })
  assert.deepEqual(
    diff.features.map((r) => r.key),
    ['encryption', 'license_unit'],
  )
  assert.equal(diff.prices.length, 1)
  // Варианты Astra: price:<объект> сливается с total:<объект>; цена без итога — своей строкой.
  const astra = compareView({
    columns: cmp.columns,
    rows: [
      {
        key: 'price:workstation',
        label: 'Цена за рабочую станцию',
        kind: 'commercial',
        cells: [v('value', 6150), v('value', 11400)],
      },
      {
        key: 'total:workstation',
        label: 'Итого на 30 рабочих станций',
        kind: 'commercial',
        cells: [v('value', 184500), v('value', 342000)],
      },
      {
        key: 'price:server',
        label: 'Цена за сервер',
        kind: 'commercial',
        cells: [v('value', 27000), v('unknown')],
      },
    ],
  })
  assert.deepEqual(
    astra.prices.map((r) => r.key),
    ['total:workstation', 'price:server'],
  )
  assert.equal(astra.prices[1]!.cells[1]!.total.text, 'нет сведений')
  assert.equal(compareView(null).prices.length, 0)
})

test('columnOffer: подходящая позиция семейства, отклонённые не берутся', () => {
  const o = (id: string, family_id: string, role: string) =>
    ({ offer_id: id, family_id, role }) as GuideOffer
  const offers = [
    o('1', 'a', 'rejected'),
    o('2', 'a', 'alternative'),
    o('3', 'a', 'match'),
    o('4', 'b', 'match'),
  ]
  assert.equal(columnOffer('a', offers)?.offer_id, '3')
  assert.equal(columnOffer('b', offers)?.offer_id, '4')
  assert.equal(columnOffer('c', offers), null)
  assert.equal(columnOffer('a', [o('1', 'a', 'rejected')]), null)
})
