# Заложено для агента SearchOps Commerce (контур коммерции)

Файл генерирует `scripts/data/gen_mock_p2.py`; числа ниже посчитаны на текущих данных. Заказы и маржа
синтетические (`catalog.json` → `synthetic`).

| id | Диагноз | Что заложено | Где | Ожидаемое действие агента |
| --- | --- | --- | --- | --- |
| P2-01 | `BROKEN_RELATION` | Связь `rel-0065`: windows-server-2025 → windows-server-cal (CAL 2022), complement, `conditional_required`, review_status approved, evidence E1. Правило `lr-ws25-cal` требует `version >= 2025`, у всех позиций windows-server-cal `attributes.version = "2022"` | relations.json, license_rules.json | `relation_patch`: remove `rel-0065` (верная связь на windows-server-cal-2025 уже есть); след в заказах — 3 заказа «WS 2025 + CAL 2022» |
| P2-02 | `MISSING_REQUIRED` | Правило `lr-ws25-rds` (RDS CAL 2025 к windows-server-2025) есть, связи windows-server-2025 → rds-cal-2025 нет | license_rules.json, relations.json | `relation_patch`: add complement `conditional_required`, conditions `remote_desktop_used`, per_user, source vendor_documentation |
| P2-03 | `MISSING_RELATION` | Пара семейств dell-poweredge-r750 ↔ ippon-innova-rt-ii (сервер ↔ ИБП, категории server-systems × ups): 9 совместных заказов из 13 заказов R750 и 11 заказов ИБП, 7 разных аккаунтов, p = 8.89e-11 (односторонний точный тест Фишера, 179 заказов с аккаунтом); проходит и поправку Бонферрони на 50 пар с ≥3 аккаунтами (порог 2.0e-04). Связи нет ни в одну сторону, общих соседей по связям нет, не заменители | events.jsonl (`order` с `order_id`, `account_id`) | Предложение связи complement, source co_purchase, evidence E4, на подтверждение человеку |
| P2-04 | `BROKEN_RELATION` (снятый товар) | Связь `rel-0092`: dell-poweredge-r740 → hpe-proliant-dl380-gen10, substitute, `to_product = srv-dl380g10-4210`; у позиции `price_mode = unavailable` (снята с производства) | relations.json, catalog.json | Убрать `to_product` или заменить на srv-dl380g10-5218 |

## Шум, который агент должен отклонить

| id | Что | Числа | Почему не связь |
| --- | --- | --- | --- |
| N-01 | hp-e24-g5 ↔ dlink-dgs-1210 | 4 совместных заказа, аккаунтов 1 (acc-007), p = 2.42e-08 | Меньше 3 аккаунтов: повтор одного клиента |
| N-02 | lenovo-thinkpad-e14-gen5 ↔ hp-probook-450-g10 | 3 совместных заказа, аккаунтов 3, p = 1.37e-01 | Заменители (связь substitute есть): сравнение, не дополнение |
| N-03 | Случайные пары из одиночных заказов | 1–2 совместных заказа | Не значимы по тесту Фишера |
| N-04 | Комплектующие одного сервера (память × диски × гарантия) | значимы | Объяснены общей связью с сервером-якорем: это «соседи», а не новая связь |
| N-05 | Производные от P2-03: dell-server-drives ↔ ippon-innova-rt-ii, dell-rdimm-ddr4 ↔ ippon-innova-rt-ii, dell-prosupport ↔ ippon-innova-rt-ii | значимы, те же заказы R750 | Сворачиваются к паре с системой-якорем (P2-03): ИБП покупают к серверу, а не к его памяти |

## Совместимость: примеры всех состояний

| Состояние | Пример | Где |
| --- | --- | --- |
| verified | mem-hpe-32g-d4 → hpe-proliant-dl380-gen10 | compatibility_rules.json |
| conditional | mem-dell-32g-d4 → dell-poweredge-r740 («работает на 2933 МТ/с…») | compatibility_rules.json |
| incompatible | mem-hpe-32g-d5 → dell-poweredge-r740 (DDR5 в слоты DDR4) | compatibility_rules.json |
| unknown | ssd-samsung-pm893-960 → любой сервер; mem-kingston-32g-d4 → hpe-proliant-dl380-gen10 | правила нет |

## Прочее

- Модель `price_mode`: fixed, from (сервер R750, VEGMAN — цена базовой сборки), quote_required (позиций: 7), unavailable (srv-dl380g10-4210).
- Отрицательные запросы набора p2 (`p2_negative`): голое число, категорийные запросы, отсутствующие товары, SQL Server по ядрам (CAL не нужны), «windows server 2025 и cal 2022».
