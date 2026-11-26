// Лента запуска агента SearchOps: редьюсер событий SSE и снимков GET, подписи шагов,
// разбиение по кластерам, колонка состояния. Без DOM: проверяют tests/agentFeed.test.ts.
import type { AgentRun, AgentRunDetail, AgentStep } from '@/api/types'
import { formatInt, plural } from './format.ts'
import { P2_TOOL_LABELS } from './relationProposal.ts'

export interface FeedState {
  run: AgentRun | null
  steps: AgentStep[]
  /** Пришло событие done или статус запуска уже не running. */
  done: boolean
}

export type FeedEvent =
  | { type: 'snapshot'; detail: AgentRunDetail }
  | { type: 'step'; step: AgentStep }
  | { type: 'status'; run: AgentRun }
  | { type: 'done' }

export const EMPTY_FEED: FeedState = { run: null, steps: [], done: false }

export function isRunActive(status: string | undefined | null): boolean {
  return status === 'running'
}

/** Шаги без дублей (SSE после снимка присылает часть шагов повторно), по возрастанию seq. */
function mergeSteps(current: AgentStep[], incoming: AgentStep[]): AgentStep[] {
  const byKey = new Map<string, AgentStep>()
  for (const s of current) byKey.set(stepKey(s), s)
  for (const s of incoming) byKey.set(stepKey(s), s)
  return [...byKey.values()].sort((a, b) => a.seq - b.seq)
}

function stepKey(s: AgentStep): string {
  return s.id ? `id:${s.id}` : `seq:${s.seq}`
}

export function feedReducer(state: FeedState, ev: FeedEvent): FeedState {
  switch (ev.type) {
    case 'snapshot': {
      const steps = mergeSteps(state.steps, ev.detail.steps ?? [])
      return { run: ev.detail.run, steps, done: !isRunActive(ev.detail.run?.status) }
    }
    case 'step': {
      if (state.run && ev.step.run_id && ev.step.run_id !== state.run.id) return state
      return { ...state, steps: mergeSteps(state.steps, [ev.step]) }
    }
    case 'status': {
      if (state.run && ev.run.id && ev.run.id !== state.run.id) return state
      return { ...state, run: ev.run, done: state.done || !isRunActive(ev.run.status) }
    }
    case 'done':
      return { ...state, done: true }
  }
}

/** Разбор data из SSE: битый JSON — null (событие пропускается, опрос догонит). */
export function parseSse<T>(data: string): T | null {
  try {
    const v = JSON.parse(data) as unknown
    return v && typeof v === 'object' ? (v as T) : null
  } catch {
    return null
  }
}

// ── Виды шагов ──

/** Три вида: вызов инструмента, вывод агента, результат проверки; плюс память и ошибка. */
export type StepGroup =
  | 'tool'
  | 'reasoning'
  | 'check'
  | 'memory'
  | 'proposal'
  | 'error'
  // контур автономии: план, проверка эффекта прошлой правки, бюджет
  | 'plan'
  | 'effect'
  | 'budget'

const KIND_META: Record<string, { label: string; group: StepGroup }> = {
  tool_call: { label: 'Инструмент', group: 'tool' },
  thought: { label: 'Рассуждение', group: 'reasoning' },
  diagnosis: { label: 'Диагноз', group: 'reasoning' },
  hypothesis: { label: 'Гипотеза', group: 'reasoning' },
  check: { label: 'Проверка', group: 'check' },
  memory: { label: 'Память', group: 'memory' },
  proposal: { label: 'Предложение', group: 'proposal' },
  error: { label: 'Ошибка', group: 'error' },
  plan: { label: 'План', group: 'plan' },
  memory_read: { label: 'Читаю', group: 'memory' },
  memory_skip: { label: 'Пропускаю', group: 'memory' },
  effect_check: { label: 'Прошлая правка', group: 'effect' },
  budget: { label: 'Бюджет', group: 'budget' },
}

/** Вердикт шага effect_check: helped | no_effect | harmed (result.verdict), иначе по ok. */
export function effectVerdict(step: AgentStep): 'helped' | 'no_effect' | 'harmed' | null {
  if (step.kind !== 'effect_check') return null
  const v = step.result?.verdict
  if (v === 'helped' || v === 'no_effect' || v === 'harmed') return v
  return step.ok ? null : 'harmed'
}

export function stepKind(kind: string): { label: string; group: StepGroup } {
  return KIND_META[kind] ?? { label: kind, group: 'reasoning' }
}

/** Подписи инструментов агента (ARCHITECTURE §3.3); неизвестный — как есть. */
export const TOOL_LABELS: Record<string, string> = {
  get_failed_queries: 'Неудачные запросы',
  get_query_cluster: 'Кластер запросов',
  search_catalog: 'Поиск по каталогу',
  get_active_rules: 'Действующие правила',
  get_search_trace: 'Трасса запроса',
  find_similar_successful_queries: 'Похожие удачные запросы',
  get_product_details: 'Карточка товара',
  get_product_relations: 'Аналоги и сопутствующие',
  get_previous_decisions: 'Прошлые решения',
  create_sandbox_patch: 'Правка в песочнице',
  run_evaluation: 'Прогон оценки',
  submit_proposal: 'Предложение в очередь',
  create_assortment_ticket: 'Задача по ассортименту',
  verify_applied_change: 'Эффект принятой правки',
  close_cluster: 'Кластер закрыт',
  submit_plan: 'План запуска',
  ...P2_TOOL_LABELS,
}

export interface ClusterOutcomeRow {
  id: string
  label: string
  diagnosis: string
  hypotheses: number
  rejected: number
  /** Итог словами: «правка в очереди», «задача продукт-менеджеру», «правкой правил не решается». */
  outcome: string
  /** Есть предложение в очереди. */
  proposalId: string | null
}

const OUTCOME_LABELS: Record<string, string> = {
  proposal: 'правка правил — в очереди',
  ticket: 'задача человеку — в очереди',
  closed: 'правкой правил не решается',
  rejected: 'гипотезы отклонены — предложения нет',
  limit: 'не доразобран: упёрся в лимит',
  solved: 'поиск уже отвечает верно',
  unfinished: 'не доразобран: модель не завершила разбор',
  repeat: 'закрыт: агент повторял отклонённый вызов',
  error: 'сбой при разборе',
  skipped: 'пропущен',
}

/** Итог по кластерам из episode.clusters в порядке разбора. */
export function clusterOutcomes(run: AgentRun | null): ClusterOutcomeRow[] {
  return (run?.episode?.clusters ?? []).map((c) => ({
    id: c.cluster_id,
    label: c.label,
    diagnosis: c.diagnosis,
    hypotheses: c.hypotheses ?? 0,
    rejected: c.rejected ?? 0,
    outcome: OUTCOME_LABELS[c.outcome] ?? c.outcome,
    proposalId: c.proposal_id || null,
  }))
}

/** Ключ гипотезы: номер гипотезы в своём кластере. */
function hypKey(s: Pick<AgentStep, 'cluster_id' | 'hypothesis_no'>): string | null {
  return s.hypothesis_no == null ? null : `${s.cluster_id ?? ''}#${s.hypothesis_no}`
}

/**
 * Отклонённые гипотезы → причина. Сервер может написать причину в сам шаг гипотезы или в шаг проверки
 * с тем же номером; зачёркиваются все шаги этой гипотезы (вывод, а не инструменты).
 */
export function rejectedHypotheses(steps: AgentStep[]): Map<string, string> {
  const out = new Map<string, string>()
  for (const s of steps) {
    const k = hypKey(s)
    if (k && s.rejected_reason) out.set(k, s.rejected_reason)
  }
  return out
}

export interface StepView {
  step: AgentStep
  kind: { label: string; group: StepGroup }
  toolLabel: string | null
  /** Строка шага без служебных вставок модели. */
  title: string
  /** Отклонённая гипотеза: шаг зачёркнут, причина видна. */
  rejected: string | null
  /** Причину пишем один раз — у шага гипотезы; у проверки — только если шага гипотезы в ленте нет. */
  showReason: boolean
  /** Шаг проверки прошёл / не прошёл; null — не проверка. */
  passed: boolean | null
  expandable: boolean
}

function hasContent(v: Record<string, unknown> | null | undefined): boolean {
  return !!v && Object.keys(v).length > 0
}

export function stepView(
  step: AgentStep,
  rejected: Map<string, string>,
  hypotheses: Set<string> = new Set(),
): StepView {
  const kind = stepKind(step.kind)
  const k = hypKey(step)
  const reason =
    step.rejected_reason ??
    // Пройденная ступень отклонённой гипотезы (лестница контура коммерции: провал на 5-й ступени) — не провал.
    (k && (step.kind === 'hypothesis' || (step.kind === 'check' && !step.ok))
      ? (rejected.get(k) ?? null)
      : null)
  return {
    step,
    kind,
    toolLabel: step.tool ? (TOOL_LABELS[step.tool] ?? step.tool) : null,
    title: stepTitle(step.title ?? ''),
    rejected: reason,
    showReason: !!reason && (step.kind !== 'check' || !k || !hypotheses.has(k)),
    passed: kind.group === 'check' ? step.ok && !reason : null,
    expandable: hasContent(step.args) || hasContent(step.result),
  }
}

export interface ClusterSegment {
  clusterId: string | null
  label: string
  steps: StepView[]
}

/** Подпись кластера: из результата шага (label, cluster.label) или из current_cluster запуска. */
function labelFrom(step: AgentStep): string | null {
  const r = step.result
  if (!r) return null
  if (typeof r.cluster_label === 'string') return r.cluster_label
  const c = r.cluster
  if (c && typeof c === 'object' && 'label' in c && typeof c.label === 'string') return c.label
  if (step.tool === 'get_query_cluster' && typeof r.label === 'string') return r.label
  return null
}

const CLUSTER_TITLE = /^Кластер «(.+?)»/

/** Подпись кластера из первого шага «Кластер «…»: N запросов…». */
function labelFromTitle(step: AgentStep): string | null {
  if (step.kind !== 'thought' || !step.cluster_id) return null
  return CLUSTER_TITLE.exec(step.title ?? '')?.[1] ?? null
}

/**
 * Строка шага для ленты: модель иногда дописывает в рассуждение вызов инструмента блоком ```…``` —
 * его в строке не показываем (полный текст остаётся в раскрытии шага).
 */
export function stepTitle(title: string): string {
  const i = title.indexOf('```')
  const head = (i >= 0 ? title.slice(0, i) : title).trim()
  return head.replace(/[:：]\s*$/, '') || title.trim()
}

/** Шаги подряд одного кластера — один сегмент с заголовком; шаги вне кластера — свой сегмент. */
export function segmentByCluster(steps: AgentStep[], run: AgentRun | null): ClusterSegment[] {
  const rejected = rejectedHypotheses(steps)
  const hypotheses = new Set<string>()
  for (const s of steps) {
    const k = hypKey(s)
    if (k && s.kind === 'hypothesis') hypotheses.add(k)
  }
  const labels = new Map<string, string>()
  for (const s of steps) {
    const l = labelFrom(s) ?? labelFromTitle(s)
    if (s.cluster_id && l && !labels.has(s.cluster_id)) labels.set(s.cluster_id, l)
  }
  if (run?.current_cluster && !labels.has(run.current_cluster.id))
    labels.set(run.current_cluster.id, run.current_cluster.label)

  const out: ClusterSegment[] = []
  for (const s of steps) {
    const id = s.cluster_id ?? null
    const last = out[out.length - 1]
    if (!last || last.clusterId !== id) {
      out.push({
        clusterId: id,
        label: id ? (labels.get(id) ?? id) : 'Подготовка запуска',
        steps: [],
      })
    }
    out[out.length - 1].steps.push(stepView(s, rejected, hypotheses))
  }
  for (const seg of out) if (!seg.clusterId) seg.label = runSegmentLabel(seg.steps, seg === out[0])
  return out
}

/** Заголовок сегмента вне кластеров (контур автономии): план и проверки эффекта в начале, бюджет и итог в конце. */
function runSegmentLabel(views: StepView[], first: boolean): string {
  const kinds = new Set(views.map((v) => v.step.kind))
  if (kinds.size === 1 && kinds.has('budget')) return 'Бюджет запуска'
  const parts: string[] = []
  if (kinds.has('plan')) parts.push('план')
  if (kinds.has('memory_read') || kinds.has('memory_skip')) parts.push('память')
  if (kinds.has('effect_check')) parts.push('проверка эффекта прошлых правок')
  if (!parts.length) return first ? 'Подготовка запуска' : 'Запуск'
  const t = parts.join(', ')
  return t.charAt(0).toUpperCase() + t.slice(1)
}

// ── Колонка состояния ──

export const DEFAULT_TOOL_LIMIT = 15

export const RUN_STATUS_LABELS: Record<string, string> = {
  running: 'Идёт',
  done: 'Завершён',
  stopped: 'Остановлен',
  failed: 'Ошибка',
}

export function runStatusLabel(status: string): string {
  return RUN_STATUS_LABELS[status] ?? status
}

const STOP_REASONS: Record<string, string> = {
  user: 'остановлен человеком',
  stopped_by_user: 'остановлен человеком',
  step_limit: 'достигнут лимит вызовов инструментов на кластер',
  token_limit: 'достигнут лимит токенов',
  timeout: 'истекло время запуска',
  llm_error: 'ошибка модели',
  no_failed_queries: 'неудачных запросов за период нет',
  budget_limit: 'исчерпан дневной бюджет LLM',
  error: 'внутренняя ошибка запуска',
  steps_budget: 'исчерпан бюджет шагов',
  cost_budget: 'исчерпан бюджет стоимости',
  time_budget: 'исчерпан бюджет времени',
  already_solved: 'поиск уже отвечает верно — разбирать нечего',
  no_findings: 'находок нет',
}

export function stopReasonLabel(reason: string | null | undefined): string | null {
  if (!reason) return null
  return STOP_REASONS[reason] ?? reason
}

/** Вызовы инструментов в текущем кластере: «шаг N из лимита». */
export function currentClusterCalls(run: AgentRun | null, steps: AgentStep[]): number {
  const id = run?.current_cluster?.id
  if (!id) return 0
  return steps.filter((s) => s.cluster_id === id && s.kind === 'tool_call').length
}

/** Итог запуска одной строкой: «Разобрано 3 из 5 кластеров · 2 предложения · 41 вызов инструментов». */
export function runOutcomeLine(run: AgentRun): string {
  const parts = [
    `Разобрано ${formatInt(run.clusters_done)} из ${formatInt(run.clusters_total)} ${plural(run.clusters_total, ['кластера', 'кластеров', 'кластеров'])}`,
    `${formatInt(run.proposals_created)} ${plural(run.proposals_created, ['предложение', 'предложения', 'предложений'])}`,
    `${formatInt(run.tool_calls)} ${plural(run.tool_calls, ['вызов', 'вызова', 'вызовов'])} инструментов`,
  ]
  return parts.join(' · ')
}

/** Длительность запуска: «2 мин 14 с», «48 с». */
export function formatDuration(fromIso: string, toIso: string | null, now = Date.now()): string {
  const from = Date.parse(fromIso)
  const to = toIso ? Date.parse(toIso) : now
  if (!Number.isFinite(from) || !Number.isFinite(to)) return ''
  const sec = Math.max(0, Math.round((to - from) / 1000))
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m > 0 ? `${m} мин ${s} с` : `${s} с`
}

/** Время шага: «14:03:27». */
export function formatClock(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
