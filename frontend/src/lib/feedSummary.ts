// Сводка «На данных заказчика» (): подписи и строки из GET /api/evaluations/feed. Ничего не
// пересчитывает — только выбирает и подписывает числа сервера. Без DOM: проверяет tests/feedSummary.test.ts.
import type { FeedClosedDetail, FeedCostRun, FeedScore, FeedSummary } from '@/api/typesFeed'
import { formatInt, formatPercent, plural } from './format.ts'

/** Конфигурации закрытой части на экране S6: BM25, гибрид, полный продукт. */
export const CLOSED_CONFIGS = ['BM25', 'B2', 'live'] as const

const SET_LABELS: Record<string, string> = {
  etalon: 'Эталоны заказчика',
  tz: 'Примеры из ТЗ',
  neighbors: 'Соседи эталонов (настройка)',
  closed: 'Отложенный набор (закрытая часть)',
  understanding: 'Набор понимания (настройка)',
  feed_regression: 'Регрессия агента',
  feed_tune: 'Настройка агента',
}

/** Подпись набора. */
export function setLabel(set: string): string {
  return SET_LABELS[set] ?? set
}

/** Подпись конфигурации из сводки; неизвестная — её id. */
export function configLabel(s: FeedSummary | null, id: string): string {
  return s?.configs.find((c) => c.id === id)?.label ?? id
}

/** «10 из 10». */
export function outOf(n: number, of: number): string {
  return `${formatInt(n)} из ${formatInt(of)}`
}

/** Доля как «95 %» или null, если считать не из чего. */
export function shareOrNull(v: number | null | undefined): string | null {
  return typeof v === 'number' && Number.isFinite(v) ? formatPercent(v, 0) : null
}

export interface ClosedCell {
  config: string
  label: string
  /** null — у конфигурации прогона закрытой части нет. */
  score: FeedScore | null
}

/** Три числа закрытой части по порядку BM25 → гибрид → полный продукт. */
export function closedCells(s: FeedSummary | null): ClosedCell[] {
  return CLOSED_CONFIGS.map((id) => ({
    config: id,
    label: configLabel(s, id),
    score: s?.closed.find((c) => c.config === id) ?? null,
  }))
}

/** Закрытая часть ещё не прогонялась ни в одной конфигурации. */
export function closedPending(s: FeedSummary | null): boolean {
  return !s || s.closed.length === 0
}

export interface ClosedKindInfo {
  label: string
  /** Примеры запросов этого вида из набора; у неизвестного вида — пусто. */
  examples: string[]
}

/** Подписи видов запросов отложенного набора и примеры из него — по ключу вида из closed_detail.kinds. */
const CLOSED_KINDS: Record<string, ClosedKindInfo> = {
  склонение: { label: 'Склонение', examples: ['астру линукс', 'сканеру-вс'] },
  транслит: { label: 'Транслит', examples: ['эни деска', 'икс спайдер'] },
  опечатка: { label: 'Опечатка', examples: ['maxpatorl', 'anydsek'] },
  раскладка: { label: 'Чужая раскладка', examples: ['fcnhf kbyerc', 'ьшскщыщае 365'] },
  атрибут: {
    label: 'Название с условиями',
    examples: ['продление kes расширенный на 50 узлов'],
  },
  'ТЗ в словоформе': { label: 'Словоформа из ТЗ', examples: ['антивиря', 'мдаемона'] },
  сервер: { label: 'Серверы', examples: ['двухпроцессорный сервер', 'серверы supermicro'] },
  'по запросу → альтернатива': {
    label: 'Продукт без цены, рядом альтернатива',
    examples: ['windows 10 pro', 'vmware vsphere'],
  },
  сервисный: { label: 'Вопрос покупателя, а не товар', examples: ['как оплатить заказ'] },
  мусорный: { label: 'Мусор', examples: ['zxcvbnm', 'кукуруза в банке'] },
}

/** Подпись и примеры вида запросов; неизвестный ключ — как есть, без примеров. */
export function closedKind(kind: string): ClosedKindInfo {
  return Object.hasOwn(CLOSED_KINDS, kind) ? CLOSED_KINDS[kind]! : { label: kind, examples: [] }
}

/** Примеры запросов строкой: «астру линукс», «сканеру-вс». */
export function closedExamples(kind: string): string {
  return closedKind(kind)
    .examples.map((e) => `«${e}»`)
    .join(', ')
}

/** «83 запроса были отложены» — число запросов набора с согласованием. */
export function closedHeldOut(total: number): string {
  return `${formatInt(total)} ${plural(total, ['запрос был отложен', 'запроса были отложены', 'запросов были отложены'])}`
}

/** «на 12 запросах из 83» — где понадобилась языковая модель. */
export function closedModelShare(d: FeedClosedDetail): string {
  return `на ${formatInt(d.model_searches)} ${plural(d.model_searches, ['запросе', 'запросах', 'запросах'])} из ${formatInt(d.total)}`
}

export interface ClosedFact {
  value: string
  text: string
}

/** Четыре факта об отложенном наборе: все числа — из closed_detail. */
export function closedFacts(d: FeedClosedDetail): ClosedFact[] {
  const { spelling: sp, product: pr } = d
  return [
    {
      value: outOf(d.live, d.total),
      text: `целевая страница первым ответом. У текстового поиска — ${formatInt(d.bm25)}`,
    },
    { value: outOf(d.top4, d.total), text: 'нужная страница в первых четырёх ответах' },
    {
      value: outOf(sp.top4, sp.total),
      text: `запросы с искажённым написанием: нужная страница в первых четырёх. Первым ответом — ${formatInt(sp.live)}, у текстового поиска — ${formatInt(sp.bm25)}`,
    },
    {
      value: `${formatInt(pr.live)} против ${formatInt(pr.bm25)}`,
      text: `из ${formatInt(pr.total)} ${plural(pr.total, ['запроса', 'запросов', 'запросов'])}, где нужно найти страницу товара (без вопросов и мусора)`,
    },
  ]
}

/** Прогон стоимости для плитки: тёплый (как у покупателя), иначе холодный. */
export function headlineCost(s: FeedSummary | null): FeedCostRun | null {
  return s?.cost?.warm ?? s?.cost?.cold ?? null
}

/** «холодный» | «тёплый». */
export function modeLabel(mode: string | undefined): string {
  if (mode === 'cold') return 'холодный'
  if (mode === 'warm') return 'тёплый'
  if (mode === 'journal') return 'журнал витрины'
  return mode ?? ''
}

/** Прогоны на фиде есть: эталоны, таблица или стоимость. */
export function hasFeedRuns(s: FeedSummary | null): boolean {
  return !!s && (s.table.length > 0 || s.etalon !== null || s.cost !== null)
}

/** Каталог — mock (прогоны до данных заказчика), по версии каталога строки прогона. */
export function isMockCatalog(version: string | null | undefined): boolean {
  return !!version && version.toLowerCase().startsWith('mock')
}

/** Подпись mock-прогонов на экранах прежних метрик. */
export const MOCK_RUN_LABEL = 'mock-каталог, до данных заказчика'

/** Короткий номер прогона без даты: «…abc123» (в run_id — время запуска). */
export function runTail(id: string | null | undefined): string {
  if (!id) return '—'
  return id.length > 6 ? `…${id.slice(-6)}` : id
}

/** Каталог прогона для подписи: mock — «mock-каталог, до данных заказчика», версия фида — «фид заказчика». */
export function catalogLabel(version: string | null | undefined): string {
  if (!version) return '—'
  if (isMockCatalog(version)) return MOCK_RUN_LABEL
  if (version.startsWith('softline-')) return 'фид заказчика'
  return version
}
