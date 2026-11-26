import { http, type RequestOptions } from './http'
import type {
  MarginApplied,
  MarginPolicy,
  MarginSimStatus,
  MarginSimulateResponse,
} from './typesCommerce'

/** GET /api/margin/policy — действующая политика маржи и история версий (versions). */
export const getMarginPolicy = (opts?: RequestOptions) =>
  http.get<MarginPolicy>('/margin/policy', opts)

/**
 * POST /api/margin/simulate — прогон всех положений рычага в песочнице, боевой поиск не меняется.
 * 503 — базовая выдача ещё собирается (прогресс — getSimulateStatus).
 */
export const simulateMargin = (positions: number[], opts?: RequestOptions, set = 'tune') =>
  http.post<MarginSimulateResponse>('/margin/simulate', { set, positions }, opts)

/** GET /api/margin/simulate/status — прогресс сборки базовой выдачи. */
export const getSimulateStatus = (opts?: RequestOptions) =>
  http.get<MarginSimStatus>('/margin/simulate/status', opts)

/** POST /api/margin/policy — применить вес: новая версия политики (за упором — 422, то же — 409). */
export const applyMarginPolicy = (weight: number, comment = '') =>
  http.post<MarginApplied>('/margin/policy', { weight, comment })

/** POST /api/margin/policy/rollback — новая версия = копия выбранной (версия 0 — исходная). */
export const rollbackMarginPolicy = (toVersion: number, comment = '') =>
  http.post<MarginApplied>('/margin/policy/rollback', { to_version: toVersion, comment })
