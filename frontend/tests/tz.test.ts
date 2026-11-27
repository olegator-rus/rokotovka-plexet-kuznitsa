// Поиск по ТЗ (контур ТЗ): проверка файла, этапы, опрос с отменой, ограничение параллельности, статус позиции,
// тексты, спецификация «из ТЗ» (src/lib/tz.ts, src/lib/spec.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { TzOperation } from '../src/api/typesTz.ts'
import {
  TZ_MAX_BYTES,
  backLabel,
  matchStatus,
  moreVariantsLabel,
  parseTzHash,
  parseViewState,
  pollOperation,
  positionLabel,
  positionPurchase,
  positionQuantity,
  positionSearchQuery,
  reasonToError,
  resultsTitle,
  runLimited,
  sameQuery,
  shownProducts,
  stageStep,
  stageSteps,
  tzSourceLabel,
  validateTzFile,
  validateTzFiles,
} from '../src/lib/tz.ts'
import {
  addLine,
  lineFromItem,
  lineFromTz,
  originLabel,
  parseStored,
  quoteRequest,
  removeLine,
  sourceNote,
  specText,
  tzPositionsInSpec,
} from '../src/lib/spec.ts'
import { TZ_DOC, item } from './tzFixtures.ts'

const [P1, , P3, P4, , , , P8] = TZ_DOC.positions as [
  (typeof TZ_DOC.positions)[number],
  ...(typeof TZ_DOC.positions)[number][],
]

test('файл: PDF и DOCX до 20 МБ; другой формат, больше 20 МБ и несколько файлов отклоняются', () => {
  assert.equal(validateTzFile({ name: 'TZ.pdf', size: 1000 }), null)
  assert.equal(validateTzFile({ name: 'TZ.DOCX', size: TZ_MAX_BYTES }), null)
  assert.equal(validateTzFile({ name: 'TZ.doc', size: 1000 }), 'format')
  assert.equal(validateTzFile({ name: 'TZ.pdf.exe', size: 1000 }), 'format')
  assert.equal(validateTzFile({ name: 'TZ.pdf', size: TZ_MAX_BYTES + 1 }), 'size')
  assert.deepEqual(
    validateTzFiles([
      { name: 'a.pdf', size: 1 },
      { name: 'b.pdf', size: 1 },
    ]),
    { file: null, error: 'multiple' },
  )
  assert.deepEqual(validateTzFiles([{ name: 'a.pdf', size: 1 }]), { file: 0, error: null })
  assert.equal(reasonToError('password'), 'password')
  assert.equal(reasonToError('что-то новое'), 'general')
})

test('этапы: «Распознаю текст» — только если сервер прислал ocr', () => {
  assert.deepEqual(
    stageSteps('read', false).map((s) => [s.key, s.state]),
    [
      ['read', 'active'],
      ['search', 'todo'],
    ],
  )
  assert.deepEqual(
    stageSteps('ocr', false).map((s) => [s.key, s.state]),
    [
      ['read', 'done'],
      ['ocr', 'active'],
      ['search', 'todo'],
    ],
  )
  assert.equal(stageSteps('search', true).length, 3)
  assert.equal(stageStep('reading'), 'read')
  assert.equal(stageStep('ocr'), 'ocr')
  assert.equal(stageStep(undefined), 'read')
})

test('опрос: этапы по ответам сервера, completed возвращается', async () => {
  const answers: TzOperation[] = [
    { status: 'processing', stage: 'reading' },
    { status: 'processing', stage: 'ocr' },
    { status: 'completed', hash: 'abc' },
  ]
  const stages: (string | undefined)[] = []
  const op = await pollOperation(
    'op1',
    {
      get: async () => answers.shift() as TzOperation,
      wait: async () => undefined,
      onStage: (s) => stages.push(s),
    },
    new AbortController().signal,
  )
  assert.deepEqual(op, { status: 'completed', hash: 'abc' })
  assert.deepEqual(stages, ['reading', 'ocr'])
})

test('опрос: отмена во время запроса — поздний completed игнорируется (null)', async () => {
  const c = new AbortController()
  let calls = 0
  const op = await pollOperation(
    'op1',
    {
      get: async () => {
        calls++
        if (calls === 2) c.abort()
        return calls === 2 ? { status: 'completed', hash: 'late' } : { status: 'processing' }
      },
      wait: async () => undefined,
    },
    c.signal,
  )
  assert.equal(op, null)
  assert.equal(calls, 2)
})

test('опрос: отмена во время паузы — новых запросов нет', async () => {
  const c = new AbortController()
  let calls = 0
  const op = await pollOperation(
    'op1',
    {
      get: async () => {
        calls++
        return { status: 'processing', stage: 'reading' }
      },
      wait: async () => {
        c.abort()
      },
    },
    c.signal,
  )
  assert.equal(op, null)
  assert.equal(calls, 1)
})

test('опрос: три сетевых сбоя подряд — общая ошибка; 404 операции — сразу ошибка', async () => {
  let calls = 0
  const op = await pollOperation(
    'op1',
    {
      get: async () => {
        calls++
        throw { status: 0 }
      },
      wait: async () => undefined,
    },
    new AbortController().signal,
  )
  assert.deepEqual(op, { status: 'error', reason: 'general' })
  assert.equal(calls, 3)
  const gone = await pollOperation(
    'op1',
    {
      get: async () => {
        throw { status: 404 }
      },
      wait: async () => undefined,
    },
    new AbortController().signal,
  )
  assert.deepEqual(gone, { status: 'error', reason: 'general' })
})

test('поиск по позициям: не больше четырёх одновременно, все в порядке очереди, ошибка не останавливает', async () => {
  let running = 0
  let peak = 0
  const started: number[] = []
  const done: number[] = []
  await runLimited(
    Array.from({ length: 10 }, (_, i) => i),
    4,
    async (i) => {
      started.push(i)
      running++
      peak = Math.max(peak, running)
      await new Promise((r) => setTimeout(r, 5 + ((i * 7) % 4)))
      running--
      if (i === 3) throw new Error('сбой позиции')
      done.push(i)
    },
  )
  assert.equal(peak, 4)
  assert.deepEqual(started, [0, 1, 2, 3, 4, 5, 6, 7, 8, 9])
  assert.equal(done.length, 9)
})

test('поиск по позициям: отмена — новые позиции не начинаются', async () => {
  const c = new AbortController()
  const started: number[] = []
  await runLimited(
    [0, 1, 2, 3, 4, 5],
    2,
    async (i) => {
      started.push(i)
      if (i === 1) c.abort()
    },
    c.signal,
  )
  assert.ok(started.length <= 3)
})

test('статус позиции по TZ-CORE.md §6', () => {
  const exact = item('kes', { match_type: 'exact' })
  const related = item('cyber', { match_type: 'related', group: 'match' })
  const fallback = item('pop', { match_type: 'fallback', group: 'analog' })
  // назван и есть точное — Найдено
  assert.equal(matchStatus(P1, [exact, related]), 'found')
  // назван, но точного нет — Похожие
  assert.equal(matchStatus(P1, [related]), 'similar')
  // не назван, есть товары — Похожие, даже при точном
  assert.equal(matchStatus(P3, [exact]), 'similar')
  // товаров нет или только «Популярное в наличии» — Нет в каталоге
  assert.equal(matchStatus(P1, []), 'not_found')
  assert.equal(matchStatus(P3, [fallback]), 'not_found')
  // работа без точного совпадения — сразу Нет в каталоге, а не «Похожие»
  assert.equal(matchStatus(P8, [related]), 'not_found')
  // старый ответ без match_type — по группе
  assert.equal(matchStatus(P1, [item('a', { group: 'exact' })]), 'found')
})

test('незавершённая позиция никогда не «Нет в каталоге»; ошибка — «Ошибка поиска»', () => {
  assert.equal(positionLabel('pending', null), 'Ожидает поиска')
  assert.equal(positionLabel('searching', null), 'Ищу товары')
  assert.equal(positionLabel('error', null), 'Ошибка поиска')
  assert.equal(positionLabel('error', 'not_found'), 'Ошибка поиска')
  assert.equal(positionLabel('ready', 'not_found'), 'Нет в каталоге')
  assert.equal(positionLabel('ready', 'found'), 'Найдено')
})

test('карточки: «Найдено» — одна и до трёх под «ещё»; «Похожие» — до четырёх', () => {
  const six = Array.from({ length: 6 }, (_, i) => item(`i${i}`))
  const f = shownProducts('found', six)
  assert.equal(f.first.length, 1)
  assert.equal(f.more.length, 3)
  const s = shownProducts('similar', six)
  assert.equal(s.first.length, 4)
  assert.equal(s.more.length, 0)
  assert.deepEqual(shownProducts('not_found', six), { first: [], more: [] })
})

test('тексты: склонения и заголовок', () => {
  assert.equal(moreVariantsLabel(1), 'Ещё 1 вариант')
  assert.equal(moreVariantsLabel(2), 'Ещё 2 варианта')
  assert.equal(moreVariantsLabel(5), 'Ещё 5 вариантов')
  assert.equal(resultsTitle('TZ.pdf', 8, 7), 'По ТЗ «TZ.pdf» — 8 позиций · найдено 7')
  assert.equal(resultsTitle('TZ.pdf', 1, null), 'По ТЗ «TZ.pdf» — 1 позиция')
  assert.equal(resultsTitle('TZ.pdf', 3, 0), 'По ТЗ «TZ.pdf» — 3 позиции · найдено 0')
  assert.equal(backLabel('TZ.pdf', 3, 8), 'Вернуться к ТЗ «TZ.pdf» · позиция 3 из 8')
  assert.equal(tzSourceLabel([3]), 'по ТЗ, п. 3')
  assert.equal(tzSourceLabel([4, 1]), 'по ТЗ, п. 1 и 4')
  assert.equal(tzSourceLabel([1, 3, 4]), 'по ТЗ, п. 1, 3 и 4')
})

test('условия позиции: количество из ТЗ — по тому же правилу, что «Выбрано по запросу»', () => {
  assert.deepEqual(positionPurchase(P1), {
    query: 'Kaspersky Endpoint Security для бизнеса Стандартный',
    deal: 'new',
    quantity: 50,
    metric: 'device',
    duration_months: 12,
  })
  assert.equal(positionPurchase(P3), null)
  // ступень количества — цена за место, мест столько, сколько в ТЗ
  assert.equal(positionQuantity(item('kes', { tier_min: 50, tier_max: 99 }), P1), 50)
  // пакет на 10 устройств при 50 — 5 пакетов
  assert.equal(
    positionQuantity(
      item('pack', {
        attrs: { license_quantity: 10, license_metric: 'device' } as never,
      }),
      P1,
    ),
    5,
  )
  // поштучная позиция — 2 серверные лицензии
  assert.equal(positionQuantity(item('csp', { license_line: '1 сервер' }), P4), 2)
  assert.equal(positionQuantity(item('x'), P3), 1)
  // позиция каталога без своей метрики и пакета («Бессрочная») — количество из ТЗ как есть: 70 000 × 2
  assert.equal(positionQuantity(item('csp', { license_line: 'Бессрочная' }), P4), 2)
  // лицензия на ядра — 1, покупатель поправит
  assert.equal(
    positionQuantity(
      item('core', { attrs: { license_metric: 'core', license_quantity: 16 } as never }),
      P4,
    ),
    1,
  )
  // пакет на 5 мест без метрики при 50 — 10 пакетов
  assert.equal(
    positionQuantity(
      item('pack5', { attrs: { license_metric: '', license_quantity: 5 } as never }),
      P1,
    ),
    10,
  )
  const q = positionSearchQuery('h1', P1)
  assert.deepEqual(q, {
    q: 'Kaspersky Endpoint Security для бизнеса Стандартный',
    f: ['license_quantity:eq:50', 'duration_months:eq:12'],
    fromTz: 'h1',
    tzItem: '1',
  })
  assert.ok(sameQuery('  kaspersky  endpoint ', 'Kaspersky Endpoint'))
  assert.equal(parseTzHash('abc_-1'), 'abc_-1')
  assert.equal(parseTzHash('../x'), null)
  assert.equal(parseTzHash(undefined), null)
})

const tzRef = (number: number) => ({ hash: 'h1', file_name: 'TZ.pdf', number })

test('спецификация: строка «из ТЗ» с подписью «по ТЗ, п. N» и условиями позиции', () => {
  const line = lineFromTz(
    item('kes', { price_rub: 2715.3 }),
    50,
    { query: 'Kaspersky', deal: 'new', duration_months: 12 },
    tzRef(1),
  )
  assert.equal(line.origin, 'tz')
  assert.equal(line.quantity, 50)
  assert.equal(originLabel(line), 'из ТЗ')
  assert.equal(sourceNote(line), 'новая лицензия · 1 год · по ТЗ, п. 1')
  assert.ok(specText([line]).includes('из ТЗ, по ТЗ, п. 1'))
  const body = quoteRequest([line], { sessionId: 's', clientId: null })
  assert.equal(body.items[0]?.origin, 'tz')
  assert.equal(body.items[0]?.purchase?.quantity, undefined)
  assert.equal(body.items[0]?.purchase?.deal, 'new')
})

test('спецификация: один товар из двух позиций — одна строка, количество сложено, «п. 1 и 4»', () => {
  let lines = addLine([], lineFromTz(item('kes'), 50, null, tzRef(1)))
  lines = addLine(lines, lineFromTz(item('kes'), 10, null, tzRef(4)))
  assert.equal(lines.length, 1)
  assert.equal(lines[0]?.quantity, 60)
  assert.deepEqual(lines[0]?.tz?.positions, [1, 4])
  assert.equal(sourceNote(lines[0] as never), 'по ТЗ, п. 1 и 4')
  // повтор из той же позиции — без дубля, количество берётся новое, как раньше
  lines = addLine(lines, lineFromTz(item('kes'), 7, null, tzRef(4)))
  assert.equal(lines.length, 1)
  assert.equal(lines[0]?.quantity, 7)
  assert.deepEqual(lines[0]?.tz?.positions, [1, 4])
  // обычный поиск не задевает прежнюю логику: повтор заменяет количество
  let plain = addLine([], lineFromItem(item('a'), 2))
  plain = addLine(plain, lineFromItem(item('a'), 3))
  assert.equal(plain[0]?.quantity, 3)
})

test('отметка позиции: ставится добавлением, снимается удалением последнего товара позиции', () => {
  let lines = addLine([], lineFromTz(item('kes'), 50, null, tzRef(1)))
  lines = addLine(lines, lineFromTz(item('astra'), 50, null, tzRef(2)))
  lines = addLine(lines, lineFromTz(item('astra2'), 50, null, tzRef(2)))
  assert.deepEqual([...tzPositionsInSpec(lines, 'h1')].sort(), [1, 2])
  lines = removeLine(lines, 'astra')
  assert.ok(tzPositionsInSpec(lines, 'h1').has(2))
  lines = removeLine(lines, 'astra2')
  assert.ok(!tzPositionsInSpec(lines, 'h1').has(2))
  // другой хеш — свои отметки, спецификация общая
  assert.equal(tzPositionsInSpec(lines, 'other').size, 0)
  assert.equal(lines.length, 1)
})

test('хранение: origin tz и привязка к ТЗ переживают перезагрузку; битая привязка отбрасывается', () => {
  const line = lineFromTz(item('kes'), 50, { query: 'q', quantity: 50 }, tzRef(1))
  const back = parseStored(JSON.stringify([line, { ...line, product_id: 'b', tz: { hash: 5 } }]))
  assert.equal(back[0]?.origin, 'tz')
  assert.deepEqual(back[0]?.tz, { hash: 'h1', file_name: 'TZ.pdf', positions: [1] })
  assert.equal(back[1]?.origin, 'tz')
  assert.equal(back[1]?.tz, undefined)
  assert.equal(parseStored(JSON.stringify([{ ...line, origin: 'чужое' }]))[0]?.origin, 'search')
})

test('раскрытые блоки и прокрутка: разбор сохранённого', () => {
  assert.deepEqual(parseViewState('{"scrollY":420,"quotes":["p1"],"variants":["p2",3]}'), {
    scrollY: 420,
    quotes: ['p1'],
    variants: ['p2'],
  })
  assert.equal(parseViewState('не json'), null)
  assert.equal(parseViewState(null), null)
})
