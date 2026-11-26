// Блок «Дополните решение», строка условно обязательного, совместимость (src/lib/companions.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  blockItems,
  classLabel,
  compatView,
  featureOn,
  firstImpression,
  hasConditional,
  impressionPayload,
  modelTitle,
  requiredHeadline,
  splitRequired,
} from '../src/lib/companions.ts'
import { companion } from './p2fixtures.ts'

test('блок: несовместимые — никогда, не больше 4, то, что в карточке, не повторяется', () => {
  const items = [
    companion('a', { class: 'complement' }),
    companion('b', { compatibility: 'incompatible', class: 'compatible' }),
    companion('c', { class: 'renewal' }),
    companion('d', { class: 'compatible' }),
    companion('e', { class: 'complement' }),
    companion('f', { class: 'complement' }),
  ]
  const block = { anchor: { product_id: 'ws', family_id: 'ws', confidence: 'high' }, items }
  assert.deepEqual(
    blockItems(block).map((c) => c.product_id),
    ['a', 'c', 'd', 'e'],
  )
  assert.deepEqual(
    blockItems(block, ['a']).map((c) => c.product_id),
    ['c', 'd', 'e', 'f'],
  )
  assert.deepEqual(blockItems(null), [])
})

test('флаги: нет features (старый сервер) — включено, false — выключено', () => {
  assert.equal(featureOn(undefined, 'companions'), true)
  assert.equal(featureOn({ companions: false }, 'companions'), false)
  assert.equal(featureOn({ companions: true }, 'hardware'), true)
})

test('подпись класса: от сервера, иначе по классу', () => {
  assert.equal(classLabel({ label: '', class: 'renewal' }), 'Продление и поддержка')
  assert.equal(
    classLabel({ label: 'Совместимо с Dell R740', class: 'compatible' }),
    'Совместимо с Dell R740',
  )
})

test('строка условно обязательного: количество из запроса, RDS отдельно, unknown — к менеджеру', () => {
  const cal = companion('CAL 2025', { title: 'лицензии доступа CAL 2025' })
  assert.equal(requiredHeadline(cal, 50), 'Проверьте необходимость: лицензии доступа CAL 2025 × 50')
  assert.equal(requiredHeadline(cal, null), 'Проверьте необходимость: лицензии доступа CAL 2025')
  const unk = companion('x', { compatibility: 'unknown', category: 'Лицензии доступа' })
  assert.equal(
    requiredHeadline(unk, 50),
    'Могут потребоваться лицензии доступа — уточните у менеджера',
  )

  const rds = companion('rds', { conditions: ['rds_planned'] })
  const bad = companion('bad', { compatibility: 'incompatible' })
  const v = splitRequired([cal, rds, bad])
  assert.deepEqual(
    v.base.map((c) => c.product_id),
    ['CAL 2025'],
  )
  assert.deepEqual(
    v.rds.map((c) => c.product_id),
    ['rds'],
  )
})

test('значок совместимости: подтверждено, при условии, не подтверждено; incompatible — без значка', () => {
  assert.deepEqual(
    compatView(
      { target_model: 'dell-poweredge-r740', state: 'verified', condition: '' },
      'Dell PowerEdge R740',
    ),
    { tone: 'ok', text: 'Подтверждено для Dell PowerEdge R740' },
  )
  assert.deepEqual(
    compatView({ target_model: 'x', state: 'conditional', condition: 'BIOS ≥ 3.06' }),
    {
      tone: 'cond',
      text: 'При условии: BIOS ≥ 3.06',
    },
  )
  assert.equal(
    compatView({ target_model: 'Dell R740', state: 'unknown', condition: '' })?.text,
    'Совместимость с Dell R740 не подтверждена',
  )
  assert.equal(compatView({ target_model: 'x', state: 'incompatible', condition: '' }), null)
  assert.equal(compatView(null), null)
  assert.equal(
    hasConditional([
      { compatibility: null },
      { compatibility: { target_model: 'x', state: 'conditional', condition: '' } },
    ]),
    true,
  )
  assert.equal(hasConditional([{}]), false)
})

test('модель для подписи', () => {
  assert.equal(modelTitle('dell-poweredge-r740'), 'Dell PowerEdge R740')
  assert.equal(modelTitle('hpe-proliant-dl380-gen10'), 'Hpe ProLiant DL380 GEN10')
})

test('показ блока — одно событие на поиск и место', () => {
  const seen = new Set<string>()
  assert.equal(firstImpression(seen, 'r1', 'block'), true)
  assert.equal(firstImpression(seen, 'r1', 'block'), false)
  assert.equal(firstImpression(seen, 'r1', 'required:ws'), true)
  assert.equal(firstImpression(seen, 'r2', 'block'), true)
  assert.equal(firstImpression(seen, '', 'block'), false)
  assert.deepEqual(impressionPayload('ws', [companion('cal')], 'block'), {
    anchor: 'ws',
    place: 'block',
    items: [{ product_id: 'cal', class: 'conditional_required' }],
  })
})
