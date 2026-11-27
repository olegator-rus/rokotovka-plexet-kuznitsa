// Страница товара: ответ GET /api/products/{id}.
import type { Reestr } from './types'
import type { Companion, Promo } from './typesP2'

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
  /** null — срок не задан; «бессрочная» — только при type = perpetual. */
  duration_months: number | null
  edition: string
  /** вид позиции фида (base | renewal | upgrade | migration | support | module | course | other); в mock нет. */
  kind?: string
  /** commercial | academic | government | nonprofit | home; в mock нет. */
  org_type?: string
  /** Для кого вариант («Образовательное учреждение»); нет — без ограничения. */
  org_label?: string
  /** ступень количества — price_rub за одно место. */
  tier_min?: number | null
  tier_max?: number | null
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
  /** Условия варианта: по ним страница показывает, чем варианты различаются. */
  license: ProductLicense
  price_rub: number | null
  /** Цена до скидки — только если выше price_rub. */
  old_price_rub?: number
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
  /** Цена до скидки по фиду — только если выше price_rub (как зачёркнутая цена в выдаче). */
  old_price_rub?: number
  /** Действующие акции семейства, как на карточке выдачи; [] — акций нет. */
  promos: Promo[]
  price_on_request: boolean
  price_mode: string
  /** Карточка позиции в магазине («На сайте ↗»); пусто — адрес не собран. */
  offer_url?: string
  /** Картинка позиции из каталога заказчика; нет — значок производителя. */
  image_url?: string
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
