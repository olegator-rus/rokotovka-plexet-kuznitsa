// Предложения агента и версии правил: разница правил словами, результат
// прогона двумя строками, статусы, сортировка, ссылка «Проверить в поиске». Без DOM: tests/proposals.test.ts.
import type {
  AppliedEffect,
  ConfigVersion,
  FoundOf,
  Proposal,
  ProposalEvaluation,
  ProposalRejectReason,
  RulePatch,
  RulePatchItem,
} from '@/api/types'
import { formatInt, plural } from './format.ts'

export const DIAGNOSIS_LABELS: Record<string, string> = {
  ALIAS_GAP: 'Неизвестный термин',
  TYPO: 'Опечатка или раскладка',
  ASSORTMENT_GAP: 'Нет в ассортименте',
  SUBSTITUTE_AVAILABLE: 'Нет товара, есть аналог',
  CONTENT_GAP: 'В карточке не хватает слов',
  RANKING_FAILURE: 'Товар найден, но стоит низко',
  CONSTRAINT_CONFLICT: 'Такой лицензии не существует',
  NON_PRODUCT: 'Нетоварный запрос',
  NOISE: 'Шум',
  UNKNOWN: 'Требует ручной проверки',
}

export const diagnosisLabel = (d: string) => DIAGNOSIS_LABELS[d] ?? d

/** Статус предложения: цвет только здесь (токены --status-*), рядом всегда подпись. */
export const PROPOSAL_STATUSES: readonly { id: string; label: string; tone: string }[] = [
  { id: 'pending', label: 'Ждёт решения', tone: 'status-pending' },
  { id: 'accepted', label: 'Принято', tone: 'status-accepted' },
  { id: 'rejected', label: 'Отклонено', tone: 'status-rejected' },
  { id: 'rolled_back', label: 'Откачено', tone: 'status-rolled-back' },
  // Ждало решения, но проверка больше не годится (новый каталог, исправленная оценка) — принять нельзя.
  { id: 'stale', label: 'Устарело', tone: 'status-rolled-back' },
]

/** Решения в памяти агента могут прийти глаголом — сводим к статусу. */
const STATUS_ALIASES: Record<string, string> = {
  approve: 'accepted',
  approved: 'accepted',
  accept: 'accepted',
  reject: 'rejected',
  rollback: 'rolled_back',
}

export function proposalStatus(id: string): { id: string; label: string; tone: string } {
  const key = STATUS_ALIASES[id] ?? id
  return (
    PROPOSAL_STATUSES.find((s) => s.id === key) ?? { id, label: id, tone: 'status-rolled-back' }
  )
}

export const PROPOSAL_ROLES: readonly { id: string; label: string }[] = [
  { id: 'ecom', label: 'Менеджер e-com' },
  { id: 'product', label: 'Продукт-менеджер' },
  { id: 'content', label: 'Контент-менеджер' },
]

export const proposalRoleLabel = (id: string) =>
  PROPOSAL_ROLES.find((r) => r.id === id)?.label ?? id

export const REJECT_REASONS: readonly { id: ProposalRejectReason; label: string }[] = [
  { id: 'too_broad', label: 'Слишком широко' },
  { id: 'wrong_product', label: 'Неверный товар' },
  { id: 'not_now', label: 'Не сейчас' },
  { id: 'other', label: 'Другое' },
]

/** decided_reason, которые ставит система, а не человек. */
const SYSTEM_REASONS: Record<string, string> = {
  catalog_changed: 'Сменился каталог',
}

export function rejectReasonLabel(id: string | null | undefined): string {
  if (!id) return ''
  return REJECT_REASONS.find((r) => r.id === id)?.label ?? SYSTEM_REASONS[id] ?? id
}

const STALE_REASONS: Record<string, string> = {
  catalog_changed: 'новый каталог: правка проверялась на прежнем словаре',
}

/**
 * Почему предложение устарело: stale_reason сервера (код или текст), без него — decided_reason
 * (catalog_changed). null — не устарело.
 */
export function staleReason(
  p: Pick<Proposal, 'status' | 'decided_reason'> & { stale_reason?: string | null },
): string | null {
  if (p.status !== 'stale') return null
  const code = (p.stale_reason ?? '').trim() || (p.decided_reason ?? '').trim()
  if (!code) return null
  return STALE_REASONS[code] ?? code
}

/** Потерянный спрос кластера — число различных сессий с неудачным поиском. */
export function lostDemandLabel(n: number | null | undefined): string {
  const v = n ?? 0
  return `потерянный спрос: ${formatInt(v)} ${plural(v, ['сессия', 'сессии', 'сессий'])}`
}

/** Предложение не меняет правила поиска (задача по карточке или ассортименту) — «Взять в работу». */
export const isTicket = (p: Pick<Proposal, 'action'>) => p.action === 'ticket'

/** Очередь: по убыванию потерянного спроса, при равенстве — новые выше. */
export function sortByLostDemand(items: Proposal[]): Proposal[] {
  return [...items].sort(
    (a, b) =>
      (b.cluster?.lost_demand ?? 0) - (a.cluster?.lost_demand ?? 0) ||
      String(b.created_at).localeCompare(String(a.created_at)),
  )
}

export const pendingCount = (items: Proposal[]) =>
  items.filter((p) => p.status === 'pending').length

/** Запросы кластера по убыванию частоты. */
export function clusterQueries(p: Pick<Proposal, 'cluster'>): { text: string; freq: number }[] {
  return [...(p.cluster?.queries ?? [])].sort((a, b) => b.freq - a.freq)
}

/** Суммарная частота запросов кластера. */
export const clusterFreq = (p: Pick<Proposal, 'cluster'>) =>
  (p.cluster?.queries ?? []).reduce((n, q) => n + (q.freq || 0), 0)

/** «Проверить в поиске»: запрос от агента (check_query) или самый частый запрос кластера, на B4. */
export function searchCheckUrl(p: Pick<Proposal, 'cluster' | 'check_query'>): string | null {
  const q = p.check_query?.trim() || clusterQueries(p)[0]?.text
  if (!q) return null
  return `/search?${new URLSearchParams({ q, config: 'B4' }).toString()}`
}

// ── Разница правил словами, не JSON ──

const KIND_LABELS: Record<string, string> = {
  alias: 'алиас',
  typo: 'исправление',
  context: 'правило контекста',
  context_rule: 'правило контекста',
  demand_gap: 'спрос без предложения',
  card_field: 'правка карточки',
  redirect: 'редирект',
  synonym: 'синоним',
}

const TARGET_LABELS: Record<string, string> = {
  term: 'термин',
  product: 'товар',
  family: 'линейка',
  vendor: 'вендор',
  category: 'категория',
}

export interface DiffLine {
  op: 'add' | 'remove'
  /** Главное: «кис» → «kaspersky internet security». */
  text: string
  /** Вид правила и цель: «алиас, односторонний · линейка kis». */
  note: string
}

function str(v: unknown): string | null {
  return typeof v === 'string' && v.trim() ? v : typeof v === 'number' ? String(v) : null
}

export function patchItemText(item: RulePatchItem): { text: string; note: string } {
  const kind = KIND_LABELS[item.kind] ?? item.kind
  const alias = str(item.alias) ?? str(item.from) ?? str(item.term) ?? str(item.query)
  const canonical = str(item.canonical) ?? str(item.to) ?? str(item.value)
  const target = item.target ? (TARGET_LABELS[item.target] ?? item.target) : null
  const targetId = str(item.target_id)
  const noteParts = [item.kind === 'alias' ? `${kind}, односторонний` : kind]
  if (target) noteParts.push(targetId ? `${target} ${targetId}` : target)
  if (item.before !== undefined || item.after !== undefined) {
    const field = str(item.field)
    if (field) noteParts.push(`поле ${field}`)
    return {
      text: `было «${str(item.before) ?? '—'}» → предлагается «${str(item.after) ?? '—'}»`,
      note: noteParts.join(' · '),
    }
  }
  if (alias && canonical)
    return { text: `«${alias}» → «${canonical}»`, note: noteParts.join(' · ') }
  if (alias) return { text: `«${alias}»`, note: noteParts.join(' · ') }
  // Незнакомая форма: пары «ключ: значение» без служебных полей, но не сырой JSON.
  const rest = Object.entries(item)
    .filter(([k, v]) => k !== 'kind' && v !== null && v !== undefined && v !== '')
    .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
  return { text: rest.join(', ') || kind, note: noteParts.join(' · ') }
}

export function diffLines(patch: RulePatch | null | undefined): DiffLine[] {
  if (!patch) return []
  return [
    ...(patch.add ?? []).map((i) => ({ op: 'add' as const, ...patchItemText(i) })),
    ...(patch.remove ?? []).map((i) => ({ op: 'remove' as const, ...patchItemText(i) })),
  ]
}

// ── Результат прогона ──

export interface EvaluationLines {
  target: string
  regression: string
  /** Дополнительно: точные запросы и нарушения условий — только если не нули. */
  extra: string[]
  passed: boolean
  /** Ухудшений нет — строка регрессии нейтральна, иначе помечается. */
  regressionWorse: number
}

const qWord = (n: number) => plural(n, ['запрос', 'запроса', 'запросов'])

export function evaluationLines(ev: ProposalEvaluation | null | undefined): EvaluationLines | null {
  if (!ev?.target) return null
  const b = ev.target.before
  const a = ev.target.after
  const target = `Целевой кластер: находилось ${formatInt(b.found)} из ${formatInt(b.total)}, стало ${formatInt(a.found)} из ${formatInt(a.total)}`
  const r = ev.regression ?? { worse: 0, better: 0, total: 0 }
  let regression = `Регрессионный набор: ухудшилось ${formatInt(r.worse)} ${qWord(r.worse)}`
  if (r.total) regression += ` из ${formatInt(r.total)}`
  if (r.better) regression += `, улучшилось ${formatInt(r.better)}`
  const extra: string[] = []
  if (ev.exact_worse)
    extra.push(`Точные запросы: ухудшилось ${formatInt(ev.exact_worse)} ${qWord(ev.exact_worse)}`)
  if (ev.constraint_violations_new)
    extra.push(`Новых нарушений условий: ${formatInt(ev.constraint_violations_new)}`)
  return { target, regression, extra, passed: !!ev.passed, regressionWorse: r.worse }
}

// ── Перепроверка при «Принять» (правила сменились после проверки агента) ──

export interface RecheckView {
  passed: boolean
  /** «Правила сменились после проверки агента (v2 → v3): на действующих правилах правка …». */
  title: string
  reasons: string[]
  /** Прогон на действующих правилах (не прошла) или первоначальный прогон агента (прошла). */
  lines: EvaluationLines | null
  linesTitle: string
  checkedAt: string | null
}

export function recheckView(p: Pick<Proposal, 'evidence'>): RecheckView | null {
  const r = p.evidence?.recheck
  if (!r || typeof r !== 'object') return null
  const versions = `v${r.from_version} → v${r.to_version}`
  if (r.passed) {
    return {
      passed: true,
      title: `Правила сменились после проверки агента (${versions}): правка перепроверена на действующих и прошла — результат выше пересчитан`,
      reasons: [],
      lines: evaluationLines(p.evidence?.initial_evaluation),
      linesTitle: `Первоначальный прогон агента (правила v${r.from_version})`,
      checkedAt: r.checked_at ?? null,
    }
  }
  return {
    passed: false,
    title: `Правила сменились после проверки агента (${versions}): на действующих правилах правка не прошла прогон, предложение осталось в очереди`,
    reasons: (r.reasons ?? []).filter((x): x is string => typeof x === 'string' && !!x),
    lines: evaluationLines(r.evaluation),
    linesTitle: `Прогон на действующих правилах v${r.to_version}`,
    checkedAt: r.checked_at ?? null,
  }
}

// ── Версии правил ──

export const AUTHOR_LABELS: Record<string, string> = {
  'agent+human': 'Агент + человек',
  human: 'Человек',
  system: 'Система',
}

export const authorLabel = (a: string) => AUTHOR_LABELS[a] ?? a

/** Новые сверху; действующая — по полю active. */
export function sortVersions(items: ConfigVersion[]): ConfigVersion[] {
  return [...items].sort((a, b) => b.version - a.version)
}

// ── Доказательства из evidence: гипотезы, запросы прогона, тикеты, эффект ──

const foundOf = (f: FoundOf | undefined) =>
  f ? `${formatInt(f.found)} из ${formatInt(f.total)}` : '—'

export interface HypothesisLine {
  no: number
  text: string
  passed: boolean
  /** «3 из 6 → 5 из 6» — целевой кластер в прогоне гипотезы. */
  target: string | null
  reason: string | null
}

/** Все гипотезы, которые агент проверил прогоном, по номеру; отклонённые — с причиной. */
export function hypothesisLines(p: Pick<Proposal, 'evidence'>): HypothesisLine[] {
  const hs = p.evidence?.hypotheses ?? []
  return [...hs]
    .sort((a, b) => a.no - b.no)
    .map((h) => {
      const t = h.evaluation?.target
      return {
        no: h.no,
        text:
          h.hypothesis ||
          diffLines(h.patch)
            .map((d) => d.text)
            .join(', '),
        passed: !!h.passed,
        target: t ? `${foundOf(t.before)} → ${foundOf(t.after)}` : null,
        reason: h.passed ? null : (h.reason ?? null),
      }
    })
}

export interface TargetQueryRow {
  text: string
  freq: number
  before: boolean
  after: boolean
  /** Нужный товар перестал находиться — ухудшение. */
  worse: boolean
}

/** Запросы целевого кластера в прогоне принятой гипотезы: находился ли нужный товар до и после. */
export function targetQueryRows(ev: ProposalEvaluation | null | undefined): TargetQueryRow[] {
  return (ev?.target?.queries ?? []).map((q) => ({
    text: q.text,
    freq: q.freq,
    before: !!q.before_found,
    after: !!q.after_found,
    worse: !!q.before_found && !q.after_found,
  }))
}

/** Что сделать по задаче (action: ticket) — словами из доказательств агента. */
export function ticketLines(p: Pick<Proposal, 'diagnosis' | 'evidence'>): string[] {
  const ev = p.evidence
  const out: string[] = []
  const subs = ev?.substitutes ?? []
  if (subs.length)
    out.push(
      `Показать покупателям замену: ${subs.map((x) => (x.vendor ? `${x.name} (${x.vendor})` : x.name)).join(', ')}`,
    )
  else if (p.diagnosis === 'ASSORTMENT_GAP')
    out.push('Решить, брать ли товар в ассортимент: замены в каталоге агент не нашёл')
  const cur = ev?.current_search ?? []
  if (cur.length) {
    const empty = cur.filter((c) => c.empty).length
    out.push(
      empty === cur.length
        ? `Сейчас поиск отвечает «нет в каталоге» на все ${formatInt(cur.length)} ${qWord(cur.length)} кластера`
        : `Сейчас поиск отвечает «нет в каталоге» на ${formatInt(empty)} из ${formatInt(cur.length)} ${plural(cur.length, ['запроса', 'запросов', 'запросов'])}`,
    )
  }
  const ref = [...(ev?.reformulations ?? [])].sort((a, b) => b.count - a.count)[0]
  if (ref)
    out.push(
      `Покупатели переформулируют: «${ref.from}» → «${ref.to}» (${formatInt(ref.count)} ${plural(ref.count, ['раз', 'раза', 'раз'])}${ref.clicked ? '' : ', без кликов'})`,
    )
  return out
}

/** Эффект принятой правки одной строкой: «до правки 0 из 4, прогон обещал 4, сейчас 4 из 4». */
export function effectLine(
  e: AppliedEffect | null | undefined,
): { text: string; holds: boolean } | null {
  if (!e || (!e.now && !e.summary)) return null
  const parts: string[] = []
  if (e.before) parts.push(`до правки находилось ${foundOf(e.before)}`)
  if (e.promised) parts.push(`прогон обещал ${formatInt(e.promised.found)}`)
  if (e.now) parts.push(`сейчас ${foundOf(e.now)}`)
  const text = parts.length ? parts.join(', ') : (e.summary ?? '')
  return { text: text.charAt(0).toUpperCase() + text.slice(1), holds: e.holds !== false }
}

/** Описание версии без кода диагноза в начале: «ALIAS_GAP: …» → «Неизвестный термин: …». */
export function versionDescription(
  v: Pick<ConfigVersion, 'description' | 'source' | 'aliases'>,
): string {
  const d = (v.description ?? '').trim()
  if (v.source === 'import' || d.startsWith('исходные словари'))
    return `Исходные словари и правила из данных каталога${v.aliases ? ` · ${formatInt(v.aliases)} ${plural(v.aliases, ['алиас', 'алиаса', 'алиасов'])}` : ''}`
  const m = /^([A-Z_]+):\s*(.*)$/.exec(d)
  if (m && DIAGNOSIS_LABELS[m[1]]) return `${DIAGNOSIS_LABELS[m[1]]}: ${m[2]}`
  return d
}

// ── Проблемный запрос → предложение агента (пока /api/analytics не присылает proposal_id) ──

const normQuery = (q: string) => q.trim().toLowerCase().replace(/\s+/g, ' ')

const STATUS_RANK: Record<string, number> = {
  pending: 0,
  accepted: 1,
  rolled_back: 2,
  rejected: 3,
  stale: 4,
}

/**
 * Запрос → предложение, в кластер которого он входит. Если запрос есть в нескольких предложениях,
 * берём ждущее решения, затем принятое, затем более новое.
 */
export function proposalsByQuery(items: Proposal[]): Map<string, Proposal> {
  const out = new Map<string, Proposal>()
  const sorted = [...items].sort(
    (a, b) =>
      (STATUS_RANK[a.status] ?? 9) - (STATUS_RANK[b.status] ?? 9) ||
      String(b.created_at).localeCompare(String(a.created_at)),
  )
  for (const p of sorted)
    for (const q of p.cluster?.queries ?? []) {
      const k = normQuery(q.text)
      if (!out.has(k)) out.set(k, p)
    }
  return out
}

export const findProposalForQuery = (index: Map<string, Proposal>, query: string) =>
  index.get(normQuery(query)) ?? null
