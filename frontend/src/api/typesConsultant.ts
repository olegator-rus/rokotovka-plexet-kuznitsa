// Консультант контура консультанта: запрос POST /api/consultant и события ответа
// (NDJSON, по строке на событие). Цены — целые копейки (`…Minor`); цены 0 не бывает, нет цены — поля нет.
import type { GuideOffer } from './typesGuide'

export type PageType = 'home' | 'search' | 'family' | 'product' | 'spec' | 'other'

export interface ConsultantPage {
  type: PageType
  family_id: string | null
  offer_id: string | null
  query: string | null
}

export interface ConsultantSpecItem {
  offer_id: string
  qty: number
}

export interface ConsultantRequest {
  session_id: string
  conversation_id: string
  text: string
  page: ConsultantPage
  spec: ConsultantSpecItem[]
}

export type PriceStatus = 'known' | 'on_request' | 'unknown'
export type Compatibility = 'compatible' | 'incompatible' | 'unverified'

export interface ProductReference {
  offerId: string
  sku: string
  title: string
  vendor: string
  licenseType: string
  quantity?: number
  duration?: string
  unitPriceMinor?: number
  totalPriceMinor?: number
  oldUnitPriceMinor?: number
  priceStatus: PriceStatus
  compatibility: Compatibility
  reason?: string
  sourceIds?: string[]
  productUrl: string
  imageUrl?: string
  offer: GuideOffer
}

export type ProductGroupKind = 'fit' | 'more' | 'unfit'

export interface ProductGroup {
  kind: ProductGroupKind
  title?: string
  items: ProductReference[]
}

export interface SpecEventAttachment {
  type: 'spec_event'
  action: 'added' | 'removed' | 'updated'
  offerId: string
  title: string
  quantity?: number
  totalPriceMinor?: number
  offer?: GuideOffer
}

export type Attachment =
  | { type: 'product_card'; product: ProductReference }
  | { type: 'product_list'; title?: string; groups: ProductGroup[] }
  | {
      type: 'comparison'
      columns: ProductReference[]
      rows: { label: string; values: string[] }[]
    }
  | {
      type: 'estimate'
      lines: { label: string; totalPriceMinor?: number; status: 'known' | 'not_calculated' }[]
      knownTotalMinor: number
      note: string
    }
  | SpecEventAttachment
  | {
      type: 'spec_summary'
      items: { offerId: string; title: string; quantity: number; totalPriceMinor?: number }[]
      totalMinor: number
      count: number
    }
  | { type: 'link'; topic: string; title: string; url: string }
  | { type: 'source'; title: string; url?: string; quote?: string }

export type StatusStep = 'searching' | 'checking' | 'calculating'

export type ConversationEvent =
  | { type: 'status'; step: StatusStep | string }
  | { type: 'message.start'; id: string }
  | { type: 'message.delta'; id: string; text: string }
  | { type: 'message.attachment'; id: string; attachment: Attachment }
  | { type: 'message.complete'; id: string; quickReplies?: string[] }
  | { type: 'message.error'; id: string; reason: string }
