# Оценка по эталонам заказчика

API: `http://127.0.0.1:8088` · каталог: **softline-feed** · версия правил: 4

Команда: `python3 scripts/eval/customer_eval.py --configs live,B1 --out data/softline/eval/results/baseline-feed`

Первая цель — поле `target`, иначе первый товар выдачи (URL семейства → оффера → вендора). URL сравниваются после нормализации: регистр хоста, схема и хост магазина, завершающий слэш, URL-кодирование, `[]` = `[0]`, порядок параметров. Семейства — разные `family_id` карточек; SKU — разные `offer_id`/`product_id` карточек и их вариантов лицензии. Причина промаха: «ссылка» — нужный `group_id` первым, но URL не тот или нет; «ранг N» — нужный `group_id` на позиции N; «цель» — нужного `group_id` в выдаче нет. Колонка match_type: если поля нет, показано распределение `group` (exact/match/analog). В B0–B2 кандидатов BM25 не больше 50, поэтому SKU = 50 — потолок, а не полнота.

## live — текущая сборка: витрина, конвейер B3 на последней принятой версии правил

| запрос | первый URL | первый товар | тип эталон/ответ | совпало | карт. | сем. | SKU | match_type | маршрут | мс |
|---|---|---|---|---|---|---|---|---|---|---|
| astra linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Сертификат техподдержки на ОС Astra Linux S… | family/family | да | 5 | 5 | 1044 | group match:5 | A | 104 |
| Microsoft Office для дома и бизнеса | https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | Microsoft Office для дома и бизнеса 2024, Э… | family/family | да | 15 | 15 | 870 | group exact:15 | B | 99 |
| kaspersky small office security | https://store.softline.ru/kaspersky/kaspersky-small-office-security | Kaspersky Small Office Security новая лицен… | family/family | да | 10 | 10 | 1204 | group exact:10 | B | 106 |
| крипто про csp 5.0 | https://store.softline.ru/kripto-pro/cryptopro-csp | КриптоПро CSP версии 4.0 (Номер ЕРРП: 515),… | family/family | да | 19 | 19 | 769 | group exact:1 match:18 | B | 91 |
| anydesk | https://store.softline.ru/anydesk/anydesk | AnyDesk (лицензия на 1 год), Solo | family/family | да | 1 | 1 | 11 | group exact:1 | B | 79 |
| кибер бэкап | https://store.softline.ru/cyberprotect/speam1-536456 | Киберпротект Кибер Бэкап Облачный (лицензия… | vendor_category/sku | нет (цель) | 11 | 11 | 821 | group exact:4 match:7 | A | 99 |
| сканер-вс | https://store.softline.ru/colortrac/03n002-322031 | Сканер Colortrac SmartLF Scan! 24 | family/sku | нет (цель) | 20 | 20 | 20 | group match:20 | A | 38 |
| мтс | https://store.softline.ru/webinar/mts-link-pro | МТС Линк (Webinar Group) МТС Линк Про (лице… | vendor/family | нет (цель) | 3 | 3 | 3 | group exact:3 | A | 123 |
| max patrol | https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | Positive Technologies MaxPatrol Endpoint Pr… | family/family | да | 3 | 3 | 205 | group match:3 | D | 86 |
| 365 | нет URL |  | family/— | нет (цель) | 0 | 0 | 0 | — | B | 23 |
| сервер | https://store.softline.ru/inferit/ifs-00909-542026 | Сервер INFERIT RS212 R1G3D32/2x6346/32x64GB… | —/sku |  | 20 | 20 | 20 | group match:20 | C | 42 |
| антивирь | https://store.softline.ru/softlineuniverse/slu-rm1-010-1m-536399 | Softline Universe (рабочее место ОС+офис+ан… | —/sku |  | 13 | 13 | 589 | group match:13 | B | 60 |
| офис | https://store.softline.ru/eiceblue/30423864-415082 | e-iceblue Spire.Cloud.Office (подписка Deve… | —/sku |  | 20 | 20 | 613 | group match:20 | B | 121 |
| винда | https://store.softline.ru/venta/ventafax | Вента VentaFax (лицензия MiniOffice, электр… | —/family |  | 16 | 16 | 956 | group match:16 | B | 99 |
| астра линукс | https://store.softline.ru/backupsolutions/-112710 | BackupSolutions Bacula Enterprise Edition (… | —/sku |  | 20 | 20 | 1164 | group match:20 | B | 88 |
| мдаемон | https://store.softline.ru/alt-n-technologies-ltd/-477649 | MDaemon Technologies, Ltd. MDaemon (лицензи… | —/sku |  | 6 | 6 | 510 | group exact:5 match:1 | B | 59 |
| Astra Linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Сертификат техподдержки на ОС Astra Linux S… | —/family |  | 5 | 5 | 1044 | group match:5 | A | 105 |

- эталоны совпали: 6 из 10 (причины промахов: цель — 4)
- ответов с ≥4 семействами: 13/17 (76%) — эталоны 6/10 (60%), ТЗ 7/7 (100%)
- ответов с ≥4 SKU: 15/17 (88%) — эталоны 8/10 (80%), ТЗ 7/7 (100%)
- ответов с ≥4 карточками: 13/17 (76%)
- задержка p50 91 мс, p95 123 мс (у клиента, включая сеть)
- первый URL есть: 16 из 17 ответов
- маршруты: A — 5, B — 10, C — 1, D — 1; стоимость LLM по трассам 0 ₽; ошибок HTTP 0
- версия каталога: softline-feed

## B1 — BM25 без улучшений (ближайшая): multi_match по мультиполям + нормализация и словари v1

| запрос | первый URL | первый товар | тип эталон/ответ | совпало | карт. | сем. | SKU | match_type | маршрут | мс |
|---|---|---|---|---|---|---|---|---|---|---|
| astra linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Сертификат техподдержки на ОС Astra Linux S… | family/family | да | 1 | 1 | 50 | group exact:1 | A | 17 |
| Microsoft Office для дома и бизнеса | https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | Microsoft Office для дома и бизнеса 2024, Э… | family/family | да | 16 | 16 | 50 | group exact:1 match:15 | A | 30 |
| kaspersky small office security | https://store.softline.ru/kaspersky/kaspersky-small-office-security | Kaspersky Small Office Security продление л… | family/family | да | 3 | 3 | 50 | group match:3 | A | 19 |
| крипто про csp 5.0 | https://store.softline.ru/kripto-pro/kripto-pro-tsp | КриптоПро Крипто-Про TSP (обновление), до в… | family/family | нет (ранг 2) | 3 | 3 | 50 | group match:3 | A | 31 |
| anydesk | https://store.softline.ru/anydesk/anydesk | AnyDesk (лицензия на 1 год), Solo | family/family | да | 1 | 1 | 11 | group exact:1 | A | 10 |
| кибер бэкап | https://store.softline.ru/cyberprotect/fva-sa-530125 | Киберпротект Кибер Бэкап Расширенная редакц… | vendor_category/sku | нет (цель) | 1 | 1 | 50 | group exact:1 | A | 12 |
| сканер-вс | https://store.softline.ru/npo-echelon/eshelon-skaner-vs | НПО «Эшелон» Сканер-ВС, НПЕШ.00606-01, Обно… | family/family | да | 1 | 1 | 50 | group exact:1 | A | 23 |
| мтс | https://store.softline.ru/webinar/mts-link-pro | МТС Линк (Webinar Group) МТС Линк Про (лице… | vendor/family | нет (цель) | 3 | 3 | 3 | group exact:3 | A | 10 |
| max patrol | https://store.softline.ru/a4tech/v8m-max-450460 | Мышь A4tech Bloody V8M Max V8M MAX | family/sku | нет (цель) | 11 | 11 | 50 | group match:11 | A | 38 |
| 365 | https://store.softline.ru/microsoft/aaa-37833-236192 | Microsoft Dynamics 365 (подписка for Sales … | family/sku | нет (цель) | 8 | 8 | 50 | group exact:8 | A | 19 |
| сервер | https://store.softline.ru/infteh/00012-2-529901 | ИнфТех ДатаПлат, Базовый сервер (бессрочная… | —/sku |  | 2 | 2 | 50 | group exact:2 | A | 19 |
| антивирь | https://store.softline.ru/softlineuniverse/slu-rm1-010-1m-536399 | Softline Universe (рабочее место ОС+офис+ан… | —/sku |  | 2 | 2 | 50 | group exact:2 | A | 19 |
| офис | https://store.softline.ru/office-kit/ok0215s145-97694 | Шредер Office Kit S145 | —/sku |  | 18 | 18 | 50 | group exact:16 match:2 | A | 14 |
| винда | https://store.softline.ru/rapo/-408160 | РАПО Сарма, Плеер для Windows (лицензия), | —/sku |  | 4 | 4 | 50 | group exact:4 | A | 20 |
| астра линукс | https://store.softline.ru/tsentr-bezopasnosti-informatsii-tsbi/tsbi-astra | Центр безопасности информации (ЦБИ) ЦБИ Аст… | —/family |  | 20 | 20 | 38 | group match:20 | A | 19 |
| мдаемон | https://store.softline.ru/alt-n-technologies-ltd/-477649 | MDaemon Technologies, Ltd. MDaemon (лицензи… | —/sku |  | 1 | 1 | 50 | group exact:1 | A | 19 |
| Astra Linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Сертификат техподдержки на ОС Astra Linux S… | —/family |  | 1 | 1 | 50 | group exact:1 | A | 18 |

- эталоны совпали: 5 из 10 (причины промахов: ранг 2 — 1, цель — 4)
- ответов с ≥4 семействами: 6/17 (35%) — эталоны 3/10 (30%), ТЗ 3/7 (43%)
- ответов с ≥4 SKU: 16/17 (94%) — эталоны 9/10 (90%), ТЗ 7/7 (100%)
- ответов с ≥4 карточками: 6/17 (35%)
- задержка p50 19 мс, p95 38 мс (у клиента, включая сеть)
- первый URL есть: 17 из 17 ответов
- маршруты: A — 17; стоимость LLM по трассам 0 ₽; ошибок HTTP 0
- версия каталога: softline-feed

