// Блок «Месяц» во вкладке «Стоимость» (ARCHITECTURE §7): два сценария объёма. Онлайн-LLM и агент —
// из замеров сервера (средняя стоимость LLM на запрос, стоимость исследования агента); сервер и число
// исследований — допущения, их правит человек. Нет замера — строка пустая, итог помечен неполным.
// Без DOM: tests/costMonth.test.ts.
import { formatInt } from './format.ts'

export interface MonthInput {
  /** Запросов в месяц. */
  requests: number
  /** Сервер, ₽ в месяц — допущение. */
  serverRub: number
  /** Средняя стоимость онлайн-LLM на один поисковый запрос, ₽ (замер); null — замера нет. */
  llmPerRequestRub: number | null
  /** Стоимость одного исследования агента, ₽ (замер); null — замера нет. */
  agentRunRub: number | null
  /** Исследований агента в месяц — допущение. */
  agentRuns: number
}

export interface MonthResult {
  server: number
  llm: number | null
  agent: number | null
  total: number
  perRequest: number | null
  /** true — в итог не вошла хотя бы одна строка без замера. */
  partial: boolean
}

function clean(n: number): number {
  return Number.isFinite(n) && n > 0 ? n : 0
}

export function monthScenario(input: MonthInput): MonthResult {
  const requests = clean(input.requests)
  const server = clean(input.serverRub)
  const llm = input.llmPerRequestRub === null ? null : clean(input.llmPerRequestRub) * requests
  const agent =
    input.agentRunRub === null ? null : clean(input.agentRunRub) * clean(input.agentRuns)
  const total = server + (llm ?? 0) + (agent ?? 0)
  return {
    server,
    llm,
    agent,
    total,
    perRequest: requests > 0 ? total / requests : null,
    partial: llm === null || agent === null,
  }
}

/** Верхние пределы допущений: сервер — 10 млн ₽ в месяц, запусков агента — 10 000 в месяц (≈ 14 в час). */
export const SERVER_RUB_MAX = 10_000_000
export const AGENT_RUNS_MAX = 10_000

export type AssumptionParse = { ok: true; value: number } | { ok: false; error: string }

/**
 * Ввод допущения человеком: цифры, пробелы-разделители тысяч, одна запятая или точка, «₽» — и только. Знак,
 * hex, экспонента, «Infinity», несколько разделителей, пусто — ошибка с текстом для поля. Запуски — целое.
 */
export function parseAssumption(
  raw: string,
  opts: { max: number; integer?: boolean; unit?: string },
): AssumptionParse {
  const s = raw.replace(/[\s\u00a0\u202f₽]/g, '')
  if (!s) return { ok: false, error: 'Введите число' }
  if (/^[-−–]/.test(s)) return { ok: false, error: 'Не меньше 0' }
  if (!/^\d+([.,]\d+)?$/.test(s)) return { ok: false, error: 'Только цифры, например 7 500' }
  const n = Number(s.replace(',', '.'))
  if (!Number.isFinite(n)) return { ok: false, error: 'Только цифры, например 7 500' }
  if (opts.integer && !Number.isInteger(n)) return { ok: false, error: 'Целое число' }
  if (n > opts.max)
    return {
      ok: false,
      error: `Не больше ${formatInt(opts.max)}${opts.unit ? ` ${opts.unit}` : ''}`,
    }
  return { ok: true, value: n }
}

/** Ввод человека («7 500», «7500,5») → число от 0 до SERVER_RUB_MAX; мусор — null. */
export function parseRubInput(raw: string): number | null {
  const r = parseAssumption(raw, { max: SERVER_RUB_MAX })
  return r.ok ? r.value : null
}

/**
 * Стоимость запуска агента для сценариев месяца: среднее по полным запускам (завершён, разобран хотя бы
 * один кластер) — отладочные и прерванные запуски цифру не занижают. Нет полных — среднее сервера.
 */
export function agentRunCost(
  runs: readonly { status: string; clusters_done: number; cost_rub: number }[],
  fallback: number | null,
): { rub: number | null; full: number } {
  const full = runs.filter((r) => r.status === 'done' && r.clusters_done > 0 && r.cost_rub > 0)
  if (!full.length) return { rub: fallback, full: 0 }
  return { rub: full.reduce((n, r) => n + r.cost_rub, 0) / full.length, full: full.length }
}

/** Число запусков из подсказки сервера «Замер — средняя стоимость завершённых запусков агента (11)»; нет — null. */
export function runsInHint(hint: string | null | undefined): number | null {
  const m = /\((\d+)\)\s*$/.exec(hint ?? '')
  return m ? Number(m[1]) : null
}
