// Карточка предложения `relation_patch`: было/стало связи, основание,
// уровень доказательности словом, два раздельных статуса. Без DOM: tests/relationProposal.test.ts.
import type { Proposal } from '@/api/types'
import type {
  CommerceProposal,
  Relation,
  RelationPatch,
  RelationPatchEval,
} from '@/api/typesCommerce'
import { companionsEvalLines, economicStatus, technicalStatus } from './commerce.ts'
import { formatInt, formatRub, plural } from './format.ts'
import {
  evidenceLabel,
  evidenceStrength,
  relationDiff,
  splitUpdate,
  type RelationDiffRow,
} from './relations.ts'

export const isRelationPatch = (p: Pick<Proposal, 'action'>) => p.action === 'relation_patch'

/**
 * О чём предложение — для диалогов и тостов (P10): у правки связей нет кластера запросов, её подпись — «Связь …».
 */
export function proposalSubject(p: Pick<Proposal, 'action' | 'cluster'>): string {
  const label = p.cluster?.label ?? ''
  return isRelationPatch(p) ? `Предложение «${label}»` : `Кластер «${label}»`
}

/** То же в косвенном падеже: «по предложению «…»» / «по кластеру «…»». */
export function proposalSubjectDat(p: Pick<Proposal, 'action' | 'cluster'>): string {
  const label = p.cluster?.label ?? ''
  return isRelationPatch(p) ? `по предложению «${label}»` : `по кластеру «${label}»`
}

/** Диагнозы контура коммерции — для словаря DIAGNOSIS_LABELS. */
export const P2_DIAGNOSIS_LABELS: Record<string, string> = {
  BROKEN_RELATION: 'Сломанная связь',
  MISSING_REQUIRED: 'Нет обязательного дополнения',
}

/** Инструменты агента контура коммерции — для подписей ленты. */
export const P2_TOOL_LABELS: Record<string, string> = {
  get_relations: 'Справочник связей',
  check_relation_integrity: 'Проверка целостности связей',
  check_compatibility: 'Проверка совместимости',
  check_license_rules: 'Правила лицензий вендора',
  create_relation_patch: 'Правка связей в песочнице',
  evaluate_companions: 'Прогон блока допродажи',
  judge: 'Судья другого семейства',
}

/** Строка основания: текст, ссылка на документ, уровень. */
export interface RelationBasis {
  text: string
  source_url?: string
  evidence_level?: string
}

export interface RelationProposalView {
  diff: RelationDiffRow[]
  evidenceLevel: string | null
  strength: string | null
  basis: RelationBasis[]
  technical: { label: string; ok: boolean | null } | null
  economic: { label: string; ok: boolean | null }
  /** Пояснения сервера к статусам (technical_note, economic_note). */
  notes: string[]
  /** «оценка приоритета 0,42 — не прогноз». */
  priority: string | null
  /** Прогон правки: находки, блок на размеченных парах, регрессия поиска. */
  evalLines: string[]
  passed: boolean | null
  reasons: string[]
}

function asRelations(v: unknown): Relation[] {
  const list = Array.isArray(v) ? v : v && typeof v === 'object' ? [v] : []
  return list.filter((r): r is Relation => !!r && typeof r === 'object' && 'id' in r)
}

const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : null)

/** Основание: evidence.basis, иначе текст находки, правила вендора и источник связи. */
function basisOf(ev: Record<string, unknown>): RelationBasis[] {
  if (Array.isArray(ev.basis))
    return ev.basis
      .map((b) =>
        typeof b === 'string'
          ? { text: b }
          : b && typeof b === 'object' && str((b as RelationBasis).text)
            ? (b as RelationBasis)
            : null,
      )
      .filter((b): b is RelationBasis => !!b)
  const out: RelationBasis[] = []
  const finding = str(ev.finding_text)
  if (finding) out.push({ text: finding })
  if (Array.isArray(ev.license_rules))
    for (const r of ev.license_rules as Record<string, unknown>[]) {
      const text = str(r?.reason)
      if (text)
        out.push({
          text: `Правило вендора${str(r.id) ? ` ${r.id}` : ''}: ${text}`,
          source_url: str(r.source_url) ?? undefined,
        })
    }
  const url = str(ev.source_url)
  if (url && !out.some((b) => b.source_url === url))
    out.push({ text: 'Документ-основание', source_url: url })
  return out
}

const qWord = (n: number) => plural(n, ['запрос', 'запроса', 'запросов'])

/** Строки прогона evaluate_companions. */
export function relationEvalLines(ev: RelationPatchEval | null | undefined): string[] {
  if (!ev) return []
  const out: string[] = []
  if (typeof ev.findings_before === 'number' && typeof ev.findings_after === 'number')
    out.push(
      `Ошибки справочника: было ${formatInt(ev.findings_before)}, стало ${formatInt(ev.findings_after)}${ev.resolved ? ' — находка снята' : ''}`,
    )
  const nf = ev.new_findings?.length ?? 0
  if (nf) out.push(`Новых ошибок после правки: ${formatInt(nf)}`)
  const b = ev.block ?? ev.companions
  if (b) {
    const lines = companionsEvalLines(b.before, b.after)
    const pairs = (ev.block as { pairs?: number } | null)?.pairs
    out.push(
      ...lines.map((l, i) =>
        i === 0 && pairs ? `${l} (на ${formatInt(pairs)} размеченных парах)` : l,
      ),
    )
  }
  const r = ev.regression
  if (r) {
    let line = `Регрессия поиска: ухудшилось ${formatInt(r.worse)} ${qWord(r.worse)} из ${formatInt(r.total)}`
    if (r.better) line += `, улучшилось ${formatInt(r.better)}`
    if (r.block_changed) line += `; блок допродажи изменился у ${formatInt(r.block_changed)}`
    out.push(line)
  }
  return out
}

export function relationProposalView(p: CommerceProposal): RelationProposalView {
  const patch = p.patch as unknown as RelationPatch | null
  const ev = (p.evidence ?? {}) as Record<string, unknown>
  const evaluation = p.evaluation as unknown as RelationPatchEval | null
  const level = p.evidence_level || str(ev.evidence_level)
  return {
    diff: relationDiff(patch, asRelations(ev.relation_before)),
    evidenceLevel: level ? evidenceLabel(level) : null,
    strength: str(ev.strength) ?? evidenceStrength(level),
    basis: basisOf(ev),
    technical: technicalStatus(p.technical_status),
    economic: economicStatus(p.economic_status),
    notes: [str(ev.technical_note), str(ev.economic_note)].filter((x): x is string => !!x),
    priority: priorityLine(p.priority_score, ev.priority),
    evalLines: relationEvalLines(evaluation),
    passed: typeof evaluation?.passed === 'boolean' ? evaluation.passed : null,
    reasons: (evaluation?.reasons ?? []).filter((x): x is string => !!str(x)),
  }
}

/** Как получена оценка приоритета (само число — в шапке карточки): «оценка приоритета, не прогноз: …». */
function priorityLine(score: number | null | undefined, info: unknown): string | null {
  if (typeof score !== 'number' || !Number.isFinite(score)) return null
  const formula = str((info as { formula?: unknown } | null)?.formula)
  return `${formatRub(score)} — оценка приоритета, не прогноз${formula ? `: ${formula}` : ''}`
}

/** Номера связей, которые трогает правка: добавляемые, изменяемые, удаляемые. */
export function relationIds(p: Pick<Proposal, 'patch'>): string[] {
  const patch = p.patch as unknown as RelationPatch | null
  return [
    ...(patch?.add ?? []).map((r) => r.id),
    ...(patch?.update ?? []).map((u) => splitUpdate(u).after.id),
    ...(patch?.remove ?? []).map((x) => (typeof x === 'string' ? x : x.id)),
  ].filter((id): id is string => typeof id === 'string' && !!id)
}
