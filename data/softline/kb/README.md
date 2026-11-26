# База знаний подбора (`data/softline/kb/`)

Фид: `softline-feed`. Данные сгенерированы, вручную их не правили.

## Файлы

| Файл | Что | Ключ |
|---|---|---|
| `product_facts.jsonl` | факты о свойствах продуктов | `id` |
| `license_rules.json` | правила лицензирования; исходный набор — `data/softline/eval/license-rules.json` | `id` |
| `vendor_families.json` | название у вендора → `family_id` фида, линейка (`line`) | `family_id` |
| `offer_scope.jsonl` | КриптоПро 22268 и 385280: область действия оффера | `offer_id` |
| `astra_variants.jsonl` | Astra Linux SE 121231: вариант, объект лицензии, уровень ТП, сертификат по атрибутам фида | `offer_id` |
| `sale_units.jsonl` | Kaspersky, Dr.Web, PRO32: единица продажи | `offer_id` |
| `security_subclasses.json` | направления ИБ; идентификаторы сверены с фидом | — |

## Статусы — отступление от схемы

В схеме два статуса: `draft → verified`. Добавлен промежуточный, потому что в показ идёт только сверенное человеком:

| `status` | Значение | В показ |
|---|---|---|
| `draft` | черновик, конфликт источников или источник не тот (только магазин там, где нужен вендор) | нет |
| `verified_agent` | агент открыл страницу и записал дословный фрагмент | решает «Ядро» |
| `verified` | человек сверил | да |

У правил есть ещё два значения: `rejected` (источник говорит другое) и `unconfirmed` (источника нет). Эти правила
не применять.

Когда человек сверит запись: `status = "verified"`, `reviewed_by = <имя>`, `reviewed_at = <дата>`. Поля
`checked_by` / `checked_at` остаются за агентом.

## Дополнительные поля факта (сверх схемы)

| Поле | Зачем |
|---|---|
| `label` | название строки таблицы сравнения: как у вендора, по-русски |
| `source_kind` | `vendor` — сайт вендора; `store` — store.softline.ru; `vendor_doc_via_store` — документ вендора на файлохранилище Softline |
| `fetch_method` | `curl` — дословный текст из HTML; `browser` — текст страницы в браузере; `webfetch` — пересказ модели, сверить обязательно; `webfetch_pdf_pdftotext` — PDF скачан и разобран, текст дословный; `pdf_image` — значения сняты глазами со страницы PDF |
| `corroborated_by` | другие источники того же значения: `source_url`, `source_section`, `excerpt`, … |
| `conflict` | в чём расходятся источники; запись тогда в `draft`, значение `unknown` или `depends` |
| `note` | оговорки применимости |
| `used_in` | в каких эталонных диалогах (D1…D10) ячейка видна |
| `reviewed_by`, `reviewed_at` | сверка человеком |

`applicability` может содержать `license_object` (`workstation`, `physical_server`), это тоже сверх схемы.
`source_date: null` — на странице нет даты документа.

`value`: как в схеме (`yes`/`no`/`unknown`/`depends`, строка, список). Числа хранятся объектом
`{"value": N, "unit": "…"}` или `{"max": N, "unit": "…"}`.

`variant`: Astra — `Орел` / `Воронеж` / `Смоленск`, как в атрибуте фида `Версия`. AnyDesk — `Solo` / `Standard` /
`Advanced`, как в названиях офферов 306539. Сканер-ВС 46421 — `Base` / `Enterprise`, как у вендора; офферы фида
вариантом не размечены (в части названий есть «Enterprise»), см. `variants_note` в `vendor_families.json`.

## Правила — отступления

- Добавлено поле `sources[]` с фрагментами (`source` из схемы оставлен — главный адрес).
- Поля `checked_at`, `checked_by`, `reviewed_by`, `reviewed_at`, `used_in`, `change`. В `change` записано, что
  изменено относительно исходного набора и почему.
- `cryptopro_tls_separate`: условие теперь `{"use": "tls_server", "server_os_not": "windows"}`, требование —
  `{"license_scope": "tls_server"}`. Это ключ из `offer_scope.jsonl`, а не `family_id`: TLS-лицензии КС3 для Astra/Альт
  лежат в 22268.
- `cyberbackup_linux_is_server` → `rejected`; вместо него `cyberbackup_linux_workstation_advanced`.
- `m365_business_max_300`: добавлены `family_ids` (65743, 169704).

## offer_scope.jsonl

`license_scope`: `workstation` · `server` · `tls_server` · `distribution` · `unknown`. `platform`: `any` или
`astra_alt_ks3` (класс КС3 для Astra Linux SE / Альт 8 СП). `max_connections` — для TLS-сервера,
`unlimited_connections: true` — «на неограниченное количество». `basis`: `title` — по названию в фиде;
`store_family_page` — название в фиде обрезано, полное взято со страницы семейства в магазине. Пять офферов остались
`unknown`: 197890 и 197891 — CSP 4.0 для иностранных лиц, 197955 и 197956 — модули, 487264 — OCSP. В подборе их не
предлагать.

## astra_variants.jsonl

`object`: `workstation` или `server`, если в «Назначении» только серверные варианты. `objects_detail` — как в фиде
(`server_2s`, `server_2s_vm`, `virtual_server`, `server_dbms`). Вариант `Другое` — Эльбрус (так в конфигураторе).

## sale_units.jsonl

| `sale_unit` | Значение | Цена в фиде |
|---|---|---|
| `per_unit_in_tier` | Kaspersky: цена за единицу (`unit`, обычно «узлов») на ступени `tier_min…tier_max` | за 1 единицу; сумма = цена × количество |
| `package` | Dr.Web, PRO32, коробки Kaspersky: комплект на `package_size` единиц | за комплект; цена за место = цена / `package_size` |
| `unknown` | не разобрано | цену за место не считать |

Проверено на офферах магазина: 4985 Kaspersky (`К-во узлов от 25 до 49`, `2 813,40 ₽/шт`), 5317 Dr.Web (`на 10 ПК`,
`20 204,60 ₽/шт`), 398991 PRO32 (`81 узел`, `137 700 ₽/шт`, «Продукт продаётся комплектами от 5 узлов»).
У Dr.Web импорт пишет `license_tier_min = license_tier_max = 1` («Кол-во от 1 до 1 шт.»). Это количество
комплектов, а не мест.
