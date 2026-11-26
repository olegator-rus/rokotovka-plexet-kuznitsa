<script setup lang="ts">
// Часть 1 «Общая картина»: из чего собрано решение и путь запроса целиком (search_b3.go, ARCHITECTURE §1–2).
import { ArrowDown } from 'lucide-vue-next'
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import TheoryFlow, { type FlowStep } from './TheoryFlow.vue'
import TheorySection from './TheorySection.vue'

const PARTS = [
  {
    name: 'OpenSearch',
    text: 'Индекс каталога: текстовый поиск BM25 и векторы. Новый импорт строит следующий индекс и переключает на него алиас одним действием — поиск во время импорта идёт по старому.',
  },
  {
    name: 'Сервис моделей',
    text: 'Свой, на своём сервере: BGE-M3 превращает текст в вектор, bge-reranker-v2-m3 заново оценивает первые семейства выдачи на маршрутах C и D. Каталог и запросы ради векторов наружу не уходят.',
  },
  {
    name: 'PostgreSQL',
    text: 'Журнал поисков и действий покупателей, трассы, кэш разборов LLM, учёт каждого вызова модели, версии правил, запуски агента.',
  },
  {
    name: 'Yandex AI Studio',
    text: 'Внешняя LLM (Alice AI LLM Flash). Её зовут только две вещи: разбор сложного запроса на маршруте D при промахе кэша и агент.',
    external: true,
  },
]

const STEPS: FlowStep[] = [
  { title: 'Нормализация', text: 'Регистр, раскладка, опечатки, сленг — к словам каталога.' },
  {
    title: 'Условия и названия',
    text: 'Количество, метрика, срок, продление; вендор, продукт, категория; исключения.',
  },
  { title: 'Маршрут', text: 'A, B, C или D — по сигналам в запросе.' },
  {
    title: 'Разбор намерения LLM',
    text: 'Только на маршруте D: сначала кэш, при промахе — модель.',
    tone: 'd',
    tag: 'только D',
  },
  {
    title: 'BM25 и векторы параллельно',
    text: `Два независимых списка, по ${P.candidatesTop} кандидатов в каждом.`,
  },
  { title: 'Слияние RRF', text: 'Из двух списков — один, по местам в списках.' },
  {
    title: 'Семейства, порог, условия',
    text: 'Позиции собираются в продукты; слабое и неподходящее отсекается с причиной.',
  },
  {
    title: 'Реранкер',
    text: `Модель заново оценивает первые ${P.rerankTop} семейств.`,
    tone: 'muted',
    tag: 'C и D',
  },
  {
    title: 'Группы и бизнес-слой',
    text: 'Точное — подходящее — аналог; внутри группы — итоговый скор.',
  },
  {
    title: 'Если пусто',
    text: 'Плашка «Товара нет в каталоге», подписанные аналоги, кнопка «Запросить поставку».',
    tone: 'muted',
  },
  { title: 'Ответ', text: 'Выдача, чипы «как мы поняли», дополнения, трасса.' },
]
</script>

<template>
  <TheorySection id="overview">
    <p>
      <strong>Поиск</strong> отвечает покупателю, пока тот ждёт: разбирает запрос, находит
      кандидатов, расставляет их и для каждой позиции записывает, почему она здесь.
      <strong>Агент SearchOps</strong> работает в фоне: читает журнал неудачных поисков, ищет
      причину и предлагает правку словаря, проверив её прогоном на копии правил. На витрине правка
      появляется, только когда сотрудник нажмёт «Утвердить» в бэк-офисе: она меняет выдачу всем
      покупателям сразу.
    </p>
    <figure class="ov">
      <figcaption class="sr-only">
        Из чего собрано решение: Go API в центре, к нему OpenSearch, сервис моделей, PostgreSQL и
        внешняя LLM
      </figcaption>
      <div class="ov__top">
        <strong>Покупатель или сайт магазина</strong>
      </div>
      <span class="ov__arrow" aria-hidden="true"><ArrowDown :size="16" /></span>
      <div class="ov__hub">
        <strong>Go API</strong>
        <span class="t-sm t-muted">
          Один процесс: поиск, подсказки, события, оценка, агент, учёт стоимости. Всё считает сервер
          — фронт только показывает.
        </span>
      </div>
      <span class="ov__arrow" aria-hidden="true"><ArrowDown :size="16" /></span>
      <ul class="ov__grid" role="list">
        <li v-for="p in PARTS" :key="p.name" :class="['ov__part', { 'ov__part--ext': p.external }]">
          <strong>{{ p.name }}</strong>
          <span v-if="p.external" class="badge">внешняя</span>
          <p class="t-sm t-muted">{{ p.text }}</p>
        </li>
      </ul>
    </figure>
    <p>
      Основную работу делают правила, словари каталога и два классических метода поиска, BM25 и
      векторы. Поэтому без внешней модели поиск продолжает отвечать; что именно при этом
      отключается, описано в разделе «Что работает, если отказала модель или база».
    </p>
  </TheorySection>

  <TheorySection id="pipeline" :tries="['винда 11 про', 'аналог фотошопа на 20 пользователей']">
    <p>
      Шаги идут в том порядке, в каком их выполняет код. Каждый из них записывается в трассу
      запроса.
    </p>
    <TheoryFlow
      :steps="STEPS"
      numbered
      label="Путь запроса: одиннадцать шагов от нормализации до ответа"
    />
    <p>
      Не каждый шаг работает на каждом запросе. Если шаг не понадобился, он остаётся в трассе с
      пометкой «пропущен» и причиной: например, «запрос не на маршруте D» у разбора намерения. По
      трассе всегда видно, звали ли модель.
    </p>
    <p>
      У шагов есть сроки. Весь поиск — не дольше {{ ru(P.searchLimitSec) }} с. Вектор запроса ждут
      {{ ru(P.queryEmbedLimitSec) }} с: если сервис моделей не ответил, векторная ветка
      пропускается, и выдача строится на тексте. Вызов модели на маршруте D ограничен
      {{ ru(P.intentCallTimeoutSec) }} с, после чего разбор откатывается на правила.
    </p>
  </TheorySection>
</template>

<style scoped>
.ov {
  display: flex;
  flex-direction: column;
  margin: 0;
}
.ov__top,
.ov__hub {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  text-align: center;
}
.ov__hub {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.ov__arrow {
  display: flex;
  justify-content: center;
  padding: 2px 0;
  color: var(--text-3);
}
.ov__grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ov__part {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
}
.ov__part p {
  flex-basis: 100%;
  margin: 0;
}
.ov__part--ext {
  border-style: dashed;
  border-color: var(--border-control);
}

@media (min-width: 768px) {
  .ov__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (min-width: 1280px) {
  .ov__grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
</style>
