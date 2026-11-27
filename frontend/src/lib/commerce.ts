// Коммерция контура коммерции: лестница C0–C3 «До / После» и карточка правки связей агента —
// подписи метрик, формат, дельты к базе, два раздельных статуса. Числа считает сервер.
// Без DOM: tests/commerce.test.ts.
import type {
  CommerceConfigRun,
  CommerceMetricKey,
  CommerceMetrics,
  CommerceResponse,
  CompanionsEval,
} from '@/api/typesCommerce'
import { formatDecimal, formatInt, formatPercent, formatPp, formatSigned } from './format.ts'

type Kind = 'share' | 'count' | 'score'

export interface CommerceMetricDef {
  key: CommerceMetricKey
  label: string
  /** Что значит, для «Как посчитано». */
  what: string
  kind: Kind
  /** Больше — лучше (у нарушений — меньше). */
  higherBetter: boolean
}

/** Столбцы в порядке экрана. */
export const COMMERCE_METRICS: readonly CommerceMetricDef[] = [
  {
    key: 'mpn_top1',
    label: 'Точность по артикулам',
    what: 'Доля запросов по парт-номеру, где нужная позиция первая.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'units_parse_acc',
    label: 'Разбор единиц',
    what: 'Доля запросов с характеристиками («32 ГБ», «24 порта»), где единицы распознаны верно.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'ndcg10',
    label: 'NDCG@10 выдачи',
    what: 'Качество основной выдачи на наборе p2: блок допродажи и маржа не должны его ронять.',
    kind: 'score',
    higherBetter: true,
  },
  {
    key: 'exact_top1',
    label: 'Точные запросы: топ-1',
    what: 'Доля точных запросов, где нужный товар первый.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'compat_violations',
    label: 'Нарушения совместимости',
    what: 'Число несовместимых пар, попавших в блок или выдачу «X для Y». Цель — 0.',
    kind: 'count',
    higherBetter: false,
  },
  {
    key: 'required_recall',
    label: 'Полнота обязательных',
    what: 'Доля якорей с условно обязательным дополнением, где оно показано.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'coverage',
    label: 'Покрытие блока',
    what: 'Доля запросов с уверенным якорем, где блок «Дополните решение» не пуст.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'block_precision_judge',
    label: 'Точность блока (судья)',
    what: 'Доля позиций блока, которые LLM-судья счёл уместными.',
    kind: 'share',
    higherBetter: true,
  },
  {
    key: 'margin_index',
    label: 'Индекс показанной маржи',
    what: 'Маржа первой десятки с весом по позиции. Индекс, не рубли и не прогноз.',
    kind: 'score',
    higherBetter: true,
  },
]

export const metricDef = (k: string) => COMMERCE_METRICS.find((m) => m.key === k) ?? null

/** Значение метрики строкой; null — не посчитано. */
export function metricValue(key: CommerceMetricKey, v: number | null | undefined): string | null {
  if (typeof v !== 'number' || !Number.isFinite(v)) return null
  const d = metricDef(key)
  if (!d) return formatDecimal(v, 3)
  if (d.kind === 'share') return formatPercent(v, 1)
  if (d.kind === 'count') return formatInt(v)
  return formatDecimal(v, 3)
}

export interface MetricDelta {
  text: string
  /** better | worse | same — знак для цвета и иконки. */
  tone: 'better' | 'worse' | 'same'
}

/** Изменение метрики к базе: «+2,1 п.п.», «−1», «+0,004». */
export function metricDelta(
  key: CommerceMetricKey,
  now: number | null | undefined,
  base: number | null | undefined,
): MetricDelta | null {
  if (typeof now !== 'number' || typeof base !== 'number') return null
  const d = metricDef(key)
  const diff = now - base
  const eps = d?.kind === 'count' ? 0.5 : 1e-4
  const tone: MetricDelta['tone'] =
    Math.abs(diff) < eps ? 'same' : diff > 0 === (d?.higherBetter ?? true) ? 'better' : 'worse'
  let text: string
  if (d?.kind === 'share') text = formatPp(diff, 1)
  else if (d?.kind === 'count') text = formatSigned(diff, 0)
  else text = formatSigned(diff, 3)
  return { text, tone }
}

/** Столбцы, по которым в лестнице есть хоть одно число (непосчитанное не показываем). */
export function ladderColumns(rows: CommerceConfigRun[]): CommerceMetricDef[] {
  return COMMERCE_METRICS.filter((m) => rows.some((r) => typeof r.metrics?.[m.key] === 'number'))
}

export const CONFIG_LABELS: Record<string, string> = {
  C0: 'C0 · базовый поиск',
  C1: 'C1 · + оборудование',
  C2: 'C2 · + блок допродажи',
  C3: 'C3 · + политика маржи',
  C4: 'C4 · + правки агента',
}

export const configLabel = (c: string) => CONFIG_LABELS[c] ?? c

/** Строки лестницы в порядке C0 → C4. */
export function sortLadder(rows: CommerceConfigRun[]): CommerceConfigRun[] {
  return [...rows].sort((a, b) => a.config.localeCompare(b.config))
}

export const COMPONENT_LABELS: Record<string, string> = {
  hardware: 'Оборудование',
  companions: 'Блок допродажи',
  margin: 'Политика маржи',
  agent: 'Правки агента',
}

/** Компонент, который добавляет ступень лестницы. */
const STEP_COMPONENT: Record<string, 'hardware' | 'companions' | 'margin'> = {
  C1: 'hardware',
  C2: 'companions',
  C3: 'margin',
}

/** Ступень готова: сервер не пометил ready=false и компонент ступени есть в трассе. */
export function ladderReady(row: CommerceConfigRun, ready: CommerceResponse['ready']): boolean {
  if (row.ready === false) return false
  const comp = STEP_COMPONENT[row.config]
  return !comp || !ready || ready[comp] !== false
}

/** Подпись прогона с одним изменением: «+ Блок допродажи», «− Оборудование». */
export function singleChangeLabel(c: CommerceConfigRun): string {
  const ch = (c.changed ?? '').trim()
  const m = /^([+-])\s*(\w+)$/.exec(ch)
  if (m) return `${m[1] === '+' ? '+' : '−'} ${COMPONENT_LABELS[m[2]] ?? m[2]}`
  return ch || c.config
}

/** Запросы, где основная выдача хуже предыдущей ступени, по всей лестнице. */
export function ladderRegressions(ladder: CommerceConfigRun[]) {
  return ladder.flatMap((r) =>
    (r.regressions ?? []).map((g) => ({
      query: g.text,
      from: g.against ?? null,
      to: r.config,
      before: typeof g.ndcg_from === 'number' ? g.ndcg_from : null,
      after: typeof g.ndcg_to === 'number' ? g.ndcg_to : null,
    })),
  )
}

/** Изменения одного компонента: улучшения и ухудшения рядом, по метрикам с обоими числами. */
export function singleChangeDeltas(
  c: CommerceConfigRun & { base_metrics?: CommerceMetrics },
  ladder: CommerceConfigRun[],
) {
  const baseId = c.against ?? c.base
  const base: CommerceMetrics =
    c.base_metrics ?? ladder.find((r) => r.config === baseId)?.metrics ?? {}
  const now: CommerceMetrics = c.metrics ?? {}
  const out: { def: CommerceMetricDef; value: string; delta: MetricDelta }[] = []
  for (const def of COMMERCE_METRICS) {
    const delta = metricDelta(def.key, now[def.key], base[def.key])
    const value = metricValue(def.key, now[def.key])
    if (delta && value) out.push({ def, value, delta })
  }
  return {
    better: out.filter((x) => x.delta.tone === 'better'),
    worse: out.filter((x) => x.delta.tone === 'worse'),
    same: out.filter((x) => x.delta.tone === 'same'),
  }
}

// ── Карточка relation_patch ──

export const TECHNICAL_STATUS: Record<string, { label: string; ok: boolean | null }> = {
  verified: { label: 'Техническая корректность: подтверждена', ok: true },
  unverified: { label: 'Техническая корректность: не проверена', ok: null },
  failed: { label: 'Техническая корректность: не подтверждена', ok: false },
}

export const ECONOMIC_STATUS: Record<string, { label: string; ok: boolean | null }> = {
  unknown: { label: 'Эффект: неизвестен, нужен замер', ok: null },
  estimated: { label: 'Эффект: оценён, не измерен', ok: null },
  measured: { label: 'Эффект: измерен', ok: true },
}

export function technicalStatus(s: string | null | undefined) {
  return s ? (TECHNICAL_STATUS[s] ?? { label: `Техническая корректность: ${s}`, ok: null }) : null
}

export function economicStatus(s: string | null | undefined) {
  return ECONOMIC_STATUS[s ?? 'unknown'] ?? { label: `Эффект: ${s}`, ok: null }
}

/** Прогон блока до/после правки связей строками (evaluate_companions). */
export function companionsEvalLines(
  before: CompanionsEval | null | undefined,
  after: CompanionsEval | null | undefined,
): string[] {
  if (!before && !after) return []
  const out: string[] = []
  const pair = (label: string, k: keyof CompanionsEval, fmt: (n: number) => string) => {
    const b = before?.[k]
    const a = after?.[k]
    if (typeof b === 'number' && typeof a === 'number')
      out.push(`${label}: было ${fmt(b)}, стало ${fmt(a)}`)
    else if (typeof a === 'number') out.push(`${label}: ${fmt(a)}`)
  }
  pair('Полнота обязательных', 'required_recall', (n) => formatPercent(n, 1))
  pair('Нарушения совместимости', 'compat_violations', (n) => formatInt(n))
  pair('Покрытие блока', 'coverage', (n) => formatPercent(n, 1))
  pair('Точность блока (судья)', 'block_precision_judge', (n) => formatPercent(n, 1))
  return out
}
