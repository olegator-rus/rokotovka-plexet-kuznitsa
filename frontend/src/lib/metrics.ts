// Метрики оценки качества (ARCHITECTURE §6): подписи, направление «лучше», формат,
// изменение к предыдущей конфигурации, перцентили. Считает сервер; здесь только отображение.
// Без DOM: tests/metrics.test.ts.
import { formatDecimal, formatMs, formatPercent, formatPp, formatSigned, signOf } from './format.ts'

/** share — доля 0..1 (изменение в п.п.), score — 0..1 без процентов, ms — время, count — штуки. */
export type MetricFormat = 'share' | 'score' | 'ms' | 'count'

export interface MetricDef {
  key: string
  label: string
  /** up — больше значит лучше, down — меньше значит лучше. */
  better: 'up' | 'down'
  format: MetricFormat
  /** Что измеряет, одной фразой. */
  what: string
  /** Как считается. */
  how: string
}

export const METRICS: readonly MetricDef[] = [
  {
    key: 'ndcg10',
    label: 'NDCG@10',
    better: 'up',
    format: 'score',
    what: 'Насколько хорошо упорядочены первые 10 позиций относительно разметки.',
    how: 'DCG первых 10 позиций по оценкам 0–3, делённый на DCG идеального порядка; среднее по запросам.',
  },
  {
    key: 'judge_agreement',
    label: 'Согласие LLM-судьи с ручной разметкой',
    better: 'up',
    format: 'share',
    what: 'Насколько оценки судьи совпадают с ручными на одних и тех же парах.',
    how: 'Доля пар «запрос — товар», где оценка судьи совпала с ручной.',
  },
  {
    key: 'mrr10',
    label: 'MRR@10',
    better: 'up',
    format: 'score',
    what: 'Как высоко стоит первый подходящий товар.',
    how: '1 / позиция первого товара с оценкой ≥ 2 в первых 10; 0, если такого нет; среднее по запросам.',
  },
  {
    key: 'recall50',
    label: 'Recall@50',
    better: 'up',
    format: 'share',
    what: 'Какая часть размеченных подходящих товаров попала в первые 50.',
    how: 'Найдено в топ-50 с оценкой ≥ 2 / всего таких в разметке; полнота относительно размеченного пула.',
  },
  {
    key: 'exact_accuracy',
    label: 'Точные запросы: верный на 1-м месте',
    better: 'up',
    format: 'share',
    what: 'Артикул или точное название — нужный товар первым.',
    how: 'Доля запросов типов exact и sku, где первая позиция имеет оценку 3.',
  },
  {
    key: 'rescued_rate',
    label: 'Спасённые проблемные запросы',
    better: 'up',
    format: 'share',
    what: 'Опечатки, раскладка, сленг, лицензии, аналоги — есть подходящий товар в первых 10.',
    how: 'Доля проблемных запросов (typo, layout, slang, alias_gap, license, analog) с оценкой ≥ 2 в топ-10.',
  },
  {
    key: 'relevant_top10',
    label: 'Есть подходящий в топ-10',
    better: 'up',
    format: 'share',
    what: 'В первых 10 позициях есть хотя бы один подходящий товар.',
    how: 'Среди запросов, у которых в разметке есть подходящий товар: доля с оценкой ≥ 2 в топ-10.',
  },
  {
    key: 'empty_rate',
    label: 'Пустые выдачи',
    better: 'down',
    format: 'share',
    what: 'Запросы, на которые не нашлось ничего.',
    how: 'Доля запросов набора с нулём результатов, каждый запрос с весом 1.',
  },
  {
    key: 'empty_rate_weighted',
    label: 'Пустые выдачи, по частоте',
    better: 'down',
    format: 'share',
    what: 'То же, но частые запросы весят больше.',
    how: 'Сумма частот запросов с пустой выдачей / сумма частот всех запросов набора.',
  },
  {
    key: 'no_relevant_top10',
    label: 'Нет подходящего в топ-10',
    better: 'down',
    format: 'share',
    what: 'Выдача есть, но подходящего товара в ней нет.',
    how: 'Среди запросов, у которых в разметке есть подходящий товар: выдача непустая, а в первых 10 нет позиции с оценкой ≥ 2.',
  },
  {
    key: 'false_rescue_rate',
    label: 'Ложное спасение',
    better: 'down',
    format: 'share',
    what: 'Товара нет в каталоге, а поиск показал что-то без пометки «аналог» или «нет».',
    how: 'Доля запросов типов absent и noise с непустой выдачей без честной пометки.',
  },
  {
    key: 'violation_rate',
    label: 'Нарушения обязательных условий',
    better: 'down',
    format: 'share',
    what: 'В первых 10 — позиция с не той метрикой, количеством или типом лицензии.',
    how: 'Доля запросов типов license и renewal, где в топ-10 есть позиция нужного продукта с не той метрикой, количеством или типом лицензии.',
  },
  {
    key: 'p50_ms',
    label: 'Время ответа P50',
    better: 'down',
    format: 'ms',
    what: 'Медианное время ответа сервиса поиска на прогоне (без HTTP).',
    how: 'Половина запросов прогона ответила быстрее этого значения.',
  },
  {
    key: 'p95_ms',
    label: 'Время ответа P95',
    better: 'down',
    format: 'ms',
    what: 'Время ответа для 95 % запросов прогона.',
    how: '95 % запросов прогона ответили быстрее этого значения.',
  },
]

const BY_KEY = new Map(METRICS.map((m) => [m.key, m]))

/** Определение метрики; неизвестный ключ сервера — показывается под своим именем как доля или число. */
export function metricDef(key: string): MetricDef {
  const known = BY_KEY.get(key)
  if (known) return known
  const looksMs = key.endsWith('_ms')
  return {
    key,
    label: key,
    better: looksMs ? 'down' : 'up',
    format: looksMs ? 'ms' : 'score',
    what: 'Метрика сервера без описания на фронте.',
    how: 'Смотрите описание прогона в docs/EVALUATION.md.',
  }
}

/** Порядок строк таблицы: известные метрики в порядке METRICS, затем неизвестные по алфавиту. */
export function orderMetricKeys(keys: Iterable<string>): string[] {
  const set = new Set(keys)
  const known = METRICS.map((m) => m.key).filter((k) => set.has(k))
  const rest = [...set].filter((k) => !BY_KEY.has(k)).sort()
  return [...known, ...rest]
}

export function formatMetric(value: number | null | undefined, format: MetricFormat): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  switch (format) {
    case 'share':
      return formatPercent(value, 1)
    case 'ms':
      return formatMs(value)
    case 'count':
      return formatDecimal(value, 0)
    default:
      return formatDecimal(value, 3)
  }
}

export type Verdict = 'better' | 'worse' | 'same'

export interface MetricDelta {
  diff: number
  /** Куда сдвинулось значение: стрелка показывает именно это. */
  arrow: 'up' | 'down' | 'flat'
  /** Лучше или хуже с учётом направления метрики: от этого — цвет. */
  verdict: Verdict
  /** «+3,2 п.п.», «−0,041», «+12 мс». */
  text: string
  /** Для чтения экраном: «лучше на 3,2 п.п.». */
  label: string
}

/** Шаг округления, ниже которого изменение считаем нулём (то, что видно на экране). */
function epsilon(format: MetricFormat): number {
  if (format === 'share') return 0.0005
  if (format === 'ms') return 0.5
  if (format === 'count') return 0.5
  return 0.0005
}

/** Изменение к предыдущему столбцу. Нет одного из значений — null (стрелку не рисуем). */
export function metricDelta(
  current: number | null | undefined,
  previous: number | null | undefined,
  def: Pick<MetricDef, 'better' | 'format'>,
): MetricDelta | null {
  if (current === null || current === undefined || !Number.isFinite(current)) return null
  if (previous === null || previous === undefined || !Number.isFinite(previous)) return null
  const diff = current - previous
  const flat = Math.abs(diff) < epsilon(def.format)
  const arrow = flat ? 'flat' : diff > 0 ? 'up' : 'down'
  const verdict: Verdict = flat ? 'same' : diff > 0 === (def.better === 'up') ? 'better' : 'worse'
  let text: string
  if (flat) text = 'без изменений'
  else if (def.format === 'share') text = formatPp(diff)
  else if (def.format === 'ms') text = `${signOf(diff)}${formatMs(Math.abs(diff))}`
  else if (def.format === 'count') text = formatSigned(diff, 0)
  else text = formatSigned(diff, 3)
  const label =
    verdict === 'same'
      ? 'без изменений'
      : `${verdict === 'better' ? 'лучше' : 'хуже'} на ${text.replace(/^[+−]/, '')}`
  return { diff, arrow, verdict, text, label }
}

/**
 * Перцентиль с линейной интерполяцией между соседними значениями (как numpy по умолчанию).
 * Пустой список — null. p в диапазоне 0..100.
 */
export function percentile(values: readonly number[], p: number): number | null {
  const xs = values.filter((v) => Number.isFinite(v)).sort((a, b) => a - b)
  if (xs.length === 0) return null
  const clamped = Math.min(100, Math.max(0, p))
  const pos = ((xs.length - 1) * clamped) / 100
  const lo = Math.floor(pos)
  const hi = Math.ceil(pos)
  if (lo === hi) return xs[lo]
  return xs[lo] + (xs[hi] - xs[lo]) * (pos - lo)
}
