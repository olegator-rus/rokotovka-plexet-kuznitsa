// Киллер-фичи контура коммерции без DOM: очередь по оценке приоритета, доказательства
// предложений по покупкам и комплекта, предложение по марже и его песочница, модель эффекта, блок «Допродажи».
// Фронт ничего не считает сам: только подписи, порядок и разбор ввода. Проверка — tests/killer.test.ts.
import type { Proposal } from '@/api/types'
import type {
  BundleCard,
  CommerceTile,
  CopurchaseEvidence,
  EffectCommerceInput,
  EffectCommerceScenario,
  EffectCommerceTerm,
  LadderRung,
  MarginPointBrief,
  MarginProposalEval,
  MarginProposalPatch,
} from '@/api/typesKiller'
import { formatDecimal, formatInt, formatPercent, formatRub, plural } from './format.ts'

type WithPriority = Proposal & { priority_score?: number | null }

const num = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : null)

// ── Очередь: по оценке приоритета, затем по потерянному спросу ──

/** Сначала предложения с priority_score по убыванию, затем прежний порядок. */
export function sortQueue<T extends WithPriority>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const pa = num(a.priority_score) ? a.priority_score : null
    const pb = num(b.priority_score) ? b.priority_score : null
    if (pa !== null && pb !== null && pa !== pb) return pb - pa
    if (pa !== null && pb === null) return -1
    if (pa === null && pb !== null) return 1
    return (
      (b.cluster?.lost_demand ?? 0) - (a.cluster?.lost_demand ?? 0) ||
      String(b.created_at).localeCompare(String(a.created_at))
    )
  })
}

export const PRIORITY_CAPTION = 'оценка приоритета, не прогноз'

/** «69 942 ₽» — число оценки приоритета; нет — null. */
export const priorityValue = (p: Pick<WithPriority, 'priority_score'>) =>
  num(p.priority_score) ? formatRub(p.priority_score) : null

export const isBundle = (p: Pick<Proposal, 'action'>) => p.action === 'bundle'
export const isMarginProposal = (p: Pick<Proposal, 'action'>) => p.action === 'margin_policy'

/** Предложение по статистике покупок: связь из co_purchase или комплект. */
export function isCopurchase(p: Pick<Proposal, 'action' | 'evidence' | 'diagnosis'>): boolean {
  if (p.action === 'bundle') return true
  const ev = (p.evidence ?? {}) as CopurchaseEvidence
  return p.diagnosis === 'MISSING_RELATION' || ev.source === 'co_purchase'
}

// ── Доказательства ──

export type RungState = 'ok' | 'failed' | 'skipped'

export interface RungView {
  no: number
  name: string
  state: RungState
  stateLabel: string
  text: string
}

export function rungView(r: LadderRung): RungView {
  const state: RungState = r.skipped ? 'skipped' : r.ok ? 'ok' : 'failed'
  const stateLabel =
    state === 'ok' ? 'пройдена' : state === 'skipped' ? 'не применялась' : 'не пройдена'
  return { no: r.no, name: r.name, state, stateLabel, text: dedupeParen(r.text) }
}

/** «возражений нет (возражений нет)» → «возражений нет»: предложения, сохранённые до правки сервера, несут дубль. */
export function dedupeParen(text: string): string {
  const m = /^(.+?)\s*\((.+)\)$/.exec(text ?? '')
  if (!m) return text
  const norm = (x: string) => x.trim().replace(/\.+$/, '').toLowerCase()
  return norm(m[1]) === norm(m[2]) ? m[1].trim() : text
}

export interface EvidenceView {
  table: { label: string; value: string }[]
  /** «сильное» | «слабое — нужна ручная проверка». */
  strength: string | null
  strong: boolean
  ladder: RungView[]
  judge: { text: string; objection: boolean | null } | null
  priority: { value: string; caption: string; formula: string | null } | null
  /** Отклонённые гипотезы с причиной (у комплекта — пересмотр без дополнения). */
  rejected: { no: number; text: string; reason: string }[]
  notes: string[]
}

export function judgeText(j: CopurchaseEvidence['judge']): EvidenceView['judge'] {
  if (!j) return null
  const model = str(j.model)
  if (!j.available)
    return { text: 'судья не ответил — ступень пропущена, нужна ручная проверка', objection: null }
  const reason = str(j.reason)
  const head = j.objection ? 'возражение' : 'возражений нет'
  const tail = reason && reason !== head ? `: ${reason}` : ''
  return { text: `${head}${tail}${model ? ` (${model})` : ''}`, objection: j.objection }
}

export function evidenceView(p: Pick<WithPriority, 'evidence' | 'priority_score'>): EvidenceView {
  const ev = (p.evidence ?? {}) as CopurchaseEvidence
  const strength = str(ev.strength)
  const prio = ev.priority
  const score = num(p.priority_score) ? p.priority_score : num(prio?.score) ? prio.score : null
  const rejected = (ev.hypotheses ?? [])
    .filter((h) => !h.passed && str(h.rejected_reason))
    .map((h) => ({
      no: h.no,
      text: str(h.hypothesis) ?? `Гипотеза ${h.no}`,
      reason: h.rejected_reason as string,
    }))
  return {
    table: (ev.table ?? []).filter((r) => str(r.label) && str(r.value)),
    strength,
    strong: strength === 'сильное',
    ladder: [...(ev.ladder ?? [])].sort((a, b) => a.no - b.no).map(rungView),
    judge: judgeText(ev.judge),
    priority:
      score === null
        ? null
        : {
            value: formatRub(score),
            caption: str(prio?.label) ?? PRIORITY_CAPTION,
            formula: str(prio?.formula),
          },
    rejected,
    notes: [str(ev.technical_note), str(ev.economic_note)].filter((x): x is string => !!x),
  }
}

// ── Комплект (action: bundle) ──

export const COMPAT_LABELS: Record<string, string> = {
  verified: 'совместимо, подтверждено',
  conditional: 'совместимо при условии',
  unknown: 'совместимость не подтверждена',
  incompatible: 'несовместимо',
}

export const CLASS_LABELS: Record<string, string> = {
  required: 'обязательное',
  conditional_required: 'условно обязательное',
  complement: 'дополнение',
  recommended: 'рекомендуемое',
  service: 'услуга',
  upgrade: 'старшая редакция',
}

export interface BundleRow {
  id: string
  title: string
  cls: string
  quantity: number
  price: string
  margin: string | null
  compat: string
  condition: string | null
  relation: string
  stats: string | null
}

export interface BundleViewModel {
  anchor: { title: string; price: string; margin: string | null }
  rows: BundleRow[]
  total: string
  marginTotal: string | null
}

const priceText = (rub: number | null | undefined, mode?: string) =>
  num(rub) ? `${mode === 'from' ? 'от ' : ''}${formatRub(rub)}` : 'цена по запросу'

export function bundleView(b: BundleCard | null | undefined): BundleViewModel | null {
  if (!b?.anchor || !Array.isArray(b.items) || !b.items.length) return null
  const rows = b.items.map((i) => {
    const stats: string[] = []
    if (num(i.attach_rate)) stats.push(`берут с якорем: ${formatPercent(i.attach_rate, 1)}`)
    if (num(i.q_value)) stats.push(`q = ${formatDecimal(i.q_value, 4)}`)
    if (typeof i.repeated === 'boolean')
      stats.push(i.repeated ? 'повторилось в позднем периоде' : 'не повторилось в позднем периоде')
    return {
      id: i.product_id,
      title: i.title,
      cls: CLASS_LABELS[i.class ?? ''] ?? i.class ?? 'дополнение',
      quantity: num(i.quantity) && i.quantity > 0 ? i.quantity : 1,
      price: priceText(i.price_rub, i.price_mode),
      margin: num(i.margin) ? formatPercent(i.margin, 1) : null,
      compat: COMPAT_LABELS[i.compatibility ?? ''] ?? i.compatibility ?? 'совместимость не указана',
      condition: str(i.condition),
      relation:
        i.relation_status === 'approved'
          ? `связь ${i.relation_id ?? ''} в справочнике`.replace('  ', ' ')
          : 'связь предложена агентом',
      stats: stats.length ? stats.join(' · ') : null,
    }
  })
  const t = b.totals ?? {}
  const n = rows.length + 1
  const priced = t.priced_all !== false && num(t.price_rub)
  return {
    anchor: {
      title: b.anchor.title,
      price: priceText(b.anchor.price_rub, b.anchor.price_mode),
      margin: num(b.anchor.margin) ? formatPercent(b.anchor.margin, 1) : null,
    },
    rows,
    total: priced
      ? formatRub(t.price_rub as number)
      : `${formatInt(n)} ${plural(n, ['позиция', 'позиции', 'позиций'])} · сумма по запросу`,
    marginTotal:
      num(t.margin_pct) && num(t.margin_rub)
        ? `${formatPercent(t.margin_pct, 1)} · ${formatRub(t.margin_rub)}`
        : null,
  }
}

// ── Предложение по марже ──

export interface MarginProposalView {
  category: string | null
  categoryId: string | null
  from: number | null
  to: number
  /** Пара чисел экрана «Маржа»: соответствие в первой десятке и индекс показанной маржи, до → после. */
  ndcg: { before: string; after: string } | null
  index: { before: string; after: string; delta: string | null } | null
  checked: string | null
  shifted: string | null
  synthetic: boolean
  /** Адрес песочницы: /margin?w=…&cat=…&p=… */
  link: { path: string; query: Record<string, string> }
}

export const indexLabel4 = (v: number | null | undefined) => (num(v) ? formatDecimal(v, 4) : null)

export function marginProposalView(
  p: Pick<Proposal, 'id' | 'patch' | 'evaluation' | 'evidence'>,
): MarginProposalView {
  const patch = (p.patch ?? {}) as unknown as MarginProposalPatch
  const ev = (p.evaluation ?? {}) as unknown as MarginProposalEval
  const evidence = (p.evidence ?? {}) as { synthetic?: boolean }
  const b: MarginPointBrief | undefined = ev.before
  const a: MarginPointBrief | undefined = ev.after
  const to = num(patch.weight) ? patch.weight : (a?.weight ?? 0)
  const catId = str(patch.category_id) ?? str(ev.scope?.category_id)
  const query: Record<string, string> = { w: String(to), p: p.id }
  if (catId) query.cat = catId
  const scope = ev.scope
  return {
    category: str(patch.category) ?? str(scope?.category),
    categoryId: catId,
    from: num(patch.weight_before) ? patch.weight_before : (b?.weight ?? null),
    to,
    ndcg:
      num(b?.ndcg10) && num(a?.ndcg10)
        ? { before: formatDecimal(b.ndcg10, 4), after: formatDecimal(a.ndcg10, 4) }
        : null,
    index:
      num(b?.margin_index) && num(a?.margin_index)
        ? {
            before: formatDecimal(b.margin_index, 4),
            after: formatDecimal(a.margin_index, 4),
            delta: num(ev.margin_index_delta) ? signed4(ev.margin_index_delta) : null,
          }
        : null,
    checked: num(ev.judged)
      ? `проверено на ${formatInt(ev.judged)} размеченных ${plural(ev.judged, ['запросе', 'запросах', 'запросах'])}${ev.set ? ` набора ${ev.set}` : ''}${scope ? `, позиции категории — в ${formatInt(scope.queries_in_scope)}` : ''}`
      : null,
    shifted: num(a?.queries_with_shifts)
      ? `переставлено в ${formatInt(a.queries_with_shifts)} ${plural(a.queries_with_shifts, ['запросе', 'запросах', 'запросах'])}`
      : null,
    synthetic: !!(ev.synthetic ?? evidence.synthetic),
    link: { path: '/margin', query },
  }
}

function signed4(v: number): string {
  const r = Number(v.toFixed(4))
  if (r === 0) return '0'
  return `${r > 0 ? '+' : '−'}${formatDecimal(Math.abs(r), 4)}`
}

/**
 * Песочница из карточки: совпадают ли числа симуляции с числами предложения. Сравнение — до 4 знаков,
 * как они показаны. null — сравнивать нечего.
 */
export function proposalMatch(
  ev: MarginProposalEval | null | undefined,
  simBefore: MarginPointBrief | null | undefined,
  simAfter: MarginPointBrief | null | undefined,
): { same: boolean; proposal: string; now: string } | null {
  const pb = ev?.before?.margin_index
  const pa = ev?.after?.margin_index
  const sb = simBefore?.margin_index
  const sa = simAfter?.margin_index
  if (!num(pb) || !num(pa) || !num(sb) || !num(sa)) return null
  const r = (x: number) => formatDecimal(x, 4)
  const nb = ev?.before?.ndcg10
  const na = ev?.after?.ndcg10
  const same =
    r(pb) === r(sb) &&
    r(pa) === r(sa) &&
    (!num(nb) || !num(simBefore?.ndcg10) || r(nb) === r(simBefore.ndcg10)) &&
    (!num(na) || !num(simAfter?.ndcg10) || r(na) === r(simAfter.ndcg10))
  return { same, proposal: `${r(pb)} → ${r(pa)}`, now: `${r(sb)} → ${r(sa)}` }
}

// ── Модель эффекта ──

export const EFFECT_SOURCE_LABELS: Record<string, string> = {
  measured: 'замер стенда',
  synthetic: 'синтетические данные',
  assumed: 'допущение',
  literature: 'литература',
  request: 'изменено вами',
  customer: 'данные заказчика',
}
export const effectSourceLabel = (s: string) => EFFECT_SOURCE_LABELS[s] ?? s

/** Входы, которые принимает GET /api/effect/commerce (EffectCommerceKeys + средний чек вилки). */
export const EFFECT_EDITABLE = new Set([
  'rescued_orders_month',
  'avg_check_rub',
  'base_margin',
  'block_coverage',
  'attach_rate',
  'incrementality',
  'companion_sum_rub',
  'companion_margin',
  'anchor_loss',
  'return_rate',
  'mix_gp_per_order',
])

export const isShareInput = (i: Pick<EffectCommerceInput, 'unit'>) => i.unit === 'доля'

/** Значение входа словами: доля — проценты, рубли — с разрядами, заказы — с одним знаком. */
export function inputText(i: Pick<EffectCommerceInput, 'unit' | 'value'>): string {
  if (!num(i.value)) return 'не посчитано'
  if (isShareInput(i)) return formatPercent(i.value, 1)
  if (i.unit === '₽') return formatRub(i.value)
  return `${formatDecimal(i.value, Number.isInteger(i.value) ? 0 : 1)} ${i.unit}`.trim()
}

/** Значение для поля ввода. */
export function inputToField(i: Pick<EffectCommerceInput, 'unit'>, v: number | null): string {
  if (!num(v)) return ''
  if (isShareInput(i)) return String(Math.round(v * 10000) / 100).replace('.', ',')
  return String(Math.round(v * 10) / 10).replace('.', ',')
}

/**
 * Разбор поля: доля — проценты 0..100 → 0..1; рубли и заказы — ≥ 0 (сумма допроданного > 0). Неверно — null.
 */
export function fieldToInput(
  i: Pick<EffectCommerceInput, 'unit' | 'key'>,
  raw: string,
): number | null {
  const s = raw.replace(/[\s %₽]/g, '').replace(',', '.')
  if (!/^\d+(\.\d+)?$/.test(s)) return null
  const n = Number(s)
  if (!Number.isFinite(n) || n < 0) return null
  if (isShareInput(i)) return n > 100 ? null : Math.round(n * 100) / 10000
  if ((i.key === 'companion_sum_rub' || i.key === 'avg_check_rub') && n <= 0) return null
  return n > 1e9 ? null : n
}

export const SCENARIO_ORDER = ['conservative', 'base', 'optimistic']

export function orderEffectScenarios(list: EffectCommerceScenario[]): EffectCommerceScenario[] {
  const rank = (id: string) => {
    const i = SCENARIO_ORDER.indexOf(id)
    return i < 0 ? SCENARIO_ORDER.length : i
  }
  return [...list].sort((a, b) => rank(a.id) - rank(b.id))
}

/** ΔGP члена: «+939 861 ₽», «−87 376 ₽»; не посчитан — «не посчитано». */
export function termText(t: Pick<EffectCommerceTerm, 'gp_rub_month'>): string {
  if (!num(t.gp_rub_month)) return 'не посчитано'
  const v = t.gp_rub_month
  if (Math.round(v) === 0) return formatRub(0)
  return `${v > 0 ? '+' : '−'}${formatRub(Math.abs(v))}`
}

export const signedRub = (v: number) => termText({ gp_rub_month: v })

/** Строки члена за членом: ключ → по сценариям. */
export function termRows(scenarios: EffectCommerceScenario[]) {
  const keys: { key: string; label: string; kind: string; formula: string }[] = []
  for (const s of scenarios)
    for (const t of s.terms)
      if (!keys.some((k) => k.key === t.key))
        keys.push({ key: t.key, label: t.label, kind: t.kind, formula: t.formula })
  return keys.map((k) => ({
    ...k,
    cells: scenarios.map((s) => {
      const t = s.terms.find((x) => x.key === k.key)
      return t ? termText(t) : '—'
    }),
  }))
}

/** Тождество ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG словами. */
export function identityText(s: Pick<EffectCommerceScenario, 'check'>): string | null {
  const c = s.check
  if (!c) return null
  return `${signedRub(c.do_g0)} + ${signedRub(c.o0_dg)} + ${signedRub(c.do_dg)} = ${signedRub(c.sum)}; напрямую ${signedRub(c.direct)}; сумма членов ${signedRub(c.terms)}`
}

// ── Блок «Допродажи» ──

export const COMMERCE_SOURCE_LABELS: Record<string, string> = {
  stand_events: 'события стенда',
  offline_eval: 'офлайн-прогон',
  catalog: 'справочники каталога',
  agent: 'очередь агента',
  synthetic: 'синтетические данные',
}
export const commerceSourceLabel = (s: string) => COMMERCE_SOURCE_LABELS[s] ?? s

/** Значение плитки; null — не посчитано (причина — в note). */
export function tileValue(t: Pick<CommerceTile, 'value' | 'unit'>): string | null {
  if (!num(t.value)) return null
  if (t.unit === 'share') return formatPercent(t.value, 1)
  if (t.unit === 'rub') return formatRub(t.value)
  return formatInt(t.value)
}
