// Справочник связей (src/lib/relations.ts): подписи, фильтры, «проверить», разница версий.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Relation, RelationVersion } from '../src/api/typesCommerce.ts'
import {
  evidenceStrength,
  filterOptions,
  filterRelations,
  needsReview,
  patchSummary,
  relationDiff,
  relationEnds,
  relationHistory,
  relationLine,
} from '../src/lib/relations.ts'

const ws: Relation = {
  id: 'rel-1',
  from_family: 'windows-server-2025',
  to_family: 'windows-server-cal-2022',
  from_title: 'Windows Server 2025',
  to_title: 'Windows Server CAL 2022',
  type: 'complement',
  requirement: 'conditional_required',
  compatibility: 'verified',
  source: 'catalog_rule',
  evidence_level: 'E3',
  review_status: 'approved',
  reviewed_at: '2026-05-22',
}
const r2: Relation = {
  id: 'rel-2',
  from_family: 'dell-poweredge-r740',
  to_family: 'ddr4-32',
  type: 'compatibility',
  source: 'vendor_documentation',
  review_status: 'pending',
  reviewed_at: '2024-07-03',
}

test('концы связи: название, иначе позиция, иначе семейство', () => {
  assert.deepEqual(relationEnds(ws), { from: 'Windows Server 2025', to: 'Windows Server CAL 2022' })
  assert.deepEqual(relationEnds(r2), { from: 'dell-poweredge-r740', to: 'ddr4-32' })
})

test('строка связи словами', () => {
  assert.equal(
    relationLine(ws),
    'дополнение · условно обязательное · совместимость подтверждена · E3',
  )
  assert.equal(relationLine(r2), 'совместимость · необязательное · совместимость не подтверждена')
})

test('«проверить»: без даты и старше 180 дней', () => {
  const now = Date.parse('2026-05-27')
  assert.equal(needsReview(ws, now), false)
  assert.equal(needsReview(r2, now), true)
  assert.equal(needsReview({ reviewed_at: null }, now), true)
})

test('фильтры: товар подстрокой, тип, источник, статус', () => {
  const all = [ws, r2]
  const f = { q: '', type: '', source: '', status: '' }
  assert.equal(filterRelations(all, { ...f, q: 'cal' }).length, 1)
  assert.equal(filterRelations(all, { ...f, q: 'R740' })[0].id, 'rel-2')
  assert.equal(filterRelations(all, { ...f, type: 'compatibility' })[0].id, 'rel-2')
  assert.equal(filterRelations(all, { ...f, status: 'approved' })[0].id, 'rel-1')
  assert.equal(filterRelations(all, { ...f, source: 'expert' }).length, 0)
  assert.deepEqual(filterOptions(all, 'review_status'), ['approved', 'pending'])
})

test('уровень доказательности: E2 и выше — сильное', () => {
  assert.equal(evidenceStrength('E2'), 'сильное')
  assert.equal(evidenceStrength('E1'), 'слабое — нужна ручная проверка')
  assert.equal(evidenceStrength(null), null)
})

test('разница: было/стало у изменённой, удалённая по id, добавленная без «было»', () => {
  const fixed = { ...ws, to_family: 'windows-server-cal-2025', to_title: 'Windows Server CAL 2025' }
  const rows = relationDiff({ update: [fixed], remove: ['rel-2'], add: [] }, [ws, r2])
  assert.equal(rows[0].op, 'update')
  assert.equal(
    rows[0].title,
    'Windows Server 2025 → Windows Server CAL 2022  ⇒  Windows Server CAL 2025',
  )
  assert.ok(rows[0].before)
  assert.equal(rows[1].op, 'remove')
  assert.equal(rows[1].after, null)
  const withBefore = relationDiff({ update: [{ ...fixed, before: ws }] })
  assert.match(withBefore[0].title, /CAL 2022/)
  assert.equal(patchSummary({ update: [fixed], remove: ['x'] }), 'изменено 1 · удалено 1')
  assert.equal(patchSummary(null), 'без изменений')
})

test('история связи: версии, где она менялась, новые сверху', () => {
  const v = (version: number, diff: RelationVersion['diff']): RelationVersion => ({
    version,
    created_at: '2026-05-27T10:00:00Z',
    diff,
  })
  const h = relationHistory(
    [v(1, null), v(2, { update: [ws] }), v(3, { remove: ['rel-1'] }), v(4, { add: [r2] })],
    'rel-1',
  )
  assert.deepEqual(
    h.map((x) => x.version),
    [3, 2],
  )
})
