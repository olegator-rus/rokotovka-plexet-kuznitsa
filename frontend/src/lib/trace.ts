// Подписи и сводки трассы. Без DOM: tests/trace.test.ts.
import type { ParseSource, Rejected, Route, Trace, TraceItem, TraceStep } from '@/api/types'
import { formatDecimal, formatInt, plural } from './format.ts'
import { MARGIN_STEP_FIELDS, marginSummary } from './traceMargin.ts'
import {
  companionsSummary,
  hardwareSummary,
  isP2Field,
  p2HasDetails,
  solutionSummary,
  upgradeSummary,
} from './traceP2.ts'

export const ROUTE_LABELS: Record<Route, string> = {
  A: 'точный',
  B: 'нормализованный',
  C: 'семантический',
  D: 'с LLM',
}

export const PARSE_SOURCE_LABELS: Record<ParseSource, string> = {
  rules: 'правила',
  cache: 'кэш',
  llm: 'LLM',
}

/** Шаги в порядке конвейера; неизвестный шаг показывается под своим именем. */
export const STEP_LABELS: Record<string, string> = {
  normalize: 'Нормализация',
  attributes: 'Атрибуты',
  route: 'Маршрут',
  intent: 'Намерение (LLM)',
  bm25: 'Текстовый поиск BM25',
  dense: 'Векторный поиск',
  fusion: 'Слияние RRF',
  rerank: 'Реранкер',
  business: 'Бизнес-слой',
  relaxation: 'Ослабление условий',
  margin: 'Маржа',
  hardware: 'Оборудование',
  companions: 'Компаньоны',
  upgrade: 'Старшая редакция',
  solution: 'Решение по шаблону',
}

export const CHANGE_KIND_LABELS: Record<string, string> = {
  layout: 'раскладка',
  typo: 'опечатка',
  alias: 'алиас',
  clean: 'очистка',
  sku: 'артикул',
}

export const REJECT_REASON_LABELS: Record<string, string> = {
  license_metric_mismatch: 'не та метрика лицензии',
  below_threshold: 'ниже порога',
  version_mismatch: 'не та версия',
  license_type_mismatch: 'не тот тип лицензии',
  license_quantity_mismatch: 'не то количество',
  origin_mismatch: 'не отечественный',
  vendor_mismatch: 'не тот вендор',
  category_mismatch: 'не та категория',
  dense_score_too_low: 'слабое сходство по смыслу',
  reference_product: 'сам продукт-референс',
}

/** Причина пропуска шага (поле `reason`, сервер присылает его сверх контракта). */
export const SKIP_REASON_LABELS: Record<string, string> = {
  not_implemented: 'ещё не реализован',
  not_in_config: 'не входит в конфигурацию',
  not_configured: 'не настроен на сервере',
  llm_off: 'режим без LLM',
  not_needed: 'не понадобился',
  disabled: 'выключен в настройках',
  not_route_d: 'запрос не на маршруте D',
  reference_analogs: 'выключен на запросах «аналог X»',
  parser_not_connected: 'разбор LLM не подключён',
  // Почему разбор намерения остался на правилах (Intent.fallback_reason → reason шага intent).
  llm_not_configured: 'модель не подключена',
  llm_error: 'ошибка модели',
  schema_error: 'ответ модели не прошёл проверку',
  catalog_not_loaded: 'каталог не загружен',
  budget_exceeded: 'дневной бюджет LLM исчерпан',
  ip_quota: 'суточная квота LLM для адреса исчерпана',
}

/** Почему выбран маршрут: сигналы шага route. */
export const SIGNAL_LABELS: Record<string, string> = {
  unknown_words: 'незнакомые слова',
  intent_words: 'слова задачи («аналог», «для»)',
  long_query: 'длинный запрос',
  non_product: 'не про товар',
  conflicting_attributes: 'условия противоречат',
  layout_hit: 'раскладка',
  typo_hit: 'опечатка',
  alias_hit: 'сленг или алиас',
  attributes: 'условия лицензии',
}

export const signalLabel = (s: string) => SIGNAL_LABELS[s] ?? s

export const WEIGHT_LABELS: Record<string, string> = {
  relevance: 'релевантность',
  popularity: 'популярность',
  margin: 'маржа',
  context: 'контекст',
}

/** Поля шага, которые панель показывает своими блоками; остальные — списком «ключ: значение». */
export const KNOWN_STEP_FIELDS = new Set([
  'name',
  'ms',
  'skipped',
  'error',
  'changes',
  'found',
  'unresolved',
  'route',
  'signals',
  'top',
  'k',
  'weights',
  'ladder',
  'reason',
])

export function isRoute(v: unknown): v is Route {
  return v === 'A' || v === 'B' || v === 'C' || v === 'D'
}

export function routeLabel(route: string | null | undefined): string {
  return isRoute(route) ? `${route} · ${ROUTE_LABELS[route]}` : 'маршрут не указан'
}

export function parseSourceLabel(source: string | null | undefined): string {
  if (!source) return 'не указан'
  return PARSE_SOURCE_LABELS[source as ParseSource] ?? source
}

export function stepLabel(name: string): string {
  return STEP_LABELS[name] ?? name
}

export function rejectReasonLabel(reason: string): string {
  return REJECT_REASON_LABELS[reason] ?? reason
}

export function changeKindLabel(kind: string): string {
  return CHANGE_KIND_LABELS[kind] ?? kind
}

export function skipReasonLabel(reason: unknown): string {
  if (typeof reason !== 'string' || !reason) return ''
  return SKIP_REASON_LABELS[reason] ?? reason
}

/** Короткий итог шага одной строкой под названием. Пропущенный — «пропущен». */
export function stepSummary(step: TraceStep): string {
  if (step.skipped) {
    const why = skipReasonLabel(step.reason)
    return why ? `пропущен · ${why}` : 'пропущен'
  }
  if (step.error) return 'ошибка, запасной путь'
  switch (step.name) {
    case 'intent': {
      const src =
        typeof step.source === 'string' && step.source ? parseSourceLabel(step.source) : ''
      const why = skipReasonLabel(step.reason)
      // ПДн (телефон, почта, ИНН…) заменены метками до кэша и модели — pii_masked шага (сервер, pii.go).
      const pii =
        typeof step.pii_masked === 'number' && step.pii_masked > 0
          ? ` · замаскировано: ${step.pii_masked}`
          : ''
      if (why) return `${src || 'правила'} · ${why}${pii}`
      return `${src || 'выполнено'}${pii}`
    }
    case 'normalize': {
      const n = step.changes?.length ?? 0
      return n === 0 ? 'без изменений' : `${n} ${plural(n, ['замена', 'замены', 'замен'])}`
    }
    case 'attributes': {
      const n = step.found?.length ?? 0
      const u = step.unresolved?.length ?? 0
      const head = n === 0 ? 'условий нет' : `${n} ${plural(n, ['условие', 'условия', 'условий'])}`
      return u > 0 ? `${head}, ${u} не разрешено` : head
    }
    case 'route': {
      const signals = step.signals?.length ? ` · ${step.signals.map(signalLabel).join(', ')}` : ''
      return `${step.route ?? '—'}${signals}`
    }
    case 'bm25':
    case 'dense':
    case 'rerank': {
      const n = step.top?.length ?? 0
      return `${formatInt(n)} ${plural(n, ['кандидат', 'кандидата', 'кандидатов'])}`
    }
    case 'fusion':
      return typeof step.k === 'number' ? `k = ${step.k}` : 'выполнено'
    case 'business': {
      const w = step.weights ?? {}
      const parts = Object.entries(w)
        .filter(([, v]) => typeof v === 'number')
        .map(([k, v]) => `${WEIGHT_LABELS[k] ?? k} ${formatDecimal(v as number, 2)}`)
      return parts.length ? parts.join(' · ') : 'выполнено'
    }
    case 'relaxation': {
      const n = step.ladder?.length ?? 0
      return n === 0 ? 'не понадобилось' : `${n} ${plural(n, ['ступень', 'ступени', 'ступеней'])}`
    }
    case 'margin':
      return marginSummary(step)
    case 'hardware':
      return hardwareSummary(step)
    case 'companions':
      return companionsSummary(step)
    case 'solution':
      return solutionSummary(step)
    case 'upgrade':
      return upgradeSummary(step)
    default:
      return 'выполнено'
  }
}

const arrLen = (v: unknown) => (Array.isArray(v) ? v.length : 0)

/** Есть ли у шага что раскрывать. */
export function stepHasDetails(step: TraceStep): boolean {
  if (step.error) return true
  if (step.changes?.length || step.found?.length || step.unresolved?.length) return true
  if (step.signals?.length || step.top?.length || step.ladder?.length) return true
  if (step.weights && Object.keys(step.weights).length) return true
  if (step.name === 'margin' && (arrLen(step.shifts) || arrLen(step.guards))) return true
  if (p2HasDetails(step)) return true
  return stepExtraFields(step).length > 0
}

/** Поля шага вне известных — показываются как есть, чтобы новое поле сервера не терялось. */
export function stepExtraFields(step: TraceStep): [string, unknown][] {
  return Object.entries(step).filter(
    ([k, v]) =>
      !KNOWN_STEP_FIELDS.has(k) &&
      !(step.name === 'margin' && MARGIN_STEP_FIELDS.includes(k)) &&
      !isP2Field(step, k) &&
      v !== undefined &&
      v !== null,
  )
}

export function traceItemFor(trace: Trace | null | undefined, productId: string | null) {
  if (!trace || !productId) return null
  return (trace.items ?? []).find((i: TraceItem) => i.product_id === productId) ?? null
}

/** Позиция товара в списке кандидатов: «№ 3» или «нет в списке». */
export function rankLabel(rank: number | null | undefined): string {
  return typeof rank === 'number' && rank > 0 ? `№ ${rank}` : 'нет в списке'
}

export function rejectedLabel(r: Rejected): string {
  const reason = rejectReasonLabel(r.reason)
  return r.detail ? `${reason}: ${r.detail}` : reason
}

/** Сырые данные трассы для «Скопировать JSON». */
export function traceJson(trace: Trace): string {
  return JSON.stringify(trace, null, 2)
}

/** Почему разбор остался на правилах (бюджет, квота адреса, режим без LLM…) — для сводки трассы. */
export function intentFallback(trace: Pick<Trace, 'steps'> | null | undefined): string | null {
  const step = (trace?.steps ?? []).find((s) => s.name === 'intent')
  if (!step || step.skipped || typeof step.reason !== 'string' || !step.reason) return null
  return skipReasonLabel(step.reason)
}
