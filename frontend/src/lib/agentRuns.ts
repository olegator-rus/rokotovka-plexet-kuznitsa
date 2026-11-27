// Контур автономии (экран «Запуски», «Очередь»): подписи триггеров, время запуска «03:00 /
// вчера 18:05», план и итог одной строкой, бюджет «израсходовано из лимита», фильтры, плашка очереди «N предложений
// от ночного запуска 03:00 · M от всплеска 14:20». Без DOM: tests/agentRuns.test.ts.
import type { AgentRun, Proposal } from '@/api/types'
import type { BudgetAmounts, RunBudget, RunOutcome } from '@/api/typesAgent'
import { dayAgo, formatDecimal, formatInt, formatMoney, plural, russianQuotes } from './format.ts'

export const TRIGGERS = [
  { id: 'schedule', label: 'расписание' },
  { id: 'spike', label: 'всплеск' },
  { id: 'catalog_change', label: 'каталог' },
  { id: 'manual', label: 'вручную' },
] as const

export type TriggerId = (typeof TRIGGERS)[number]['id']

const TRIGGER_LABELS: Record<string, string> = Object.fromEntries(
  TRIGGERS.map((t) => [t.id, t.label]),
)

/** Короткая подпись триггера: «расписание», «всплеск», «каталог», «вручную». */
export function triggerLabel(trigger: string | null | undefined): string {
  return TRIGGER_LABELS[trigger ?? ''] ?? (trigger || 'вручную')
}

const SPIKE_FIRED: Record<string, string> = {
  empty_share: 'всплеск пустых выдач',
  reform_share: 'всплеск переформулировок',
  new_query: 'новый неудачный запрос',
}

function obj(v: unknown): Record<string, unknown> | null {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : null
}

/**
 * Подробная подпись триггера для строки и ленты: «всплеск пустых выдач», «обновление каталога», «разбор запроса».
 * Берётся из input запуска; без него — короткая.
 */
export function triggerDetail(run: Pick<AgentRun, 'trigger' | 'input' | 'query'>): string {
  const t = run.trigger
  if (t === 'spike') {
    const fired = obj(obj(run.input)?.spike)?.fired
    const first = Array.isArray(fired)
      ? fired.map((f) => SPIKE_FIRED[String(f)]).find(Boolean)
      : null
    return first ?? 'всплеск неудачных поисков'
  }
  if (t === 'schedule') return 'по расписанию'
  if (t === 'catalog_change') return 'обновление каталога'
  if (run.query) return 'разбор запроса'
  return 'вручную'
}

const p2 = (n: number) => String(n).padStart(2, '0')
const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() &&
  a.getMonth() === b.getMonth() &&
  a.getDate() === b.getDate()

/** Время запуска как в макете: сегодня — «03:00», вчера — «вчера 18:05», раньше — «26.05, 18:05». */
export function runWhen(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const hm = `${p2(d.getHours())}:${p2(d.getMinutes())}`
  if (sameDay(d, now)) return hm
  const y = new Date(now)
  y.setDate(now.getDate() - 1)
  if (sameDay(d, y)) return `вчера ${hm}`
  return `${dayAgo(d, now)}, ${hm}`
}

/** Запуск до контура автономии: без плана и бюджета, итог — только счётчики старой формы. */
export function isLegacyRun(run: AgentRun): boolean {
  return !run.plan && !run.budget
}

/** План одной строкой: сводка последней редакции; запуск на одном запросе — «Разбор запроса «…»». */
export function planLine(run: AgentRun): string {
  const s = run.plan?.summary ? russianQuotes(run.plan.summary.trim()) : ''

  if (s) return run.plan!.revision > 1 ? `${s} (редакция ${run.plan!.revision})` : s
  if (run.query) return `Разбор запроса «${run.query}»`
  if (run.mode === 'gaps') return 'Пробелы подбора'
  if (run.mode === 'commerce') return 'Связи и правила лицензий'
  return ''
}

const EMPTY_OUTCOME: RunOutcome = {
  proposals: 0,
  tasks: 0,
  rollbacks: 0,
  drafts: 0,
  skipped_memory: 0,
  effect_checks: 0,
  helped: 0,
  no_effect: 0,
  harmed: 0,
  checked: 0,
  stale: 0,
}

/** Счётчики итога; у запуска до контура автономии — предложения из proposals_created. */
export function outcomeOf(run: AgentRun): RunOutcome {
  const o = { ...EMPTY_OUTCOME, ...(run.outcome ?? {}) }
  if (isLegacyRun(run) && !o.proposals && !o.tasks) o.proposals = run.proposals_created ?? 0
  return o
}

const n = (v: number, forms: [string, string, string]) => `${formatInt(v)} ${plural(v, forms)}`

/**
 * Итог одной строкой: «4 предложения, 2 задачи, 1 пропущено по памяти» + проверки эффекта и откаты, если были.
 * Три первых счётчика — всегда (так в макете), остальные — только ненулевые.
 */
export function outcomeLine(run: AgentRun): string {
  const o = outcomeOf(run)
  // До контура автономии задачи и пропуски по памяти не считались — показываем только то, что измерено.
  if (isLegacyRun(run)) return n(o.proposals, ['предложение', 'предложения', 'предложений'])
  const parts = [
    n(o.proposals, ['предложение', 'предложения', 'предложений']),
    n(o.tasks, ['задача', 'задачи', 'задач']),
    `${formatInt(o.skipped_memory)} пропущено по памяти`,
  ]
  if (o.effect_checks) {
    const verdicts = [
      o.helped ? `помогло ${formatInt(o.helped)}` : '',
      o.no_effect ? `без эффекта ${formatInt(o.no_effect)}` : '',
      o.harmed ? `навредило ${formatInt(o.harmed)}` : '',
    ].filter(Boolean)
    parts.push(
      `${n(o.effect_checks, ['проверка эффекта', 'проверки эффекта', 'проверок эффекта'])}${verdicts.length ? ` (${verdicts.join(', ')})` : ''}`,
    )
  }
  if (o.rollbacks) parts.push(n(o.rollbacks, ['откат', 'отката', 'откатов']))
  if (o.drafts) parts.push(n(o.drafts, ['черновик', 'черновика', 'черновиков']))
  if (o.checked) parts.push(`проверено ${formatInt(o.checked)}`)
  if (o.stale) parts.push(`устаревших ${formatInt(o.stale)}`)
  return parts.join(', ')
}

/** Фильтр «Итог»: что запуск дал. */
export const OUTCOME_FILTERS = [
  { value: 'proposals', label: 'Есть предложения' },
  { value: 'tasks', label: 'Есть задачи' },
  { value: 'memory', label: 'Пропуски по памяти' },
  { value: 'effect', label: 'Проверки эффекта' },
  { value: 'rollbacks', label: 'Предложен откат' },
  { value: 'nothing', label: 'Без находок' },
] as const

export function matchesOutcome(run: AgentRun, filter: string): boolean {
  const o = outcomeOf(run)
  switch (filter) {
    case 'proposals':
      return o.proposals + o.rollbacks > 0
    case 'tasks':
      return o.tasks > 0
    case 'memory':
      return o.skipped_memory > 0
    case 'effect':
      return o.effect_checks > 0
    case 'rollbacks':
      return o.rollbacks > 0
    case 'nothing':
      return o.proposals + o.tasks + o.rollbacks + o.drafts === 0
    default:
      return true
  }
}

/** Длительность: duration_ms сервера, иначе по меткам времени; у идущего — до now. */
export function runDurationMs(run: AgentRun, now = Date.now()): number | null {
  if (typeof run.duration_ms === 'number' && run.finished_at) return run.duration_ms
  const from = Date.parse(run.started_at)
  const to = run.finished_at ? Date.parse(run.finished_at) : now
  if (!Number.isFinite(from) || !Number.isFinite(to)) return null
  return Math.max(0, to - from)
}

export function formatRunDuration(ms: number | null): string {
  if (ms == null) return ''
  const sec = Math.round(ms / 1000)
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m > 0 ? `${m} мин ${s} с` : `${s} с`
}

export interface BudgetRow {
  /** cluster_calls — лимит вызовов инструментов на кластер (упор в него — stop_reason step_limit). */
  key: keyof BudgetAmounts | 'cluster_calls'
  label: string
  used: string
  limit: string
  /** 0..1, для полосы. */
  share: number
  exhausted: boolean
}

const BUDGET_LABELS: Record<keyof BudgetAmounts, string> = {
  steps: 'Шаги',
  tokens: 'Токены',
  cost_rub: 'Стоимость',
  seconds: 'Время',
}

const EXHAUSTED_KEY: Record<string, keyof BudgetAmounts> = {
  steps_budget: 'steps',
  token_limit: 'tokens',
  cost_budget: 'cost_rub',
  time_budget: 'seconds',
}

function fmtAmount(key: keyof BudgetAmounts, v: number): string {
  if (key === 'cost_rub') return formatMoney(v)
  if (key === 'seconds') return formatRunDuration(v * 1000)
  return formatInt(v)
}

/** Бюджет «израсходовано из лимита» по четырём ресурсам; без лимитов — пусто. */
export function budgetRows(
  budget: RunBudget | null | undefined,
  stopReason?: string | null,
  run?: Pick<AgentRun, 'limits' | 'episode'> | null,
): BudgetRow[] {
  if (!budget?.limits) return []
  const rows = runBudgetRows(budget, stopReason)
  const cl = clusterLimitRow(stopReason, run)
  return cl ? [cl, ...rows] : rows
}

/**
 * step_limit — упор в лимит вызовов инструментов на кластер (15), а не в бюджет шагов запуска (80): строка
 * «Вызовы на кластер «…» 15 из 15» показывает лимит, в который упёрлись.
 */
function clusterLimitRow(
  stopReason: string | null | undefined,
  run: Pick<AgentRun, 'limits' | 'episode'> | null | undefined,
): BudgetRow | null {
  if (stopReason !== 'step_limit') return null
  const limit = Number(run?.limits?.tool_calls_per_cluster) || 0
  const hit = (run?.episode?.clusters ?? []).filter((c) => c.outcome === 'limit')
  if (!limit || !hit.length) return null
  const used = Math.max(...hit.map((c) => Number(c.tool_calls) || 0))
  return {
    key: 'cluster_calls',
    label:
      hit.length === 1
        ? `Вызовы инструментов на кластер «${hit[0]!.label}»`
        : `Вызовы инструментов на кластер (упёрлись ${formatInt(hit.length)} ${plural(hit.length, ['кластер', 'кластера', 'кластеров'])})`,
    used: formatInt(used),
    limit: formatInt(limit),
    share: Math.min(1, Math.max(0, used / limit)),
    exhausted: true,
  }
}

function runBudgetRows(budget: RunBudget, stopReason?: string | null): BudgetRow[] {
  const used = budget.used ?? { steps: 0, tokens: 0, cost_rub: 0, seconds: 0 }
  const hit =
    EXHAUSTED_KEY[typeof budget.exhausted === 'string' ? budget.exhausted : ''] ??
    EXHAUSTED_KEY[stopReason ?? '']
  return (Object.keys(BUDGET_LABELS) as (keyof BudgetAmounts)[])
    .filter((k) => Number(budget.limits[k]) > 0)
    .map((k) => {
      const u = Number(used[k]) || 0
      const l = Number(budget.limits[k])
      return {
        key: k,
        label: BUDGET_LABELS[k],
        used: fmtAmount(k, u),
        limit: fmtAmount(k, l),
        share: Math.min(1, Math.max(0, u / l)),
        exhausted: hit === k,
      }
    })
}

// ── Плашка очереди и происхождение карточки ──

/** Откуда предложение словами: «ночного запуска 03:00», «всплеска 14:20», «проверки каталога вчера 18:05». */
export function originPhrase(
  trigger: string | null | undefined,
  startedAt: string | null | undefined,
  now: Date = new Date(),
): string {
  const when = runWhen(startedAt, now)
  const d = startedAt ? new Date(startedAt) : null
  const night = !!d && !Number.isNaN(d.getTime()) && d.getHours() < 7
  const what =
    trigger === 'schedule'
      ? night
        ? 'ночного запуска'
        : 'запуска по расписанию'
      : trigger === 'spike'
        ? 'всплеска'
        : trigger === 'catalog_change'
          ? 'проверки каталога'
          : 'разбора вручную'
  return when ? `${what} ${when}` : what
}

export interface QueueOrigin {
  runId: string
  trigger: string
  startedAt: string | null
  count: number
  phrase: string
}

/** Ждущие предложения по запускам, в порядке времени запуска (старые первыми, как «03:00 · 14:20»). */
export function queueOrigins(items: Proposal[], now: Date = new Date()): QueueOrigin[] {
  const by = new Map<string, QueueOrigin>()
  for (const p of items) {
    if (p.status !== 'pending' || !p.run_id) continue
    const cur = by.get(p.run_id)
    if (cur) {
      cur.count++
      continue
    }
    by.set(p.run_id, {
      runId: p.run_id,
      trigger: p.trigger ?? '',
      startedAt: p.run_started_at ?? null,
      count: 1,
      phrase: originPhrase(p.trigger, p.run_started_at, now),
    })
  }
  return [...by.values()].sort((a, b) =>
    String(a.startedAt ?? '').localeCompare(String(b.startedAt ?? '')),
  )
}

/** Часть плашки: первая — «4 предложения от ночного запуска 03:00», следующие — «1 от всплеска 14:20». */
export function originText(o: QueueOrigin, first: boolean): string {
  return first
    ? `${n(o.count, ['предложение', 'предложения', 'предложений'])} от ${o.phrase}`
    : `${formatInt(o.count)} от ${o.phrase}`
}

/** Плашка: «4 предложения от ночного запуска 03:00 · 1 от всплеска 14:20». Без источников — пусто. */
export function queueBannerLine(origins: QueueOrigin[]): string {
  return origins.map((o, i) => originText(o, i === 0)).join(' · ')
}

/** Пороги триггеров одной строкой, если сервер не прислал свою (line). */
export function shareLabel(v: number): string {
  return `${formatDecimal(v * 100, 0)} %`
}
