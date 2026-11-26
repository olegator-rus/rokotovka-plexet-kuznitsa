// Представление ответа GET /api/evaluations для «До / После»: столбцы по лестнице
// B0–B4 (только посчитанные), строки метрик, строка воспроизводимости, ухудшения.
// Без DOM: tests/evalView.test.ts.
import type {
  ConfigId,
  EvalChange,
  EvalMetrics,
  EvaluationView,
  EvaluationsResponse,
  JudgeAgreement,
} from '@/api/types'
import { metricDef, metricDelta, orderMetricKeys, type MetricDelta } from './metrics.ts'

const LADDER: readonly ConfigId[] = ['B0', 'B1', 'B2', 'B3', 'B4']

function ladderRank(c: string): number {
  const i = (LADDER as readonly string[]).indexOf(c)
  return i < 0 ? LADDER.length : i
}

/** Последний результат каждой конфигурации в порядке лестницы. */
export function evalColumns(resp: EvaluationsResponse | null | undefined): EvaluationView[] {
  const latest = new Map<string, EvaluationView>()
  for (const run of resp?.configs ?? []) {
    const prev = latest.get(run.config)
    if (!prev || (run.created_at ?? '') > (prev.created_at ?? '')) latest.set(run.config, run)
  }
  return [...latest.values()].sort((a, b) => ladderRank(a.config) - ladderRank(b.config))
}

const num = (v: unknown): number | null => (typeof v === 'number' && Number.isFinite(v) ? v : null)

/**
 * Метрики сервера → плоский словарь «ключ → значение» для таблицы. Счётчики (queries, errors,
 * violation_items) метриками не считаются; время ответа берётся из latency (P50/P95), если были замеры.
 */
export function flattenMetrics(m: EvalMetrics | null | undefined): Record<string, number | null> {
  if (!m) return {}
  const out: Record<string, number | null> = {}
  const skip = new Set(['queries', 'errors', 'violation_items', 'latency', 'by_route'])
  for (const [k, v] of Object.entries(m)) if (!skip.has(k)) out[k] = num(v)
  if (m.latency && m.latency.count > 0) {
    out.p50_ms = num(m.latency.p50)
    out.p95_ms = num(m.latency.p95)
  }
  return out
}

export interface MetricCell {
  config: ConfigId
  value: number | null
  delta: MetricDelta | null
  /** С какой конфигурацией сравнивается дельта (предыдущий столбец). */
  prevConfig: ConfigId | null
  prevValue: number | null
}

export interface MetricRow {
  key: string
  cells: MetricCell[]
}

/** Строки таблицы: метрики, у которых есть хотя бы одно значение. Непосчитанные не показываются. */
export function metricRows(columns: readonly EvaluationView[]): MetricRow[] {
  const flat = columns.map((c) => flattenMetrics(c.metrics))
  const keys = new Set<string>()
  for (const f of flat) for (const [k, v] of Object.entries(f)) if (v !== null) keys.add(k)
  return orderMetricKeys(keys).map((key) => {
    const def = metricDef(key)
    return {
      key,
      cells: columns.map((run, i) => {
        const value = flat[i][key] ?? null
        const prevValue = i > 0 ? (flat[i - 1][key] ?? null) : null
        return {
          config: run.config,
          value,
          delta: i > 0 ? metricDelta(value, prevValue, def) : null,
          prevConfig: i > 0 ? columns[i - 1].config : null,
          prevValue,
        }
      }),
    }
  })
}

/** Значение метрики по одному типу запросов (для объяснения ячейки). */
export function metricByType(run: EvaluationView, key: string): { type: string; value: number }[] {
  return Object.entries(run.by_type ?? {})
    .map(([type, m]) => ({ type, value: flattenMetrics(m)[key] ?? null, n: m?.queries ?? 0 }))
    .filter((r): r is { type: string; value: number; n: number } => r.value !== null)
    .sort((a, b) => b.n - a.n)
    .map(({ type, value }) => ({ type, value }))
}

export interface Reproducibility {
  set: string
  setHash: string
  queryCount: number
  rulesVersions: number[]
  catalogVersions: string[]
  commits: string[]
  /** true — все столбцы посчитаны на одном наборе (одинаковый хэш). */
  sameSet: boolean
  /** Строки воспроизводимости сервера, без повторов. */
  repro: string[]
}

export function reproducibility(columns: readonly EvaluationView[]): Reproducibility | null {
  if (columns.length === 0) return null
  const uniq = <T>(xs: T[]) => [...new Set(xs)]
  const hashes = uniq(columns.map((r) => r.set_hash))
  return {
    set: columns[0].set,
    setHash: columns[0].set_hash,
    queryCount: columns[0].queries,
    rulesVersions: uniq(columns.map((r) => r.rules_version)).sort((a, b) => a - b),
    catalogVersions: uniq(columns.map((r) => r.catalog_version).filter(Boolean)),
    commits: uniq(columns.map((r) => r.commit).filter(Boolean)),
    sameSet: hashes.length === 1,
    repro: uniq(columns.map((r) => r.repro).filter(Boolean)),
  }
}

/** Самый свежий прогон среди столбцов. */
export function latestRun(columns: readonly EvaluationView[]): EvaluationView | null {
  let best: EvaluationView | null = null
  for (const c of columns) if (!best || c.created_at > best.created_at) best = c
  return best
}

/** Короткий хэш для экрана: первые 8 символов, полный — в подсказке. */
export function shortHash(h: string | null | undefined): string {
  if (!h) return '—'
  return h.length > 10 ? h.slice(0, 8) : h
}

export interface RegressionRow extends EvalChange {
  config: ConfigId
}

/** Все ухудшения: худшие сверху (по потере NDCG@10), при равенстве — частые запросы выше. */
export function regressionRows(columns: readonly EvaluationView[]): RegressionRow[] {
  const out: RegressionRow[] = []
  columns.forEach((run, i) => {
    for (const r of run.regressions ?? [])
      out.push({ ...r, config: run.config, against: r.against || columns[i - 1]?.config || '' })
  })
  return out.sort((a, b) => a.ndcg_delta - b.ndcg_delta || b.freq - a.freq)
}

/** Подписи типов запросов набора (queries.json). */
export const QUERY_TYPE_LABELS: Record<string, string> = {
  exact: 'точное название',
  sku: 'артикул',
  typo: 'опечатка',
  layout: 'раскладка',
  slang: 'сленг',
  purpose: 'назначение',
  license: 'условия лицензии',
  analog: 'аналог',
  absent: 'нет в каталоге',
  alias_gap: 'неизвестный алиас',
  non_product: 'не товар',
  noise: 'шум',
  renewal: 'продление',
}

export function queryTypeLabel(t: string | null | undefined): string {
  if (!t) return ''
  return QUERY_TYPE_LABELS[t] ?? t
}

/** Что изменилось по запросу (EvalChange.reasons). */
export const CHANGE_REASON_LABELS: Record<string, string> = {
  ndcg: 'порядок хуже',
  lost_relevant: 'пропал подходящий товар',
  became_empty: 'выдача стала пустой',
  lost_exact: 'точный товар ушёл с 1-го места',
  new_violation: 'нарушено условие лицензии',
  false_rescue: 'показано лишнее вместо «нет»',
  gained_relevant: 'появился подходящий товар',
  no_longer_empty: 'выдача больше не пустая',
  gained_exact: 'точный товар на 1-м месте',
  fixed_violation: 'условие лицензии соблюдено',
  false_rescue_fixed: 'честное «нет» вместо лишнего',
}

export function changeReasonLabel(r: string): string {
  return CHANGE_REASON_LABELS[r] ?? r
}

/** Сколько ухудшений у NDCG — без учёта других причин; для бейджа вкладки. */
export function regressionCount(columns: readonly EvaluationView[]): number {
  return columns.reduce((n, c) => n + (c.regressions?.length ?? 0), 0)
}

// ── LLM-судья: рядом с NDCG — согласие судьи с ручной разметкой ──

export interface JudgeRow {
  config: string
  labels: number
  judge: number
  /** Судья минус разметка. */
  diff: number
}

export interface JudgeView {
  rows: JudgeRow[]
  /** Пары соседних конфигураций, которые судья ставит в обратном порядке, чем разметка. */
  swapped: [string, string][]
}

/** Строки NDCG по разметке и по судье в порядке лестницы; где порядок расходится. */
export function judgeView(ja: JudgeAgreement | null | undefined): JudgeView | null {
  if (!ja?.by_config?.length) return null
  const rows = [...ja.by_config]
    .sort((a, b) => ladderRank(a.config) - ladderRank(b.config))
    .map((r) => ({
      config: r.config,
      labels: r.ndcg_labels,
      judge: r.ndcg_judge,
      diff: r.ndcg_judge - r.ndcg_labels,
    }))
  const swapped: [string, string][] = []
  for (let i = 1; i < rows.length; i++) {
    const a = rows[i - 1]
    const b = rows[i]
    if (Math.sign(b.labels - a.labels) !== Math.sign(b.judge - a.judge))
      swapped.push([a.config, b.config])
  }
  return { rows, swapped }
}
