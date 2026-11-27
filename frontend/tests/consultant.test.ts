// Консультант контура консультанта (src/lib/consultant.ts): разбор текста, цены из копеек, NDJSON, лента из событий,
// применение spec_event, хранение, флаг.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { GuideOffer } from '../src/api/typesGuide.ts'
import type { ConversationEvent, ProductReference } from '../src/api/typesConsultant.ts'
import {
  activeQuickReplies,
  applyEvent,
  applySpecOp,
  type ChatMessage,
  comparisonCostRow,
  consultantEnabled,
  formatMinor,
  pageContext,
  parseConversation,
  parseEvent,
  parseRich,
  productCalc,
  productPrice,
  readNdjson,
  resolveSpecEvent,
  specPayload,
  splitNdjson,
  splitVisible,
  turnFailed,
} from '../src/lib/consultant.ts'
import { mockEvents } from '../src/lib/consultantMock.ts'
import type { SpecLine } from '../src/lib/spec.ts'

const sp = (s: string) => s.replace(/[  ]/g, ' ')

test('formatMinor: копейки → рубли ru-RU, без копеек у целых, 0 и пустое — пусто', () => {
  assert.equal(sp(formatMinor(8440200)), '84 402 ₽')
  assert.equal(sp(formatMinor(12244500)), '122 445 ₽')
  assert.equal(sp(formatMinor(394020)), '3 940,20 ₽')
  assert.equal(sp(formatMinor(19701000)), '197 010 ₽')
  assert.equal(sp(formatMinor(185000)), '1 850 ₽')
  assert.equal(formatMinor(0), '')
  assert.equal(formatMinor(-100), '')
  assert.equal(formatMinor(undefined), '')
  assert.equal(formatMinor(Number.NaN), '')
  assert.ok(!formatMinor(8440200).includes(' '), 'разделитель — неразрывный пробел')
})

const ref = (over: Partial<ProductReference> = {}): ProductReference => ({
  offerId: 'o1',
  sku: '',
  title: 'X',
  vendor: 'V',
  licenseType: 'Новая лицензия',
  priceStatus: 'known',
  compatibility: 'compatible',
  productUrl: 'https://store.softline.ru/x',
  offer: offerOf('o1'),
  ...over,
})

test('цена позиции: 0 и неизвестная — «Цена по запросу»; расчёт только при количестве > 1', () => {
  assert.equal(sp(productPrice(ref({ totalPriceMinor: 19701000 }))), '197 010 ₽')
  assert.equal(productPrice(ref({ totalPriceMinor: 0 })), 'Цена по запросу')
  assert.equal(
    productPrice(ref({ priceStatus: 'on_request', totalPriceMinor: 100 })),
    'Цена по запросу',
  )
  assert.equal(sp(productCalc(ref({ quantity: 50, unitPriceMinor: 394020 }))), '50 × 3 940,20 ₽')
  assert.equal(productCalc(ref({ quantity: 1, unitPriceMinor: 394020 })), '')
  assert.equal(productCalc(ref({ quantity: 50, unitPriceMinor: 0 })), '')
})

test('строка стоимости сравнения — из totalPriceMinor столбцов', () => {
  const row = comparisonCostRow([
    ref({ totalPriceMinor: 8440200 }),
    ref({ totalPriceMinor: 12244500 }),
    ref({ priceStatus: 'on_request' }),
    ref({ priceStatus: 'unknown' }),
    ref({ totalPriceMinor: 0 }),
  ]).map(sp)
  assert.deepEqual(row, ['84 402 ₽', '122 445 ₽', 'Цена по запросу', 'Нет данных', 'Нет данных'])
})

test('parseRich: полужирное, ссылки только на store.softline.ru, HTML остаётся текстом', () => {
  const rich = (t: string) => parseRich(t).map((n) => ({ ...n, text: sp(n.text) }))
  assert.deepEqual(rich('Стоит **84 402 ₽**, а [КЕС](https://store.softline.ru/kes) — нет'), [
    { kind: 'text', text: 'Стоит ' },
    { kind: 'bold', text: '84 402 ₽' },
    { kind: 'text', text: ', а ' },
    { kind: 'link', text: 'КЕС', href: 'https://store.softline.ru/kes' },
    { kind: 'text', text: ' — нет' },
  ])
  assert.deepEqual(parseRich('[зло](https://evil.example/x) и [js](javascript:alert(1))'), [
    { kind: 'text', text: 'зло и js)' },
  ])
  assert.deepEqual(parseRich('[поддомен](https://store.softline.ru.evil.io/)'), [
    { kind: 'text', text: 'поддомен' },
  ])
  assert.deepEqual(parseRich('<img src=x onerror=alert(1)>'), [
    { kind: 'text', text: '<img src=x onerror=alert(1)>' },
  ])
  assert.deepEqual(parseRich('a ** b'), [{ kind: 'text', text: 'a ** b' }])
  assert.equal(
    parseRich('теперь 3 940,20 ₽ вместо 4 081,50 ₽, 30 компьютеров')[0]?.text,
    'теперь 3\u00a0940,20\u00a0₽ вместо 4\u00a0081,50\u00a0₽, 30 компьютеров',
  )
  assert.deepEqual(parseRich(''), [])
})

test('splitNdjson: строки целиком, хвост — до следующего куска', () => {
  const a = splitNdjson('', '{"type":"status","step":"searching"}\n{"type":"mess')
  assert.deepEqual(a.lines, ['{"type":"status","step":"searching"}'])
  assert.equal(a.rest, '{"type":"mess')
  const b = splitNdjson(a.rest, 'age.start","id":"m1"}\n\n')
  assert.deepEqual(b.lines, ['{"type":"message.start","id":"m1"}'])
  assert.equal(b.rest, '')
})

test('parseEvent: битые и чужие строки пропускаются', () => {
  assert.deepEqual(parseEvent('{"type":"message.start","id":"m1"}'), {
    type: 'message.start',
    id: 'm1',
  })
  assert.equal(parseEvent('не json'), null)
  assert.equal(parseEvent('{"type":"other","id":"m1"}'), null)
  assert.equal(parseEvent('{"type":"message.delta","id":"m1"}'), null)
  assert.equal(parseEvent('{"type":"message.attachment","id":"m1"}'), null)
  assert.equal(parseEvent('{"type":"message.start"}'), null)
})

test('readNdjson: поток кусками посреди строки и UTF-8 символа', async () => {
  const text =
    '{"type":"status","step":"checking"}\n{"type":"message.delta","id":"m1","text":"Цена **1 850 ₽**"}\n{"type":"message.complete","id":"m1"}'
  const bytes = new TextEncoder().encode(text)
  const cut = [5, 47, 60, bytes.length]
  const stream = new ReadableStream<Uint8Array>({
    start(ctrl) {
      let prev = 0
      for (const c of cut) {
        ctrl.enqueue(bytes.slice(prev, c))
        prev = c
      }
      ctrl.close()
    },
  })
  const got: ConversationEvent[] = []
  await readNdjson(stream, (e) => got.push(e))
  assert.deepEqual(
    got.map((e) => e.type),
    ['status', 'message.delta', 'message.complete'],
  )
  assert.equal((got[1] as { text: string }).text, 'Цена **1 850 ₽**')
})

function play(events: ConversationEvent[], turn = 1, start: ChatMessage[] = []): ChatMessage[] {
  return events.reduce((m, e) => applyEvent(m, e, turn), start)
}

test('лента из событий сценария 2: два пузыря, вложения у второго, быстрые ответы до трёх', () => {
  const msgs = play(
    mockEvents({
      text: 'Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров',
      spec: [],
    }),
  )
  assert.equal(msgs.length, 2)
  assert.ok(msgs[0]?.text.startsWith('В обеих редакциях'))
  assert.equal(msgs[0]?.attachments.length, 0)
  assert.deepEqual(
    msgs[1]?.attachments.map((a) => a.data.type),
    ['comparison', 'source'],
  )
  assert.equal(msgs[1]?.status, 'complete')
  assert.deepEqual(activeQuickReplies(msgs), {
    id: '1:m2',
    replies: ['Шифрование нужно', 'Не нужно'],
  })
  // id сервера повторяются от хода к ходу — не склеиваются.
  const next = play(
    [
      { type: 'message.start', id: 'm1' },
      { type: 'message.delta', id: 'm1', text: 'ок' },
    ],
    2,
    msgs,
  )
  assert.equal(next.length, 3)
})

test('быстрые ответы: не больше трёх и исчезают после реплики покупателя', () => {
  const msgs = play([
    { type: 'message.start', id: 'm1' },
    { type: 'message.delta', id: 'm1', text: 'Вопрос?' },
    { type: 'message.complete', id: 'm1', quickReplies: ['a', 'b', 'c', 'd'] },
  ])
  assert.deepEqual(activeQuickReplies(msgs)?.replies, ['a', 'b', 'c'])
  const after = [
    ...msgs,
    {
      id: '2:u',
      role: 'user' as const,
      text: 'a',
      attachments: [],
      status: 'complete' as const,
      turn: 2,
    },
  ]
  assert.equal(activeQuickReplies(after), null)
})

test('ошибка модели: пустое сообщение с ошибкой скрыто, запасной ответ есть — ход не провален', () => {
  const fb = play(mockEvents({ text: 'ошибка', spec: [] }))
  assert.equal(fb[0]?.status, 'error')
  assert.equal(turnFailed(fb, 1), false)
  const dead = play(mockEvents({ text: 'сбой', spec: [] }))
  assert.equal(turnFailed(dead, 1), true)
})

test('список: четыре позиции видны без раскрытия, сквозь группы', () => {
  const g = [{ items: [1, 2] }, { items: [3] }, { items: [4, 5, 6] }]
  assert.deepEqual(splitVisible(g, false), { shown: [[1, 2], [3], [4]], hidden: 2 })
  assert.deepEqual(splitVisible(g, true), { shown: [[1, 2], [3], [4, 5, 6]], hidden: 0 })
})

function offerOf(id: string, unit = 394020, qty = 50): GuideOffer {
  return {
    offer_id: id,
    title: 'Kaspersky Endpoint Security — Расширенный',
    sku: 'KL4867',
    tier: '50–99',
    unit: 'компьютер',
    price_per_unit: unit / 100,
    total: (unit * qty) / 100,
    price_state: 'fixed',
    offer_url: 'https://store.softline.ru/kes',
    quantity: qty,
  }
}

test('spec_event added: позиция из lineFromOffer, повтор — без дубля, количество обновляется', () => {
  let lines: SpecLine[] = []
  const ev = {
    type: 'spec_event' as const,
    action: 'added' as const,
    offerId: 'kes',
    title: 'KES',
    quantity: 50,
    offer: offerOf('kes', 394020, 30),
  }
  const r = resolveSpecEvent(lines, ev, 'касперский')
  assert.ok(r.ok)
  if (!r.ok) return
  lines = applySpecOp(lines, r.op)
  assert.equal(lines.length, 1)
  assert.equal(lines[0]?.product_id, 'kes')
  assert.equal(lines[0]?.quantity, 50, 'количество события важнее количества оффера')
  assert.equal(lines[0]?.price_rub, 3940.2)
  const again = resolveSpecEvent(lines, { ...ev, quantity: 60 })
  assert.ok(again.ok)
  if (!again.ok) return
  lines = applySpecOp(lines, again.op)
  assert.equal(lines.length, 1)
  assert.equal(lines[0]?.quantity, 60)
})

test('spec_event removed/updated по offerId; применить нельзя — ошибка', () => {
  const base = resolveSpecEvent([], {
    type: 'spec_event',
    action: 'added',
    offerId: 'kes',
    title: 'KES',
    offer: offerOf('kes'),
  })
  assert.ok(base.ok)
  if (!base.ok) return
  const lines = applySpecOp([], base.op)
  const upd = resolveSpecEvent(lines, {
    type: 'spec_event',
    action: 'updated',
    offerId: 'kes',
    title: 'KES',
    quantity: 70,
  })
  assert.ok(upd.ok)
  if (upd.ok) assert.equal(applySpecOp(lines, upd.op)[0]?.quantity, 70)
  const rm = resolveSpecEvent(lines, {
    type: 'spec_event',
    action: 'removed',
    offerId: 'kes',
    title: 'KES',
  })
  assert.ok(rm.ok)
  if (rm.ok) assert.deepEqual(applySpecOp(lines, rm.op), [])

  assert.deepEqual(
    resolveSpecEvent(lines, { type: 'spec_event', action: 'added', offerId: 'x', title: 'X' }),
    { ok: false, error: 'Не удалось добавить товар.' },
  )
  assert.deepEqual(
    resolveSpecEvent(lines, {
      type: 'spec_event',
      action: 'added',
      offerId: 'x',
      title: 'X',
      offer: offerOf('y'),
    }),
    { ok: false, error: 'Не удалось добавить товар.' },
  )
  assert.deepEqual(
    resolveSpecEvent([], { type: 'spec_event', action: 'removed', offerId: 'kes', title: 'KES' }),
    { ok: false, error: 'Не удалось удалить товар.' },
  )
  assert.deepEqual(
    resolveSpecEvent(lines, {
      type: 'spec_event',
      action: 'updated',
      offerId: 'kes',
      title: 'KES',
      quantity: 0,
    }),
    { ok: false, error: 'Не удалось изменить количество.' },
  )
})

test('spec в запрос и контекст страницы', () => {
  const lines = [
    { product_id: 'a', quantity: 3 },
    { product_id: 'b', quantity: 1 },
  ] as SpecLine[]
  assert.deepEqual(specPayload(lines), [
    { offer_id: 'a', qty: 3 },
    { offer_id: 'b', qty: 1 },
  ])
  assert.deepEqual(pageContext('search', {}, { q: ' касперский ' }, false), {
    type: 'search',
    family_id: null,
    offer_id: null,
    query: 'касперский',
  })
  assert.deepEqual(pageContext('product', { id: '413744' }, {}, false).offer_id, '413744')
  assert.equal(pageContext('search', {}, { q: 'x' }, true).type, 'spec')
  assert.equal(pageContext('proofs', {}, {}, false).type, 'other')
})

test('хранение: битое — пустой разговор с новым id, оборванный ответ — завершён', () => {
  const empty = parseConversation('{bad')
  assert.equal(empty.messages.length, 0)
  assert.ok(empty.conversationId.length >= 16)
  const saved = parseConversation(
    JSON.stringify({
      conversationId: 'c1',
      messages: [
        { id: '1:u', role: 'user', text: 'привет', attachments: [], status: 'complete', turn: 1 },
        {
          id: '1:m1',
          role: 'assistant',
          text: 'Здра',
          attachments: [],
          status: 'streaming',
          turn: 1,
        },
        { id: 'x', role: 'robot' },
      ],
      draft: 'черновик',
      open: true,
      expanded: true,
    }),
  )
  assert.equal(saved.conversationId, 'c1')
  assert.equal(saved.messages.length, 2)
  assert.equal(saved.messages[1]?.status, 'complete')
  assert.equal(saved.draft, 'черновик')
  assert.equal(saved.open, true)
  assert.equal(saved.expanded, true)
})

test('флаг: консультант по умолчанию; помощник контура помощника — по ?consultant=0, localStorage или VITE_CONSULTANT=0', () => {
  assert.equal(consultantEnabled({}), true)
  assert.equal(consultantEnabled({ env: '1' }), true)
  assert.equal(consultantEnabled({ env: '0' }), false)
  assert.equal(consultantEnabled({ search: '?q=x&consultant=1', env: '0' }), true)
  assert.equal(consultantEnabled({ search: '?consultant=0' }), false)
  assert.equal(consultantEnabled({ stored: '0' }), false)
  assert.equal(consultantEnabled({ stored: '1', env: '0' }), true)
  assert.equal(consultantEnabled({ search: '?consultant=1', stored: '0' }), true)
})
