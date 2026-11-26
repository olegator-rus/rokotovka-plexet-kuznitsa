// Вилка эффекта руководителя: поля допущений и подписи сценариев. Все числа считает сервер
// (GET /api/effect); здесь — только перевод долей в проценты для полей ввода и разбор ввода.
// Без DOM: tests/effect.test.ts.
import type { EffectParam, EffectScenario } from '@/api/types'
import { formatInt, formatPercent } from './format.ts'

/** Порядок сценариев на экране: консервативный первым. */
export const SCENARIO_ORDER = ['conservative', 'base', 'optimistic']

export function orderScenarios(list: EffectScenario[]): EffectScenario[] {
  const rank = (id: string) => {
    const i = SCENARIO_ORDER.indexOf(id)
    return i < 0 ? SCENARIO_ORDER.length : i
  }
  return [...list].sort((a, b) => rank(a.id) - rank(b.id))
}

/** Доля в поле ввода показывается в процентах. */
export function isShare(p: Pick<EffectParam, 'unit'>): boolean {
  return p.unit === 'доля'
}

/** Значение допущения для поля ввода: доля → проценты с одним знаком, рубли — целые. */
export function paramToInput(p: Pick<EffectParam, 'unit'>, value: number): string {
  if (!Number.isFinite(value)) return ''
  if (isShare(p)) return String(Math.round(value * 1000) / 10).replace('.', ',')
  return String(Math.round(value))
}

/** Разбор ввода: проценты (0; 100] → доля, рубли > 0. Неверный ввод — null. */
export function inputToParam(p: Pick<EffectParam, 'unit'>, raw: string): number | null {
  const s = raw.replace(/\s| /g, '').replace(',', '.')
  if (s === '' || !/^\d+(\.\d+)?$/.test(s)) return null
  const n = Number(s)
  if (!Number.isFinite(n) || n <= 0) return null
  if (isShare(p)) return n > 100 ? null : Math.round(n * 10) / 1000
  return n > 1e9 ? null : n
}

/** Подпись прироста: «+32 % к 245 заказам поиска сейчас». */
export function upliftText(s: Pick<EffectScenario, 'uplift'>, ordersMonth: number): string {
  if (s.uplift === null || !Number.isFinite(s.uplift))
    return 'текущих заказов из поиска нет — прирост не посчитан'
  return `+${formatPercent(s.uplift, 0)} к ${formatInt(ordersMonth)} заказам поиска сейчас`
}

/** Откуда значение допущения — подпись рядом с полем. */
export function sourceLabel(source: string): string {
  if (source === 'measured') return 'замер'
  if (source === 'request') return 'изменено'
  return 'допущение'
}
