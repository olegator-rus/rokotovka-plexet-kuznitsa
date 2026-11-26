# Оценка на запросах-соседях эталонов (контур эталонов)

API: `http://127.0.0.1:8092` · каталог: **softline-feed** · версия правил: 4

Команда: `python3 scripts/eval/neighbor_eval.py --before data/softline/eval/results/neighbors-before.json --out data/softline/eval/results/neighbors-after`

Строго — первая цель ответа (поле `target`, иначе первый товар) совпадает с ожидаемой по URL после нормализации. «В первых 4» — строго или ожидаемое семейство (для вендора — карточка вендора) среди первых четырёх карточек.

**Итого: строго 114/114 (100%), в первых 4 114/114 (100%).**

## По вендорам

| группа | строго | в первых 4 | было строго | было в первых 4 |
|---|---|---|---|---|
| astralinux | 18/18 (100%) | 18/18 (100%) | 14/18 (78%) | 17/18 (94%) |
| microsoft | 18/18 (100%) | 18/18 (100%) | 13/18 (72%) | 18/18 (100%) |
| kaspersky | 19/19 (100%) | 19/19 (100%) | 14/19 (74%) | 15/19 (79%) |
| kripto-pro | 15/15 (100%) | 15/15 (100%) | 9/15 (60%) | 15/15 (100%) |
| anydesk | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) |
| cyberprotect | 13/13 (100%) | 13/13 (100%) | 10/13 (77%) | 13/13 (100%) |
| npo-echelon | 8/8 (100%) | 8/8 (100%) | 6/8 (75%) | 6/8 (75%) |
| webinar | 9/9 (100%) | 9/9 (100%) | 5/9 (56%) | 7/9 (78%) |
| positive-technologies | 8/8 (100%) | 8/8 (100%) | 5/8 (62%) | 6/8 (75%) |

## По виду искажения

| группа | строго | в первых 4 | было строго | было в первых 4 |
|---|---|---|---|---|
| склонение | 9/9 (100%) | 9/9 (100%) | 6/9 (67%) | 7/9 (78%) |
| транслит | 15/15 (100%) | 15/15 (100%) | 9/15 (60%) | 12/15 (80%) |
| опечатка | 4/4 (100%) | 4/4 (100%) | 3/4 (75%) | 4/4 (100%) |
| раскладка | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| атрибут | 11/11 (100%) | 11/11 (100%) | 9/11 (82%) | 10/11 (91%) |
| написание | 23/23 (100%) | 23/23 (100%) | 11/23 (48%) | 19/23 (83%) |
| название | 49/49 (100%) | 49/49 (100%) | 42/49 (86%) | 48/49 (98%) |

## По виду формулировки

| группа | строго | в первых 4 | было строго | было в первых 4 |
|---|---|---|---|---|
| транслит | 9/9 (100%) | 9/9 (100%) | 4/9 (44%) | 6/9 (67%) |
| склонение | 8/8 (100%) | 8/8 (100%) | 6/8 (75%) | 7/8 (88%) |
| полное название | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| слитно | 6/6 (100%) | 6/6 (100%) | 2/6 (33%) | 4/6 (67%) |
| сокращение | 5/5 (100%) | 5/5 (100%) | 3/5 (60%) | 4/5 (80%) |
| раскладка | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| опечатка | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) | 3/3 (100%) |
| редакция | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| атрибут лицензии | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| атрибут | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| бренд | 9/9 (100%) | 9/9 (100%) | 6/9 (67%) | 9/9 (100%) |
| другой продукт вендора | 5/5 (100%) | 5/5 (100%) | 5/5 (100%) | 5/5 (100%) |
| жаргон | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| английское название | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| соседнее семейство | 23/23 (100%) | 23/23 (100%) | 22/23 (96%) | 22/23 (96%) |
| прежнее название | 3/3 (100%) | 3/3 (100%) | 1/3 (33%) | 2/3 (67%) |
| уточнение | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| две редакции | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) |
| смешанное написание | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| склонение + транслит | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 0/1 (0%) |
| неполное название | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| атрибут количества | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 0/1 (0%) |
| вендор | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| линейка | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| английское написание | 2/2 (100%) | 2/2 (100%) | 1/2 (50%) | 2/2 (100%) |
| версия | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| дефис | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| смешанные буквы | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| раздельно | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| е вместо э | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| вендор + продукт | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| преемник | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| без дефиса | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| смешанное | 1/1 (100%) | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| слитно + транслит | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |

## Все запросы

| вендор | запрос | вид | ожидаем | строго | в 4 | место | первая цель | маршрут |
|---|---|---|---|---|---|---|---|---|
| astralinux | астра линукс | транслит | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | астры линукс | склонение | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | для астра линукс | склонение | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | astra linux special edition | полное название | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | A |
| astralinux | astralinux | слитно | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | астра линукс се | сокращение | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | фыекф дштгч | раскладка | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | астра линкус | опечатка | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | astra linux воронеж | редакция | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | A |
| astralinux | астра линукс смоленск | редакция | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | техподдержка астра линукс | атрибут лицензии | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | астра линукс для сервера | атрибут | 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | B |
| astralinux | астра | бренд | vendor | да | да | — | vendor https://store.softline.ru/astralinux | B |
| astralinux | алд про | другой продукт вендора | 287015 | да | да | 1 | family https://store.softline.ru/astralinux/ald-pro | B |
| astralinux | ald pro | другой продукт вендора | 287015 | да | да | 1 | family https://store.softline.ru/astralinux/ald-pro | A |
| astralinux | термидеск | другой продукт вендора | 299975 | да | да | 1 | family https://store.softline.ru/astralinux/termidesk | B |
| astralinux | рубэкап | другой продукт вендора | 406074 | да | да | 1 | family https://store.softline.ru/astralinux/rubackup-oneclick | B |
| astralinux | тантор | другой продукт вендора | 342351 | да | да | 1 | family https://store.softline.ru/astralinux/tantor | C |
| microsoft | офис для дома и бизнеса | жаргон | 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | B |
| microsoft | майкрософт офис для дома и бизнеса | транслит | 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | B |
| microsoft | office home and business 2024 | английское название | 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | A |
| microsoft | ms office для дома и бизнеса 2024 | сокращение | 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | B |
| microsoft | офиса для дома и бизнеса | склонение | 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | B |
| microsoft | офис для дома и учебы | соседнее семейство | 391335 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-ucheby-2024 | B |
| microsoft | office home and student | соседнее семейство | 391335 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-ucheby-2024 | C |
| microsoft | microsoft 365 | полное название | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | B |
| microsoft | майкрософт 365 | транслит | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | B |
| microsoft | офис 365 | жаргон | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | B |
| microsoft | office 365 | прежнее название | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | B |
| microsoft | 365 для семьи | уточнение | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | A |
| microsoft | м365 | сокращение | 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | B |
| microsoft | 365 персональный | соседнее семейство | 283494 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-personal | A |
| microsoft | visio | две редакции | 391350,391347 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-visio-standard-2024 | A |
| microsoft | ms project | две редакции | 391344,391341 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-project-standard-2024 | B |
| microsoft | windows 11 | две редакции | 286676,310176 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | A |
| microsoft | виндовс 11 про | транслит | 286676 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | B |
| kaspersky | касперский small office security | смешанное написание | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | касперского смолл офис секьюрити | склонение + транслит | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | kaspersky small office | неполное название | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | ksos | сокращение | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | kaspersky smal ofice security | опечатка | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | продление kaspersky small office security | атрибут лицензии | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | касперский смол офис на 10 пк | атрибут количества | 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | B |
| kaspersky | касперский | бренд | vendor | да | да | — | vendor https://store.softline.ru/kaspersky | B |
| kaspersky | kaspersky | бренд | vendor | да | да | — | vendor https://store.softline.ru/kaspersky | B |
| kaspersky | касперского | склонение | vendor | да | да | — | vendor https://store.softline.ru/kaspersky | B |
| kaspersky | каспер | жаргон | vendor | да | да | — | vendor https://store.softline.ru/kaspersky | B |
| kaspersky | лаборатория касперского | вендор | vendor | да | да | — | vendor https://store.softline.ru/kaspersky | B |
| kaspersky | kaspersky endpoint security | линейка | vendor_category | да | да | 1 | vendor_category https://store.softline.ru/kaspersky?filter[category][]=650 | B |
| kaspersky | касперский эндпоинт стандартный | соседнее семейство | 3953 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-standartnyiy | B |
| kaspersky | kes расширенный | соседнее семейство | 3935 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-rasshirennyiy | B |
| kaspersky | kaspersky total security для бизнеса | соседнее семейство | 3971 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-total-security-dlya-biznesa | B |
| kaspersky | касперский для почтовых серверов | соседнее семейство | 3707 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-security-dlya-pochtovyih-serverov | B |
| kaspersky | kaspersky endpoint security cloud | соседнее семейство | 4052 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-cloud | B |
| kaspersky | kaspersky standard | соседнее семейство | 341252 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-standard | B |
| kripto-pro | криптопро csp | слитно | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | крипто про цсп | транслит | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | cryptopro csp 5.0 | английское написание | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | криптопро csp 5 | версия | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | крипто-про csp | дефис | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | rhbgnjghj csp | раскладка | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | лицензия криптопро csp | атрибут | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | крипто про сsp 5.0 | смешанные буквы | 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | B |
| kripto-pro | криптопро | бренд | vendor | да | да | — | vendor https://store.softline.ru/kripto-pro | B |
| kripto-pro | крипто про | бренд | vendor | да | да | — | vendor https://store.softline.ru/kripto-pro | B |
| kripto-pro | cryptopro | бренд | vendor | да | да | — | vendor https://store.softline.ru/kripto-pro | B |
| kripto-pro | криптопро jcp | соседнее семейство | 22211 | да | да | 1 | family https://store.softline.ru/kripto-pro/kripto-pro-jcp | B |
| kripto-pro | криптопро ngate | соседнее семейство | 167189 | да | да | 1 | family https://store.softline.ru/kripto-pro/kriptopro-ngate | B |
| kripto-pro | криптопро tsp | соседнее семейство | 22181 | да | да | 1 | family https://store.softline.ru/kripto-pro/kripto-pro-tsp | B |
| kripto-pro | криптопро csp tls | соседнее семейство | 385280 | да | да | 1 | family https://store.softline.ru/kripto-pro/kriptopro-csp-tls | B |
| anydesk | энидеск | транслит | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| anydesk | эни деск | раздельно | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| anydesk | any desk | раздельно | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| anydesk | anydesk лицензия | атрибут | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| anydesk | энидекс | опечатка | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| anydesk | фтнвуыл | раскладка | 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | B |
| cyberprotect | кибербэкап | слитно | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| cyberprotect | кибер бекап | е вместо э | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| cyberprotect | cyber backup | английское написание | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| cyberprotect | киберпротект бэкап | вендор + продукт | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| cyberprotect | кибер бэкапа | склонение | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| cyberprotect | кибер бэкап расширенная редакция | соседнее семейство | 173615 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-rasshirennaya | B |
| cyberprotect | кибер бэкап стандартная | соседнее семейство | 173614 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap | B |
| cyberprotect | кибер бэкап старт | соседнее семейство | 444870 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-start | B |
| cyberprotect | кибер бэкап облачный | соседнее семейство | 442025 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-oblacnyi | B |
| cyberprotect | кибер хранилище | соседнее семейство | 412674 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-khranilishche | A |
| cyberprotect | кибер файлы | соседнее семейство | 394620 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-fayly | A |
| cyberprotect | киберпротект | бренд | vendor | да | да | — | vendor https://store.softline.ru/cyberprotect | B |
| cyberprotect | акронис | преемник | vendor_category | да | да | — | vendor_category https://store.softline.ru/cyberprotect?filter[category][]=1033 | B |
| npo-echelon | сканер вс | без дефиса | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | B |
| npo-echelon | сканервс | слитно | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | B |
| npo-echelon | skaner vs | транслит | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | B |
| npo-echelon | эшелон сканер-вс | вендор + продукт | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | A |
| npo-echelon | сканера вс | склонение | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | B |
| npo-echelon | сканер-вс 6 | версия | 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | A |
| npo-echelon | эшелон | бренд | vendor | да | да | — | vendor https://store.softline.ru/npo-echelon | A |
| npo-echelon | ак-вс 3 | соседнее семейство | 411254 | да | да | 1 | family https://store.softline.ru/npo-echelon/pk-ak-vs-3 | A |
| webinar | мтс линк | полное название | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | mts link | транслит | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | мтслинк | слитно | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | вебинар | прежнее название | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | webinar | прежнее название | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | мтс линка | склонение | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | мтс вебинары | смешанное | vendor | да | да | — | vendor https://store.softline.ru/webinar | B |
| webinar | мтс линк про | соседнее семейство | 447243 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-pro | B |
| webinar | мтс линк стандарт | соседнее семейство | 447242 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-standart | B |
| positive-technologies | макс патрол | транслит | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | B |
| positive-technologies | maxpatrol | слитно | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | A |
| positive-technologies | макспатрол | слитно + транслит | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | B |
| positive-technologies | max patrol epp | сокращение | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | A |
| positive-technologies | maxpatrol endpoint protection | неполное название | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | A |
| positive-technologies | макс патрола | склонение | 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | B |
| positive-technologies | xspider | соседнее семейство | 28574,428013 | да | да | 1 | family https://store.softline.ru/positive-technologies/positive-technologies-xspider | A |
| positive-technologies | позитив технолоджис | бренд | vendor | да | да | — | vendor https://store.softline.ru/positive-technologies | B |
