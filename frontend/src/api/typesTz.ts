// Поиск по ТЗ (контур ТЗ): типы API.
import type { SearchFilter } from './typesP2'

/** POST /api/tz → операция разбора. */
export interface TzUploadResponse {
  operation_id: string
  /** Проверка формата и размера до разбора: отказ может прийти сразу, без опроса. */
  status?: 'processing' | 'error' | string
  reason?: string
}

/** Этап разбора — только то, что сервер действительно делает; ocr — только для скана. */
export type TzStage = 'reading' | 'ocr' | string

export type TzErrorReason =
  'format' | 'size' | 'damaged' | 'password' | 'ocr' | 'no_positions' | 'general'

/** GET /api/tz/operations/:id — опрос раз в секунду. */
export type TzOperation =
  | { status: 'processing'; stage?: TzStage }
  | { status: 'completed'; hash: string }
  | { status: 'error'; reason?: TzErrorReason | string }

export interface TzAttribute {
  key: string
  label: string
  value: string
}

/** Условия позиции в формате шага закупки (services/purchase_stage.go). */
export interface TzPurchase {
  deal?: string
  quantity?: number
  metric?: string
  duration_months?: number
  org_type?: string
  edition?: string
}

export interface TzPosition {
  id: string
  number: number
  title: string
  short_title: string
  kind: 'product' | 'service' | string
  named: boolean
  attributes: TzAttribute[]
  purchase?: TzPurchase | null
  search_query: string
  filters: SearchFilter[]
  source_quote: string
  unverified_requirements: string[]
}

/** GET /api/tz/:hash — позиции без товаров; 404 — хеш неизвестен или истёк. */
export interface TzDocument {
  hash: string
  file_name: string
  extracted_count: number
  processed_count: number
  positions: TzPosition[]
}
