// Рабочая область подбора (src/lib/guideView.ts, контур экрана): поля условий, строка условий для телефона, колонки на
// телефоне, итоговая строка для выбранной позиции без пересчёта, запасной экран из прежних полей.
import test from 'node:test'
import assert from 'node:assert/strict'
import type {
  GuideCondition,
  GuideOffer,
  GuideResponse,
  GuideScreen,
} from '../src/api/typesGuide.ts'
import {
  clampCount,
  conditionsLine,
  countFields,
  featureLabel,
  offerSubtitle,
  offerTitle,
  otherConditions,
  phoneColumns,
  screenOf,
  summaryFor,
  taskText,
  termLabel,
  termOptions,
} from '../src/lib/guideView.ts'

/** Пробелы в числах — неразрывные (format.ts): сравниваем по обычным. */
const plain = (s: string | undefined) => (s ?? '').replace(/[\u00a0\u202f]/g, ' ')

const cond = (key: string, label: string, value: string, origin = 'user'): GuideCondition => ({
  key,
  label,
  value,
  origin,
})

const offer = (over: Partial<GuideOffer>): GuideOffer => ({
  offer_id: 'o1',
  title: 'Kaspersky Endpoint Security для бизнеса Расширенный базовая лицензия на 1 год',
  family_title: 'Kaspersky Endpoint Security для бизнеса Расширенный',
  sku: 'KL4867RAQFS',
  tier: '50–99',
  unit: 'за компьютер',
  price_per_unit: 3940.2,
  total: 197010,
  price_state: 'priced',
  offer_url: '',
  quantity: 50,
  term: '1 год',
  role: 'match',
  ...over,
})

test('поля количества: по строке на объект, число из значения', () => {
  const f = countFields([
    cond('direction', 'антивирус', 'endpoint', 'inferred'),
    cond('count:workstation', '50 ПК', '50'),
    cond('count:server', '2 сервера', '2'),
  ])
  assert.deepEqual(
    f.map((x) => [x.key, x.label, x.value]),
    [
      ['count:workstation', 'Компьютеры', 50],
      ['count:server', 'Серверы', 2],
    ],
  )
})

test('количество: целое, не меньше 1, мусор — null', () => {
  assert.equal(clampCount('30'), 30)
  assert.equal(clampCount(0), null)
  assert.equal(clampCount('abc'), null)
  assert.equal(clampCount(2.6), 3)
})

test('строка условий для телефона: «50 ПК · Новая · 1 год · Шифрование данных»', () => {
  const line = conditionsLine([
    cond('count:workstation', '50 ПК', '50'),
    cond('operation', 'новая лицензия', 'new', 'default'),
    cond('term_months', '1 год', '12', 'default'),
    cond('feature:encryption', 'нужно шифрование данных', 'encryption'),
  ])
  assert.equal(line, '50 ПК · Новая · 1 год · Шифрование данных')
})

test('прочие условия: тип организации по умолчанию не показывается, заданный — показывается', () => {
  const list = [
    cond('org_type', 'коммерческая организация', 'commercial', 'default'),
    cond('platform', 'Astra Linux', 'astra'),
    cond('operation', 'новая лицензия', 'new', 'default'),
  ]
  assert.deepEqual(
    otherConditions(list).map((c) => c.key),
    ['platform'],
  )
  assert.deepEqual(
    otherConditions([cond('org_type', 'для дома', 'home')]).map((c) => c.key),
    ['org_type'],
  )
})

test('срок: подписи и вариант сервера вне списка', () => {
  assert.equal(termLabel(12), '1 год')
  assert.equal(termLabel(24), '2 года')
  assert.equal(termLabel(0), 'бессрочная')
  assert.equal(termOptions('12').length, 3)
  assert.equal(termOptions('6').at(-1)?.label, '6 месяцев')
})

test('функция без «нужно:»', () => {
  assert.equal(featureLabel('нужно: шифрование данных'), 'Шифрование данных')
})

test('колонки на телефоне: не больше двух', () => {
  assert.deepEqual(phoneColumns(2, 0), [0, 1])
  assert.deepEqual(phoneColumns(3, 2), [0, 2])
  assert.deepEqual(phoneColumns(3, 0), [0, 1])
})

test('название лицензии: готовое от сервера главнее полей каталога', () => {
  const o = offer({
    display: {
      title: 'Kaspersky Endpoint Security — Расширенный',
      subtitle: 'Новая лицензия · 50 компьютеров · 1 год',
    },
  })
  assert.equal(offerTitle(o), 'Kaspersky Endpoint Security — Расширенный')
  assert.equal(offerSubtitle(o), 'Новая лицензия · 50 компьютеров · 1 год')
  assert.equal(offerTitle(offer({})), 'Kaspersky Endpoint Security для бизнеса Расширенный')
})

test('итоговая строка: предвыбранная — готовая сводка, другая — из полей карточки без пересчёта', () => {
  const screen: Pick<GuideScreen, 'summary'> = {
    summary: {
      offer_id: 'std',
      label: 'Стоимость закупки',
      total: 84402,
      total_text: '84 402 ₽',
      formula: '30 × 2 813,40 ₽',
      action: 'add_to_spec',
      hint: '',
    },
  }
  const std = offer({ offer_id: 'std' })
  assert.equal(summaryFor(screen, std)?.total_text, '84 402 ₽')
  const adv = offer({ offer_id: 'adv', total: 122445, price_line: '30 × 4 081,50 ₽' })
  const s = summaryFor(screen, adv)!
  assert.equal(s.offer_id, 'adv')
  assert.equal(plain(s.total_text), '122 445 ₽')
  assert.equal(s.formula, '30 × 4 081,50 ₽')
  assert.equal(s.action, 'add_to_spec')
  const quote = summaryFor(
    screen,
    offer({ offer_id: 'q', price_state: 'on_request', total: null }),
  )!
  assert.equal(quote.action, 'request_quote')
  assert.equal(quote.total_text, 'Цена по запросу')
})

test('итоговая строка без выбора: только «выберите вариант»', () => {
  const choose = {
    summary: {
      offer_id: '',
      label: '',
      total: null,
      total_text: '',
      formula: '',
      action: 'choose',
      hint: 'Выберите вариант, чтобы добавить в спецификацию',
    },
  }
  assert.equal(summaryFor(choose, null)?.action, 'choose')
  assert.equal(summaryFor({ summary: null }, null), null)
})

test('экран — только от сервера; задача — из screen или условий', () => {
  const r = {
    type: 'selection',
    offers: [offer({ offer_id: 'a' }), offer({ offer_id: 'b' })],
    conditions: [cond('vendor', 'КриптоПро', 'kripto-pro')],
    claims: [],
  } as unknown as GuideResponse
  assert.equal(screenOf(r), null)
  assert.equal(taskText(r), 'КриптоПро')
  const withScreen = { ...r, screen: { task: 'Своё' } as GuideScreen }
  assert.equal(screenOf(withScreen)?.task, 'Своё')
  assert.equal(taskText(withScreen), 'Своё')
})
