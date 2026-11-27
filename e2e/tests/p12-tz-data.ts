// Заглушка GET /api/tz/:hash по восьми позициям тестового ТЗ (TZ-CORE.md §2) — для p12-tz.spec.ts.
export const TZ_HASH = "h0123456789abcdef";

export const TZ_DOC = {
  "hash": "h0123456789abcdef",
  "file_name": "TZ_Severny_veter.pdf",
  "extracted_count": 8,
  "processed_count": 8,
  "positions": [
    {
      "id": "p1",
      "short_title": "Kaspersky Endpoint Security",
      "kind": "product",
      "named": true,
      "attributes": [
        {
          "key": "quantity",
          "label": "Количество",
          "value": "50 узлов"
        },
        {
          "key": "duration",
          "label": "Срок",
          "value": "1 год"
        },
        {
          "key": "deal",
          "label": "Тип",
          "value": "Новая лицензия"
        }
      ],
      "purchase": {
        "deal": "new",
        "quantity": 50,
        "metric": "device",
        "duration_months": 12
      },
      "search_query": "Kaspersky Endpoint Security для бизнеса Стандартный",
      "filters": [
        {
          "key": "license_quantity",
          "op": "eq",
          "value": "50"
        },
        {
          "key": "duration_months",
          "op": "eq",
          "value": "12"
        }
      ],
      "source_quote": "Лицензия должна обеспечивать защиту 50 (пятидесяти) узлов…",
      "unverified_requirements": [],
      "number": 1,
      "title": "Kaspersky Endpoint Security для бизнеса – Стандартный, новая лицензия на 1 год"
    },
    {
      "id": "p2",
      "short_title": "Astra Linux «Орёл»",
      "kind": "product",
      "named": true,
      "attributes": [],
      "purchase": {
        "deal": "new",
        "quantity": 50,
        "duration_months": 12
      },
      "search_query": "Astra Linux Special Edition Орёл",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [],
      "number": 2,
      "title": "ОС Астра Линукс Special Edition, уровень защищённости «Орёл»"
    },
    {
      "id": "p3",
      "short_title": "Резервное копирование",
      "kind": "product",
      "named": false,
      "attributes": [],
      "purchase": null,
      "search_query": "резервное копирование",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [
        "включение в Единый реестр российских программ"
      ],
      "number": 3,
      "title": "Программное обеспечение резервного копирования (или эквивалент)"
    },
    {
      "id": "p4",
      "short_title": "КриптоПро CSP",
      "kind": "product",
      "named": true,
      "attributes": [],
      "purchase": {
        "quantity": 2
      },
      "search_query": "КриптоПро CSP 5.0 серверная",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [],
      "number": 4,
      "title": "КриптоПро CSP 5.0 серверная лицензия"
    },
    {
      "id": "p5",
      "short_title": "Облачный офисный пакет",
      "kind": "product",
      "named": false,
      "attributes": [],
      "purchase": {
        "quantity": 50,
        "metric": "user"
      },
      "search_query": "облачный офисный пакет",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [],
      "number": 5,
      "title": "Облачный офисный пакет"
    },
    {
      "id": "p6",
      "short_title": "Межсетевой экран нового поколе",
      "kind": "product",
      "named": false,
      "attributes": [],
      "purchase": null,
      "search_query": "межсетевой экран",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [
        "сертификат ФСТЭК"
      ],
      "number": 6,
      "title": "Межсетевой экран нового поколения"
    },
    {
      "id": "p7",
      "short_title": "Сервер стоечный 2U",
      "kind": "product",
      "named": false,
      "attributes": [],
      "purchase": {
        "quantity": 2
      },
      "search_query": "сервер стоечный 2U",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [],
      "number": 7,
      "title": "Сервер стоечный 2U"
    },
    {
      "id": "p8",
      "short_title": "Работы по монтажу и пусконалад",
      "kind": "service",
      "named": false,
      "attributes": [],
      "purchase": null,
      "search_query": "монтаж и пусконаладка серверного оборудования",
      "filters": [],
      "source_quote": "",
      "unverified_requirements": [],
      "number": 8,
      "title": "Работы по монтажу и пусконаладке серверного оборудования"
    }
  ]
};
