import { http } from './http'
import type { QuoteRequest, QuoteResponse } from './typesP2'

/** POST /api/quotes — запрос КП по спецификации, витрина без входа → 201. */
export const postQuote = (body: QuoteRequest) => http.post<QuoteResponse>('/quotes', body)
