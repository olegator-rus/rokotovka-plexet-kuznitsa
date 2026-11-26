<script setup lang="ts">
// Из чего собрано решение (ARCHITECTURE §1): Go API в центре, внутри своего контура OpenSearch, сервис моделей
// и PostgreSQL; наружу, к Yandex AI Studio, уходит только замаскированный текст разбора D и агента.
import DgArrow from '../diagram/DgArrow.vue'
import DgDot from '../diagram/DgDot.vue'
import DgGroup from '../diagram/DgGroup.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'
</script>

<template>
  <TheoryDiagram
    title="Из чего собрано решение"
    desc="Покупатель или сайт магазина шлёт запрос в Go API и получает выдачу. Go API, OpenSearch, сервис моделей и PostgreSQL работают на своём сервере: поиск идёт в OpenSearch, векторы и оценки реранкера — в сервис моделей, журнал, трассы и кэш — в PostgreSQL. За границу сервера, во внешнюю Yandex AI Studio, уходит только текст с маской персональных данных — для разбора на маршруте D и для агента."
    :wide="[720, 344]"
    :narrow="[328, 440]"
  >
    <template #wide>
      <DgGroup :x="186" :y="8" :w="528" :h="250" label="Свой сервер" />
      <DgNode :x="0" :y="110" :w="120" :h="54" kind="term" title="Покупатель" sub="или сайт" />
      <DgArrow pts="120,128 206,128" label="запрос" :lx="158" :ly="118" />
      <DgArrow pts="206,148 120,148" label="выдача" :lx="158" :ly="159" />
      <DgNode
        :x="206"
        :y="40"
        :w="170"
        :h="194"
        kind="hub"
        title="Go API"
        :sub="['один процесс:', 'поиск, подсказки,', 'события, оценка,', 'агент, стоимость']"
      />
      <DgArrow pts="376,67 476,67" label="поиск" :lx="426" :ly="58" />
      <DgArrow pts="376,137 476,137" label="векторы" :lx="426" :ly="128" />
      <DgArrow pts="376,207 476,207" label="запись" :lx="426" :ly="198" />
      <DgNode :x="476" :y="40" :w="224" :h="54" title="OpenSearch" sub="BM25 и векторы HNSW" />
      <DgNode :x="476" :y="110" :w="224" :h="54" title="Сервис моделей" sub="BGE-M3, реранкер" />
      <DgNode :x="476" :y="180" :w="224" :h="54" title="PostgreSQL" sub="журнал, трассы, кэш" />
      <DgArrow pts="291,234 291,307 476,307" tone="d" />
      <DgLabel :x="384" :y="297" text="маска ПДн" tone="d" />
      <DgLabel :x="384" :y="320" text="разбор D и агент" />
      <DgNode
        :x="476"
        :y="280"
        :w="224"
        :h="54"
        kind="ext"
        tag="внешняя LLM"
        title="Yandex AI Studio"
        sub="Alice AI LLM Flash"
      />
    </template>

    <template #narrow>
      <DgGroup :x="4" :y="70" :w="320" :h="286" label="Свой сервер" />
      <DgNode :x="64" :y="0" :w="200" :h="40" kind="term" title="Покупатель или сайт" />
      <DgArrow pts="150,40 150,98" label="запрос" :lx="142" :ly="56" anchor="end" />
      <DgArrow pts="178,98 178,40" label="выдача" :lx="186" :ly="56" anchor="start" />
      <DgNode
        :x="16"
        :y="98"
        :w="296"
        :h="60"
        kind="hub"
        title="Go API"
        :sub="['поиск, подсказки, события, оценка,', 'агент, учёт стоимости']"
      />
      <DgArrow pts="40,158 40,198 106,198" label="поиск" :lx="73" :ly="189" />
      <DgArrow pts="40,198 40,258 106,258" label="векторы" :lx="73" :ly="249" />
      <DgArrow pts="40,258 40,318 106,318" label="запись" :lx="73" :ly="309" />
      <DgDot :x="40" :y="198" />
      <DgDot :x="40" :y="258" />
      <DgDot :x="40" :y="318" />
      <DgNode :x="106" :y="174" :w="206" :h="48" title="OpenSearch" sub="BM25 и векторы HNSW" />
      <DgNode :x="106" :y="234" :w="206" :h="48" title="Сервис моделей" sub="BGE-M3, реранкер" />
      <DgNode :x="106" :y="294" :w="206" :h="48" title="PostgreSQL" sub="журнал, трассы, кэш" />
      <DgArrow pts="40,318 40,412 106,412" tone="d" />
      <DgLabel :x="70" :y="428" text="маска ПДн" tone="d" />
      <DgNode
        :x="106"
        :y="388"
        :w="206"
        :h="48"
        kind="ext"
        tag="внешняя LLM"
        title="Yandex AI Studio"
        sub="Alice AI LLM Flash"
      />
    </template>

    <template #caption>
      Пунктирная рамка — граница своего сервера. Наружу её пересекает одна стрелка: текст разбора на
      маршруте D и запросы агента, после маски персональных данных.
    </template>
  </TheoryDiagram>
</template>
