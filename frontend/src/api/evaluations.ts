import { http, type RequestOptions } from './http'
import type { EvalJob, EvalRunRequest, EvaluationsResponse } from './types'

/**
 * GET /api/evaluations — последний прогон по каждой конфигурации, ухудшения, воспроизводимость.
 * set — набор (по умолчанию test).
 */
export const getEvaluations = (opts?: RequestOptions, set?: string) =>
  http.get<EvaluationsResponse>(
    set ? `/evaluations?${new URLSearchParams({ set }).toString()}` : '/evaluations',
    opts,
  )

/** POST /api/evaluations/run — запуск прогона, асинхронно. */
export const runEvaluation = (body: EvalRunRequest, opts?: RequestOptions) =>
  http.post<EvalJob>('/evaluations/run', body, opts)
