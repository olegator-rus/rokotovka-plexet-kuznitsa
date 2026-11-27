import { http, type RequestOptions } from './http'
import type { TzDocument, TzOperation, TzUploadResponse } from './typesTz'

/** POST /api/tz — multipart, одно поле с файлом (TZ-CORE.md §2). */
export function postTz(file: File, opts?: RequestOptions) {
  const form = new FormData()
  form.append('file', file, file.name)
  return http.post<TzUploadResponse>('/tz', form, opts)
}

/** GET /api/tz/operations/:id — состояние разбора. */
export const getTzOperation = (id: string, opts?: RequestOptions) =>
  http.get<TzOperation>(`/tz/operations/${encodeURIComponent(id)}`, opts)

/** DELETE /api/tz/operations/:id — отмена разбора. */
export const deleteTzOperation = (id: string) =>
  http.del(`/tz/operations/${encodeURIComponent(id)}`)

/** GET /api/tz/:hash — позиции разобранного ТЗ; повторное открытие разбор не запускает. */
export const getTzDocument = (hash: string, opts?: RequestOptions) =>
  http.get<TzDocument>(`/tz/${encodeURIComponent(hash)}`, opts)
