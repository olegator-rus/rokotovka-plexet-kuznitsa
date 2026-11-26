// Вилка эффекта руководителя (src/lib/effect.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  inputToParam,
  orderScenarios,
  paramToInput,
  sourceLabel,
  upliftText,
} from '../src/lib/effect.ts'
import type { EffectScenario } from '../src/api/types.ts'

const share = { unit: 'доля' }
const rub = { unit: '₽' }

test('paramToInput: доля — проценты с запятой, рубли — целые', () => {
  assert.equal(paramToInput(share, 0.1772), '17,7')
  assert.equal(paramToInput(share, 0.3), '30')
  assert.equal(paramToInput(rub, 54175.4), '54175')
})

test('inputToParam: проценты → доля, неверный ввод — null', () => {
  assert.equal(inputToParam(share, '30'), 0.3)
  assert.equal(inputToParam(share, '17,7'), 0.177)
  assert.equal(inputToParam(share, '0'), null)
  assert.equal(inputToParam(share, '120'), null)
  assert.equal(inputToParam(share, 'abc'), null)
  assert.equal(inputToParam(rub, '54 175'), 54175)
  assert.equal(inputToParam(rub, '-5'), null)
})

test('orderScenarios: консервативный первым', () => {
  const s = (id: string) => ({ id }) as EffectScenario
  assert.deepEqual(
    orderScenarios([s('optimistic'), s('base'), s('conservative')]).map((x) => x.id),
    ['conservative', 'base', 'optimistic'],
  )
})

test('upliftText и sourceLabel', () => {
  assert.equal(
    upliftText({ uplift: 0.3199 }, 245).replace(/\u00a0/g, ' '),
    '+32 % к 245 заказам поиска сейчас',
  )
  assert.match(upliftText({ uplift: null }, 0), /не посчитан/)
  assert.equal(sourceLabel('measured'), 'замер')
  assert.equal(sourceLabel('assumed'), 'допущение')
  assert.equal(sourceLabel('request'), 'изменено')
})
