// Очередь SearchOps (src/lib/queue.ts) и семь причин с владельцами (src/lib/causes.ts, контур доводки).
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Proposal } from '../src/api/types.ts'
import { sortQueue } from '../src/lib/queue.ts'
import { CAUSES, OWNER_OPTIONS, causeOfCode, causeView } from '../src/lib/causes.ts'

const prop = (over: Record<string, unknown>) =>
  ({
    id: 'p',
    status: 'pending',
    action: 'rules_patch',
    diagnosis: 'X',
    cluster: { id: 'c', label: 'c', queries: [], lost_demand: 0 },
    created_at: '2026-05-27T10:00:00Z',
    patch: null,
    evaluation: null,
    evidence: null,
    ...over,
  }) as unknown as Proposal & { priority_score?: number | null }

test('sortQueue: оценка приоритета по убыванию, затем потерянный спрос, затем новые', () => {
  const items = [
    prop({ id: 'a', cluster: { lost_demand: 50 } }),
    prop({ id: 'b', priority_score: 69942 }),
    prop({ id: 'c', priority_score: 281918.4 }),
    prop({ id: 'd', cluster: { lost_demand: 7 } }),
    prop({
      id: 'e',
      priority_score: null,
      cluster: { lost_demand: 50 },
      created_at: '2026-05-28T00:00:00Z',
    }),
  ]
  assert.deepEqual(
    sortQueue(items).map((p) => p.id),
    ['c', 'b', 'e', 'a', 'd'],
  )
})

test('причины: 7 штук, каждый код диагноза — ровно в одной, владельцы по таблице', () => {
  assert.equal(CAUSES.length, 7)
  const codes = CAUSES.flatMap((c) => c.codes)
  assert.equal(new Set(codes).size, codes.length)
  assert.equal(causeOfCode('ALIAS_GAP')?.owner, 'Администратор поиска')
  assert.equal(causeOfCode('ASSORTMENT_GAP')?.label, 'Товара нет в каталоге')
  assert.equal(causeOfCode('CONTENT_GAP')?.owner, 'Каталог')
  assert.equal(causeOfCode('MARGIN_OPPORTUNITY'), null)
  assert.equal(OWNER_OPTIONS.length, 7)
})

test('causeView: сервер главнее кода; ticket — задача владельцу; без причины — ручной разбор', () => {
  const fix = causeView({ diagnosis: 'TYPO', action: 'rules_patch' })
  assert.equal(fix.label, 'Поиск не понял запрос')
  assert.equal(fix.kind, 'search_fix')
  assert.deepEqual(fix.codes, ['TYPO'])
  const task = causeView({ diagnosis: 'ASSORTMENT_GAP', action: 'ticket' })
  assert.equal(task.kind, 'owner_task')
  assert.equal(task.owner, 'Категорийный менеджер')
  const server = causeView({ cause: 'content_gap', owner: 'Отдел каталога', diagnosis: 'TYPO' })
  assert.equal(server.label, 'В карточке не хватает данных')
  assert.equal(server.owner, 'Отдел каталога')
  const byLabel = causeView({ cause: 'Запрос не о товаре', action: 'ticket' })
  assert.equal(byLabel.cause?.id, 'non_product')
  assert.equal(causeView({ diagnosis: 'TYPO', owner: 'ecom' }).owner, 'Администратор поиска')
  const none = causeView({})
  assert.equal(none.label, 'Шум или недостаточно данных')
  assert.equal(none.owner, 'Ручной разбор')
})
