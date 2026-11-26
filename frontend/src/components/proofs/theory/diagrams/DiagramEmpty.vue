<script setup lang="ts">
// Пустая выдача (search_b3.go): запасные ходы по порядку. Первые три чинят выдачу — она показывается с
// пояснением; два следующих ищут аналоги для плашки «Товара нет в каталоге»; если не нашлось и их — плашка
// без аналогов.
import DgArrow from '../diagram/DgArrow.vue'
import DgDot from '../diagram/DgDot.vue'
import DgNode from '../diagram/DgNode.vue'
import type { Pt } from '../diagram/geometry'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const E = [
  { title: 'Все слова → любое слово', sub: 'повтор с «хотя бы одним»' },
  { title: 'Продления нет', sub: 'та же лицензия, новый срок' },
  { title: 'Метрика не та', sub: 'продукт с пояснением' },
  { title: 'Продукт назван', sub: 'аналоги из справочника связей' },
  {
    title: 'Понятно, на что похож',
    sub: 'замены того же вендора и категории',
    narrow: 'замены вендора и категории',
  },
] as const

/** Стрелка «пусто» к следующему ходу и стрелка вправо, к исходу (узкая раскладка). */
const wideNext = (i: number): Pt[] => [
  [165, i * 80 + 50],
  [165, i * 80 + 80],
]
const narrowNext = (i: number): Pt[] => [
  [105, i * 72 + 46],
  [105, i * 72 + 72],
]
const narrowOut = (i: number): Pt[] => [
  [210, i * 72 + 23],
  [232, i * 72 + 23],
]
</script>

<template>
  <TheoryDiagram
    title="Что делает поиск, когда найти нечего"
    desc="Запасные ходы по порядку. Повтор текстового поиска с любым словом вместо всех; если нет позиции продления — та же лицензия; если метрика не та — продукт с пояснением. Если что-то нашлось, это выдача с пояснением над ней. Дальше — аналоги: из справочника связей, если продукт назван, и замены того же вендора и категории, если понятно, на что товар похож. Найденные аналоги показываются под плашкой «Товара нет в каталоге»; если аналогов нет — плашка без них."
    :wide="[720, 404]"
    :narrow="[328, 370]"
  >
    <template #wide>
      <template v-for="(e, i) in E" :key="e.title">
        <DgNode :x="0" :y="i * 80" :w="330" :h="50" :title="e.title" :sub="e.sub" />
        <DgArrow
          v-if="i < E.length - 1"
          :pts="wideNext(i)"
          label="пусто"
          :lx="173"
          :ly="i * 80 + 65"
          anchor="start"
        />
      </template>
      <DgArrow pts="330,25 400,25 400,105" :head="false" />
      <DgArrow pts="330,105 470,105" label="нашлось" :lx="435" :ly="96" />
      <DgArrow pts="330,185 400,185 400,105" :head="false" />
      <DgDot :x="400" :y="105" />
      <DgNode
        :x="470"
        :y="80"
        :w="250"
        :h="50"
        kind="term"
        title="Выдача"
        sub="с пояснением, что ослаблено"
      />

      <DgArrow pts="330,265 400,265 400,305 470,305" label="аналоги" :lx="435" :ly="296" />
      <DgArrow pts="330,345 400,345 400,305" :head="false" />
      <DgDot :x="400" :y="305" />
      <DgArrow pts="165,370 165,396 595,396 595,330" label="аналогов нет" :lx="380" :ly="387" />
      <DgNode
        :x="470"
        :y="280"
        :w="250"
        :h="50"
        kind="term"
        title="«Товара нет в каталоге»"
        sub="аналоги, «Запросить поставку»"
      />
    </template>

    <template #narrow>
      <template v-for="(e, i) in E" :key="e.title">
        <DgNode
          :x="0"
          :y="i * 72"
          :w="210"
          :h="46"
          :title="e.title"
          :sub="'narrow' in e ? e.narrow : e.sub"
        />
        <DgArrow
          v-if="i < E.length - 1"
          :pts="narrowNext(i)"
          label="пусто"
          :lx="113"
          :ly="i * 72 + 59"
          anchor="start"
        />
        <DgArrow :pts="narrowOut(i)" />
      </template>
      <DgNode :x="232" :y="0" :w="96" :h="190" kind="sink" title="Выдача" sub="с пояснением" />
      <DgNode
        :x="232"
        :y="216"
        :w="96"
        :h="150"
        kind="sink"
        :title="['Товара', 'нет']"
        :sub="['в каталоге', '+ аналоги']"
      />
      <DgArrow pts="105,334 105,352 232,352" label="аналогов нет" :lx="168" :ly="343" />
    </template>

    <template #caption>
      Первые три хода чинят выдачу, и над ней пишется, что ослаблено. Если и они не помогли, поиск
      ищет аналоги и честно пишет, что самого товара нет.
    </template>
  </TheoryDiagram>
</template>
