// Акции, строка сравнения и ступень количества (src/lib/offerNotes.ts), подписи основания (src/lib/companions.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Comparison } from '../src/api/types.ts'
import {
  BLOCK_TITLE,
  blockItems,
  blockTitle,
  companionBadges,
  evidenceView,
  noDates,
  reasonText,
} from '../src/lib/companions.ts'
import {
  comparisonView,
  hasTier,
  perPlaceLabel,
  promoView,
  promoViews,
  tierLabel,
} from '../src/lib/offerNotes.ts'
import { companion } from './p2fixtures.ts'

const NBSP = '\u00a0'

test('основание: A — документ вендора со ссылкой, B — каталог, C — проверьте совместимость', () => {
  const a = evidenceView(
    companion('cp', {
      evidence_level: 'A',
      evidence: {
        level: 'A',
        kind: 'compat_document',
        title: 'сертификат Ready for Astra № 29073/2025 от 23.01.2025',
        url: 'https://astra.ru/cert.pdf',
        doc_number: '29073/2025',
      },
    }),
  )
  assert.deepEqual(a, {
    level: 'A',
    tag: 'по документу вендора',
    reasonInTag: false,
    docUrl: 'https://astra.ru/cert.pdf',
    docTitle: 'сертификат Ready for Astra № 29073/2025',
  })
  const b = evidenceView(
    companion('sup', {
      evidence: {
        level: 'B',
        kind: 'license_kind',
        title: 'Каталог',
        url: 'https://store.example/fam/',
        doc_number: '',
      },
    }),
  )
  assert.equal(b?.tag, 'из каталога: тип лицензии')
  assert.equal(b?.docUrl, '', 'у B ссылки на документ нет')
  const c = evidenceView(companion('x', { evidence_level: 'c', source_url: 'https://e.x/doc' }))
  assert.equal(c?.tag, 'проверьте совместимость')
  assert.equal(c?.docUrl, '')
})

test('основание class_logic: «по логике класса: <причина>», причина отдельной строкой не повторяется', () => {
  const v = evidenceView(
    companion('kis', {
      reason: 'Офису нужна защита. Вариант для Microsoft Office 365',
      evidence: {
        level: 'B',
        kind: 'class_logic',
        title: 'Логика заказчика: офис → антивирус',
        url: '',
        doc_number: '',
      },
    }),
  )
  assert.equal(v?.tag, 'по логике класса: офису нужна защита. Вариант для Microsoft Office 365')
  assert.equal(v?.reasonInTag, true)
  assert.equal(v?.docUrl, '')
  const sku = evidenceView(
    companion('x', {
      evidence: { level: 'B', kind: 'sku_platform', title: '', url: '', doc_number: '' },
    }),
  )
  assert.equal(sku?.tag, 'из каталога: название позиции')
  // Без причины — просто «по логике класса».
  const bare = evidenceView(
    companion('y', {
      reason: '',
      evidence: { level: 'B', kind: 'class_logic', title: '', url: '', doc_number: '' },
    }),
  )
  assert.deepEqual([bare?.tag, bare?.reasonInTag], ['по логике класса', false])
})

test('бейджи карточки блока: «Скидка N%» тоном, «Акция» — только без строки акции, без повторов', () => {
  assert.deepEqual(companionBadges({ badges: ['Скидка 26%', 'Акция', 'Скидка 26%'], promos: [] }), [
    { text: 'Скидка 26%', discount: true },
    { text: 'Акция', discount: false },
  ])
  assert.deepEqual(
    companionBadges({
      badges: ['Акция'],
      promos: [{ id: 'p', title: 't', text: 'Акция: t', url: '' }],
    }),
    [],
  )
  assert.deepEqual(companionBadges({ badges: null, promos: null }), [])
})

test('основание: без уровня (справочник mock E4) — null; javascript: — не ссылка', () => {
  assert.equal(evidenceView(companion('m')), null)
  const bad = evidenceView(
    companion('b', { evidence_level: 'A', source_url: 'javascript:alert(1)' }),
  )
  assert.equal(bad?.docUrl, '')
})

test('заголовок блока: из ответа; «Часто покупают вместе» не пишем', () => {
  assert.equal(blockTitle({ title: 'Дополните решение' }), 'Дополните решение')
  assert.equal(blockTitle({ title: 'К этому подойдёт' }), 'К этому подойдёт')
  assert.equal(blockTitle({ title: 'Часто покупают вместе' }), BLOCK_TITLE)
  assert.equal(blockTitle({ title: '' }), BLOCK_TITLE)
  assert.equal(blockTitle(null), BLOCK_TITLE)
})

test('блок на фиде (evidence) — до 6 позиций, справочник mock — до 4', () => {
  const feed = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map((id) =>
    companion(id, {
      evidence: { level: 'B', kind: 'license_kind', title: '', url: '', doc_number: '' },
    }),
  )
  assert.equal(
    blockItems({ anchor: { product_id: 'x', family_id: 'y', confidence: 'high' }, items: feed })
      .length,
    6,
  )
  const mock = feed.map((c) => ({ ...c, evidence: undefined }))
  assert.equal(
    blockItems({ anchor: { product_id: 'x', family_id: 'y', confidence: 'high' }, items: mock })
      .length,
    4,
  )
})

test('даты из причины и подписей убираются, версии остаются', () => {
  assert.equal(
    reasonText({
      reason:
        'Совместимо с Astra Linux SE 1.7 — сертификат Ready for Astra № 29073/2025 от 23.01.2025',
    }),
    'Совместимо с Astra Linux SE 1.7 — сертификат Ready for Astra № 29073/2025',
  )
  assert.equal(noDates('KES 12.2.0 для Linux, до 2025-09-30'), 'KES 12.2.0 для Linux,')
  assert.equal(noDates('Astra Linux SE 1.6–1.8.0'), 'Astra Linux SE 1.6–1.8.0')
})

test('акция: «Акция: …» и ссылка «условия», хвост «условия по ссылке» снимается', () => {
  assert.deepEqual(
    promoView({
      id: 'drweb-50',
      title: 'Переход',
      text: 'Акция: −50% при переходе с антивируса другого производителя, условия по ссылке',
      url: 'https://store.example/specials/drweb/',
    }),
    {
      id: 'drweb-50',
      text: 'Акция: −50% при переходе с антивируса другого производителя',
      url: 'https://store.example/specials/drweb/',
    },
  )
  const noUrl = promoView({ id: 'p', title: 'скидка студентам', text: '', url: 'ftp://x' })
  assert.deepEqual(noUrl, { id: 'p', text: 'Акция: Скидка студентам', url: '' })
  assert.equal(promoView({ id: 'e', title: '', text: '', url: '' }), null)
  assert.equal(
    promoView({ id: 'd', title: '', text: 'Акция: до 02.07.2026 −20%', url: '' })?.text,
    'Акция: −20%',
  )
})

test('акции позиции: без повторов, не больше двух', () => {
  const p = { id: 'a', title: 'A', text: 'Акция: A', url: '' }
  assert.equal(
    promoViews([p, p, { ...p, id: 'b', text: 'Акция: B' }, { ...p, id: 'c', text: 'Акция: C' }])
      .length,
    2,
  )
  assert.deepEqual(promoViews(null), [])
})

const kesPair: Comparison = {
  text: 'При этих условиях Стандартный на 31% дешевле, чем Расширенный, за узел',
  items: [
    {
      product_id: '1',
      family_id: 'f1',
      name: 'Стандартный',
      title: 'KES Ст',
      per_place_rub: 3070.8,
    },
    {
      product_id: '2',
      family_id: 'f2',
      name: 'Расширенный',
      title: 'KES Расш',
      per_place_rub: 4451.94,
    },
  ],
  conditions: 'Продление · 2 года · 100–149 устройств · для коммерческих организаций',
  percent: 31,
}

test('сравнение: текст сервера и условия мелко; цены пары за место', () => {
  assert.deepEqual(comparisonView(kesPair), {
    text: 'При этих условиях Стандартный на 31% дешевле, чем Расширенный, за узел',
    conditions: 'Продление · 2 года · 100–149 устройств · для коммерческих организаций',
    prices: `Стандартный — 3${NBSP}070,80${NBSP}₽, Расширенный — 4${NBSP}451,94${NBSP}₽`,
  })
})

test('сравнение: оценочные слова и пустой текст — текст собирается из пары; нечего сравнивать — null', () => {
  assert.equal(
    comparisonView({ ...kesPair, text: 'Стандартный выгоднее Расширенного' })?.text,
    'При этих условиях Стандартный на 31% дешевле, чем Расширенный, за место',
  )
  assert.equal(
    comparisonView({ ...kesPair, text: '' })?.text,
    'При этих условиях Стандартный на 31% дешевле, чем Расширенный, за место',
  )
  assert.equal(comparisonView({ ...kesPair, text: '', items: [] }), null)
  assert.equal(comparisonView(null), null)
  assert.equal(
    comparisonView({ ...kesPair, conditions: 'Продление · цены на 27.05.2026' })?.conditions,
    'Продление · цены на',
  )
})

test('ступень количества: подпись и цена за место', () => {
  assert.equal(hasTier({ tier_min: 100, tier_max: 149 }), true)
  assert.equal(hasTier({}), false)
  assert.equal(tierLabel({ tier_min: 100, tier_max: 149 }), '100–149')
  assert.equal(tierLabel({ tier_min: 250, tier_max: null }), 'от 250')
  assert.equal(tierLabel({ tier_min: 5, tier_max: 5 }), '5')
  assert.equal(perPlaceLabel('device'), 'за узел')
  assert.equal(perPlaceLabel('user'), 'за пользователя')
})
