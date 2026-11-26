// Страница товара: ответ GET /api/products/{id}.
import type { Reestr } from './types'
import type { Companion } from './typesP2'

export interface ProductVendor {
  id: string
  name: string
  /** Происхождение позиции: ru | foreign. */
  origin: string
}

export interface ProductCategory {
  id: string
  name: string
}

export interface ProductLicense {
  type: string
  metric: string | null
  quantity: number | null
  /** null — бессрочная. */
  duration_months: number | null
  edition: string
}

/** Атрибут с подписью сервера; значение — строкой. */
export interface ProductAttr {
  key: string
  label: string
  value: string
}

/** Числовая характеристика оборудования. */
export interface ProductSpec {
  key: string
  label: string
  value: number
  unit: string
}

/** Другая позиция того же семейства. */
export interface ProductVariant {
  product_id: string
  title: string
  license_line: string
  price_rub: number | null
  price_on_request: boolean
  price_mode: string
}

export interface ProductCompat {
  target_model: string
  state: 'verified' | 'conditional' | 'incompatible' | 'unknown' | string
  condition: string
  source_url: string
}

/** Аналог по связи substitute от семейства. */
export interface ProductAnalog {
  family_id: string
  family_name: string
  product_id: string
  title: string
  vendor: string
  license_line: string
  price_rub: number | null
  price_on_request: boolean
  price_mode: string
  note: string
}

export interface ProductPage {
  catalog_version: string
  product_id: string
  /** Пусто — артикул не указан. */
  sku: string
  /** Парт-номер оборудования; пусто у ПО. */
  mpn: string
  kind: 'software' | 'hardware' | string
  family_id: string
  family_name: string
  title: string
  vendor: ProductVendor
  category: ProductCategory
  description: string
  license_line: string
  license: ProductLicense
  price_rub: number | null
  price_on_request: boolean
  price_mode: string
  origin: string
  /** null у оборудования. */
  reestr: Reestr | null
  attributes: ProductAttr[]
  /** [] у ПО. */
  specs: ProductSpec[]
  variants: ProductVariant[]
  compatibility: ProductCompat[]
  companions: { required: Companion[]; items: Companion[] }
  analogs: ProductAnalog[]
}
