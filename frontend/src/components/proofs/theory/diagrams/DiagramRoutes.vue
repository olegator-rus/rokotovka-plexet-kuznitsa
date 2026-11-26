<script setup lang="ts">
// Выбор маршрута (search_core.go routeSignals): вопросы по порядку, первый ответ «да» решает исход.
import { ROUTE_LABELS } from '@/lib/trace'
import DgArrow from '../diagram/DgArrow.vue'
import DgDecision from '../diagram/DgDecision.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const Q1 = {
  title: 'Запрос — шум?',
  sub: ['разметка, SQL, меньше двух букв', 'без единого слова каталога'],
}
const Q2 = {
  title: 'Признак трудного запроса?',
  sub: [
    'незнакомые слова; «аналог», «замена»,',
    '«вместо»; длинный запрос с условиями;',
    'вопрос не о товаре; продление + бессрочно',
  ],
}
const Q3 = {
  title: 'Поиск исправил слово или нашёл условия?',
  sub: ['раскладка, опечатка, алиас, транслит,', 'достройка; количество, срок, вендор'],
}
const Q4 = { title: 'Артикул или названо семейство?', sub: 'код, похожий на артикул' }

const O1 = { title: `B · ${ROUTE_LABELS.B}`, sub: 'без модели, «товаров нет»' }
const O2 = { title: `D · ${ROUTE_LABELS.D}`, sub: 'разбор: кэш, затем LLM' }
const O3 = { title: `B · ${ROUTE_LABELS.B}`, sub: 'исправлено или есть условия' }
const O4 = { title: `A · ${ROUTE_LABELS.A}`, sub: 'названо точно' }
const O5 = { title: `C · ${ROUTE_LABELS.C}`, sub: 'названий из каталога нет' }
</script>

<template>
  <TheoryDiagram
    title="Как выбирается маршрут A–D"
    desc="Четыре вопроса по порядку, первый ответ «да» решает. Запрос — шум: маршрут B без модели, ответ «товаров нет». Есть признак трудного запроса: маршрут D, разбор кэшем, затем LLM. Поиск исправил слово или нашёл условия: маршрут B. Артикул или названо семейство: маршрут A. Иначе — маршрут C, запрос описывает задачу."
    :wide="[720, 450]"
    :narrow="[328, 680]"
  >
    <template #wide>
      <DgDecision :x="0" :y="0" :w="330" :h="70" v-bind="Q1" />
      <DgDecision :x="0" :y="100" :w="330" :h="88" v-bind="Q2" />
      <DgDecision :x="0" :y="218" :w="330" :h="70" v-bind="Q3" />
      <DgDecision :x="0" :y="318" :w="330" :h="52" v-bind="Q4" />

      <DgArrow pts="165,70 165,100" label="нет" :lx="173" :ly="85" anchor="start" />
      <DgArrow pts="165,188 165,218" label="нет" :lx="173" :ly="203" anchor="start" />
      <DgArrow pts="165,288 165,318" label="нет" :lx="173" :ly="303" anchor="start" />
      <DgArrow pts="165,370 165,423 430,423" label="нет" :lx="173" :ly="396" anchor="start" />

      <DgArrow pts="330,35 430,35" tone="b" label="да" :lx="380" :ly="26" />
      <DgArrow pts="330,144 430,144" tone="d" label="да" :lx="380" :ly="135" />
      <DgArrow pts="330,253 430,253" tone="b" label="да" :lx="380" :ly="244" />
      <DgArrow pts="330,344 430,344" tone="a" label="да" :lx="380" :ly="335" />

      <DgNode :x="430" :y="12" :w="290" :h="46" tone="b" v-bind="O1" />
      <DgNode :x="430" :y="121" :w="290" :h="46" tone="d" v-bind="O2" />
      <DgNode :x="430" :y="230" :w="290" :h="46" tone="b" v-bind="O3" />
      <DgNode :x="430" :y="321" :w="290" :h="46" tone="a" v-bind="O4" />
      <DgNode :x="430" :y="400" :w="290" :h="46" tone="c" v-bind="O5" />
    </template>

    <template #narrow>
      <DgDecision :x="0" :y="0" :w="328" :h="62" v-bind="Q1" />
      <DgArrow pts="192,62 192,84" tone="b" label="да" :lx="200" :ly="73" anchor="start" />
      <DgNode :x="56" :y="84" :w="272" :h="46" tone="b" v-bind="O1" />
      <DgArrow pts="24,62 24,152" label="нет" :lx="30" :ly="141" anchor="start" />

      <DgDecision :x="0" :y="152" :w="328" :h="78" v-bind="Q2" />
      <DgArrow pts="192,230 192,252" tone="d" label="да" :lx="200" :ly="241" anchor="start" />
      <DgNode :x="56" :y="252" :w="272" :h="46" tone="d" v-bind="O2" />
      <DgArrow pts="24,230 24,320" label="нет" :lx="30" :ly="309" anchor="start" />

      <DgDecision :x="0" :y="320" :w="328" :h="62" v-bind="Q3" />
      <DgArrow pts="192,382 192,404" tone="b" label="да" :lx="200" :ly="393" anchor="start" />
      <DgNode :x="56" :y="404" :w="272" :h="46" tone="b" v-bind="O3" />
      <DgArrow pts="24,382 24,472" label="нет" :lx="30" :ly="461" anchor="start" />

      <DgDecision :x="0" :y="472" :w="328" :h="46" v-bind="Q4" />
      <DgArrow pts="192,518 192,540" tone="a" label="да" :lx="200" :ly="529" anchor="start" />
      <DgNode :x="56" :y="540" :w="272" :h="46" tone="a" v-bind="O4" />
      <DgArrow pts="24,518 24,653 56,653" label="нет" :lx="30" :ly="608" anchor="start" />
      <DgNode :x="56" :y="630" :w="272" :h="46" tone="c" v-bind="O5" />
    </template>

    <template #caption>
      Вопросы проверяются сверху вниз, и первый ответ «да» решает маршрут. Цвет исхода — цвет
      маршрута в трассе.
    </template>
  </TheoryDiagram>
</template>
