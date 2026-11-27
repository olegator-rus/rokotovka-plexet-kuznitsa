// Ответы POST /api/guide для заглушек помощника (контур помощника): записаны с живого API ядра на фиде магазина
// (сценарии 1, 2); поля сокращены до тех, что читает фронт.
// Пересобрать: тот же запрос к своему экземпляру API. E2E_GUIDE_LIVE=1 — заглушки не ставятся.
/* eslint-disable */
export const REPLIES: Record<'s1' | 's2a' | 's2b' | 's3' | 'pay', Record<string, unknown>> = {
 "s2a": {
  "type": "comparison",
  "lead": "В редакции «Расширенный» есть адаптивный контроль аномалий, шифрование данных и патч-менеджмент (установка обновлений программ).",
  "offers": [
   {
    "offer_id": "4985",
    "family_id": "3953",
    "title": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Стандартный",
    "sku": "KL4863RAPFS",
    "tier": "25–49",
    "unit": "за компьютер",
    "quantity": 30,
    "price_per_unit": 2813.4,
    "total": 84402,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4863lapfs-4985/",
    "term": "1 год",
    "role": "match",
    "display": {
     "title": "Kaspersky Endpoint Security — Стандартный",
     "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4863RAPFS"
      },
      {
       "label": "Ступень",
       "value": "25–49 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 2813.4,
    "pay_quantity": 30,
    "price_line": "30 × 2 813,40 ₽",
    "price_text": "2 813,40 ₽ за компьютер",
    "package": null,
    "reason": null
   },
   {
    "offer_id": "4856",
    "family_id": "3935",
    "title": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Расширенный",
    "sku": "KL4867RAPFS",
    "tier": "25–49",
    "unit": "за компьютер",
    "quantity": 30,
    "price_per_unit": 4081.5,
    "total": 122445,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4867lapfs-4856/",
    "term": "1 год",
    "role": "match",
    "display": {
     "title": "Kaspersky Endpoint Security — Расширенный",
     "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4867RAPFS"
      },
      {
       "label": "Ступень",
       "value": "25–49 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 4081.5,
    "pay_quantity": 30,
    "price_line": "30 × 4 081,50 ₽",
    "price_text": "4 081,50 ₽ за компьютер",
    "package": null,
    "reason": null
   },
   {
    "offer_id": "503515",
    "family_id": "419620",
    "title": "Kaspersky Endpoint Security для бизнеса Расширенный Плюс лицензия Base на 1 год по количеству узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Расширенный Плюс",
    "sku": "KL4210RAPFS",
    "tier": "25–49",
    "unit": "за компьютер",
    "quantity": 30,
    "price_per_unit": 6462,
    "total": 193860,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4210rapfs-503515/",
    "term": "1 год",
    "role": "alternative",
    "note": "Другая редакция линейки",
    "display": {
     "title": "Kaspersky Endpoint Security — Расширенный Плюс",
     "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4210RAPFS"
      },
      {
       "label": "Ступень",
       "value": "25–49 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Расширенный Плюс лицензия Base на 1 год по количеству узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 6462,
    "pay_quantity": 30,
    "price_line": "30 × 6 462 ₽",
    "price_text": "6 462 ₽ за компьютер",
    "package": null,
    "reason": {
     "code": "other_conditions",
     "text": "Другая редакция линейки"
    }
   },
   {
    "offer_id": "222174",
    "family_id": "4052",
    "title": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов",
    "family_title": "Kaspersky Endpoint Security Cloud",
    "sku": "KL4742RAPFS",
    "tier": "25–49",
    "unit": "за компьютер",
    "quantity": 30,
    "price_per_unit": 2640.6,
    "total": 79218,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4742rapfs-222174/",
    "term": "1 год",
    "role": "alternative",
    "note": "Другой продукт Kaspersky; состав функций в документации не подтверждён",
    "display": {
     "title": "Kaspersky Endpoint Security Cloud",
     "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4742RAPFS"
      },
      {
       "label": "Ступень",
       "value": "25–49 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 2640.6,
    "pay_quantity": 30,
    "price_line": "30 × 2 640,60 ₽",
    "price_text": "2 640,60 ₽ за компьютер",
    "package": null,
    "reason": {
     "code": "other_conditions",
     "text": "Другой продукт Kaspersky; состав функций в документации не подтверждён"
    }
   }
  ],
  "conditions": [
   {
    "key": "vendor",
    "label": "Kaspersky",
    "value": "kaspersky",
    "origin": "user"
   },
   {
    "key": "count:workstation",
    "label": "30 ПК",
    "value": "30",
    "origin": "user"
   },
   {
    "key": "operation",
    "label": "новая лицензия",
    "value": "new",
    "origin": "default"
   },
   {
    "key": "term_months",
    "label": "1 год",
    "value": "12",
    "origin": "default"
   }
  ],
  "chat": {
   "status": "answer",
   "text": [
    "В редакции «Расширенный» есть адаптивный контроль аномалий, шифрование данных и патч-менеджмент (установка обновлений программ).",
    "Стоимость посчитана на 30 компьютеров на один год."
   ],
   "warning": "",
   "comparison": {
    "title": "Различия редакций",
    "columns": [
     {
      "offer_id": "4985",
      "family_id": "3953",
      "title": "Стандартный"
     },
     {
      "offer_id": "4856",
      "family_id": "3935",
      "title": "Расширенный"
     }
    ],
    "rows": [
     {
      "key": "encryption",
      "label": "Шифрование данных",
      "kind": "feature",
      "cells": [
       {
        "mark": "no",
        "text": "Нет",
        "fact_id": "kb_kes_std_encryption"
       },
       {
        "mark": "yes",
        "text": "Есть",
        "fact_id": "kb_kes_adv_encryption"
       }
      ]
     },
     {
      "key": "patch_management",
      "label": "Патч-менеджмент (установка обновлений программ)",
      "kind": "feature",
      "cells": [
       {
        "mark": "no",
        "text": "Нет",
        "fact_id": "kb_kes_std_patch"
       },
       {
        "mark": "yes",
        "text": "Есть",
        "fact_id": "kb_kes_adv_patch"
       }
      ]
     },
     {
      "key": "adaptive_anomaly_control",
      "label": "Адаптивный контроль аномалий",
      "kind": "feature",
      "cells": [
       {
        "mark": "no",
        "text": "Нет",
        "fact_id": "kb_kes_std_anomaly"
       },
       {
        "mark": "yes",
        "text": "Есть",
        "fact_id": "kb_kes_adv_anomaly"
       }
      ]
     },
     {
      "key": "unit_price",
      "label": "За место",
      "kind": "commercial",
      "cells": [
       {
        "mark": "value",
        "text": "2 813,40 ₽"
       },
       {
        "mark": "value",
        "text": "4 081,50 ₽"
       }
      ]
     },
     {
      "key": "total",
      "label": "Итого",
      "kind": "commercial",
      "cells": [
       {
        "mark": "value",
        "text": "84 402 ₽"
       },
       {
        "mark": "value",
        "text": "122 445 ₽"
       }
      ]
     }
    ],
    "source": {
     "label": "Документация Kaspersky",
     "fragment": "Встроенное шифрование | функция недоступна | функция доступна | функция доступна",
     "url": "https://www.kaspersky.ru/small-to-medium-business-security/endpoint-select"
    }
   },
   "groups": [
    {
     "key": "match",
     "title": "",
     "cards": [
      {
       "offer_id": "4985",
       "family_id": "3953",
       "badge": null,
       "title": "Kaspersky Endpoint Security — Стандартный",
       "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
       "price_text": "84 402 ₽",
       "total": 84402,
       "calc": "30 × 2 813,40 ₽",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4863lapfs-4985/",
       "quantity": 30,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4863RAPFS"
        },
        {
         "label": "Ступень",
         "value": "25–49 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
        }
       ]
      },
      {
       "offer_id": "4856",
       "family_id": "3935",
       "badge": null,
       "title": "Kaspersky Endpoint Security — Расширенный",
       "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
       "price_text": "122 445 ₽",
       "total": 122445,
       "calc": "30 × 4 081,50 ₽",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4867lapfs-4856/",
       "quantity": 30,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4867RAPFS"
        },
        {
         "label": "Ступень",
         "value": "25–49 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов"
        }
       ]
      }
     ]
    },
    {
     "key": "other",
     "title": "Другие варианты",
     "cards": [
      {
       "offer_id": "503515",
       "family_id": "419620",
       "badge": null,
       "title": "Kaspersky Endpoint Security — Расширенный Плюс",
       "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
       "price_text": "193 860 ₽",
       "total": 193860,
       "calc": "30 × 6 462 ₽",
       "reason": "Другая редакция линейки",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4210rapfs-503515/",
       "quantity": 30,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4210RAPFS"
        },
        {
         "label": "Ступень",
         "value": "25–49 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Расширенный Плюс лицензия Base на 1 год по количеству узлов"
        }
       ]
      },
      {
       "offer_id": "222174",
       "family_id": "4052",
       "badge": null,
       "title": "Kaspersky Endpoint Security Cloud",
       "subtitle": "Новая лицензия · 30 компьютеров · 1 год",
       "price_text": "79 218 ₽",
       "total": 79218,
       "calc": "30 × 2 640,60 ₽",
       "reason": "Другой продукт Kaspersky; состав функций в документации не подтверждён",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4742rapfs-222174/",
       "quantity": 30,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4742RAPFS"
        },
        {
         "label": "Ступень",
         "value": "25–49 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов"
        }
       ]
      }
     ]
    }
   ],
   "more": null,
   "sources": [
    {
     "label": "Документация Kaspersky",
     "fragment": "Встроенное шифрование | функция недоступна | функция доступна | функция доступна",
     "url": "https://www.kaspersky.ru/small-to-medium-business-security/endpoint-select"
    }
   ],
   "complements": [],
   "question": null,
   "suggestions": [
    {
     "label": "Нужно шифрование",
     "message": "нужно шифрование"
    },
    {
     "label": "Рассчитать на 50 ПК",
     "message": "на 50 компьютеров"
    },
    {
     "label": "Это продление",
     "message": "это продление",
     "chip": {
      "label": "",
      "key": "operation",
      "value": "renewal"
     }
    }
   ],
   "conditions": {
    "label": "Учитываю: 30 ПК · 1 год · новая",
    "items": [
     "30 ПК",
     "1 год",
     "новая"
    ]
   },
   "links": [],
   "has_promo": false
  },
  "state": {
   "selection_id": "43becf87-7b62-4d97-b0a5-9405b77d4429",
   "intent": "comparison",
   "requirements": {
    "task": "",
    "vendor": "kaspersky",
    "objects": [
     {
      "kind": "workstation",
      "count": 30
     }
    ],
    "platform": "",
    "operation": "new",
    "term_months": 12,
    "org_type": "commercial",
    "required_features": null,
    "use": "",
    "variant": "",
    "cheapest": false
   },
   "origin": {
    "count:workstation": "user",
    "operation": "default",
    "org_type": "default",
    "term_months": "default",
    "vendor": "user"
   },
   "candidate_family_ids": [
    "3953",
    "3935"
   ],
   "selected_offer_ids": [
    "4985",
    "4856"
   ],
   "fact_ids": [
    "kb_kes_adv_anomaly",
    "kb_kes_adv_encryption",
    "kb_kes_adv_patch",
    "kb_kes_adv_sysadmin",
    "kb_kes_adv_app_launch"
   ],
   "unresolved": [],
   "feed_version": "softline-feed",
   "kb_version": "kb-ba021e12bec8",
   "turn": 1,
   "questions": 0,
   "skipped": 0,
   "pending": "",
   "quote_notes": null
  }
 },
 "s2b": {
  "type": "selection",
  "lead": "Расширенная редакция обеспечивает шифрование данных",
  "offers": [
   {
    "offer_id": "4857",
    "family_id": "3935",
    "title": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Расширенный",
    "sku": "KL4867RAQFS",
    "tier": "50–99",
    "unit": "за компьютер",
    "quantity": 50,
    "price_per_unit": 3940.2,
    "total": 197010,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4867laqfs-4857/",
    "term": "1 год",
    "role": "match",
    "display": {
     "title": "Kaspersky Endpoint Security — Расширенный",
     "subtitle": "Новая лицензия · 50 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4867RAQFS"
      },
      {
       "label": "Ступень",
       "value": "50–99 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 3940.2,
    "pay_quantity": 50,
    "price_line": "50 × 3 940,20 ₽",
    "price_text": "3 940,20 ₽ за компьютер",
    "package": null,
    "reason": null
   },
   {
    "offer_id": "4986",
    "family_id": "3953",
    "title": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Стандартный",
    "sku": "KL4863RAQFS",
    "tier": "50–99",
    "unit": "за компьютер",
    "quantity": 50,
    "price_per_unit": 2715.3,
    "total": 135765,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4863laqfs-4986/",
    "term": "1 год",
    "role": "alternative",
    "note": "Стандартный: Без шифрования",
    "display": {
     "title": "Kaspersky Endpoint Security — Стандартный",
     "subtitle": "Новая лицензия · 50 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4863RAQFS"
      },
      {
       "label": "Ступень",
       "value": "50–99 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 2715.3,
    "pay_quantity": 50,
    "price_line": "50 × 2 715,30 ₽",
    "price_text": "2 715,30 ₽ за компьютер",
    "package": null,
    "reason": {
     "code": "missing_feature",
     "text": "Не подходит: нет шифрования данных"
    }
   }
  ],
  "conditions": [
   {
    "key": "vendor",
    "label": "Kaspersky",
    "value": "kaspersky",
    "origin": "user"
   },
   {
    "key": "count:workstation",
    "label": "50 ПК",
    "value": "50",
    "origin": "user"
   },
   {
    "key": "operation",
    "label": "новая лицензия",
    "value": "new",
    "origin": "default"
   },
   {
    "key": "term_months",
    "label": "1 год",
    "value": "12",
    "origin": "default"
   },
   {
    "key": "feature:encryption",
    "label": "нужно шифрование данных",
    "value": "encryption",
    "origin": "user"
   }
  ],
  "chat": {
   "status": "answer",
   "text": [
    "На 50 компьютеров действует другая ценовая ступень.",
    "Расширенная редакция обеспечивает шифрование данных."
   ],
   "warning": "",
   "comparison": null,
   "groups": [
    {
     "key": "match",
     "title": "",
     "cards": [
      {
       "offer_id": "4857",
       "family_id": "3935",
       "badge": {
        "code": "required_feature",
        "text": "Есть нужное шифрование"
       },
       "title": "Kaspersky Endpoint Security — Расширенный",
       "subtitle": "Новая лицензия · 50 компьютеров · 1 год",
       "price_text": "197 010 ₽",
       "total": 197010,
       "calc": "50 × 3 940,20 ₽",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "Цена за место ниже: 3 940,20 ₽ вместо 4 081,50 ₽",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4867laqfs-4857/",
       "quantity": 50,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4867RAQFS"
        },
        {
         "label": "Ступень",
         "value": "50–99 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Расширенный лицензия Base на 1 год по количеству узлов"
        }
       ]
      }
     ]
    },
    {
     "key": "rejected",
     "title": "Не соответствуют требованию",
     "cards": [
      {
       "offer_id": "4986",
       "family_id": "3953",
       "badge": {
        "code": "not_required",
        "text": "Не соответствует требованию"
       },
       "title": "Kaspersky Endpoint Security — Стандартный",
       "subtitle": "Новая лицензия · 50 компьютеров · 1 год",
       "price_text": "135 765 ₽",
       "total": 135765,
       "calc": "50 × 2 715,30 ₽",
       "reason": "Шифрования данных в этой редакции нет",
       "unsuitable": true,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "open_store",
       "offer_url": "https://store.softline.ru/kaspersky/kl4863laqfs-4986/",
       "quantity": 50,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4863RAQFS"
        },
        {
         "label": "Ступень",
         "value": "50–99 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
        }
       ]
      }
     ]
    }
   ],
   "more": null,
   "sources": [
    {
     "label": "Документация Kaspersky",
     "fragment": "Встроенное шифрование | функция недоступна | функция доступна | функция доступна",
     "url": "https://www.kaspersky.ru/small-to-medium-business-security/endpoint-select"
    }
   ],
   "complements": [],
   "question": null,
   "suggestions": [
    {
     "label": "Рассчитать на 100 ПК",
     "message": "на 100 компьютеров"
    },
    {
     "label": "На 2 года",
     "message": "на 2 года",
     "chip": {
      "label": "",
      "key": "term_months",
      "value": "24"
     }
    },
    {
     "label": "Это продление",
     "message": "это продление",
     "chip": {
      "label": "",
      "key": "operation",
      "value": "renewal"
     }
    }
   ],
   "conditions": {
    "label": "Учитываю: 50 ПК · 1 год · новая · шифрование",
    "items": [
     "50 ПК",
     "1 год",
     "новая",
     "шифрование"
    ]
   },
   "links": [],
   "has_promo": false
  },
  "state": {
   "selection_id": "43becf87-7b62-4d97-b0a5-9405b77d4429",
   "intent": "comparison",
   "requirements": {
    "task": "",
    "vendor": "kaspersky",
    "objects": [
     {
      "kind": "workstation",
      "count": 50
     }
    ],
    "platform": "",
    "operation": "new",
    "term_months": 12,
    "org_type": "commercial",
    "required_features": [
     "encryption"
    ],
    "use": "",
    "variant": "",
    "cheapest": false
   },
   "origin": {
    "count:workstation": "user",
    "feature:encryption": "user",
    "operation": "default",
    "org_type": "default",
    "term_months": "default",
    "vendor": "user"
   },
   "candidate_family_ids": [
    "3953",
    "3935"
   ],
   "selected_offer_ids": [
    "4857"
   ],
   "fact_ids": [
    "kb_kes_adv_encryption"
   ],
   "unresolved": [],
   "feed_version": "softline-feed",
   "kb_version": "kb-ba021e12bec8",
   "turn": 2,
   "questions": 0,
   "skipped": 0,
   "pending": "",
   "quote_notes": null
  }
 },
 "s3": {
  "type": "selection",
  "lead": "Лицензия на рабочее место не действует в серверной ОС — нужна серверная.",
  "offers": [
   {
    "offer_id": "197958",
    "family_id": "22268",
    "title": "КриптоПро CSP 5.0: серверная лицензия СКЗИ",
    "family_title": "Программа КриптоПро CSP 5.0",
    "sku": "",
    "tier": "",
    "unit": "за сервер",
    "quantity": 1,
    "price_per_unit": 70000,
    "total": 70000,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kripto-pro/-197958/",
    "term": "бессрочная",
    "role": "match",
    "display": {
     "title": "КриптоПро CSP 5.0",
     "subtitle": "Серверная лицензия · бессрочная",
     "details": [
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "КриптоПро CSP 5.0: серверная лицензия СКЗИ"
      }
     ]
    },
    "sale_unit": "за сервер",
    "sale_unit_kind": "per_object",
    "unit_price": 70000,
    "pay_quantity": 1,
    "price_line": "1 × 70 000 ₽",
    "price_text": "70 000 ₽ за сервер",
    "package": null,
    "reason": null
   },
   {
    "offer_id": "411620",
    "family_id": "22268",
    "title": "КриптоПро CSP 5.0: годовая лицензия на 1 рабочее место",
    "family_title": "Программа КриптоПро CSP 5.0",
    "sku": "",
    "tier": "",
    "unit": "за рабочее место",
    "quantity": 1,
    "price_per_unit": 1850,
    "total": 1850,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kripto-pro/-411620/",
    "term": "1 год",
    "role": "rejected",
    "note": "Лицензия на рабочее место не действует в серверной ОС — нужна серверная",
    "rule_id": "cryptopro_workstation_not_server",
    "rule_source": "https://cryptopro.ru/buy/price",
    "display": {
     "title": "КриптоПро CSP 5.0",
     "subtitle": "Рабочее место · 1 год",
     "details": [
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "КриптоПро CSP 5.0: годовая лицензия на 1 рабочее место"
      }
     ]
    },
    "sale_unit": "за рабочее место",
    "sale_unit_kind": "per_object",
    "unit_price": 1850,
    "pay_quantity": 1,
    "price_line": "1 × 1 850 ₽",
    "price_text": "1 850 ₽ за рабочее место",
    "package": null,
    "reason": {
     "code": "not_for_server_os",
     "text": "Не подходит для серверной ОС"
    }
   },
   {
    "offer_id": "410137",
    "family_id": "22268",
    "title": "КриптоПро CSP (бессрочная лицензия СКЗИ КриптоПро CSP 5.0), на одном рабочем месте",
    "family_title": "Программа КриптоПро CSP 5.0",
    "sku": "",
    "tier": "",
    "unit": "за рабочее место",
    "quantity": 1,
    "price_per_unit": 3700,
    "total": 3700,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kripto-pro/-410137/",
    "term": "бессрочная",
    "role": "rejected",
    "note": "Лицензия на рабочее место не действует в серверной ОС — нужна серверная",
    "rule_id": "cryptopro_workstation_not_server",
    "rule_source": "https://cryptopro.ru/buy/price",
    "display": {
     "title": "КриптоПро CSP 5.0",
     "subtitle": "Рабочее место · бессрочная",
     "details": [
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "КриптоПро CSP (бессрочная лицензия СКЗИ КриптоПро CSP 5.0), на одном рабочем месте"
      }
     ]
    },
    "sale_unit": "за рабочее место",
    "sale_unit_kind": "per_object",
    "unit_price": 3700,
    "pay_quantity": 1,
    "price_line": "1 × 3 700 ₽",
    "price_text": "3 700 ₽ за рабочее место",
    "package": null,
    "reason": {
     "code": "not_for_server_os",
     "text": "Не подходит для серверной ОС"
    }
   },
   {
    "offer_id": "365870",
    "family_id": "385280",
    "title": "КриптоПро CSP TLS (Номер ЕРРП: 4332), Лицензия на право использования СКЗИ КриптоПро CSP версии 5.0 для одного",
    "family_title": "КриптоПро CSP TLS",
    "sku": "",
    "tier": "",
    "unit": "за сервер",
    "quantity": 1,
    "price_per_unit": 100000,
    "total": 100000,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kripto-pro/-365870/",
    "term": "бессрочная",
    "role": "alternative",
    "note": "Отдельная лицензия для TLS-сервера",
    "display": {
     "title": "КриптоПро CSP TLS",
     "subtitle": "Для TLS-сервера · бессрочная",
     "details": [
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "КриптоПро CSP TLS (Номер ЕРРП: 4332), Лицензия на право использования СКЗИ КриптоПро CSP версии 5.0 для одного"
      }
     ]
    },
    "sale_unit": "за сервер",
    "sale_unit_kind": "per_object",
    "unit_price": 100000,
    "pay_quantity": 1,
    "price_line": "1 × 100 000 ₽",
    "price_text": "100 000 ₽ за сервер",
    "package": null,
    "reason": {
     "code": "tls_server_only",
     "text": "Отдельная лицензия для TLS-сервера"
    }
   }
  ],
  "conditions": [
   {
    "key": "vendor",
    "label": "КриптоПро",
    "value": "kripto-pro",
    "origin": "user"
   },
   {
    "key": "object",
    "label": "для сервера",
    "value": "server",
    "origin": "user"
   },
   {
    "key": "operation",
    "label": "новая лицензия",
    "value": "new",
    "origin": "default"
   }
  ],
  "chat": {
   "status": "answer",
   "text": [
    "Лицензия на рабочее место не действует в серверной ОС — нужна серверная.",
    "Серверная лицензия стоит 70 000 ₽."
   ],
   "warning": "",
   "comparison": null,
   "groups": [
    {
     "key": "match",
     "title": "",
     "cards": [
      {
       "offer_id": "197958",
       "family_id": "22268",
       "badge": {
        "code": "server_os",
        "text": "Для серверной ОС"
       },
       "title": "КриптоПро CSP 5.0",
       "subtitle": "Серверная лицензия · бессрочная",
       "price_text": "70 000 ₽",
       "total": 70000,
       "calc": "",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kripto-pro/-197958/",
       "quantity": 1,
       "details": [
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "КриптоПро CSP 5.0: серверная лицензия СКЗИ"
        }
       ]
      },
      {
       "offer_id": "365870",
       "family_id": "385280",
       "badge": null,
       "title": "КриптоПро CSP TLS",
       "subtitle": "Для TLS-сервера · бессрочная",
       "price_text": "100 000 ₽",
       "total": 100000,
       "calc": "",
       "reason": "Отдельная лицензия для TLS-сервера",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kripto-pro/-365870/",
       "quantity": 1,
       "details": [
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "КриптоПро CSP TLS (Номер ЕРРП: 4332), Лицензия на право использования СКЗИ КриптоПро CSP версии 5.0 для одного"
        }
       ]
      }
     ]
    },
    {
     "key": "rejected",
     "title": "Не подходят для серверной ОС",
     "cards": [
      {
       "offer_id": "411620",
       "family_id": "22268",
       "badge": {
        "code": "unsuitable",
        "text": "Не подходит для сервера"
       },
       "title": "КриптоПро CSP 5.0",
       "subtitle": "Рабочее место · 1 год",
       "price_text": "1 850 ₽",
       "total": 1850,
       "calc": "",
       "reason": "Лицензия для рабочей станции",
       "unsuitable": true,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "open_store",
       "offer_url": "https://store.softline.ru/kripto-pro/-411620/",
       "quantity": 1,
       "details": [
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "КриптоПро CSP 5.0: годовая лицензия на 1 рабочее место"
        }
       ]
      },
      {
       "offer_id": "410137",
       "family_id": "22268",
       "badge": {
        "code": "unsuitable",
        "text": "Не подходит для сервера"
       },
       "title": "КриптоПро CSP 5.0",
       "subtitle": "Рабочее место · бессрочная",
       "price_text": "3 700 ₽",
       "total": 3700,
       "calc": "",
       "reason": "Лицензия для рабочей станции",
       "unsuitable": true,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "open_store",
       "offer_url": "https://store.softline.ru/kripto-pro/-410137/",
       "quantity": 1,
       "details": [
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "КриптоПро CSP (бессрочная лицензия СКЗИ КриптоПро CSP 5.0), на одном рабочем месте"
        }
       ]
      }
     ]
    }
   ],
   "more": null,
   "sources": [
    {
     "label": "Условия лицензирования КриптоПро",
     "fragment": "Лицензия на рабочее место не действует в серверной ОС — нужна серверная",
     "url": "https://cryptopro.ru/buy/price"
    }
   ],
   "complements": [],
   "question": null,
   "suggestions": [
    {
     "label": "Сервер для TLS",
     "message": "сервер для TLS",
     "chip": {
      "label": "",
      "key": "use",
      "value": "tls_server"
     }
    },
    {
     "label": "На 3 сервера",
     "message": "на 3 сервера"
    }
   ],
   "conditions": {
    "label": "Учитываю: для сервера · новая",
    "items": [
     "для сервера",
     "новая"
    ]
   },
   "links": [],
   "has_promo": false
  },
  "state": {
   "selection_id": "54de95ad-923a-49d7-b628-c3c37390aef9",
   "intent": "license_selection",
   "requirements": {
    "task": "",
    "vendor": "kripto-pro",
    "objects": [
     {
      "kind": "server",
      "count": 0
     }
    ],
    "platform": "",
    "operation": "new",
    "term_months": 0,
    "org_type": "commercial",
    "required_features": null,
    "use": "",
    "variant": "",
    "cheapest": true
   },
   "origin": {
    "count:server": "user",
    "operation": "default",
    "org_type": "default",
    "vendor": "user"
   },
   "candidate_family_ids": [
    "385280",
    "22268"
   ],
   "selected_offer_ids": [
    "197958"
   ],
   "fact_ids": [],
   "unresolved": [],
   "feed_version": "softline-feed",
   "kb_version": "kb-ba021e12bec8",
   "turn": 1,
   "questions": 0,
   "skipped": 0,
   "pending": "",
   "quote_notes": null
  }
 },
 "pay": {
  "type": "out_of_scope",
  "lead": "Оплату, доставку и счета подбор не обсуждает — ответ в справке магазина над выдачей.",
  "offers": [],
  "conditions": [
   {
    "key": "operation",
    "label": "новая лицензия",
    "value": "new",
    "origin": "default"
   },
   {
    "key": "term_months",
    "label": "1 год",
    "value": "12",
    "origin": "default"
   }
  ],
  "chat": {
   "status": "out_of_scope",
   "text": [
    "Помощник подбирает товары и лицензии. Условия оплаты — в разделе магазина."
   ],
   "warning": "",
   "comparison": null,
   "groups": [],
   "more": null,
   "sources": [],
   "complements": [],
   "question": null,
   "suggestions": [],
   "conditions": {
    "label": "",
    "items": [
     "1 год",
     "новая"
    ]
   },
   "links": [
    {
     "kind": "payment",
     "label": "Перейти к условиям оплаты",
     "url": "/help"
    }
   ],
   "has_promo": false
  },
  "state": {
   "selection_id": "de8554ef-cec8-4b79-98e2-b37f9709cd3b",
   "intent": "license_selection",
   "requirements": {
    "task": "",
    "vendor": "",
    "objects": null,
    "platform": "",
    "operation": "new",
    "term_months": 12,
    "org_type": "commercial",
    "required_features": null,
    "use": "",
    "variant": "",
    "cheapest": false
   },
   "origin": {
    "operation": "default",
    "org_type": "default",
    "term_months": "default"
   },
   "candidate_family_ids": [],
   "selected_offer_ids": [],
   "fact_ids": [],
   "unresolved": [],
   "feed_version": "softline-feed",
   "kb_version": "kb-ba021e12bec8",
   "turn": 1,
   "questions": 0,
   "skipped": 0,
   "pending": "",
   "quote_notes": null
  }
 },
 "s1": {
  "type": "selection",
  "lead": "Для 20 ПК дешевле всего за место — «Kaspersky Small Office Security». Состав функций у продуктов различается.",
  "offers": [
   {
    "offer_id": "248970",
    "family_id": "3761",
    "title": "Kaspersky Small Office Security новая лицензия на 5 ПК/мобильных устройств/пользователей на 1 год",
    "family_title": "Kaspersky Small Office Security",
    "sku": "KL4141RCEFS",
    "tier": "",
    "unit": "за место, пакетами по 5",
    "quantity": 20,
    "price_per_unit": 1347.84,
    "total": 26956.8,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4141rcefs-248970/",
    "term": "1 год",
    "role": "match",
    "package_price": 6739.2,
    "package_size": 5,
    "display": {
     "title": "Kaspersky Small Office Security",
     "subtitle": "Новая лицензия · 4 пакета на 5 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4141RCEFS"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Small Office Security новая лицензия на 5 ПК/мобильных устройств/пользователей на 1 год"
      }
     ]
    },
    "sale_unit": "пакет на 5 компьютеров",
    "sale_unit_kind": "package",
    "unit_price": 6739.2,
    "pay_quantity": 4,
    "price_line": "4 × 6 739,20 ₽",
    "price_text": "6 739,20 ₽ за пакет на 5 компьютеров",
    "package": {
     "size": 5,
     "packs": 4,
     "seats": 20,
     "spare": 0,
     "text": ""
    },
    "reason": null
   },
   {
    "offer_id": "228680",
    "family_id": "4069",
    "title": "Доктор Веб Комплексная защита + Центр управления на 1 год на 20 ПК",
    "family_title": "Dr.Web Desktop Security Suite",
    "sku": "LBW-BC-12M-20-A3",
    "tier": "",
    "unit": "за место, пакетами по 20",
    "quantity": 20,
    "price_per_unit": 1818.41,
    "total": 36368.28,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/drweb/lbw-bc-12m-20-a3-228680/",
    "term": "1 год",
    "role": "match",
    "package_price": 36368.28,
    "package_size": 20,
    "display": {
     "title": "Dr.Web Desktop Security Suite",
     "subtitle": "Новая лицензия · пакет на 20 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "LBW-BC-12M-20-A3"
      },
      {
       "label": "Позиция каталога",
       "value": "Доктор Веб Комплексная защита + Центр управления на 1 год на 20 ПК"
      }
     ]
    },
    "sale_unit": "пакет на 20 компьютеров",
    "sale_unit_kind": "package",
    "unit_price": 36368.28,
    "pay_quantity": 1,
    "price_line": "1 × 36 368,28 ₽",
    "price_text": "36 368,28 ₽ за пакет на 20 компьютеров",
    "package": {
     "size": 20,
     "packs": 1,
     "seats": 20,
     "spare": 0,
     "text": ""
    },
    "reason": null
   },
   {
    "offer_id": "222173",
    "family_id": "4052",
    "title": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов",
    "family_title": "Kaspersky Endpoint Security Cloud",
    "sku": "KL4742RANFS",
    "tier": "20–24",
    "unit": "за компьютер",
    "quantity": 20,
    "price_per_unit": 2833.2,
    "total": 56664,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4742ranfs-222173/",
    "term": "1 год",
    "role": "match",
    "display": {
     "title": "Kaspersky Endpoint Security Cloud",
     "subtitle": "Новая лицензия · 20 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4742RANFS"
      },
      {
       "label": "Ступень",
       "value": "20–24 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 2833.2,
    "pay_quantity": 20,
    "price_line": "20 × 2 833,20 ₽",
    "price_text": "2 833,20 ₽ за компьютер",
    "package": null,
    "reason": null
   },
   {
    "offer_id": "4984",
    "family_id": "3953",
    "title": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов",
    "family_title": "Kaspersky Endpoint Security для бизнеса Стандартный",
    "sku": "KL4863RANFS",
    "tier": "20–24",
    "unit": "за компьютер",
    "quantity": 20,
    "price_per_unit": 3004.02,
    "total": 60080.4,
    "price_state": "priced",
    "offer_url": "https://store.softline.ru/kaspersky/kl4863lanfs-4984/",
    "term": "1 год",
    "role": "match",
    "display": {
     "title": "Kaspersky Endpoint Security — Стандартный",
     "subtitle": "Новая лицензия · 20 компьютеров · 1 год",
     "details": [
      {
       "label": "Артикул",
       "value": "KL4863RANFS"
      },
      {
       "label": "Ступень",
       "value": "20–24 компьютеров"
      },
      {
       "label": "Тип организации",
       "value": "Коммерческая организация"
      },
      {
       "label": "Позиция каталога",
       "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
      }
     ]
    },
    "sale_unit": "за компьютер",
    "sale_unit_kind": "per_object",
    "unit_price": 3004.02,
    "pay_quantity": 20,
    "price_line": "20 × 3 004,02 ₽",
    "price_text": "3 004,02 ₽ за компьютер",
    "package": null,
    "reason": null
   }
  ],
  "conditions": [
   {
    "key": "direction",
    "label": "антивирус для рабочих мест",
    "value": "endpoint",
    "origin": "inferred"
   },
   {
    "key": "count:workstation",
    "label": "20 ПК",
    "value": "20",
    "origin": "user"
   },
   {
    "key": "operation",
    "label": "новая лицензия",
    "value": "new",
    "origin": "default"
   },
   {
    "key": "term_months",
    "label": "1 год",
    "value": "12",
    "origin": "default"
   },
   {
    "key": "org_type",
    "label": "коммерческая организация",
    "value": "commercial",
    "origin": "default"
   }
  ],
  "chat": {
   "status": "answer",
   "text": [
    "Для 20 ПК дешевле всего за место — «Kaspersky Small Office Security».",
    "Состав функций у продуктов различается.",
    "Считаю новую лицензию на один год."
   ],
   "warning": "",
   "comparison": null,
   "groups": [
    {
     "key": "match",
     "title": "",
     "cards": [
      {
       "offer_id": "248970",
       "family_id": "3761",
       "badge": {
        "code": "lowest_total",
        "text": "Меньшая стоимость среди найденных"
       },
       "title": "Kaspersky Small Office Security",
       "subtitle": "Новая лицензия · 4 пакета на 5 компьютеров · 1 год",
       "price_text": "26 956,80 ₽",
       "total": 26956.8,
       "calc": "4 × 6 739,20 ₽ · пакет на 5 компьютеров",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4141rcefs-248970/",
       "quantity": 20,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4141RCEFS"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Small Office Security новая лицензия на 5 ПК/мобильных устройств/пользователей на 1 год"
        }
       ]
      },
      {
       "offer_id": "228680",
       "family_id": "4069",
       "badge": null,
       "title": "Dr.Web Desktop Security Suite",
       "subtitle": "Новая лицензия · пакет на 20 компьютеров · 1 год",
       "price_text": "36 368,28 ₽",
       "total": 36368.28,
       "calc": "Пакет на 20 компьютеров",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/drweb/lbw-bc-12m-20-a3-228680/",
       "quantity": 20,
       "details": [
        {
         "label": "Артикул",
         "value": "LBW-BC-12M-20-A3"
        },
        {
         "label": "Позиция каталога",
         "value": "Доктор Веб Комплексная защита + Центр управления на 1 год на 20 ПК"
        }
       ]
      },
      {
       "offer_id": "222173",
       "family_id": "4052",
       "badge": null,
       "title": "Kaspersky Endpoint Security Cloud",
       "subtitle": "Новая лицензия · 20 компьютеров · 1 год",
       "price_text": "56 664 ₽",
       "total": 56664,
       "calc": "20 × 2 833,20 ₽",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4742ranfs-222173/",
       "quantity": 20,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4742RANFS"
        },
        {
         "label": "Ступень",
         "value": "20–24 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security Cloud лицензия на 1 год по количеству рабочих станций/файловых серверов"
        }
       ]
      },
      {
       "offer_id": "4984",
       "family_id": "3953",
       "badge": null,
       "title": "Kaspersky Endpoint Security — Стандартный",
       "subtitle": "Новая лицензия · 20 компьютеров · 1 год",
       "price_text": "60 080,40 ₽",
       "total": 60080.4,
       "calc": "20 × 3 004,02 ₽",
       "reason": "",
       "unsuitable": false,
       "price_state": "priced",
       "promo": false,
       "old_price_text": "",
       "note": "",
       "action": "add_to_spec",
       "offer_url": "https://store.softline.ru/kaspersky/kl4863lanfs-4984/",
       "quantity": 20,
       "details": [
        {
         "label": "Артикул",
         "value": "KL4863RANFS"
        },
        {
         "label": "Ступень",
         "value": "20–24 компьютеров"
        },
        {
         "label": "Тип организации",
         "value": "Коммерческая организация"
        },
        {
         "label": "Позиция каталога",
         "value": "Kaspersky Endpoint Security для бизнеса Стандартный базовая лицензия на 1 год. Количество узлов"
        }
       ]
      }
     ]
    }
   ],
   "more": {
    "label": "Показать все 7 вариантов в поиске",
    "query": "нужен антивирус в офис на 20 компьютеров",
    "count": 7
   },
   "sources": [],
   "complements": [],
   "question": null,
   "suggestions": [
    {
     "label": "Рассчитать на 30 ПК",
     "message": "на 30 компьютеров"
    },
    {
     "label": "Это продление",
     "message": "это продление",
     "chip": {
      "label": "",
      "key": "operation",
      "value": "renewal"
     }
    }
   ],
   "conditions": {
    "label": "Учитываю: 20 ПК · 1 год · новая",
    "items": [
     "20 ПК",
     "1 год",
     "новая"
    ]
   },
   "links": [],
   "has_promo": false
  },
  "state": {
   "selection_id": "51e041f0-6a70-44e1-a5a9-180117ff2a61",
   "intent": "license_selection",
   "requirements": {
    "task": "endpoint_protection",
    "vendor": "",
    "objects": [
     {
      "kind": "workstation",
      "count": 20
     }
    ],
    "platform": "",
    "operation": "new",
    "term_months": 12,
    "org_type": "commercial",
    "required_features": null,
    "use": "",
    "variant": "",
    "cheapest": false
   },
   "origin": {
    "count:workstation": "user",
    "operation": "default",
    "org_type": "default",
    "task": "inferred",
    "term_months": "default"
   },
   "candidate_family_ids": [
    "3761",
    "3953",
    "3935",
    "419620",
    "4052",
    "420044",
    "4069",
    "4092"
   ],
   "selected_offer_ids": [
    "248970",
    "228680",
    "222173",
    "4984"
   ],
   "fact_ids": [],
   "unresolved": [],
   "feed_version": "softline-feed",
   "kb_version": "kb-ba021e12bec8",
   "turn": 1,
   "questions": 0,
   "skipped": 0,
   "pending": "",
   "quote_notes": null
  }
 }
}
