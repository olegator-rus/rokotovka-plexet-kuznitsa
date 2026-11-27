<script setup lang="ts">
// Мало или нет подходящего (search_b3.go): запасные ходы. Первые четыре чинят выдачу — она показывается с
// пояснением, что ослаблено; пятый показывает преемника или замену под строкой «Товара нет в каталоге». Если не
// нашлось ничего — та же строка без товаров. В обоих случаях лестница (search_ladder.go) добирает «Популярное в
// наличии», если разных семейств меньше четырёх; жёсткие условия при этом не ослабляются.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgDot from '../diagram/DgDot.vue'
import DgNode from '../diagram/DgNode.vue'
import type { Pt } from '../diagram/geometry'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const E = [
  { title: 'Все слова → любое слово', sub: 'повтор с «хотя бы одним»' },
  { title: 'Продления нет', sub: 'та же лицензия, новый срок' },
  { title: 'Метрика не та', sub: 'названный продукт с пояснением', narrow: 'продукт с пояснением' },
  {
    title: 'Мягкое условие',
    sub: 'другой срок или тип, с пояснением',
    narrow: 'другой срок, с пояснением',
  },
  {
    title: 'Замена ушедшего продукта',
    sub: 'из словаря: eset → PRO32, или аналог',
    narrow: 'eset → PRO32',
  },
] as const

/** Стрелка «пусто» к следующему ходу и стрелка вправо, к исходу (узкая раскладка). */
const wideNext = (i: number): Pt[] => [
  [165, i * 80 + 50],
  [165, i * 80 + 80],
]
const wideOut = (i: number): Pt[] => [
  [330, i * 80 + 25],
  [400, i * 80 + 25],
  [400, 145],
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
    title="Что делает поиск, когда подходящего мало или нет"
    :desc="`Запасные ходы: повтор текстового поиска с любым словом вместо всех; если нет позиции продления — та же лицензия с новым сроком; если метрика не та — названный продукт с пояснением; если не выполнено мягкое условие — другой срок или тип с пояснением. Если что-то нашлось, это выдача с пояснением над ней. Дальше — замена ушедшего продукта из словаря или аналог: они показываются под строкой «Товара нет в каталоге». Если не нашлось ничего — та же строка без товаров. В обоих случаях, если разных семейств меньше ${ru(P.ladderMin)}, добор выдачи подставляет «Популярное в наличии».`"
    :wide="[720, 404]"
    :narrow="[328, 372]"
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
        <DgArrow v-if="i < 4" :pts="wideOut(i)" :head="false" />
      </template>
      <DgArrow pts="400,145 470,145" label="нашлось" :lx="435" :ly="136" />
      <DgDot :x="400" :y="145" />
      <DgNode
        :x="470"
        :y="120"
        :w="250"
        :h="50"
        kind="term"
        title="Выдача"
        sub="с пояснением, что ослаблено"
      />

      <DgArrow pts="330,345 470,345" label="нашлось" :lx="400" :ly="336" />
      <DgArrow pts="165,370 165,396 595,396 595,370" label="ничего" :lx="380" :ly="387" />
      <DgNode
        :x="470"
        :y="320"
        :w="250"
        :h="50"
        kind="term"
        title="«Товара нет в каталоге»"
        sub="замена с пояснением"
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
      <DgNode :x="232" :y="0" :w="96" :h="262" kind="sink" title="Выдача" sub="с пояснением" />
      <DgNode
        :x="232"
        :y="288"
        :w="96"
        :h="80"
        kind="sink"
        :title="['Товара', 'нет']"
        :sub="['в каталоге', '+ замена']"
      />
      <DgArrow pts="105,334 105,352 232,352" label="ничего" :lx="168" :ly="343" />
    </template>

    <template #caption>
      Первые четыре хода чинят выдачу, и над ней пишется, что ослаблено. Если они не помогли, поиск
      прямо пишет, что самого товара нет. В обоих случаях, если разных семейств меньше
      {{ ru(P.ladderMin) }}, добор выдачи из раздела выше подставляет «Популярное в наличии».
      Жёсткие условия не ослабляются ни одним ходом.
    </template>
  </TheoryDiagram>
</template>
