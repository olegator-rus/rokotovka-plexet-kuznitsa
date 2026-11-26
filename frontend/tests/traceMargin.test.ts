// Шаг трассы margin (src/lib/traceMargin.ts + подключение в src/lib/trace.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { TraceStep } from '../src/api/types.ts'
import { marginGuardLines, marginShiftLines, marginSummary } from '../src/lib/traceMargin.ts'
import { stepExtraFields, stepHasDetails, stepLabel, stepSummary } from '../src/lib/trace.ts'

const step: TraceStep = {
  name: 'margin',
  ms: 1,
  policy_version: 3,
  weight: 0.05,
  shifts: [{ product_id: 'p4', from: 4, to: 3 }],
  guards: ['exact_top1'],
}

test('итог шага маржи и строки сдвигов', () => {
  assert.equal(marginSummary(step), '1 сдвиг · 1 защита · вес 0,05')
  assert.equal(marginSummary({ name: 'margin', ms: 0 }), 'без сдвигов')
  assert.deepEqual(marginShiftLines(step, { p4: 'Kaspersky' }), [
    { title: 'Kaspersky', text: 'позиция 4 → 3: маржа', up: true },
  ])
  assert.deepEqual(marginGuardLines(step), ['первая позиция точного запроса неподвижна'])
})

test('трасса: подпись, сводка, раскрытие, поля не дублируются в «ключ: значение»', () => {
  assert.equal(stepLabel('margin'), 'Маржа')
  assert.equal(stepSummary(step), '1 сдвиг · 1 защита · вес 0,05')
  assert.equal(stepHasDetails(step), true)
  assert.deepEqual(stepExtraFields(step), [])
})
