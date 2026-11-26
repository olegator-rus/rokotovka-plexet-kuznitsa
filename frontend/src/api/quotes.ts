import { http } from './http'
import type { QuoteRequest, QuoteResponse } from './typesP2'

/** POST /api/quotes — запрос КП по спецификации, витрина без входа → 201. */
export const postQuote = (body: QuoteRequest) => http.post<QuoteResponse>('/quotes', body)

/** Запрос КП покупателя в «Мои запросы КП» (GET /api/quotes/mine). */
export interface MyQuote extends QuoteResponse {
  created_at: string
  comment: string
  items: { product_id: string; title: string; quantity: number; price_rub: number | null }[]
}

/** GET /api/quotes/mine — свои запросы КП вошедшего покупателя (клиент — из сессии). */
export const getMyQuotes = () => http.get<{ items: MyQuote[] }>('/quotes/mine')
