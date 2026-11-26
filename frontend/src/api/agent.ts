import { http, type RequestOptions } from './http'
import type {
  AgentRunDetail,
  AgentRunRequest,
  AgentRunsResponse,
  ApproveResponse,
  ConfigsResponse,
  MemoryResponse,
  Proposal,
  ProposalRejectReason,
  ProposalsResponse,
  RollbackResponse,
} from './types'
import type { AgentSettings, MemoryLayerId, MemoryLayerResponse } from './typesAgent'
import type { SearchOpsReport } from './typesReport'

/** GET /api/agent/runs — прошлые запуски. */
export const getRuns = (opts?: RequestOptions) => http.get<AgentRunsResponse>('/agent/runs', opts)

/** POST /api/agent/runs → 202 {run_id}. */
export const startRun = (body: AgentRunRequest, opts?: RequestOptions) =>
  http.post<{ run_id: string }>('/agent/runs', body, opts)

/** GET /api/agent/runs/{id} — запуск и все его шаги. */
export const getRun = (id: string, opts?: RequestOptions) =>
  http.get<AgentRunDetail>(`/agent/runs/${encodeURIComponent(id)}`, opts)

/** POST /api/agent/runs/{id}/stop → 202. */
export const stopRun = (id: string) => http.post<void>(`/agent/runs/${encodeURIComponent(id)}/stop`)

/** Адрес SSE-ленты запуска (EventSource ходит мимо http.ts: это поток, а не JSON). */
export const runStreamUrl = (id: string) => `/api/agent/runs/${encodeURIComponent(id)}/stream`

/** GET /api/proposals?status=&role= */
export function getProposals(
  filter: { status?: string; role?: string } = {},
  opts?: RequestOptions,
) {
  const params = new URLSearchParams()
  if (filter.status) params.set('status', filter.status)
  if (filter.role) params.set('role', filter.role)
  const qs = params.toString()
  return http.get<ProposalsResponse>(`/proposals${qs ? `?${qs}` : ''}`, opts)
}

export const getProposal = (id: string, opts?: RequestOptions) =>
  http.get<Proposal>(`/proposals/${encodeURIComponent(id)}`, opts)

/** POST /api/proposals/{id}/approve — правило действует сразу, создаётся новая версия. */
export const approveProposal = (id: string, comment = '') =>
  http.post<ApproveResponse>(`/proposals/${encodeURIComponent(id)}/approve`, { comment })

/** POST /api/proposals/{id}/reject — причина уходит в память агента. */
export const rejectProposal = (id: string, reason: ProposalRejectReason, comment = '') =>
  http.post<{ proposal: Proposal }>(`/proposals/${encodeURIComponent(id)}/reject`, {
    reason,
    comment,
  })

/** GET /api/configs — версии правил, новые сверху. */
export const getConfigs = (opts?: RequestOptions) => http.get<ConfigsResponse>('/configs', opts)

/** POST /api/configs/rollback — новая версия = копия выбранной. */
export const rollbackConfig = (toVersion: number, comment = '') =>
  http.post<RollbackResponse>('/configs/rollback', { to_version: toVersion, comment })

/** GET /api/agent/memory — решения человека, которые агент помнит. */
export const getMemory = (opts?: RequestOptions) => http.get<MemoryResponse>('/agent/memory', opts)

// ── Контур автономии ──

/** GET /api/agent/runs?trigger= — запуски одного вида; без фильтра — все. */
export function getRunsBy(trigger: string | null, opts?: RequestOptions) {
  const qs = trigger ? `?trigger=${encodeURIComponent(trigger)}` : ''
  return http.get<AgentRunsResponse>(`/agent/runs${qs}`, opts)
}

/** POST /api/agent/runs {"query"} — «Разобрать» на одном запросе (триггер manual) → 202 {run_id}. */
export const startQueryRun = (query: string) =>
  http.post<{ run_id: string }>('/agent/runs', { query })

/** GET /api/agent/settings — режим, расписание, пороги триггеров, бюджеты. */
export const getAgentSettings = (opts?: RequestOptions) =>
  http.get<AgentSettings>('/agent/settings', opts)

/** GET /api/agent/memory?layer=decisions|suppress|episodes */
export const getMemoryLayer = (layer: MemoryLayerId, opts?: RequestOptions) =>
  http.get<MemoryLayerResponse>(`/agent/memory?layer=${layer}`, opts)

/** POST /api/agent/memory/suppress/{id}/remove {"reason"} — человек снимает «не предлагать»; это тоже решение. */
export const removeSuppress = (id: string, reason: string) =>
  http.post<unknown>(`/agent/memory/suppress/${encodeURIComponent(id)}/remove`, { reason })

/** GET /api/agent/report?days= — первый экран SearchOps: отчёт для маркетолога (контур фида). */
export const getReport = (days: number, opts?: RequestOptions) =>
  http.get<SearchOpsReport>(`/agent/report?days=${encodeURIComponent(String(days))}`, opts)
