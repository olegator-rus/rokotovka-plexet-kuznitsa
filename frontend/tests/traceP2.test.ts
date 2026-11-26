// Шаги трассы контура коммерции: hardware и companions (src/lib/traceP2.ts, подписи в src/lib/trace.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { stepExtraFields, stepHasDetails, stepLabel, stepSummary } from '../src/lib/trace.ts'
import { companionRejectLabel, unitLabel } from '../src/lib/traceP2.ts'

test('шаг hardware: парт-номер, единицы, модель', () => {
  const step = {
    name: 'hardware',
    ms: 1,
    mpn_detected: 'HMA82GR7CJR8NXN',
    units: [{ text: '32 гб', key: 'ram_gb', op: 'eq', value: 32, unit: 'ГБ' }],
    compat_target: 'dell-poweredge-r740',
  }
  assert.equal(stepLabel('hardware'), 'Оборудование')
  assert.equal(
    stepSummary(step),
    'парт-номер HMA82GR7CJR8NXN · 1 единица · совместимость с dell-poweredge-r740',
  )
  assert.equal(stepHasDetails(step), true)
  assert.deepEqual(stepExtraFields(step), [])
  assert.equal(
    unitLabel({ key: 'screen_inch', op: 'gte', value: 27, unit: '″' }),
    'screen_inch ≥ 27 ″',
  )
  assert.equal(stepSummary({ name: 'hardware', ms: 0 }), 'номеров и единиц нет')
})

test('шаг companions: якорь, кандидаты, отклонённые с причиной', () => {
  const step = {
    name: 'companions',
    ms: 2,
    anchor: { product_id: 'ws-2025-std', confidence: 'high' },
    considered: [
      { product_id: 'cal-2025', class: 'conditional_required', source: 'vendor_documentation' },
    ],
    rejected: [{ product_id: 'cal-2022', reason: 'version_mismatch' }],
  }
  assert.equal(stepLabel('companions'), 'Компаньоны')
  assert.equal(stepSummary(step), 'якорь найден · 1 кандидат · отклонено 1')
  assert.deepEqual(stepExtraFields(step), [])
  assert.equal(companionRejectLabel('version_mismatch'), 'несовместимо по версии')
  assert.equal(companionRejectLabel('что-то новое'), 'что-то новое')
  assert.equal(
    stepSummary({ name: 'companions', ms: 0, anchor: null }),
    'якоря нет — блок не показан',
  )
})
