// Закупка под задачу: выбор в слотах, условный слот, строки «Добавить всё» (src/lib/solution.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { SearchItemLite, Solution } from '../src/api/typesP2.ts'
import {
  chosenItem,
  defaultChoice,
  slotActive,
  slotQty,
  solutionLines,
  solutionSummary,
} from '../src/lib/solution.ts'
import { solutionSummary as solutionSummaryOf, upgradeSummary } from '../src/lib/traceP2.ts'
import { STEP_LABELS } from '../src/lib/trace.ts'

const stepLabel = (name: string) => STEP_LABELS[name]

const item = (id: string, price: number | null): SearchItemLite => ({
  product_id: id,
  family_id: id,
  title: id,
  vendor: 'v',
  category: 'c',
  license_line: '',
  price_rub: price,
  price_on_request: price === null,
  price_mode: price === null ? 'quote_required' : 'fixed',
})

const sol: Solution = {
  template_id: 'tmpl-workplace',
  title: 'Рабочее место сотрудника',
  seats: 20,
  source: 'Собрано по шаблону Softline',
  total_rub: null,
  on_request: 0,
  slots: [
    {
      id: 'laptop',
      label: 'Ноутбук',
      category_id: 'laptops',
      query: 'ноутбук',
      quantity_strategy: 'per_seat',
      quantity: 20,
      item: item('nb-os', 100),
      alternatives: [item('nb-noos', 90)],
      quantities: { 'nb-os': 20, 'nb-noos': 20 },
      active: true,
      condition: null,
    },
    {
      id: 'os',
      label: 'ОС',
      category_id: 'os-desktop',
      query: 'windows',
      quantity_strategy: 'per_seat',
      quantity: 20,
      item: item('win', 10),
      alternatives: [],
      quantities: { win: 20 },
      active: false,
      condition: { slot: 'laptop', key: 'os', value: 'без ОС', product_ids: ['nb-noos'] },
    },
    {
      id: 'security',
      label: 'Защита',
      category_id: 'antivirus-business',
      query: 'kes',
      quantity_strategy: 'per_seat',
      quantity: 2,
      item: item('kes-10', 30),
      alternatives: [item('kes-25', null)],
      quantities: { 'kes-10': 2, 'kes-25': 1 },
      active: true,
      condition: null,
    },
  ],
}

test('по умолчанию: условный слот не нужен, фасовка — от сервера', () => {
  const ch = defaultChoice(sol)
  assert.equal(slotActive(sol.slots[1]!, sol, ch), false)
  const lines = solutionLines(sol, ch)
  assert.deepEqual(
    lines.map((l) => [l.product_id, l.quantity, l.origin]),
    [
      ['nb-os', 20, 'template'],
      ['kes-10', 2, 'template'],
    ],
  )
  assert.equal(lines[0]!.anchor, 'Рабочее место сотрудника')
  assert.equal(solutionSummary(sol, ch).knownRub, 100 * 20 + 30 * 2)
})

test('ноутбук без ОС включает слот ОС; замена берёт своё количество', () => {
  const ch = { ...defaultChoice(sol), laptop: 'nb-noos', security: 'kes-25' }
  assert.equal(slotActive(sol.slots[1]!, sol, ch), true)
  assert.equal(chosenItem(sol.slots[2]!, ch)?.product_id, 'kes-25')
  assert.equal(slotQty(sol.slots[2]!, ch), 1)
  const s = solutionSummary(sol, ch)
  assert.equal(s.positions, 3)
  assert.equal(s.onRequest, 1)
})

test('убранный покупателем слот не попадает в спецификацию', () => {
  const lines = solutionLines(sol, defaultChoice(sol), new Set(['security']))
  assert.deepEqual(
    lines.map((l) => l.product_id),
    ['nb-os'],
  )
})

test('шаги трассы solution и upgrade: короткая сводка', () => {
  assert.equal(
    solutionSummaryOf({
      name: 'solution',
      ms: 1,
      template_id: 'tmpl-workplace',
      seats: 20,
      slots: [{ id: 'laptop', product_id: 'nb' }, { id: 'os' }],
    }),
    'tmpl-workplace · 20 мест · слотов 1 из 2',
  )
  assert.equal(
    solutionSummaryOf({ name: 'solution', ms: 0, skipped: true, reason: 'no_template', slots: [] }),
    'шаблон не подошёл — обычная выдача',
  )
  assert.equal(
    upgradeSummary({
      name: 'upgrade',
      ms: 0,
      for_product_id: 'ws25-std16',
      product_id: 'ws25-dc16',
    }),
    'ws25-std16 → ws25-dc16',
  )
  assert.equal(stepLabel('solution'), 'Решение по шаблону')
})
