<script setup lang="ts">
// Отбор после слияния (search_b3.go, rank.go, search_negation.go): шесть ступеней, четыре из них отсекают
// в отказы с причиной (они видны в трассе), одна добирает названное из каталога.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDot from '../diagram/DgDot.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const S1 = { title: 'Кандидаты слияния', sub: 'позиции, а не продукты' }
const S2 = {
  title: 'Только по смыслу',
  sub: ['нашли одни векторы:', `остаётся при ≥ ${ru(P.denseOnlyMin)}`],
}
const S3 = {
  title: 'Семейства и порог',
  sub: ['позиции → карточки', `порог — ${ru(P.relThreshold)} от лучшего`],
}
const S4 = { title: 'Обязательные условия', sub: ['происхождение, вендор,', 'продление, метрика'] }
const S5 = {
  title: 'Добор названного',
  sub: [`категория — до ${ru(P.namedFillMax)},`, `вендор — до ${ru(P.namedFillMax / 2)} семейств`],
}
const S6 = {
  title: 'Чужие категории',
  sub: ['и не то количество', `кроме первых ${ru(P.textTopN)} мест BM25`],
}
</script>

<template>
  <TheoryDiagram
    title="Отбор после слияния: от позиций к семействам"
    :desc="`Кандидаты после слияния — отдельные позиции. Найденное одними векторами остаётся только при близости от ${ru(P.denseOnlyMin)}. Позиции собираются в семейства, всё ниже ${ru(P.relThreshold)} от лучшего уходит в отказы. Проверяются обязательные условия. Названная категория добирается из каталога до ${ru(P.namedFillMax)} семейств, названный вендор — до ${ru(P.namedFillMax / 2)}. Чужие категории и неподходящее количество отсекаются. Всё отсечённое попадает в отказы с причиной, их видно в трассе.`"
    :wide="[720, 320]"
    :narrow="[328, 564]"
  >
    <template #wide>
      <DgNode :x="0" :y="0" :w="190" :h="70" v-bind="S1" />
      <DgArrow pts="190,35 235,35" />
      <DgNode :x="235" :y="0" :w="190" :h="70" v-bind="S2" />
      <DgArrow pts="425,35 470,35" />
      <DgNode :x="470" :y="0" :w="190" :h="70" v-bind="S3" />
      <DgArrow pts="660,35 690,35 690,211 660,211" />

      <DgNode
        :x="0"
        :y="100"
        :w="660"
        :h="40"
        kind="sink"
        title="Отказы с причиной — в трассировке"
      />
      <DgArrow pts="330,70 330,100" label="слабое сходство" :lx="338" :ly="85" anchor="start" />
      <DgArrow pts="565,70 565,100" label="ниже порога" :lx="573" :ly="85" anchor="start" />
      <DgArrow pts="565,176 565,140" label="нарушено условие" :lx="557" :ly="158" anchor="end" />
      <DgArrow pts="95,176 95,140" label="другая категория" :lx="103" :ly="158" anchor="start" />

      <DgNode :x="470" :y="176" :w="190" :h="70" v-bind="S4" />
      <DgArrow pts="470,211 425,211" />
      <DgNode :x="235" :y="176" :w="190" :h="70" kind="cond" v-bind="S5" />
      <DgArrow pts="235,211 190,211" />
      <DgNode :x="0" :y="176" :w="190" :h="70" v-bind="S6" />

      <DgNode :x="235" :y="280" :w="190" :h="36" title="Каталог" />
      <DgArrow pts="330,280 330,246" label="+ названное" :lx="338" :ly="263" anchor="start" />
      <DgArrow pts="95,246 95,280" />
      <DgNode :x="0" :y="280" :w="190" :h="36" kind="term" title="В ранжирование" />
    </template>

    <template #narrow>
      <DgNode :x="84" :y="0" :w="200" :h="46" v-bind="S1" />
      <DgArrow pts="184,46 184,68" />
      <DgNode :x="84" :y="68" :w="200" :h="61" v-bind="S2" />
      <DgArrow pts="184,129 184,151" />
      <DgNode :x="84" :y="151" :w="200" :h="61" v-bind="S3" />
      <DgArrow pts="184,212 184,234" />
      <DgNode :x="84" :y="234" :w="200" :h="61" v-bind="S4" />
      <DgArrow pts="184,295 184,317" />
      <DgNode :x="84" :y="317" :w="200" :h="61" kind="cond" v-bind="S5" />
      <DgNode :x="0" :y="324" :w="68" :h="46" title="Каталог" />
      <DgArrow pts="68,347 84,347" />
      <DgArrow pts="184,378 184,400" />
      <DgNode :x="84" :y="400" :w="200" :h="61" v-bind="S6" />
      <DgArrow pts="184,461 184,489" label="в ранжирование" :lx="176" :ly="476" anchor="end" />

      <DgArrow pts="284,98 312,98 312,500" />
      <DgArrow pts="284,181 312,181" :head="false" />
      <DgArrow pts="284,264 312,264" :head="false" />
      <DgArrow pts="284,430 312,430" :head="false" />
      <DgDot :x="312" :y="181" />
      <DgDot :x="312" :y="264" />
      <DgDot :x="312" :y="430" />
      <DgNode
        :x="200"
        :y="500"
        :w="128"
        :h="61"
        kind="sink"
        title="Отказы"
        :sub="['с причиной,', 'в трассировке']"
      />
    </template>

    <template #caption>
      Четыре ступени отсекают, и ничего не пропадает молча: у каждого отказа есть причина в
      трассировке. Пунктирная ступень работает, только если в запросе названы категория или вендор,
      которых поиск не принёс.
    </template>
  </TheoryDiagram>
</template>
