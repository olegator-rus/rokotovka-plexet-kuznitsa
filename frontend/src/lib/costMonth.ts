// Блок «Месяц» в «Экономике» (ARCHITECTURE §7): два сценария объёма. Онлайн-LLM и агент —
// из замеров сервера (средняя стоимость LLM на запрос, стоимость исследования агента); сервер и число
// исследований — допущения, их правит человек. Нет замера — строка пустая, итог помечен неполным.
// Без DOM: tests/costMonth.test.ts.

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

/** Ввод человека («7 500», «7500,5») → число ≥ 0; мусор — null. */
export function parseRubInput(raw: string): number | null {
  const s = raw.replace(/[\s ₽]/g, '').replace(',', '.')
  if (!s) return null
  const n = Number(s)
  return Number.isFinite(n) && n >= 0 ? n : null
}
