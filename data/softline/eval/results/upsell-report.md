# Блок «Дополните решение»: эталоны и примеры ТЗ

Требования 5 и К2. Скрипт `scripts/eval/upsell_eval.py`, поиск с `source=compare`. Правила проверки — в шапке скрипта. Знаменатель у долей — ответы, где блок есть.

## Итог

| Показатель | Всего | Эталоны | ТЗ |
| --- | --- | --- | --- |
| Ответов с блоком | 13/17 | 10/10 | 3/7 |
| Карточек всего | 35 | 26 | 9 |
| У всех карточек есть причина и уровень основания | 13/13 | 10/10 | 3/3 |
| Все карточки с ценой | 13/13 | 10/10 | 3/3 |
| Одно семейство на вендора | 13/13 | 10/10 | 3/3 |
| Не больше 4 карточек | 13/13 | 10/10 | 3/3 |
| Нет карточки из семейства якоря, кроме техподдержки и продления | 12/13 | 9/10 | 3/3 |
| Уровень A там, где есть документ A (карточек) | 11/11 | 5/5 | 6/6 |

Уровни оснований по карточкам: A — 11, B — 24.
Виды оснований: class_logic — 15, compat_document — 11, license_kind — 8, sku_platform — 1; классы карточек: compatible — 12, complement — 15, renewal — 8.
Карточек из семейства якоря: 9; из них не техподдержка и не продление: Вариант под платформу.
Распределение числа карточек в блоке: 1 — 1, 2 — 6, 3 — 2, 4 — 4.

Блока нет у запросов-категорий ТЗ («сервер», «антивирь», «офис», «винда»): якорь блока — только названное семейство (`TestUpsellAnchorNeedsNamedTarget`).

## По запросам

| Набор | Запрос | Якорь | Карточек | Причина и основание | Цена | Вендор | ≤4 | A по документу | Своё семейство не продление | Карточки (уровень, вид) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| etalon | astra linux | Astra Linux Special Edition | 4 | да | да | да | да | 3/3 | нет | Техподдержка: Astra Linux Special Edition (B, license_kind); Антивирус для организаций: Kaspersky Endpoint Security для бизнеса Расширенный (A, compat_document); СКЗИ, защита информации: Программа КриптоПро CSP 5.0 (A, compat_document); Резервное копирование: Кибер Бэкап Расширенная редакция (A, compat_document) |
| etalon | Microsoft Office для дома и бизнеса | Microsoft Office 2024 для дома и бизнеса | 2 | да | да | да | да | — | нет | Антивирус для организаций: Dr.Web Desktop Security Suite (B, class_logic); Резервное копирование: Кибер Бэкап Расширенная редакция (B, class_logic) |
| etalon | kaspersky small office security | Kaspersky Small Office Security | 2 | да | да | да | да | — | нет | Продление: Kaspersky Small Office Security (B, license_kind); Резервное копирование: Кибер Бэкап Расширенная редакция (B, class_logic) |
| etalon | крипто про csp 5.0 | Программа КриптоПро CSP 5.0 | 2 | да | да | да | да | 1/1 | Вариант под платформу | Вариант под платформу: Программа КриптоПро CSP 5.0 (B, sku_platform); Операционная система: Astra Linux Special Edition (A, compat_document) |
| etalon | anydesk | AnyDesk | 3 | да | да | да | да | — | нет | Антивирус для организаций: Dr.Web Desktop Security Suite (B, class_logic); Российский удалённый доступ: Joint Advanced (B, class_logic); Российский удалённый доступ: «Эликонт-КС» (B, class_logic) |
| etalon | кибер бэкап | Кибер Бэкап Стандартная редакция | 4 | да | да | да | да | 1/1 | нет | Техподдержка: Кибер Бэкап Стандартная редакция (B, license_kind); Хранилище: Кибер Хранилище (B, class_logic); Операционная система: Astra Linux Special Edition (A, compat_document); Виртуализация: ROSA Virtualization (B, class_logic) |
| etalon | сканер-вс | Эшелон «СКАНЕР-ВС» | 3 | да | да | да | да | — | нет | Продление: Эшелон «СКАНЕР-ВС» (B, license_kind); Антивирус для организаций: Dr.Web Desktop Security Suite (B, class_logic); СКЗИ, защита информации: Код безопасности: Secret Net Studio 8 (B, class_logic) |
| etalon | мтс | МТС Линк Стандарт | 2 | да | да | да | да | — | нет | Офисный пакет: Microsoft 365 Family (для семьи) (B, class_logic); Офисный пакет: Workspad (B, class_logic) |
| etalon | max patrol | MaxPatrol Endpoint Protection Platform | 2 | да | да | да | да | — | нет | Продление: MaxPatrol Endpoint Protection Platform (B, license_kind); Резервное копирование: Кибер Бэкап Расширенная редакция (B, class_logic) |
| etalon | 365 | Microsoft 365 Family (для семьи) | 2 | да | да | да | да | — | нет | Антивирус для организаций: Kaspersky Security для Microsoft Office 365 (B, class_logic); Резервное копирование: Кибер Бэкап Расширенная редакция (B, class_logic) |
| tz | сервер | блока нет | 0 | — | — | — | — | — | — | — |
| tz | антивирь | блока нет | 0 | — | — | — | — | — | — | — |
| tz | офис | блока нет | 0 | — | — | — | — | — | — | — |
| tz | винда | блока нет | 0 | — | — | — | — | — | — | — |
| tz | астра линукс | Astra Linux Special Edition | 4 | да | да | да | да | 3/3 | нет | Техподдержка: Astra Linux Special Edition (B, license_kind); Антивирус для организаций: Kaspersky Endpoint Security для бизнеса Расширенный (A, compat_document); СКЗИ, защита информации: Программа КриптоПро CSP 5.0 (A, compat_document); Резервное копирование: Кибер Бэкап Расширенная редакция (A, compat_document) |
| tz | мдаемон | MDaemon | 1 | да | да | да | да | — | нет | Продление: MDaemon (B, license_kind) |
| tz | Astra Linux | Astra Linux Special Edition | 4 | да | да | да | да | 3/3 | нет | Техподдержка: Astra Linux Special Edition (B, license_kind); Антивирус для организаций: Kaspersky Endpoint Security для бизнеса Расширенный (A, compat_document); СКЗИ, защита информации: Программа КриптоПро CSP 5.0 (A, compat_document); Резервное копирование: Кибер Бэкап Расширенная редакция (A, compat_document) |

Точность самого подбора (нужна ли покупателю карточка) не размечалась: скрипт проверяет правила сборки блока и основания, а не полезность.
