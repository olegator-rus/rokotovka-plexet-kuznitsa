<script setup lang="ts">
// «По запросу → можно купить сейчас» на живом запросе «windows server» (search_target.go — главный продукт линейки,
// search_ladder.go alternatives, search_alternatives.go — класс и назначение). Продукты и цены — наблюдение на
// стенде, правило — код.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDecision from '../diagram/DgDecision.vue'
import DgDot from '../diagram/DgDot.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const ALTS = [
  { title: 'Astra Linux SE «Орёл»', sub: 'серверная · 20 000 ₽' },
  { title: 'Альт Сервер 11', sub: 'на 1 год · 6 080 ₽' },
  { title: 'РЕД ОС, Конфигурация Сервер', sub: 'бессрочная · 15 000 ₽' },
] as const
const altsLabel = `«Можно купить сейчас» — до ${ru(P.maxAlternatives)} карточек`
</script>

<template>
  <TheoryDiagram
    title="«windows server»: от продукта по запросу к тому, что можно купить сейчас"
    :desc="`Запрос «windows server». Вся линейка Microsoft в каталоге только по запросу, поэтому цель и первая карточка — главный продукт линейки, Windows Server Standard 2019: цена по запросу, только для госучреждений. Купить сразу нельзя ни одну точную карточку, поэтому ищутся продукты того же класса — операционные системы Windows и российские операционные системы — и того же назначения, серверная редакция. Под первой карточкой до ${ru(P.maxAlternatives)} таких продуктов с ценой: Astra Linux SE «Орёл» серверная за 20 000 ₽, Альт Сервер 11 на год за 6 080 ₽, РЕД ОС Конфигурация Сервер бессрочная за 15 000 ₽.`"
    :wide="[720, 302]"
    :narrow="[328, 690]"
  >
    <template #wide>
      <DgNode :x="0" :y="0" :w="150" :h="66" kind="term" title="Запрос" sub="«windows server»" />
      <DgArrow pts="150,33 174,33" />
      <DgNode
        :x="174"
        :y="0"
        :w="240"
        :h="66"
        title="Цель и первая карточка"
        :sub="['вся линейка Microsoft', 'только по запросу']"
      />
      <DgArrow pts="414,33 438,33" />
      <DgNode
        :x="438"
        :y="0"
        :w="282"
        :h="66"
        title="Windows Server Standard 2019"
        :sub="['«Цена по запросу»,', 'только для госучреждений']"
      />
      <DgArrow pts="579,66 579,100" />
      <DgDecision
        :x="479"
        :y="100"
        :w="200"
        :h="56"
        :title="['Можно купить', 'хоть одну точную?']"
      />
      <DgArrow pts="479,128 436,128" label="нет" :lx="457" :ly="118" />
      <DgNode
        :x="226"
        :y="98"
        :w="210"
        :h="60"
        title="Тот же класс"
        :sub="['ОС Windows ↔', 'Российские ОС']"
      />
      <DgArrow pts="226,128 190,128" />
      <DgNode :x="0" :y="98" :w="190" :h="60" title="То же назначение" sub="серверная редакция" />
      <DgLabel :x="360" :y="186" :text="altsLabel" tone="accent" />
      <DgArrow pts="95,158 95,206 122,206 122,230" />
      <DgArrow pts="122,206 360,206 360,230" />
      <DgArrow pts="360,206 598,206 598,230" />
      <DgDot :x="122" :y="206" />
      <DgDot :x="360" :y="206" />
      <template v-for="(a, i) in ALTS" :key="a.title">
        <DgNode :x="12 + i * 238" :y="230" :w="220" :h="56" :title="a.title" :sub="a.sub" />
      </template>
    </template>

    <template #narrow>
      <DgNode :x="24" :y="0" :w="280" :h="46" kind="term" title="Запрос «windows server»" />
      <DgArrow pts="164,46 164,68" />
      <DgNode
        :x="24"
        :y="68"
        :w="280"
        :h="62"
        title="Цель и первая карточка"
        :sub="['вся линейка Microsoft', 'только по запросу']"
      />
      <DgArrow pts="164,130 164,152" />
      <DgNode
        :x="24"
        :y="152"
        :w="280"
        :h="62"
        title="Windows Server Standard 2019"
        :sub="['«Цена по запросу»,', 'только для госучреждений']"
      />
      <DgArrow pts="164,214 164,236" />
      <DgDecision
        :x="44"
        :y="236"
        :w="240"
        :h="54"
        :title="['Можно купить', 'хоть одну точную?']"
      />
      <DgArrow pts="164,290 164,320" label="нет" :lx="172" :ly="305" anchor="start" />
      <DgNode
        :x="24"
        :y="320"
        :w="280"
        :h="46"
        title="Тот же класс"
        sub="ОС Windows ↔ Российские ОС"
      />
      <DgArrow pts="164,366 164,388" />
      <DgNode :x="24" :y="388" :w="280" :h="46" title="То же назначение" sub="серверная редакция" />
      <DgArrow pts="164,434 164,492" />
      <DgLabel :x="164" :y="463" :text="altsLabel" tone="accent" />
      <template v-for="(a, i) in ALTS" :key="a.title">
        <DgNode :x="24" :y="492 + i * 70" :w="280" :h="56" :title="a.title" :sub="a.sub" />
      </template>
    </template>

    <template #caption>
      Продукт, который назвал покупатель, остаётся первым с кнопкой «Запросить цену». Если хотя бы
      одну точную карточку можно купить сразу, блока «Можно купить сейчас» нет. Если назначение
      продукта не определяется по названию или параметрам фида, блока тоже нет: неясно, что
      предлагать вместо.
    </template>
  </TheoryDiagram>
</template>
