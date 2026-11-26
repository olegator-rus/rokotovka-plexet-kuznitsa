// Раздел «Маржа»: рычаг, кривая качество ↔ индекс показанной маржи,
// «было / стало», защиты. Сервер считает всё; здесь только подписи, выбор точки и геометрия кривой.
// Без DOM: tests/margin.test.ts.
import type {
  MarginAffected,
  MarginPolicy,
  MarginRankItem,
  MarginSimPoint,
  MarginSimulateResponse,
} from '@/api/typesCommerce'
import { formatDecimal, formatInt, formatPercent, plural } from './format.ts'

/** Вес как на рычаге: 0,05. */
export const weightLabel = (w: number) => formatDecimal(w, 2)

const near = (a: number, b: number) => Math.abs(a - b) < 1e-9

/** Индекс положения рычага для веса; вес вне положений — ближайшее. */
export function positionIndex(positions: number[], w: number): number {
  if (!positions.length) return -1
  let best = 0
  for (let i = 1; i < positions.length; i++)
    if (Math.abs(positions[i] - w) < Math.abs(positions[best] - w)) best = i
  return best
}

/** Индекс упора: последнее доступное положение. Нет упора — последнее положение. */
export function stopIndex(positions: number[], stopWeight: number | null | undefined): number {
  if (!positions.length) return -1
  if (typeof stopWeight !== 'number') return positions.length - 1
  return positionIndex(positions, stopWeight)
}

export interface SimBounds {
  items?: Pick<MarginSimPoint, 'weight' | 'available'>[]
  stop_weight?: number | null
  min_weight?: number | null
}

/**
 * Положение за упором: сервер пометил available=false, или вес выше stop_weight / ниже min_weight.
 * Без симуляции — недоступно всё, кроме действующего.
 */
export function isLocked(w: number, sim: SimBounds | null | undefined): boolean {
  if (!sim) return false
  const p = (sim.items ?? []).find((x) => near(x.weight, w))
  if (p && p.available === false) return true
  if (typeof sim.stop_weight === 'number' && w > sim.stop_weight + 1e-9) return true
  if (typeof sim.min_weight === 'number' && w < sim.min_weight - 1e-9) return true
  return false
}

/** Точка симуляции для веса или null, если сервер её не прислал. */
export function pointFor(
  sim: Pick<MarginSimulateResponse, 'items'> | null | undefined,
  w: number,
): MarginSimPoint | null {
  return (sim?.items ?? []).find((p) => near(p.weight, w)) ?? null
}

/** Есть ли в данных маржа: без неё рычаг недоступен («Нет данных о марже»). */
export function hasMarginData(sim: Pick<MarginSimulateResponse, 'items'> | null | undefined) {
  return (sim?.items ?? []).some((p) => typeof p.margin_index === 'number')
}

/**
 * Изменение качества языком владельца (не NDCG): «в среднем на 0,4 % меньше лучших по соответствию товаров
 * в первой десятке, чем сейчас». delta — изменение NDCG@10 к выдаче контура поиска (вес base, 0,05).
 */
export function qualityLossText(delta: number | null | undefined, base = 0.05): string {
  if (typeof delta !== 'number' || !Number.isFinite(delta)) return 'Изменение качества не посчитано'
  const than = `чем при весе ${weightLabel(base)}`
  if (Math.abs(delta) < 0.0005)
    return `Лучшие по соответствию товары в первой десятке — как при весе ${weightLabel(base)}`
  const word = delta < 0 ? 'меньше' : 'больше'
  return `В среднем на ${formatPercent(Math.abs(delta), 1)} ${word} лучших по соответствию товаров в первой десятке, ${than}`
}

/** «ΔNDCG@10 −0,004» — мелкая подпись для технического читателя. */
export function ndcgDeltaLabel(delta: number | null | undefined): string {
  if (typeof delta !== 'number' || !Number.isFinite(delta)) return ''
  return `ΔNDCG@10 ${formatDecimal(delta, 3)}`
}

/** Индекс показанной маржи: 1,234 (безразмерный, по показам в первой десятке). */
export const marginIndexLabel = (v: number | null | undefined) =>
  typeof v === 'number' && Number.isFinite(v) ? formatDecimal(v, 3) : null

export const queriesLabel = (n: number) =>
  `${formatInt(n)} ${plural(n, ['запрос', 'запроса', 'запросов'])}`

/** Подпись под показателями: природа оценки. */
export function estimateNote(queries: number | null | undefined, set?: string): string {
  const n =
    typeof queries === 'number'
      ? `${formatInt(queries)} размеченных ${plural(queries, ['запросе', 'запросах', 'запросах'])}`
      : 'размеченном наборе'
  const s = set ? ` (набор ${set})` : ''
  return `Оценка на ${n}${s}. Маржа по показам в первой десятке. Не прогноз продаж.`
}

/** Почему дальше упора нельзя. */
export function stopReason(epsilon: number | null | undefined): string {
  const eps = typeof epsilon === 'number' ? formatDecimal(epsilon, 2) : '0,01'
  return `Дальше упора часть запросов теряет больше ${eps} NDCG@10 — выходит за бюджет качества. Такие положения недоступны.`
}

// ── Защиты ──

export const GUARD_LABELS: Record<string, string> = {
  exact_top1: 'первая позиция точного запроса неподвижна',
  exact_query: 'точный запрос: сдвиг отключён',
  group_lock: 'переход между группами запрещён',
  unknown_margin_neutral: 'маржа неизвестна: нейтральное значение',
  negative_margin_soft: 'отрицательная маржа: мягкое понижение',
  exact: 'точный запрос: сдвиг отключён',
  mpn: 'парт-номер: сдвиг отключён',
  max_shift: 'сдвиг ограничен потолком',
  group: 'переход между группами запрещён',
  compatibility: 'совместимость неприкосновенна',
  required: 'обязательное условие неприкосновенно',
  epsilon: 'бюджет качества: перестановка откачена',
  epsilon_rollback: 'бюджет качества: перестановка откачена',
  explicit_sort: 'явная сортировка покупателя',
}

export const guardLabel = (g: string) => GUARD_LABELS[g] ?? g

/** Нередактируемые индикаторы защит на экране «Маржа». */
export function guardIndicators(policy: Pick<MarginPolicy, 'max_shift'> | null): string[] {
  const shift = policy?.max_shift ?? 2
  return [
    'Точные запросы защищены',
    `Сдвиг не больше ${formatInt(shift)} ${plural(shift, ['позиции', 'позиций', 'позиций'])}`,
    'Совместимость неприкосновенна',
  ]
}

// ── Затронутые запросы ──

const REASON_RANK: Record<string, number> = { top1: 0, top3: 1, worst: 2 }
export const AFFECTED_REASON_LABELS: Record<string, string> = {
  top1: 'сменился первый результат',
  top3: 'изменилась первая тройка',
  worst: 'среди худших по потере качества',
}

/** Почему запрос в списке: reason сервера или флаги top1_changed / top3_changed. */
export function affectedReason(a: MarginAffected): string {
  if (a.reason) return a.reason
  if (a.top1_changed) return 'top1'
  if (a.top3_changed) return 'top3'
  return 'worst'
}

/** Затронутые запросы: сначала сменившие топ-1, затем топ-3, затем худшие; без повторов. */
export function affectedList(point: MarginSimPoint | null): MarginAffected[] {
  const seen = new Set<string>()
  return [...(point?.affected ?? [])]
    .map((a) => ({ ...a, reason: affectedReason(a) }))
    .sort(
      (a, b) =>
        (REASON_RANK[a.reason ?? ''] ?? 3) - (REASON_RANK[b.reason ?? ''] ?? 3) ||
        (a.ndcg_delta ?? 0) - (b.ndcg_delta ?? 0),
    )
    .filter((a) => (seen.has(a.query) ? false : (seen.add(a.query), true)))
}

const itemId = (i: MarginRankItem) => (typeof i === 'string' ? i : i.product_id)
const itemTitle = (i: MarginRankItem) =>
  typeof i === 'string' ? i : i.title?.trim() || i.product_id

export interface RankRow {
  id: string
  title: string
  pos: number
  /** Позиция в другой колонке; null — в другой колонке нет. */
  otherPos: number | null
  /** up — поднялась в «стало», down — опустилась, same — на месте. */
  move: 'up' | 'down' | 'same' | 'new'
}

/** Колонки «было / стало» с перемещениями: стрелка у переставленных позиций. */
export function rankColumns(a: MarginAffected): { before: RankRow[]; after: RankRow[] } {
  const bIdx = new Map(a.before.map((x, i) => [itemId(x), i + 1]))
  const aIdx = new Map(a.after.map((x, i) => [itemId(x), i + 1]))
  const row = (
    x: MarginRankItem,
    i: number,
    other: Map<string, number>,
    isAfter: boolean,
  ): RankRow => {
    const id = itemId(x)
    const pos = i + 1
    const o = other.get(id) ?? null
    let move: RankRow['move'] = 'new'
    if (o !== null) {
      const from = isAfter ? o : pos
      const to = isAfter ? pos : o
      move = to < from ? 'up' : to > from ? 'down' : 'same'
    }
    return { id, title: itemTitle(x), pos, otherPos: o, move }
  }
  return {
    before: a.before.map((x, i) => row(x, i, aIdx, false)),
    after: a.after.map((x, i) => row(x, i, bIdx, true)),
  }
}

// ── Пример «было / стало» (контур доводки: «Политика ранжирования», один пример) ──

/** Сколько позиций переставлено в первой тройке (up/down/new в «стало»). */
const movedTop3 = (a: MarginAffected) =>
  rankColumns(a)
    .after.slice(0, 3)
    .filter((r) => r.move !== 'same').length

/** true — в первой тройке поменялись местами ровно два соседних товара. */
export function isNeighbourSwap(a: MarginAffected): boolean {
  const moved = rankColumns(a)
    .after.slice(0, 3)
    .filter((r) => r.move !== 'same')
  return (
    moved.length === 2 &&
    moved.every((r) => r.otherPos !== null && Math.abs(r.otherPos - r.pos) === 1)
  )
}

/**
 * Один пример для экрана: запрос, где в первой тройке поменялись местами ровно два близких товара (обмен соседей),
 * иначе — запрос с наименьшим числом перестановок. Запросы, где первая тройка не изменилась, примером не бывают:
 * показывать нечего. Нет подходящих — null. Точные запросы среди затронутых не бывает: их защищает сервер.
 */
export function swapExample(items: MarginAffected[]): MarginAffected | null {
  const changed = items.filter((a) => movedTop3(a) > 0)
  return (
    changed.find(isNeighbourSwap) ??
    [...changed].sort((a, b) => movedTop3(a) - movedTop3(b))[0] ??
    null
  )
}
