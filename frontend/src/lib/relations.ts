// Справочник связей и правка связей агента: подписи полей словами, фильтры,
// пометка «проверить», разница версий «было / стало». Без DOM: tests/relations.test.ts.
import type { Relation, RelationPatch, RelationUpdate, RelationVersion } from '@/api/typesCommerce'
import { formatInt, plural } from './format.ts'

export const RELATION_TYPE_LABELS: Record<string, string> = {
  substitute: 'замена',
  complement: 'дополнение',
  renewal: 'продление',
  compatibility: 'совместимость',
  upgrade: 'старшая редакция',
}

export const REQUIREMENT_LABELS: Record<string, string> = {
  conditional_required: 'условно обязательное',
  recommended: 'рекомендуемое',
  optional: 'необязательное',
}

export const COMPATIBILITY_LABELS: Record<string, string> = {
  verified: 'подтверждена',
  conditional: 'с условием',
  incompatible: 'несовместимо',
  unknown: 'не подтверждена',
}

export const SOURCE_LABELS: Record<string, string> = {
  vendor_documentation: 'документ вендора',
  catalog_rule: 'правило каталога',
  expert: 'эксперт',
  co_purchase: 'совместные покупки',
  agent_proposal: 'предложение агента',
}

export const REVIEW_LABELS: Record<string, string> = {
  approved: 'утверждена',
  pending: 'ждёт проверки',
  rejected: 'отклонена',
}

/** Уровни доказательности E0–E5 словом. */
export const EVIDENCE_LABELS: Record<string, string> = {
  E0: 'E0 — нет доказательств',
  E1: 'E1 — описание товара',
  E2: 'E2 — документ вендора',
  E3: 'E3 — правило каталога, проверено человеком',
  E4: 'E4 — совместные покупки, значимо',
  E5: 'E5 — подтверждено замером',
}

/** Сила доказательства словом: E2 и выше — сильное. */
export function evidenceStrength(level: string | null | undefined): string | null {
  if (!level) return null
  const n = Number(/^E(\d)$/.exec(level)?.[1])
  if (!Number.isFinite(n)) return null
  return n >= 2 ? 'сильное' : 'слабое — нужна ручная проверка'
}

const lbl = (map: Record<string, string>, v: string | null | undefined, empty = '—') =>
  v ? (map[v] ?? v) : empty

export const relationTypeLabel = (v?: string | null) => lbl(RELATION_TYPE_LABELS, v)
export const requirementLabel = (v?: string | null) => lbl(REQUIREMENT_LABELS, v, 'необязательное')
export const compatibilityLabel = (v?: string | null) =>
  lbl(COMPATIBILITY_LABELS, v, 'не подтверждена')
export const sourceLabel = (v?: string | null) => lbl(SOURCE_LABELS, v)
export const reviewLabel = (v?: string | null) => lbl(REVIEW_LABELS, v)
export const evidenceLabel = (v?: string | null) => lbl(EVIDENCE_LABELS, v)

/** Название конца связи: позиция → семейство → id. */
export function relationEnds(r: Relation): { from: string; to: string } {
  return {
    from: r.from_name?.trim() || r.from_title?.trim() || r.from_product || r.from_family,
    to: r.to_name?.trim() || r.to_title?.trim() || r.to_product || r.to_family,
  }
}

/** Связь без проверки дольше срока помечается «проверить»; срок сервера — stale_after_days. */
export const REVIEW_MAX_DAYS = 365

export function needsReview(
  r: Pick<Relation, 'reviewed_at' | 'needs_review'>,
  now: number = Date.now(),
  days: number = REVIEW_MAX_DAYS,
): boolean {
  if (typeof r.needs_review === 'boolean') return r.needs_review
  if (!r.reviewed_at) return true
  const t = Date.parse(r.reviewed_at)
  if (!Number.isFinite(t)) return true
  return now - t > days * 86_400_000
}

export interface RelationFilter {
  q: string
  type: string
  source: string
  status: string
}

export const EMPTY_FILTER: RelationFilter = { q: '', type: '', source: '', status: '' }

/** Фильтр справочника: товар (подстрока в концах связи), тип, источник, статус проверки. */
export function filterRelations(items: Relation[], f: RelationFilter): Relation[] {
  const q = f.q.trim().toLowerCase()
  return items.filter((r) => {
    if (f.type && r.type !== f.type) return false
    if (f.source && r.source !== f.source) return false
    if (f.status && (r.review_status ?? 'approved') !== f.status) return false
    if (!q) return true
    const e = relationEnds(r)
    return [e.from, e.to, r.from_family, r.to_family, r.from_product, r.to_product, r.id]
      .filter(Boolean)
      .some((s) => String(s).toLowerCase().includes(q))
  })
}

/** Значения для выпадающих фильтров — из данных справочника. */
export function filterOptions(items: Relation[], key: 'type' | 'source' | 'review_status') {
  return [...new Set(items.map((r) => r[key] ?? (key === 'review_status' ? 'approved' : '')))]
    .filter(Boolean)
    .sort() as string[]
}

// ── Разница: «было / стало» ──

export interface RelationDiffRow {
  op: 'add' | 'remove' | 'update'
  id: string
  /** «Windows Server 2025 → CAL 2025». */
  title: string
  before: string | null
  after: string | null
}

/** Связь одной строкой: тип, обязательность, совместимость, основание. */
export function relationLine(r: Relation): string {
  const parts = [relationTypeLabel(r.type), requirementLabel(r.requirement)]
  parts.push(`совместимость ${compatibilityLabel(r.compatibility)}`)
  if (r.evidence_level) parts.push(r.evidence_level)
  return parts.join(' · ')
}

const endsLine = (r: Relation) => {
  const e = relationEnds(r)
  return `${e.from} → ${e.to}`
}

/** Изменённая связь: целиком (с необязательным before) или парой {before, after}. */
export function splitUpdate(u: RelationUpdate): { before: Relation | null; after: Relation } {
  if ('after' in u && u.after && typeof u.after === 'object')
    return { before: u.before ?? null, after: u.after }
  const r = u as Relation & { before?: Relation | null }
  return { before: r.before ?? null, after: r }
}

/** «A → B  ⇒  C»: общий начальный конец не повторяется. */
function changeTitle(old: Relation, now: Relation): string {
  const a = relationEnds(old)
  const b = relationEnds(now)
  if (a.from === b.from) return `${a.from} → ${a.to}  ⇒  ${b.to}`
  return `${endsLine(old)}  ⇒  ${endsLine(now)}`
}

/**
 * Правка связей построчно. before — связи справочника до правки (по id), чтобы показать «было»
 * у изменённых и удалённых.
 */
export function relationDiff(
  patch: RelationPatch | null | undefined,
  before: Relation[] = [],
): RelationDiffRow[] {
  if (!patch) return []
  const byId = new Map(before.map((r) => [r.id, r]))
  const rows: RelationDiffRow[] = []
  for (const u of patch.update ?? []) {
    const { before: b, after: r } = splitUpdate(u)
    const old = b ?? byId.get(r.id)
    rows.push({
      op: 'update',
      id: r.id,
      title: old ? changeTitle(old, r) : endsLine(r),
      before: old ? relationLine(old) : null,
      after: relationLine(r),
    })
  }
  for (const r of patch.add ?? [])
    rows.push({ op: 'add', id: r.id, title: endsLine(r), before: null, after: relationLine(r) })
  for (const x of patch.remove ?? []) {
    const id = typeof x === 'string' ? x : x.id
    const old = typeof x === 'string' ? byId.get(id) : x
    rows.push({
      op: 'remove',
      id,
      title: old ? endsLine(old) : `связь ${id}`,
      before: old ? relationLine(old) : null,
      after: null,
    })
  }
  return rows
}

/** Сводка правки: «+1 · изменено 1 · −0». */
export function patchSummary(patch: RelationPatch | null | undefined): string {
  const a = patch?.add?.length ?? 0
  const u = patch?.update?.length ?? 0
  const r = patch?.remove?.length ?? 0
  const out: string[] = []
  if (a) out.push(`добавлено ${formatInt(a)}`)
  if (u) out.push(`изменено ${formatInt(u)}`)
  if (r) out.push(`удалено ${formatInt(r)}`)
  return out.length ? out.join(' · ') : 'без изменений'
}

/** Версии, в которых менялась связь id, новые сверху. */
export function relationHistory(versions: RelationVersion[], id: string): RelationVersion[] {
  return [...versions]
    .filter(
      (v) =>
        (v.diff?.add ?? []).some((r) => r.id === id) ||
        (v.diff?.update ?? []).some((u) => splitUpdate(u).after.id === id) ||
        (v.diff?.remove ?? []).some((x) => (typeof x === 'string' ? x : x.id) === id),
    )
    .sort((a, b) => b.version - a.version)
}

export const relationsCount = (n: number) =>
  `${formatInt(n)} ${plural(n, ['связь', 'связи', 'связей'])}`
