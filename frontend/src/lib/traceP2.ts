// Шаги трассы контура коммерции на витрине: hardware и companions. Без DOM:
// tests/traceP2.test.ts. Шаг margin — lib/traceMargin.ts (трек бэк-офиса).
import type { TraceStep } from '@/api/types'
import { plural } from './format.ts'

export interface HardwareUnit {
  text?: string
  key?: string
  op?: string
  value?: unknown
  unit?: string
}

export interface CompanionConsidered {
  product_id: string
  class?: string
  source?: string
}

export interface CompanionRejected {
  product_id: string
  reason?: string
}

/** Поля шагов, которые рисует TraceStepP2.vue (в «прочие поля» не попадают). */
export const P2_STEP_FIELDS: Record<string, readonly string[]> = {
  hardware: ['mpn_detected', 'units', 'compat_target'],
  companions: ['anchor', 'considered', 'rejected'],
}

export const isP2Field = (step: TraceStep, key: string) =>
  (P2_STEP_FIELDS[step.name] ?? []).includes(key)

export const OP_SIGNS: Record<string, string> = {
  eq: '=',
  gte: '≥',
  lte: '≤',
  range: '∈',
  not: '≠',
}

/** Причины, по которым кандидат не попал в блок; неизвестная — как есть. */
export const COMPANION_REJECT_LABELS: Record<string, string> = {
  incompatible: 'несовместимо',
  version_mismatch: 'несовместимо по версии',
  edition_mismatch: 'не та редакция',
  unavailable: 'нет в наличии',
  anchor_not_confident: 'якорь не уверенный',
  low_confidence: 'якорь не уверенный',
  class_taken: 'тип уже занят',
  duplicate_class: 'тип уже занят',
  limit: 'сверх 4 позиций',
  over_limit: 'сверх 4 позиций',
  in_results: 'уже есть в выдаче',
  not_approved: 'связь не утверждена',
  pending_review: 'связь не утверждена',
  unknown_compat: 'совместимость не подтверждена',
  condition_unmet: 'условие не выполнено',
}

export const companionRejectLabel = (r: string | undefined) =>
  r ? (COMPANION_REJECT_LABELS[r] ?? r) : 'причина не указана'

export function asList<T>(v: unknown): T[] {
  return Array.isArray(v) ? (v.filter((x) => x && typeof x === 'object') as T[]) : []
}

/** Распознанный парт-номер: сервер может прислать строку или true. */
export function mpnOf(step: TraceStep): string | null {
  const v = step.mpn_detected
  if (typeof v === 'string' && v) return v
  if (v === true) return 'да'
  return null
}

/** Модель «X для Y»: строка или объект с id/title. */
export function compatTargetOf(step: TraceStep): string | null {
  const v = step.compat_target
  if (typeof v === 'string' && v) return v
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>
    const t = o.title ?? o.model ?? o.target_model ?? o.id
    if (typeof t === 'string' && t) return t
  }
  return null
}

/** «ram_gb ≥ 32 ГБ» из распознанной единицы. */
export function unitLabel(u: HardwareUnit): string {
  const sign = OP_SIGNS[u.op ?? ''] ?? u.op ?? ''
  const value = Array.isArray(u.value) ? u.value.join('–') : String(u.value ?? '')
  return [u.key ?? u.text ?? '', sign, value, u.unit ?? ''].filter(Boolean).join(' ')
}

export function hardwareSummary(step: TraceStep): string {
  const parts: string[] = []
  const mpn = mpnOf(step)
  if (mpn) parts.push(mpn === 'да' ? 'парт-номер' : `парт-номер ${mpn}`)
  const n = asList<HardwareUnit>(step.units).length
  if (n) parts.push(`${n} ${plural(n, ['единица', 'единицы', 'единиц'])}`)
  const target = compatTargetOf(step)
  if (target) parts.push(`совместимость с ${target}`)
  return parts.length ? parts.join(' · ') : 'номеров и единиц нет'
}

/** Якорь блока: объект { product_id, confidence } или строка id; null — блока нет. */
export function companionAnchorOf(step: TraceStep): { id: string; confidence: string } | null {
  const a = step.anchor
  if (typeof a === 'string' && a) return { id: a, confidence: '' }
  if (a && typeof a === 'object') {
    const o = a as Record<string, unknown>
    const id = typeof o.product_id === 'string' ? o.product_id : ''
    if (id) return { id, confidence: typeof o.confidence === 'string' ? o.confidence : '' }
  }
  return null
}

export function companionsSummary(step: TraceStep): string {
  const anchor = companionAnchorOf(step)
  const considered = asList<CompanionConsidered>(step.considered).length
  const rejected = asList<CompanionRejected>(step.rejected).length
  if (!anchor && !considered) return 'якоря нет — блок не показан'
  const parts = [anchor ? 'якорь найден' : 'без якоря']
  parts.push(`${considered} ${plural(considered, ['кандидат', 'кандидата', 'кандидатов'])}`)
  if (rejected) parts.push(`отклонено ${rejected}`)
  return parts.join(' · ')
}

/** Есть ли у шага контура коммерции что раскрывать. */
export function p2HasDetails(step: TraceStep): boolean {
  if (step.name === 'hardware')
    return Boolean(mpnOf(step) || compatTargetOf(step) || asList(step.units).length)
  if (step.name === 'companions')
    return Boolean(
      companionAnchorOf(step) || asList(step.considered).length || asList(step.rejected).length,
    )
  return false
}
