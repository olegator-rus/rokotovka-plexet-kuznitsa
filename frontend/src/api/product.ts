import { http, type RequestOptions } from './http'
import type { ProductPage } from './typesProduct'

/** GET /api/products/{id} — страница товара, витрина без входа. */
export const getProduct = (id: string, opts?: RequestOptions) =>
  http.get<ProductPage>(`/products/${encodeURIComponent(id)}`, opts)
