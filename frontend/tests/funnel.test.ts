// Воронка поиска: доли переходов (src/lib/funnel.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import { funnelSteps, safeShare } from '../src/lib/funnel.ts'

test('funnelSteps: доля от предыдущего и от первого шага', () => {
  const s = funnelSteps([
    { key: 'searches', label: 'Поиски', count: 200 },
    { key: 'clicks', label: 'Клики', count: 100 },
    { key: 'carts', label: 'Корзины', count: 25 },
    { key: 'orders', label: 'Заказы', count: 5 },
  ])
  assert.equal(s[0].fromPrev, null)
  assert.equal(s[0].fromFirst, 1)
  assert.equal(s[1].fromPrev, 0.5)
  assert.equal(s[2].fromPrev, 0.25)
  assert.equal(s[3].fromPrev, 0.2)
  assert.equal(s[3].fromFirst, 0.025)
})

test('funnelSteps: нулевые шаги не дают NaN', () => {
  const s = funnelSteps([
    { key: 'searches', label: 'Поиски', count: 0 },
    { key: 'clicks', label: 'Клики', count: 0 },
  ])
  assert.equal(s[1].fromPrev, 0)
  assert.equal(s[1].fromFirst, 0)
})

test('safeShare: делитель 0 — null', () => {
  assert.equal(safeShare(1, 4), 0.25)
  assert.equal(safeShare(1, 0), null)
})
