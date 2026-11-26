import { http } from './http'
import type { ProductCompanions } from './typesP2'

/** GET /api/companions — компаньоны к позиции спецификации («К этой позиции»), витрина без входа. */
export const getProductCompanions = (productId: string, quantity: number, signal?: AbortSignal) =>
  http.get<ProductCompanions>(
    `/companions?product_id=${encodeURIComponent(productId)}&quantity=${Math.max(0, Math.round(quantity))}`,
    { signal },
  )
