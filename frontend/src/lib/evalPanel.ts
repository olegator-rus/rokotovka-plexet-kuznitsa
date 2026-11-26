// «До / После» → «Панель»: подписи и формат метрик прогона по префиксам. Числа считает сервер
// (GET /api/evaluations?set=panel); здесь — только отображение и выбор строк.
import type {
  IntentRate,
  PanelChange,
  PanelEvalRun,
  PanelMetrics,
  PanelModeResult,
} from '@/api/typesEvalPanel'
import { formatDecimal, formatInt, formatPercent, plural } from './format.ts'

const NBSP = ' '

export type PanelMetricKey =
  | 'suggest_mrr5'
  | 'suggest_success5'
  | 'product_mrr5'
  | 'product_success5'
  | 'saved_keystrokes'
  | 'saved_share'
  | 'empty_suggest_rate'
  | 'family_dup_rate'
  | 'p50_ms'
  | 'p95_ms'
  | 'p50_ms_cached'
  | 'p95_ms_cached'

export interface PanelMetricDef {
  key: PanelMetricKey
  label: string
  kind: 'index' | 'share' | 'chars' | 'ms'
  /** Больше — лучше (для стрелки изменения). */
  higherBetter: boolean
  what: string
}

export const PANEL_METRICS: PanelMetricDef[] = [
  {
    key: 'suggest_mrr5',
    label: 'MRR@5 подсказок',
    kind: 'index',
    higherBetter: true,
    what: 'Среднее 1/место целевого запроса среди первых пяти подсказок; не нашёлся — 0. Префиксы без результатов не входят.',
  },
  {
    key: 'suggest_success5',
    label: 'Success@5 подсказок',
    kind: 'share',
    higherBetter: true,
    what: 'Доля префиксов, где целевой запрос — в первых пяти подсказках.',
  },
  {
    key: 'product_mrr5',
    label: 'MRR@5 товаров',
    kind: 'index',
    higherBetter: true,
    what: 'Среднее 1/место целевого семейства среди первых пяти товаров панели.',
  },
  {
    key: 'product_success5',
    label: 'Success@5 товаров',
    kind: 'share',
    higherBetter: true,
    what: 'Доля префиксов, где целевое семейство — в первых пяти товарах панели.',
  },
  {
    key: 'saved_keystrokes',
    label: 'Сэкономлено нажатий',
    kind: 'chars',
    higherBetter: true,
    what: 'На цель позднего периода: длина запроса минус кратчайший префикс, на котором он появился в первых пяти подсказках.',
  },
  {
    key: 'saved_share',
    label: 'Доля сэкономленных нажатий',
    kind: 'share',
    higherBetter: true,
    what: 'Сэкономленные нажатия, делённые на суммарную длину целевых запросов.',
  },
  {
    key: 'empty_suggest_rate',
    label: 'Пустые подсказки',
    kind: 'share',
    higherBetter: false,
    what: 'Доля префиксов (кроме заведомо пустых), на которые панель не показала ни одной подсказки.',
  },
  {
    key: 'family_dup_rate',
    label: 'Дубли семейств',
    kind: 'share',
    higherBetter: false,
    what: 'Доля ответов с товарами, где в шести слотах два варианта одного семейства.',
  },
  {
    key: 'p50_ms',
    label: 'p50 сервера',
    kind: 'ms',
    higherBetter: false,
    what: 'Медиана времени ответа на сервере, первый проход (товары без кэша).',
  },
  {
    key: 'p95_ms',
    label: 'p95 сервера',
    kind: 'ms',
    higherBetter: false,
    what: '95-й перцентиль времени ответа на сервере, первый проход (товары без кэша).',
  },
  {
    key: 'p50_ms_cached',
    label: 'p50 из кэша',
    kind: 'ms',
    higherBetter: false,
    what: 'Медиана времени ответа на повторе того же префикса.',
  },
  {
    key: 'p95_ms_cached',
    label: 'p95 из кэша',
    kind: 'ms',
    higherBetter: false,
    what: '95-й перцентиль времени ответа на повторе того же префикса.',
  },
]

/** Миллисекунды с двумя знаками до 10 мс: панель отвечает быстрее миллисекунды. */
export function formatPanelMs(ms: number): string {
  if (!Number.isFinite(ms)) return ''
  return `${ms < 10 ? formatDecimal(ms, 2) : formatInt(Math.round(ms))}${NBSP}мс`
}

/** Значение метрики строкой; null — не посчитано. */
export function panelValue(def: PanelMetricDef, m: PanelMetrics | undefined): string | null {
  const v = m?.[def.key]
  if (v === null || v === undefined) return null
  switch (def.kind) {
    case 'index':
      return formatDecimal(v, 3)
    case 'share':
      return formatPercent(v, 1)
    case 'chars':
      return `${formatDecimal(v, 1)}${NBSP}симв.`
    default:
      return formatPanelMs(v)
  }
}

/** Доля намерений строкой («37,5 %») или null. */
export function rateValue(r: IntentRate | null | undefined): string | null {
  return r && r.value !== null ? formatPercent(r.value, 1) : null
}

export function rateSub(r: IntentRate | null | undefined, unit: [string, string, string]): string {
  return r ? `${formatInt(r.num)} из ${formatInt(r.den)} ${plural(r.den, unit)}` : ''
}

/** Режим прогона по имени (old | new). */
export function panelMode(
  run: PanelEvalRun | null | undefined,
  mode: string,
): PanelModeResult | null {
  return run?.modes.find((m) => m.mode === mode) ?? null
}

/** Изменение new против old: лучше | хуже | равно | нет данных. */
export function panelDelta(
  def: PanelMetricDef,
  before: PanelMetrics | undefined,
  after: PanelMetrics | undefined,
): 'better' | 'worse' | 'same' | null {
  const a = before?.[def.key]
  const b = after?.[def.key]
  if (a === null || a === undefined || b === null || b === undefined) return null
  if (Math.abs(a - b) < 1e-9) return 'same'
  return b > a === def.higherBetter ? 'better' : 'worse'
}

/** Блокирующие проверки панели контура выдачи: точные не вытеснены, нет дублей семейств, нет подсказок в пустую выдачу. */
export function blockingChecks(m: PanelMetrics) {
  return [
    {
      key: 'exact',
      label: 'Точные совпадения не вытеснены',
      ok: m.exact_displaced === 0,
      detail: `вытеснено ${formatInt(m.exact_displaced)} из ${formatInt(m.exact_checks)} полных артикулов`,
    },
    {
      key: 'dups',
      label: 'Нет дублей семейств в шести слотах',
      ok: m.family_dup_prefixes === 0,
      detail: `с дублями ${formatInt(m.family_dup_prefixes)} из ${formatInt(m.product_prefixes)} ответов с товарами`,
    },
    {
      key: 'empty',
      label: 'Нет подсказок в пустую выдачу',
      ok: m.suggestions_to_empty === 0,
      detail: `${formatInt(m.suggestions_to_empty)} из ${formatInt(m.suggestions_checked)} проверенных поиском подсказок`,
    },
    {
      key: 'models',
      label: 'Ноль вызовов моделей на нажатие',
      ok: m.llm_calls + m.embed_calls + m.rerank_calls === 0,
      detail: `LLM ${formatInt(m.llm_calls)} · векторизаций ${formatInt(m.embed_calls)} · реранкер ${formatInt(m.rerank_calls)}`,
    },
  ]
}

/** Ухудшения и улучшения рядом: две колонки. */
export function splitChanges(changes: PanelChange[]) {
  return {
    worse: changes.filter((c) => c.kind === 'regression'),
    better: changes.filter((c) => c.kind === 'improvement'),
  }
}

/** Место в списке строкой: 0 — «нет в первых 5». */
export function rankLabel(r: number): string {
  return r > 0 ? `${r}-е место` : 'нет в первых 5'
}

export const SECTION_LABELS: Record<string, string> = {
  suggestions: 'подсказки',
  products: 'товары',
}

export const PREFIX_TYPE_LABELS: Record<string, string> = {
  broad: 'широкие',
  family: 'семейства',
  sku: 'артикулы и парт-номера',
  layout: 'раскладка',
  typo: 'опечатки',
  no_results: 'без результатов',
  license_dupes: 'дубли лицензий',
}
