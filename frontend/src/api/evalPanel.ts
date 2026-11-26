import { http, type RequestOptions } from './http'
import type { PanelEvaluation } from './typesEvalPanel'

/** GET /api/evaluations?set=panel — прогон панели по префиксам (до / после) и учёт намерений по событиям стенда. */
export const getPanelEvaluation = (opts?: RequestOptions) =>
  http.get<PanelEvaluation>('/evaluations?set=panel', opts)
