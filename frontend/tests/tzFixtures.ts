// Заглушка ответа GET /api/tz/:hash по восьми позициям тестового ТЗ (TZ-CORE.md §2). Только для тестов.
import type { TzDocument, TzPosition } from '../src/api/typesTz.ts'
import type { SearchItem } from '../src/api/types.ts'

const pos = (over: Partial<TzPosition> & Pick<TzPosition, 'number' | 'title'>): TzPosition => ({
  id: `p${over.number}`,
  short_title: over.title.slice(0, 30),
  kind: 'product',
  named: true,
  attributes: [],
  purchase: null,
  search_query: over.title,
  filters: [],
  source_quote: '',
  unverified_requirements: [],
  ...over,
})

export const TZ_DOC: TzDocument = {
  hash: 'h0123456789abcdef',
  file_name: 'TZ_Severny_veter.pdf',
  extracted_count: 8,
  processed_count: 8,
  positions: [
    pos({
      number: 1,
      title: 'Kaspersky Endpoint Security для бизнеса – Стандартный, новая лицензия на 1 год',
      short_title: 'Kaspersky Endpoint Security',
      attributes: [
        { key: 'quantity', label: 'Количество', value: '50 узлов' },
        { key: 'duration', label: 'Срок', value: '1 год' },
        { key: 'deal', label: 'Тип', value: 'Новая лицензия' },
      ],
      purchase: { deal: 'new', quantity: 50, metric: 'device', duration_months: 12 },
      search_query: 'Kaspersky Endpoint Security для бизнеса Стандартный',
      filters: [
        { key: 'license_quantity', op: 'eq', value: '50' },
        { key: 'duration_months', op: 'eq', value: '12' },
      ],
      source_quote: 'Лицензия должна обеспечивать защиту 50 (пятидесяти) узлов…',
    }),
    pos({
      number: 2,
      title: 'ОС Астра Линукс Special Edition, уровень защищённости «Орёл»',
      short_title: 'Astra Linux «Орёл»',
      purchase: { deal: 'new', quantity: 50, duration_months: 12 },
      search_query: 'Astra Linux Special Edition Орёл',
    }),
    pos({
      number: 3,
      title: 'Программное обеспечение резервного копирования (или эквивалент)',
      short_title: 'Резервное копирование',
      named: false,
      search_query: 'резервное копирование',
      unverified_requirements: ['включение в Единый реестр российских программ'],
    }),
    pos({
      number: 4,
      title: 'КриптоПро CSP 5.0 серверная лицензия',
      short_title: 'КриптоПро CSP',
      purchase: { quantity: 2 },
      search_query: 'КриптоПро CSP 5.0 серверная',
    }),
    pos({
      number: 5,
      title: 'Облачный офисный пакет',
      named: false,
      purchase: { quantity: 50, metric: 'user' },
      search_query: 'облачный офисный пакет',
    }),
    pos({
      number: 6,
      title: 'Межсетевой экран нового поколения',
      named: false,
      search_query: 'межсетевой экран',
      unverified_requirements: ['сертификат ФСТЭК'],
    }),
    pos({
      number: 7,
      title: 'Сервер стоечный 2U',
      named: false,
      purchase: { quantity: 2 },
      search_query: 'сервер стоечный 2U',
    }),
    pos({
      number: 8,
      title: 'Работы по монтажу и пусконаладке серверного оборудования',
      kind: 'service',
      named: false,
      search_query: 'монтаж и пусконаладка серверного оборудования',
    }),
  ],
}

export const item = (id: string, over: Partial<SearchItem> = {}): SearchItem => ({
  product_id: id,
  family_id: `f-${id}`,
  title: id,
  vendor: 'Vendor',
  category: 'ПО',
  license_line: '',
  price_rub: 1000,
  price_on_request: false,
  group: 'exact',
  matches_query: true,
  purchased_before: false,
  final: 0.9,
  variants: [],
  ...over,
})
