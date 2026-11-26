// Форматирование чисел (src/lib/format.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  formatCost,
  formatDateTime,
  formatDecimal,
  formatFound,
  formatInt,
  formatMs,
  formatPercent,
  formatPp,
  formatPrice,
  formatRub,
  formatScore,
  formatSigned,
  plural,
  signOf,
} from '../src/lib/format.ts'

const NBSP = ' '

test('plural: 1, 2–4, 5–20, 11–14, 21', () => {
  const f: [string, string, string] = ['товар', 'товара', 'товаров']
  assert.equal(plural(1, f), 'товар')
  assert.equal(plural(3, f), 'товара')
  assert.equal(plural(5, f), 'товаров')
  assert.equal(plural(11, f), 'товаров')
  assert.equal(plural(14, f), 'товаров')
  assert.equal(plural(21, f), 'товар')
  assert.equal(plural(0, f), 'товаров')
})

test('formatInt: разряды через неразрывный пробел, минус — типографский', () => {
  assert.equal(formatInt(12990), `12${NBSP}990`)
  assert.equal(formatInt(1234567), `1${NBSP}234${NBSP}567`)
  assert.equal(formatInt(999), '999')
  assert.equal(formatInt(-1500), `−1${NBSP}500`)
})

test('formatDecimal: запятая и точность', () => {
  assert.equal(formatDecimal(0.8234), '0,82')
  assert.equal(formatDecimal(1234.5, 1), `1${NBSP}234,5`)
  assert.equal(formatDecimal(-0.001, 2), '0,00')
})

test('formatPrice: рубли, null — пусто', () => {
  assert.equal(formatPrice(12990), `12${NBSP}990${NBSP}₽`)
  assert.equal(formatPrice(99.5), `99,50${NBSP}₽`)
  assert.equal(formatPrice(null), '')
})

test('formatMs: мс до секунды, дальше секунды', () => {
  assert.equal(formatMs(41), `41${NBSP}мс`)
  assert.equal(formatMs(0.01), `<1${NBSP}мс`)
  assert.equal(formatMs(0), `0${NBSP}мс`)
  assert.equal(formatMs(97.75), `98${NBSP}мс`)
  assert.equal(formatMs(1240), `1,2${NBSP}с`)
})

test('formatCost: ноль, доли рубля, рубли', () => {
  assert.equal(formatCost(0), `0${NBSP}₽`)
  assert.equal(formatCost(0.0425), `0,0425${NBSP}₽`)
  assert.equal(formatCost(0.05), `0,05${NBSP}₽`)
  assert.equal(formatCost(1.5), `1,50${NBSP}₽`)
})

test('formatScore и formatFound', () => {
  assert.equal(formatScore(0.82), '0,82')
  assert.equal(formatFound(7, 41), `Найдено 7 · 41${NBSP}мс`)
  assert.equal(formatFound(1200, null), `Найдено 1${NBSP}200`)
})

test('formatPercent: доля в проценты, целые без дроби', () => {
  assert.equal(formatPercent(0.1234), `12,3${NBSP}%`)
  assert.equal(formatPercent(1), `100${NBSP}%`)
  assert.equal(formatPercent(0), `0${NBSP}%`)
  assert.equal(formatPercent(0.0005), `0,1${NBSP}%`)
  assert.equal(formatPercent(Number.NaN), '')
})

test('formatPp: процентные пункты со знаком', () => {
  assert.equal(formatPp(0.032), `+3,2${NBSP}п.п.`)
  assert.equal(formatPp(-0.1), `−10,0${NBSP}п.п.`)
  assert.equal(formatPp(0.00001), `0${NBSP}п.п.`)
})

test('formatSigned и signOf', () => {
  assert.equal(formatSigned(0.0512, 3), '+0,051')
  assert.equal(formatSigned(-1.5, 1), '−1,5')
  assert.equal(formatSigned(0.0001, 2), '0')
  assert.equal(signOf(-2), '−')
  assert.equal(signOf(0), '')
})

test('formatRub: от 100 ₽ — целые с разрядами, меньше — копейки', () => {
  assert.equal(formatRub(8096.4), `8${NBSP}096${NBSP}₽`)
  assert.equal(formatRub(2.6), `2,60${NBSP}₽`)
  assert.equal(formatRub(0.0168), `0,0168${NBSP}₽`)
})

test('formatDateTime: ISO → дд.мм.гггг, чч:мм; мусор — как есть', () => {
  const d = new Date(2026, 4, 27, 9, 5)
  assert.equal(formatDateTime(d.toISOString()), '27.05.2026, 09:05')
  assert.equal(formatDateTime('не дата'), 'не дата')
  assert.equal(formatDateTime(null), '')
})
