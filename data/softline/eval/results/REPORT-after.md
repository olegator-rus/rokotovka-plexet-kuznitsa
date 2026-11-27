# Оценка режима подбора

Каталог: фид заказчика · коммит 819922a+dirty · прогон …23c12d · диалогов 48 (по вендорам 36, отрицательных 12) · прогонов 3.

Команда: `go run . guide-eval --runs 3 --report ../data/softline/eval/results/REPORT-after.md --out <папка>`

## Гейт — **пройден**

| условие | полный подбор | на чём проверено |
|---|---|---|
| выдуманные идентификаторы офферов и фактов | 0 | 0 из 461 |
| выдуманные или неверные цены и суммы | 0 | 0 из 291 |
| нарушения правил лицензий у подходящих карточек | 0 | 0 из 198 |
| утверждения о свойстве без факта | 0 из 110 | 0 из 110 |

## Задачи

| вариант | решено 3 из 3 | по вендорам | отрицательные | решено по прогонам |
|---|---|---|---|---|
| поиск без подбора | 11 из 48 (один прогон) | 8 из 36 | 3 из 12 | 11 из 48 |
| модель с поиском без проверок (эмуляция) | 47 из 48 | 35 из 36 | 12 из 12 | 143 из 144 |
| полный подбор | 48 из 48 | 36 из 36 | 12 из 12 | 144 из 144 |

Задача решена — во всех прогонах выполнены ожидания каждого хода (тип ответа, вопрос, подходящее семейство, блок «Не подходит», факты, «по запросу», изменения) и ни одна автопроверка не нарушена.

## Автопроверки на ответе

| проверка | поиск без подбора | без проверок (эмуляция) | полный подбор |
|---|---|---|---|
| У каждого утверждения о свойстве есть факт | — | 0 из 110 | 0 из 110 |
| Нет фактов ниже порога показа | — | 0 из 218 | 0 из 218 |
| Нет запрещённых вызовов | — | 0 из 618 | 0 из 618 |
| Подходящие карточки — из семейств задачи | — | 0 из 198 | 0 из 198 |
| Идентификаторы офферов и фактов существуют | 0 из 236 | 0 из 461 | 0 из 461 |
| Во фразе нет чисел, которых нет в карточках и условиях | — | 0 из 180 | 0 из 180 |
| Во фразе нет запрещённых слов | — | 0 из 180 | 0 из 180 |
| Правила лицензий не нарушены | 5 из 59 | 0 из 198 | 0 из 198 |
| Цена и сумма в карточке совпадают с каталогом | 0 из 236 | 0 из 291 | 0 из 291 |
| Вопросов в диалоге не больше допустимого | — | 0 из 144 | 0 из 144 |
| Вызовов модели и времени хода не больше предела | — | 0 из 213 | 0 из 213 |

Число — нарушений из проверенного (карточек, фактов, ходов, диалогов). У поиска проверены первые четыре карточки и правила лицензий у первой карточки при требованиях, которые разобрал подбор; вопросов, фраз и утверждений у поиска нет.

Сняли проверки до показа (по классам, все прогоны): lead — 1.

- Эмуляция: тот же ход подбора, к показанному добавлено то, что сняли проверки до показа (guide_turns.removed). Карточки, цены и артикулы и в этом варианте строит код: выдуманной цены модель дать не может, поэтому «цены» здесь не отличаются от полного подбора.

## Время и расход полного подбора

| момент | p50, мс | p95, мс |
|---|---|---|
| первый вывод (офферы собраны) | 2 | 139 |
| проверенные строки | 0 | 244 |
| весь ход | 56 | 1318 |
| весь ход с вызовом модели (9 ходов) | 2200 | 3782 |

Ходов 213; по журналу `guide_turns` — 213 строк: вызовов модели 9, токенов на входе 6 294, на выходе 494, 0,7282 ₽. По `llm_usage` (request_id хода): вызовов 9, 0,7282 ₽. Стоимость диалога в среднем — **0,0051 ₽** (144 диалогов-прогонов); диалога, где модель вызывалась (9), — 0,0809 ₽.

## Ошибки

поиск без подбора:

- `astralinux_narrow`, прогон 1, ход 1 — первая карточка поиска — семейство 365726
- `astralinux_clarify`, прогон 1, ход 1 — нет в поиске: вопрос, сравнение, факты, «не подходит» или запрос КП
- `astralinux_change`, прогон 1, ход 2 — чипа и правки условия в поиске нет
- `microsoft_narrow`, прогон 1, ход 1 — первая карточка поиска — семейство offer-406514
- `microsoft_clarify`, прогон 1, ход 2 — первая карточка поиска — семейство offer-428243
- `microsoft_change`, прогон 1, ход 2 — чипа и правки условия в поиске нет

## Диалоги полного подбора

| диалог | тип | прогоны | типы ответов (прогон 1) | вызовов модели | причина |
|---|---|---|---|---|---|
| astralinux_narrow | narrow | 3 из 3 | selection | 0 |  |
| astralinux_clarify | clarify | 3 из 3 | clarification → comparison → selection | 0 |  |
| astralinux_change | change | 3 из 3 | selection → selection | 0 |  |
| astralinux_trap | trap | 3 из 3 | clarification | 0 |  |
| microsoft_narrow | narrow | 3 из 3 | selection | 0 |  |
| microsoft_clarify | clarify | 3 из 3 | selection → selection | 0 |  |
| microsoft_change | change | 3 из 3 | selection → selection | 0 |  |
| microsoft_trap | trap | 3 из 3 | selection → selection | 0 |  |
| kaspersky_narrow | narrow | 3 из 3 | selection | 0 |  |
| kaspersky_clarify | clarify | 3 из 3 | clarification → selection → comparison → selection | 3 |  |
| kaspersky_change | change | 3 из 3 | selection → selection | 0 |  |
| kaspersky_trap | trap | 3 из 3 | selection | 0 |  |
| kripto-pro_narrow | narrow | 3 из 3 | selection | 0 |  |
| kripto-pro_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| kripto-pro_change | change | 3 из 3 | selection → selection | 0 |  |
| kripto-pro_trap | trap | 3 из 3 | selection → selection | 0 |  |
| anydesk_narrow | narrow | 3 из 3 | clarification → selection | 0 |  |
| anydesk_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| anydesk_change | change | 3 из 3 | selection → selection → selection | 0 |  |
| anydesk_trap | trap | 3 из 3 | clarification | 0 |  |
| cyberprotect_narrow | narrow | 3 из 3 | selection | 0 |  |
| cyberprotect_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| cyberprotect_change | change | 3 из 3 | selection → selection | 3 |  |
| cyberprotect_trap | trap | 3 из 3 | selection | 0 |  |
| npo-echelon_narrow | narrow | 3 из 3 | selection | 0 |  |
| npo-echelon_clarify | clarify | 3 из 3 | selection → selection | 0 |  |
| npo-echelon_change | change | 3 из 3 | selection → selection | 3 |  |
| npo-echelon_trap | trap | 3 из 3 | selection | 0 |  |
| webinar_narrow | narrow | 3 из 3 | selection | 0 |  |
| webinar_clarify | clarify | 3 из 3 | no_data | 0 |  |
| webinar_change | change | 3 из 3 | no_data → no_data | 0 |  |
| webinar_trap | trap | 3 из 3 | selection | 0 |  |
| positive-technologies_narrow | narrow | 3 из 3 | selection | 0 |  |
| positive-technologies_clarify | clarify | 3 из 3 | no_data | 0 |  |
| positive-technologies_change | change | 3 из 3 | selection → selection | 0 |  |
| positive-technologies_trap | trap | 3 из 3 | selection | 0 |  |
| negative_01 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_02 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_03 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_04 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_05 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_06 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_07 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_08 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_09 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_10 | negative | 3 из 3 | clarification | 0 |  |
| negative_11 | negative | 3 из 3 | clarification | 0 |  |
| negative_12 | negative | 3 из 3 | no_data | 0 |  |
