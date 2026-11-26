<script setup lang="ts">
// Два источника кандидатов (search_query.go, search.go): BM25 и векторы работают параллельно, на одних фильтрах
// условий, каждый отдаёт до candidatesTop кандидатов; вектор запроса ждут queryEmbedLimit, потом ветка
// пропускается. Оба списка сходятся в слиянии RRF.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDot from '../diagram/DgDot.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const top = `до ${ru(P.candidatesTop)}`
const wait = `вектор запроса — до ${ru(P.queryEmbedLimitSec)} с`
</script>

<template>
  <TheoryDiagram
    title="BM25 и векторы работают параллельно"
    :desc="`Запрос после нормализации вместе с фильтрами условий идёт в две ветки сразу. BM25 ищет слова запроса в полях карточки: точно различает названия, версии и артикулы, но не находит запрос-описание. Векторы BGE-M3 сравнивают смысл запроса и карточки: находят описание задачи, но путают близкие версии; ${wait}, иначе ветка пропускается. Каждая ветка отдаёт ${top} кандидатов, оба списка сходятся в слиянии RRF.`"
    :wide="[720, 216]"
    :narrow="[328, 270]"
  >
    <template #wide>
      <DgNode
        :x="0"
        :y="71"
        :w="150"
        :h="70"
        kind="term"
        title="Запрос"
        :sub="['после нормализации', '+ фильтры условий']"
      />
      <DgArrow pts="150,106 176,106 176,50 200,50" />
      <DgArrow pts="176,106 176,162 200,162" />
      <DgDot :x="176" :y="106" />
      <DgNode
        :x="200"
        :y="8"
        :w="320"
        :h="84"
        align="start"
        title="BM25 — по словам"
        :sub="[
          'ищет слова запроса в полях карточки',
          '+ точно различает версии и артикулы',
          '− не находит запрос-описание',
        ]"
      />
      <DgNode
        :x="200"
        :y="112"
        :w="320"
        :h="101"
        align="start"
        title="Векторы BGE-M3 — по смыслу"
        :sub="[
          'близость векторов запроса и карточки',
          '+ находит описание задачи',
          '− путает близкие версии',
          wait,
        ]"
      />
      <DgArrow pts="520,50 560,50 560,106 584,106" />
      <DgArrow pts="520,162 560,162 560,106" :head="false" />
      <DgDot :x="560" :y="106" />
      <DgLabel :x="540" :y="40" :text="top" />
      <DgLabel :x="540" :y="152" :text="top" />
      <DgNode :x="584" :y="81" :w="136" :h="50" title="Слияние RRF" sub="один список" />
    </template>

    <template #narrow>
      <DgNode
        :x="24"
        :y="0"
        :w="280"
        :h="46"
        kind="term"
        title="Запрос после нормализации"
        sub="+ фильтры условий"
      />
      <DgArrow pts="164,46 164,62 79,62 79,78" />
      <DgArrow pts="164,62 249,62 249,78" />
      <DgDot :x="164" :y="62" />
      <DgNode
        :x="0"
        :y="78"
        :w="158"
        :h="91"
        title="BM25"
        :sub="['по словам', '+ артикулы, версии', '− описание задачи']"
      />
      <DgNode
        :x="170"
        :y="78"
        :w="158"
        :h="91"
        title="Векторы BGE-M3"
        :sub="[
          'по смыслу',
          '+ описание задачи',
          '− близкие версии',
          `ждём до ${ru(P.queryEmbedLimitSec)} с`,
        ]"
      />
      <DgArrow pts="79,169 79,195 164,195 164,221" />
      <DgArrow pts="249,169 249,195 164,195" :head="false" />
      <DgDot :x="164" :y="195" />
      <DgLabel :x="87" :y="182" :text="top" anchor="start" />
      <DgLabel :x="257" :y="182" :text="top" anchor="start" />
      <DgNode :x="64" :y="221" :w="200" :h="46" title="Слияние RRF" sub="один список" />
    </template>

    <template #caption>
      Ветки не ждут друг друга и сильны на разных запросах. Если сервис моделей не ответил вовремя,
      слияние получает только текстовый список.
    </template>
  </TheoryDiagram>
</template>
