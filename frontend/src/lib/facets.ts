// Фасеты оборудования: фасет ответа → фильтр запроса → адрес. Без DOM:
// tests/facets.test.ts. В адресе фильтр — `f=key:op:value[:value2]` (повторяется).
import type { Facet, FilterOp, SearchFilter } from '@/api/typesP2'
import { formatDecimal } from './format.ts'

export const MAX_FILTERS = 20
const OPS: readonly FilterOp[] = ['eq', 'gte', 'lte', 'range', 'not']

function scalar(raw: string): string | number | boolean {
  if (raw === 'true') return true
  if (raw === 'false') return false
  const n = Number(raw)
  return raw.trim() !== '' && Number.isFinite(n) ? n : raw
}

/** «ram_gb:range:16:64» → фильтр; неверная форма — null. */
export function parseFilter(raw: string): SearchFilter | null {
  const parts = raw.split(':')
  if (parts.length < 3) return null
  const [key, op] = parts as [string, string]
  if (!key || !(OPS as readonly string[]).includes(op)) return null
  if (op === 'range') {
    if (parts.length !== 4) return null
    const a = Number(parts[2])
    const b = Number(parts[3])
    if (!Number.isFinite(a) || !Number.isFinite(b)) return null
    return { key, op: 'range', value: Math.min(a, b), value2: Math.max(a, b) }
  }
  const value = parts.slice(2).join(':')
  if (!value) return null
  return { key, op: op as FilterOp, value: scalar(value) }
}

export function serializeFilter(f: SearchFilter): string {
  if (f.op === 'range') return `${f.key}:range:${f.value}:${f.value2 ?? f.value}`
  return `${f.key}:${f.op}:${String(f.value)}`
}

export function parseFilters(raw: readonly string[]): SearchFilter[] {
  const out: SearchFilter[] = []
  for (const r of raw) {
    const f = parseFilter(r)
    if (f && !out.some((x) => x.key === f.key)) out.push(f)
    if (out.length >= MAX_FILTERS) break
  }
  return out
}

/** Поставить или заменить фильтр по ключу; null — снять. */
export function withFilter(
  filters: readonly SearchFilter[],
  key: string,
  next: SearchFilter | null,
): SearchFilter[] {
  const rest = filters.filter((f) => f.key !== key)
  return next ? [...rest, next].slice(0, MAX_FILTERS) : rest
}

/** Диапазон ползунка: вся ширина фасета — фильтра нет. */
export function rangeFilter(facet: Facet, lo: number, hi: number): SearchFilter | null {
  const min = facet.min ?? lo
  const max = facet.max ?? hi
  const a = Math.max(min, Math.min(lo, hi))
  const b = Math.min(max, Math.max(lo, hi))
  if (a <= min && b >= max) return null
  if (a > min && b >= max) return { key: facet.key, op: 'gte', value: a }
  if (a <= min && b < max) return { key: facet.key, op: 'lte', value: b }
  return { key: facet.key, op: 'range', value: a, value2: b }
}

/** Текущий диапазон фасета из фильтра (или весь). */
export function rangeOf(facet: Facet, f: SearchFilter | undefined): [number, number] {
  const min = facet.min ?? 0
  const max = facet.max ?? min
  if (!f || typeof f.value !== 'number') return [min, max]
  if (f.op === 'gte') return [f.value, max]
  if (f.op === 'lte') return [min, f.value]
  if (f.op === 'eq') return [f.value, f.value]
  if (f.op === 'range') return [f.value, f.value2 ?? max]
  return [min, max]
}

/** Шаг ползунка по ширине диапазона: целые для ГБ и ватт, десятые для дюймов. */
export function rangeStep(facet: Facet): number {
  const span = (facet.max ?? 0) - (facet.min ?? 0)
  const fractional = [facet.min, facet.max].some((v) => v !== null && !Number.isInteger(v))
  if (fractional || (span > 0 && span <= 5)) return 0.1
  return 1
}

export function formatFacetNumber(v: number, unit = ''): string {
  const s = Number.isInteger(v) ? String(v) : formatDecimal(v, 1)
  return unit ? `${s} ${unit}` : s
}

/** Фасет показывается, если есть из чего выбирать. */
export function usableFacet(f: Facet): boolean {
  if (f.type === 'number') return f.min !== null && f.max !== null && f.max > f.min
  return (f.values ?? []).length > 0
}

/** Подпись значения перечислимого фасета. */
export function facetValueLabel(v: string | number | boolean): string {
  if (v === true) return 'Да'
  if (v === false) return 'Нет'
  return String(v)
}

const CHIP_OPS: Record<string, string> = { gte: '≥', lte: '≤', not: 'не', eq: '' }

/**
 * Подпись чипа: сервер присылает готовую («Память ≥ 32 ГБ»); чип парт-номера всегда начинается с
 * «Парт-номер»; без подписи — собирается из поля, оператора, значения и единицы.
 */
export function chipLabel(chip: {
  field: string
  value: string
  label: string
  op?: string
  unit?: string
}): string {
  const label = chip.label?.trim()
  if (chip.field === 'mpn')
    return label && /парт/i.test(label) ? label : `Парт-номер: ${chip.value}`
  if (label) return label
  const op = chip.op ? (CHIP_OPS[chip.op] ?? chip.op) : ''
  return [chip.field, op, chip.value, chip.unit ?? ''].filter(Boolean).join(' ')
}
