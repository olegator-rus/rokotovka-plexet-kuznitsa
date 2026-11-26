# URL страниц семейств store.softline.ru

Файл: `data/softline/family_urls.jsonl` — одна строка на `group_id`:
`group_id`, `family_url` (абсолютный, нормализованный), `source: "breadcrumbs"`, `sku_url`, `offer_id`,
`fetched_at`, плюс `crumb_name` (текст крошки) и `name_match` (совпал ли он с названием категории фида).
Неудачи — `data/softline/family_urls_failed.jsonl`. Сборщик — `scripts/data/softline_family_urls.py`.

## Как собиралось

- 1 803 запроса за 94 мин.
- Фид `Mindbox.yml.xml`: 136 040 офферов, 8 125 без `group_id` (электроника, пропущена),
  4 413 групп, из них 1 809 с хотя бы одним `available=true` — это цель.
- На группу один SKU: сначала доступный с ценой, потом доступный, потом прочие; не более двух попыток.
- Разметка: крошки на странице SKU — `категория 1 → категория 2 → семейство` (schema.org BreadcrumbList),
  самого товара в крошках нет. Берётся последняя крошка; если она — сам товар (URL страницы или
  `-<offer_id>/` в конце), берётся предыдущая; если она — категория, группа уходит в неудачи.
- Вежливость: robots.txt прочитан (Crawl-delay нет, правила `User-agent: *` соблюдаются, включая `*` и `$`);
  один поток; пауза 1,1 с после каждого ответа; UA `Mozilla/5.0 (compatible; catalog-check/1.0)`;
  таймаут 20 с; только GET; на 429/403/5xx/таймаут — пауза 60 с, три подряд — остановка.
  За сбор: два таймаута подряд в 13:05–13:07 (обрыв сети у нас), после паузы продолжил. 429/403/5xx и капчи не было.
- Сбор возобновляем: повторный `collect` пропускает собранные группы; `--retry-failed` повторяет неудачи.

## Покрытие

| | |
|---|---|
| Групп с доступным оффером | 1 809 |
| Собрано | **1 808 (99,94 %)** |
| Неудачи | 1 — `http_404` |
| Название крошки = название категории фида | 1 808 из 1 808 |
| Редиректов | 0 |
| Уникальных URL | 1 804 |

Неудача: группа 393527 «СУОТ24 Цифровая охрана труда» — оба SKU отвечают 404
(например `https://store.softline.ru/seijpro/-487571/`). Для неё цель ведёт на SKU, промах покрытия.

Один URL на несколько групп (сайт сам объединяет; у групп одинаковое название категории):
- `/ksplus/sike-elektronnyi-kurs-ximiceskie-texnologii-neorganiceskix-veshh/` — 445074, 445077, 445080;
- `/ksplus/sike-elektronnyy-kurs-slesar-gornorudnogo-oborudovaniya-shakhtny/` — 406415, 406420;
- `/ksplus/sike-elektronnyy-kurs-sike-elektroslesar-oiro-demontazh-elektrod/` — 416988, 416992.

## Сверка с sitemap

`/https-sitemap.xml` → `sitemap/families.xml`: 6 411 URL (6 406 уникальных). В BASIS было «около 4,9 тыс.» —
сейчас больше.

- Собранные URL в sitemap: **1 804 из 1 804 (100 %)**.
- В sitemap, но не собраны: 4 602. Это ожидаемо: собирали только 1 809 групп с доступным оффером из 4 413;
  сопоставить остальные URL sitemap с группами без запроса страниц нельзя, поэтому разбивку не даю.

`known-family-urls.txt`: 38 из 45 собраны. Не собраны 7, все есть в sitemap:
`/webinar/mts-link-pro-150/`, `/ncloudtech/moyofis-standartnyy-2/`,
`/microsoft/microsoft-windows-server-datacenter-2022/`, `/positive-technologies/maxpatrol-siem/`,
`/s-terra/s-terra-klient/`, `/adobe/adobe-incopy/`, `/atol/frontol-xpos/`.
Для четырёх по названию нашлись группы без доступных офферов (Adobe InCopy 556, С-Терра Клиент 59547,
МойОфис Стандартный 3 344604, Windows Server Datacenter 2022 281974) — в цель сборщика не входят.
Для МТС Линк Pro 150, MaxPatrol SIEM и Frontol xPOS группу по названию однозначно не нашёл — не проверено.

## Цена «от»

15 случайных собранных семейств (seed фиксирован). Цена страницы — `AggregateOffer.lowPrice` в JSON-LD
семейства; цена фида — минимальная цена доступного оффера группы с ценой > 0.

Совпало **13 из 15**. Расхождения:

| group_id | Семейство | Страница | Фид | Офферов стр./фид |
|---|---|---|---|---|
| 435947 | `/objectivesystems/asn1c-v8/` | 16 652 | 16 619 | 27 / 27 |
| 288521 | `/nanosoft/nanocad-geonics/` | 9 800 | 8 900 | 54 / 68 |

Причины не установлены. Предположения: у ASN1C — пересчёт цены между выгрузкой фида и
сайтом; у nanoCAD GeoniCS сайт показывает 54 оффера из 68 доступных в фиде, самый дешёвый,
видимо, среди скрытых. Подробности — в журнале проверки (не коммитится).

## Эталоны

Все **8 из 8** целей типа `family` из `customer-queries.jsonl` совпали после нормализации
(хост, завершающий слэш): Astra Linux SE, Office 2024 для дома и бизнеса, Kaspersky Small Office Security,
КриптоПро CSP (`22268 → /kripto-pro/cryptopro-csp/`), AnyDesk, Эшелон «Сканер-ВС», MaxPatrol EPP,
Microsoft 365 Family. Дополнительно до массового запуска проверены 173614 → `/cyberprotect/kiber-bekap/`
и 447243 → `/webinar/mts-link-pro/` — совпали.

## Повторить

```
python3 scripts/data/softline_family_urls.py collect            # возобновляемо
python3 scripts/data/softline_family_urls.py stats -v
python3 scripts/data/softline_family_urls.py refs
python3 scripts/data/softline_family_urls.py sitemap --save-dir <каталог>
python3 scripts/data/softline_family_urls.py prices -n 15
```
