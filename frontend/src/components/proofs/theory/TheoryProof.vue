<script setup lang="ts">
// Часть 4 «Как мы это проверяем»: метрики (eval_metrics.go, eval.go, eval_contribution.go, eval_judge.go,
// evalsets_split.go), стоимость (costs.go, costs_month.go, llmusage.go), деградация (search_modes.go,
// llmguard.go, ARCHITECTURE §1), решения (ARCHITECTURE §6, §8) и словарь терминов. Чисел качества и денег
// здесь нет — только ссылки на вкладки, где они посчитаны.
import { RouterLink } from 'vue-router'
import { THEORY_PARAMS as P, pct, ru } from '@/lib/theoryParams'
import DiagramLadder from './diagrams/DiagramLadder.vue'
import TheoryFormula from './TheoryFormula.vue'
import TheorySection from './TheorySection.vue'

const OFFLINE = [
  [
    'LLM выключена, нет ключа или кончился бюджет',
    'Маршрут D работает на правилах и кэше разборов; причина — в трассе. Агент не запускается.',
  ],
  [
    'Не отвечает сервис моделей',
    'Шаги «Векторный поиск» и «Реранкер» пропущены с причиной; выдача — на тексте и правилах.',
  ],
  [
    'Режим «без моделей»',
    'Одна настройка выключает и LLM, и векторы — поиск по тексту, правилам и кэшу.',
  ],
  ['Лежит PostgreSQL', 'Поиск отвечает, но не пишет журнал.'],
  ['Лежит OpenSearch', 'Поиск отвечает ошибкой «сервис недоступен»: без индекса искать негде.'],
]

const GLOSSARY = [
  [
    'BM25',
    'Классическая формула текстового поиска: чем больше редких слов запроса в документе, тем выше он.',
  ],
  [
    'Вектор, эмбеддинг',
    'Текст, превращённый моделью в набор чисел так, что близкие по смыслу тексты оказываются рядом.',
  ],
  [
    'kNN, HNSW',
    'Поиск ближайших векторов; HNSW — граф соседей, который делает этот поиск быстрым.',
  ],
  ['RRF', 'Reciprocal Rank Fusion: слияние списков по местам, а не по оценкам.'],
  [
    'Реранкер',
    'Модель, которая читает запрос и карточку вместе и заново оценивает короткий список кандидатов.',
  ],
  ['LLM', 'Большая языковая модель. Здесь — разбор трудных запросов на маршруте D и агент.'],
  [
    'Семейство',
    'Продукт со всеми вариантами лицензий; одна карточка в выдаче. На фиде — группа предложений магазина, у техники — сам товар.',
  ],
  [
    'Фид, предложение',
    'Фид — выгрузка каталога магазина. Предложение — одна строка фида: конкретная лицензия или товар со своей ценой.',
  ],
  [
    'Ступень количества',
    'Цена за единицу, которая действует в диапазоне: «от 100 до 149 узлов». Закрывает любое число внутри диапазона.',
  ],
  [
    'Маршрут A–D',
    'Ветка обработки, которую правила выбирают для запроса: A — точный, B — нормализованный, C — семантический, D — с разбором LLM.',
  ],
  ['Алиас', 'Запись словаря «как пишут → как в каталоге»: «винда» → Windows.'],
  ['Трасса', 'Разбор одного запроса: шаги, решения, отсечённое с причинами.'],
  ['NDCG@10', 'Насколько порядок первых десяти позиций близок к идеальному по разметке.'],
  ['MRR@10', 'Насколько высоко стоит первая подходящая позиция.'],
  [
    'Ложное спасение',
    'Товара нет, а поиск показал что-то без пометки «нет в каталоге» или «аналог».',
  ],
  ['Песочница', 'Прогон правки агента на копии правил: боевой поиск она не трогает.'],
]
</script>

<template>
  <TheorySection id="quality">
    <p>
      Сначала о данных. Метрики на вкладках «Метрики» и «Вклад компонентов» получены на
      mock-каталоге из 319 позиций и тестовых наборах команды, разметку собрали правилами. По этим
      числам можно сравнивать версии поиска между собой, но качество на фиде заказчика они не
      предсказывают. Первый замер на данных заказчика уже сделан: сверка выдачи с эталонами
      заказчика и примерами из ТЗ. Его числа меняются вместе с поиском, поэтому здесь их нет.
    </p>
    <p>
      Каждый прогон отправляет одни и те же запросы в тот же сервис поиска, что отвечает покупателю.
      Ниже описано, как это устроено; сами числа — на вкладках
      <RouterLink
        :to="{ path: '/proofs', query: { tab: 'compare', view: 'metrics' } }"
        class="theory-link"
        >«Метрики»</RouterLink
      >
      и
      <RouterLink :to="{ path: '/proofs', query: { tab: 'contribution' } }" class="theory-link"
        >«Вклад компонентов»</RouterLink
      >.
    </p>
    <ul>
      <li>
        <strong>Наборы.</strong> Запросы делятся на tune, test и regression в пропорции
        {{ pct(P.shareTune) }} / {{ pct(P.shareTest) }} / {{ pct(P.shareRegression) }} — по
        кластерам, а не по строкам: «тимвьюер» и «teamviewer» всегда в одном наборе, иначе настройка
        подглядывала бы в test через соседнюю формулировку. Настраиваемся только на tune; test
        заморожен, его хэш пишется в каждый прогон.
      </li>
      <li>
        <strong>Разметка</strong> — оценка пары «запрос — товар» от 0 до 3. Подходящим считается
        оценка от {{ ru(P.relevantGrade) }}.
      </li>
      <li>
        <strong>Метрики.</strong> NDCG@10 (польза позиции — 2<sup>оценка</sup> − 1, идеальный
        порядок — по семействам), MRR@10, Recall@50, точные запросы на первом месте, доля пустых
        выдач, ложное спасение, нарушения условий лицензии, время ответа P50 и P95.
      </li>
      <li>
        <strong>Пустые выдачи — только рядом с ложным спасением.</strong> Векторный поиск без порога
        обнуляет пустые выдачи, показывая что угодно. Одна метрика без другой вводит в заблуждение.
      </li>
      <li>
        <strong>LLM-судья.</strong> Разметку перепроверяет модель другого производителя, вслепую —
        не зная версии поиска и места товара. Её согласие с разметкой публикуется рядом с метриками.
      </li>
    </ul>
    <TheoryFormula
      kind="настройка"
      :label="`NDCG@${P.evalTopK} равно DCG@${P.evalTopK}, делённому на IDCG@${P.evalTopK}. DCG@${P.evalTopK} — сумма по местам i от 1 до ${P.evalTopK} дробей: два в степени оценки g i минус один, делённое на двоичный логарифм от i плюс один.`"
    >
      <math displaystyle="true">
        <mtext>NDCG@{{ P.evalTopK }}</mtext
        ><mo>=</mo>
        <mfrac>
          <mtext>DCG@{{ P.evalTopK }}</mtext>
          <mtext>IDCG@{{ P.evalTopK }}</mtext>
        </mfrac>
        <mo>,</mo>
      </math>
      <math displaystyle="true">
        <mtext>DCG@{{ P.evalTopK }}</mtext
        ><mo>=</mo>
        <munderover>
          <mo>∑</mo>
          <mrow><mi>i</mi><mo>=</mo><mn>1</mn></mrow>
          <mn>{{ P.evalTopK }}</mn>
        </munderover>
        <mfrac>
          <mrow
            ><msup
              ><mn>2</mn><msub><mi>g</mi><mi>i</mi></msub></msup
            ><mo>−</mo><mn>1</mn></mrow
          >
          <mrow>
            <msub><mi>log</mi><mn>2</mn></msub
            ><mo stretchy="false">(</mo><mi>i</mi><mo>+</mo><mn>1</mn><mo stretchy="false">)</mo>
          </mrow>
        </mfrac>
      </math>
      <template #where>
        <div>
          <dt>
            <math><mi>i</mi></math>
          </dt>
          <dd>— место в выдаче, считая с 1;</dd>
        </div>
        <div>
          <dt>
            <math
              ><msub><mi>g</mi><mi>i</mi></msub></math
            >
          </dt>
          <dd>
            — оценка разметки для товара на месте <math><mi>i</mi></math
            >, от 0 до 3;
          </dd>
        </div>
        <div>
          <dt>
            <math
              ><mtext>IDCG@{{ P.evalTopK }}</mtext></math
            >
          </dt>
          <dd>
            — DCG идеального порядка: лучшая оценка каждого размеченного семейства, по убыванию.
          </dd>
        </div>
      </template>
    </TheoryFormula>
    <TheoryFormula
      kind="настройка"
      :label="`MRR@${P.evalTopK} равно среднему по запросам Q величины один, делённое на место первой подходящей позиции rank q.`"
    >
      <math displaystyle="true">
        <mtext>MRR@{{ P.evalTopK }}</mtext
        ><mo>=</mo>
        <mfrac>
          <mn>1</mn>
          <mrow><mo>|</mo><mi>Q</mi><mo>|</mo></mrow>
        </mfrac>
        <munder>
          <mo>∑</mo>
          <mrow><mi>q</mi><mo>∈</mo><mi>Q</mi></mrow>
        </munder>
        <mfrac>
          <mn>1</mn>
          <msub><mi>rank</mi><mi>q</mi></msub>
        </mfrac>
      </math>
      <template #where>
        <div>
          <dt>
            <math><mi>Q</mi></math>
          </dt>
          <dd>— запросы, у которых в разметке есть подходящий товар;</dd>
        </div>
        <div>
          <dt>
            <math
              ><msub><mi>rank</mi><mi>q</mi></msub></math
            >
          </dt>
          <dd>
            — место первой позиции с оценкой от {{ ru(P.relevantGrade) }} среди первых
            {{ ru(P.evalTopK) }}; если такой нет, слагаемое равно 0.
          </dd>
        </div>
      </template>
    </TheoryFormula>
    <p>
      Лестница версий поиска B0–B4: B0 — исходный поиск магазина по подстроке, каждая следующая
      версия добавляет к предыдущей один слой. На витрине работает B4 — так написано в трассе.
    </p>
    <DiagramLadder />
    <p>
      «Вклад компонентов» идёт с другой стороны: из полного пути выключается по одному компоненту —
      векторы, LLM, все модели сразу — и видно, что каждый даёт и чего стоит по времени.
    </p>
  </TheorySection>

  <TheorySection id="cost">
    <p>
      Деньги тратит только внешняя модель, и только в двух местах: разбор на маршруте D при промахе
      кэша и агент. Эмбеддер и реранкер работают на своём сервере — это часть стоимости сервера, а
      не запроса. Каждый вызов модели записывается: токены, задержка, версия прайса, стоимость по
      прайсу.
    </p>
    <TheoryFormula
      label="Расход за месяц равен стоимости сервера плюс число запросов, умноженное на долю маршрута D, на единицу минус доля попаданий в кэш и на стоимость разбора, плюс число запусков агента, умноженное на стоимость запуска."
    >
      <math displaystyle="true">
        <msub><mi>C</mi><mtext>мес</mtext></msub
        ><mo>=</mo><msub><mi>C</mi><mtext>сервер</mtext></msub>
      </math>
      <math displaystyle="true">
        <mo form="infix" lspace="0">+</mo><mi>N</mi><mo>⋅</mo><msub><mi>p</mi><mi>D</mi></msub
        ><mo>⋅</mo><mo stretchy="false">(</mo><mn>1</mn><mo>−</mo><mi>h</mi
        ><mo stretchy="false">)</mo><mo>⋅</mo><msub><mi>c</mi><mtext>разбор</mtext></msub>
      </math>
      <math displaystyle="true">
        <mo form="infix" lspace="0">+</mo><msub><mi>N</mi><mtext>агент</mtext></msub
        ><mo>⋅</mo><msub><mi>c</mi><mtext>запуск</mtext></msub>
      </math>
      <template #where>
        <div>
          <dt>
            <math
              ><msub><mi>C</mi><mtext>сервер</mtext></msub></math
            >
          </dt>
          <dd>— сервер, вместе с эмбеддером и реранкером;</dd>
        </div>
        <div>
          <dt>
            <math><mi>N</mi></math>
          </dt>
          <dd>— запросов за месяц;</dd>
        </div>
        <div>
          <dt>
            <math
              ><msub><mi>p</mi><mi>D</mi></msub></math
            >
          </dt>
          <dd>— доля запросов на маршруте D;</dd>
        </div>
        <div>
          <dt>
            <math><mi>h</mi></math>
          </dt>
          <dd>— доля попаданий в кэш разборов;</dd>
        </div>
        <div>
          <dt>
            <math
              ><msub><mi>c</mi><mtext>разбор</mtext></msub></math
            >
          </dt>
          <dd>— стоимость одного разбора моделью;</dd>
        </div>
        <div>
          <dt>
            <math
              ><msub><mi>N</mi><mtext>агент</mtext></msub
              ><mo>,</mo><msub><mi>c</mi><mtext>запуск</mtext></msub></math
            >
          </dt>
          <dd>— запусков агента за месяц и стоимость одного запуска.</dd>
        </div>
      </template>
    </TheoryFormula>
    <p>
      Доля маршрута D, доля кэша и стоимость разбора — замеры из журналов, сервер и число запусков
      агента — допущения. Все величины с источником каждой — на вкладке
      <RouterLink :to="{ path: '/proofs', query: { tab: 'cost' } }" class="theory-link"
        >«Стоимость»</RouterLink
      >.
    </p>
  </TheorySection>

  <TheorySection id="offline">
    <p>
      Поиск рассчитан на то, что любая внешняя часть может отказать. Сбой шага не роняет ответ: шаг
      пропускается, причина пишется в трассу.
    </p>
    <div class="data-scroll">
      <table class="data-table theory-table">
        <thead>
          <tr>
            <th scope="col">Что случилось</th>
            <th scope="col">Что делает поиск</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in OFFLINE" :key="r[0]">
            <th scope="row">{{ r[0] }}</th>
            <td>{{ r[1] }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p>
      Режимы LLM и векторов переключаются без перезапуска. Как меняется качество без моделей —
      строка «без моделей» на вкладке
      <RouterLink :to="{ path: '/proofs', query: { tab: 'contribution' } }" class="theory-link"
        >«Вклад компонентов»</RouterLink
      >.
    </p>
  </TheorySection>

  <TheorySection id="why">
    <ul>
      <li>
        <strong>OpenSearch, а не Elasticsearch.</strong> Гибридный поиск и kNN в OpenSearch — под
        Apache 2.0. Слияние RRF мы всё равно считаем в своём коде, поэтому выбор движка от лицензии
        не зависит.
      </li>
      <li>
        <strong>LLM только на маршруте D.</strong> Модель нужна запросам со смыслом, а не «windows
        11 pro». Остальное быстрее и предсказуемее решают правила, и без внешнего сервиса поиск
        продолжает работать.
      </li>
      <li>
        <strong>Модель возвращает структуру.</strong> Всё, что она говорит, сверяется со
        справочниками, а запрос к движку строит код. Инструкция, спрятанная в запросе покупателя,
        ничего не выполнит.
      </li>
      <li>
        <strong>Порог вместо «показать хоть что-то».</strong> Если подходящего нет, покупатель видит
        «нет в каталоге» и подписанные аналоги, а не случайную выдачу. Запрос остаётся в журнале как
        неудовлетворённый спрос.
      </li>
      <li>
        <strong>Агент не применяет правки сам.</strong> Расследует и проверяет он без человека, но
        правка словаря меняет выдачу всем, поэтому её утверждает сотрудник; у каждой правки есть
        версия и откат.
      </li>
      <li>
        <strong>Свои модели эмбеддингов</strong> — пороги настроены на них, данные не уходят наружу.
      </li>
    </ul>
    <h4>Чего пока нет</h4>
    <ul>
      <li>Обучения ранжированию: веса бизнес-слоя — стартовая конфигурация.</li>
      <li>
        Полной оценки на фиде заказчика: наборы tune, test и regression и разметка построены на
        mock-каталоге. На фиде пока есть только первый замер по эталонам заказчика.
      </li>
      <li>
        Распознавания имён и адресов при маскировании персональных данных — только номера и почта.
      </li>
    </ul>
  </TheorySection>

  <TheorySection id="glossary">
    <dl class="glossary">
      <div v-for="g in GLOSSARY" :key="g[0]" class="glossary__row">
        <dt>{{ g[0] }}</dt>
        <dd>{{ g[1] }}</dd>
      </div>
    </dl>
  </TheorySection>
</template>

<style scoped>
.glossary {
  display: grid;
  gap: var(--space-2);
  margin: 0;
}
.glossary__row {
  display: grid;
  gap: var(--space-1);
  padding-bottom: var(--space-2);
  border-bottom: var(--border-w) solid var(--border);
}
.glossary dt {
  font-weight: var(--fw-semibold);
}
.glossary dd {
  margin: 0;
  color: var(--text-2);
}
@media (min-width: 768px) {
  .glossary__row {
    grid-template-columns: 12rem minmax(0, 1fr);
  }
}
</style>
