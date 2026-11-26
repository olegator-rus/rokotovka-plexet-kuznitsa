<script setup lang="ts">
// Разбор намерения на маршруте D (intent.go parseMasked, callModel): маска → ключ → кэш? (да — сразу разбор);
// режим и бюджет? (нет — правила); основная модель, при сбое — резервная; ответ не по схеме — один повтор,
// снова не по схеме — правила; сверка со справочниками → слияние с правилами → в кэш.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDecision from '../diagram/DgDecision.vue'
import DgDot from '../diagram/DgDot.vue'
import DgGroup from '../diagram/DgGroup.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const limit = `≤ ${ru(P.intentCallTimeoutSec)} с`
</script>

<template>
  <TheoryDiagram
    title="Разбор трудного запроса на маршруте D"
    :desc="`Запрос маскируется, из него и версий строится ключ кэша. Если разбор уже в кэше — он готов сразу, за 0 ₽, модель не зовётся. Иначе проверяются режим LLM и бюджет: нет — разбор по правилам. Есть — вызов основной модели, ${limit}; при сбое — резервная модель, если она задана, при её сбое — правила. Ответ не по схеме — один повтор, снова не по схеме — правила. Удачный ответ сверяется со справочниками, сливается с правилами и кладётся в кэш.`"
    :wide="[720, 305]"
    :narrow="[328, 690]"
  >
    <template #wide>
      <g transform="translate(0 -25)">
        <DgNode :x="0" :y="33" :w="96" :h="50" title="Маска ПДн" sub="до ключа" />
        <DgArrow pts="96,58 112,58" />
        <DgNode :x="112" :y="33" :w="96" :h="50" title="Ключ кэша" sub="+ версии" />
        <DgArrow pts="208,58 222,58" />
        <DgDecision :x="222" :y="38" :w="90" :h="40" title="В кэше?" />
        <DgArrow pts="312,58 360,58" label="да" :lx="336" :ly="49" />
        <DgLabel :x="336" :y="69" text="0 ₽" />
        <DgNode :x="360" :y="41" :w="100" :h="34" kind="term" title="Разбор готов" />
        <DgNode :x="482" :y="33" :w="100" :h="50" title="В кэш" sub="повтор — 0 ₽" />
        <DgArrow pts="482,58 460,58" />
        <DgNode :x="600" :y="33" :w="120" :h="50" title="Слияние" sub="с правилами" />
        <DgArrow pts="600,58 582,58" />

        <DgArrow pts="267,78 267,148" label="нет" :lx="275" :ly="113" anchor="start" />
        <DgDecision :x="209" :y="148" :w="116" :h="40" title="Бюджет есть?" />
        <DgArrow pts="325,168 353,168" label="да" :lx="334" :ly="159" />

        <DgGroup :x="345" :y="120" :w="110" :h="172" :label="`вызов ${limit}`" />
        <DgNode :x="353" :y="143" :w="94" :h="50" tone="d" title="Основная" sub="модель" />
        <DgArrow pts="400,193 400,226" tone="d" label="сбой" :lx="408" :ly="210" anchor="start" />
        <DgNode
          :x="353"
          :y="226"
          :w="94"
          :h="50"
          kind="cond"
          tone="d"
          title="Резервная"
          sub="если задана"
        />
        <DgArrow pts="455,168 477,168" />
        <DgDecision :x="477" :y="148" :w="92" :h="40" title="По схеме?" />
        <DgArrow
          pts="523,148 523,116 436,116 436,143"
          kind="back"
          label="нет: один повтор"
          :lx="480"
          :ly="106"
        />
        <DgArrow pts="569,168 600,168" label="да" :lx="585" :ly="159" />
        <DgNode :x="600" :y="143" :w="120" :h="50" title="Сверка" sub="по справочникам" />
        <DgArrow pts="660,143 660,83" />

        <DgNode
          :x="40"
          :y="236"
          :w="160"
          :h="52"
          kind="sink"
          title="Разбор по правилам"
          sub="причина в трассе"
        />
        <DgArrow pts="209,168 120,168 120,236" label="нет" :lx="165" :ly="159" />
        <DgArrow pts="353,251 200,251" label="сбой" :lx="276" :ly="242" />
        <DgArrow
          pts="523,188 523,318 120,318 120,288"
          label="снова не по схеме"
          :lx="330"
          :ly="309"
        />
      </g>
    </template>

    <template #narrow>
      <DgNode :x="74" :y="0" :w="180" :h="32" title="Маска ПДн" />
      <DgArrow pts="164,32 164,52" />
      <DgNode :x="74" :y="52" :w="180" :h="32" title="Ключ кэша" />
      <DgArrow pts="164,84 164,104" />
      <DgDecision :x="104" :y="104" :w="120" :h="36" title="В кэше?" />
      <DgArrow pts="104,122 12,122 12,602 74,602" kind="skip" label="да, 0 ₽" :lx="58" :ly="113" />
      <DgArrow pts="164,140 164,162" label="нет" :lx="172" :ly="151" anchor="start" />
      <DgDecision :x="94" :y="162" :w="140" :h="36" title="Бюджет есть?" />
      <DgArrow pts="164,198 164,222 97,222 97,246" label="да" :lx="172" :ly="210" anchor="start" />
      <DgArrow pts="234,180 316,180 316,663 300,663" label="нет" :lx="275" :ly="171" />

      <DgNode
        :x="44"
        :y="246"
        :w="106"
        :h="46"
        tone="d"
        title="Основная"
        :sub="`модель, ${limit}`"
      />
      <DgArrow pts="150,269 184,269" tone="d" label="сбой" :lx="167" :ly="258" />
      <DgNode
        :x="184"
        :y="246"
        :w="100"
        :h="46"
        kind="cond"
        tone="d"
        title="Резервная"
        sub="если задана"
      />
      <DgArrow pts="284,269 316,269" :head="false" />
      <DgDot :x="316" :y="269" />
      <DgArrow pts="97,292 97,308 164,308 164,326" />
      <DgArrow pts="234,292 234,308 164,308" :head="false" />
      <DgDot :x="164" :y="308" />

      <DgDecision :x="104" :y="326" :w="120" :h="36" title="По схеме?" />
      <DgArrow pts="104,344 30,344 30,269 44,269" kind="back" label="1 повтор" :lx="67" :ly="335" />
      <DgArrow pts="224,344 316,344" :head="false" label="снова нет" :lx="270" :ly="335" />
      <DgDot :x="316" :y="344" />
      <DgArrow pts="164,362 164,384" label="да" :lx="172" :ly="373" anchor="start" />
      <DgNode :x="74" :y="384" :w="180" :h="46" title="Сверка" sub="по справочникам" />
      <DgArrow pts="164,430 164,450" />
      <DgNode :x="74" :y="450" :w="180" :h="46" title="Слияние" sub="с правилами" />
      <DgArrow pts="164,496 164,516" />
      <DgNode :x="74" :y="516" :w="180" :h="46" title="В кэш" sub="повтор — 0 ₽" />
      <DgArrow pts="164,562 164,584" />
      <DgNode :x="74" :y="584" :w="180" :h="36" kind="term" title="Разбор готов" />
      <DgNode
        :x="120"
        :y="640"
        :w="180"
        :h="46"
        kind="sink"
        title="Разбор по правилам"
        sub="причина в трассе"
      />
    </template>

    <template #caption>
      Короткий путь — попадание в кэш: модель не зовётся, шаги до самого ответа пропускаются. Любой
      сбой ведёт в разбор по правилам, и поиск отвечает дальше без модели.
    </template>
  </TheoryDiagram>
</template>
