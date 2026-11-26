// Отчёт SearchOps для маркетолога (контур фида): GET /api/agent/report — backend/out/agent_report.go.
import type { AgentVerdict } from './types'

export type ReportSectionCode = 'no_offer' | 'not_found' | 'unknown_words' | 'service'
/** Происхождение запроса: слой тестового потока или живой поиск. */
export type ReportLayerCode = 'customer' | 'site' | 'synthetic' | 'live'

export interface ReportLayer {
  code: ReportLayerCode | string
  label: string
  queries: number
  searches: number
}

export interface ReportFound {
  family_id: string
  title: string
  match_type?: string
  price_state?: string
  target?: string
}

export interface ReportProposal {
  id: string
  status: string
  action: string
  diagnosis: string
  run_id: string
  cluster_id?: string
}

export interface ReportRow {
  query: string
  freq: number
  failures: number
  empty: number
  no_click: number
  reformulated: number
  layers: (ReportLayerCode | string)[]
  origin?: string
  kind: string
  kind_label: string
  evidence: string
  found?: ReportFound
  words?: string[]
  examples?: string[]
  agent?: AgentVerdict
  proposal?: ReportProposal
}

export interface ReportSection {
  code: ReportSectionCode | string
  title: string
  hint: string
  rows: ReportRow[]
  searches: number
  failures: number
}

export interface SearchOpsReport {
  catalog_version: string
  rules_version: number
  days: number
  stream: { label: string; freq_label: string; layers: ReportLayer[] }
  totals: { searches: number; failed_searches: number; queries: number; failed_queries: number }
  sections: ReportSection[]
  pending: ReportRow[]
  checked_at: string
  truncated?: boolean
  /** «Вопросы, на которые подбор не ответил». */
  guide?: ReportGuide
}

export interface ReportGuideRow {
  code: string
  label: string
  question: string
  count: number
  families: { family_id: string; title: string }[]
  /** Причина kb_missing (нет факта в базе знаний): по вопросу создаётся задача владельцу. */
  kb_missing: boolean
  agent?: AgentVerdict
  proposal?: ReportProposal
}

export interface ReportGuide {
  title: string
  hint: string
  cause: { code: string; label: string; owner: string; owner_label: string }
  total: number
  rows: ReportGuideRow[]
}
