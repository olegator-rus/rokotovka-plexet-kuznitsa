# Оценка по эталонам заказчика

API: `http://127.0.0.1:8092` · каталог: **softline-feed** · версия правил: 4

Команда: `python3 scripts/eval/customer_eval.py --configs live --out data/softline/eval/results/understanding-customer-after`

Первая цель — поле `target`, иначе первый товар выдачи (URL семейства → оффера → вендора). URL сравниваются после нормализации: регистр хоста, схема и хост магазина, завершающий слэш, URL-кодирование, `[]` = `[0]`, порядок параметров. Семейства — разные `family_id` карточек; SKU — разные `offer_id`/`product_id` карточек и их вариантов лицензии. Причина промаха: «ссылка» — нужный `group_id` первым, но URL не тот или нет; «ранг N» — нужный `group_id` на позиции N; «цель» — нужного `group_id` в выдаче нет. Колонка match_type: если поля нет, показано распределение `group` (exact/match/analog). В B0–B2 кандидатов BM25 не больше 50, поэтому SKU = 50 — потолок, а не полнота.

## live — текущая сборка: витрина, конвейер B3 на последней принятой версии правил

| запрос | первый URL | первый товар | тип эталон/ответ | совпало | карт. | сем. | SKU | match_type | маршрут | мс |
|---|---|---|---|---|---|---|---|---|---|---|
| astra linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Astra Linux Special Edition на базе п/а Эль… | family/family | да | 5 | 5 | 827 | exact:1 related:4 | A | 127 |
| Microsoft Office для дома и бизнеса | https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | Microsoft Office для дома и бизнеса 2024, Э… | family/family | да | 14 | 14 | 453 | exact:14 | B | 82 |
| kaspersky small office security | https://store.softline.ru/kaspersky/kaspersky-small-office-security | Kaspersky Small Office Security новая лицен… | family/family | да | 15 | 15 | 1052 | exact:15 | B | 90 |
| крипто про csp 5.0 | https://store.softline.ru/kripto-pro/cryptopro-csp | КриптоПро CSP версии 4.0 (Номер ЕРРП: 515),… | family/family | да | 12 | 12 | 629 | exact:11 related:1 | B | 77 |
| anydesk | https://store.softline.ru/anydesk/anydesk | AnyDesk (лицензия на 1 год), Solo | family/family | да | 4 | 4 | 30 | exact:1 fallback:3 | B | 41 |
| кибер бэкап | https://store.softline.ru/cyberprotect?filter[category][]=1033 | Киберпротект Кибер Бэкап Облачный (лицензия… | vendor_category/vendor_category | да | 11 | 11 | 589 | exact:4 related:7 | A | 71 |
| сканер-вс | https://store.softline.ru/npo-echelon/eshelon-skaner-vs | НПО «Эшелон» СКАНЕР-ВС 7, Основная лицензия… | family/family | да | 10 | 10 | 481 | exact:1 related:9 | A | 70 |
| мтс | https://store.softline.ru/webinar | МТС Линк (Webinar Group) МТС Линк Про (лице… | vendor/vendor | да | 4 | 4 | 10 | exact:3 fallback:1 | A | 50 |
| max patrol | https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | Positive Technologies MaxPatrol Endpoint Pr… | family/family | да | 4 | 4 | 3156 | exact:1 fallback:3 | A | 136 |
| 365 | https://store.softline.ru/microsoft/microsoft-365-for-family | Microsoft Dynamics 365 (подписка на 1 месяц… | family/family | да | 8 | 8 | 298 | exact:8 | B | 79 |
| сервер | нет URL | Сервер INFERIT RS212 R1G3D32/2x6346/32x64GB… | —/category |  | 19 | 19 | 19 | related:19 | C | 71 |
| антивирь | нет URL | Доктор Веб Антивирус + Центр Управления лиц… | —/category |  | 20 | 20 | 4325 | related:20 | B | 284 |
| офис | нет URL | Microsoft Office для дома и бизнеса 2024, Э… | —/category |  | 20 | 20 | 99 | related:20 | B | 47 |
| винда | нет URL | Microsoft Windows 11 Pro FQC-10572, электро… | —/category |  | 14 | 14 | 135 | related:14 | B | 41 |
| астра линукс | https://store.softline.ru/astralinux/astra-linux-special-edition | Astra Linux Special Edition на базе п/а Эль… | —/family |  | 9 | 9 | 849 | exact:1 related:8 | B | 98 |
| мдаемон | https://store.softline.ru/alt-n-technologies-ltd | MDaemon Technologies, Ltd. MDaemon (лицензи… | —/vendor |  | 5 | 5 | 495 | exact:5 | B | 49 |
| Astra Linux | https://store.softline.ru/astralinux/astra-linux-special-edition | Astra Linux Special Edition на базе п/а Эль… | —/family |  | 5 | 5 | 827 | exact:1 related:4 | A | 96 |

- эталоны совпали: 10 из 10
- ответов с ≥4 семействами: 17/17 (100%) — эталоны 10/10 (100%), ТЗ 7/7 (100%)
- ответов с ≥4 SKU: 17/17 (100%) — эталоны 10/10 (100%), ТЗ 7/7 (100%)
- ответов с ≥4 карточками: 17/17 (100%)
- задержка p50 77 мс, p95 284 мс (у клиента, включая сеть)
- первый URL есть: 13 из 17 ответов
- маршруты: A — 6, B — 10, C — 1; стоимость LLM по трассам 0 ₽; ошибок HTTP 0
- версия каталога: softline-feed

