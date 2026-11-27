<script setup lang="ts">
// Порядок выдачи (rank.go sortFamilies): шесть ключей сравниваются по очереди. Следующий ключ решает, только если по
// предыдущим два товара равны; первый различающийся ключ решает порядок.
import DgArrow from '../diagram/DgArrow.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import type { Pt } from '../diagram/geometry'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const KEYS = [
  {
    title: '1 · Группа',
    wide: ['точное →', 'подходящее →', 'аналог'],
    narrow: 'точное → подходящее → аналог',
  },
  {
    title: '2 · Ступень',
    wide: ['названо', 'целиком — выше'],
    narrow: 'названо целиком — выше',
  },
  {
    title: '3 · Масштаб',
    wide: ['бизнес-запрос:', 'домашние ниже'],
    narrow: 'бизнес-запрос: домашние ниже',
  },
  {
    title: '4 · Линейка',
    wide: ['основная', 'у вендора выше'],
    narrow: 'основная линейка вендора выше',
  },
  {
    title: '5 · Ровно N',
    wide: ['ровно N выше', 'поштучной'],
    narrow: 'ровно на N — выше поштучной',
  },
  {
    title: '6 · Итог',
    wide: ['итоговое', 'ранжирование'],
    narrow: 'итоговое ранжирование, формула ниже',
  },
] as const

const W = 108
const WX = (i: number) => i * 122.4
const NY = (i: number) => i * 70

/** Широкая: дуга «равны» к следующему ключу и стрелка «разные» вниз. Узкая: «равны» вниз, «разные» вправо. */
const wideEq = (i: number): Pt[] => [
  [WX(i) + 80, 36],
  [WX(i) + 80, 18],
  [WX(i + 1) + 28, 18],
  [WX(i + 1) + 28, 36],
]
const wideDiff = (i: number): Pt[] => [
  [WX(i) + W / 2, 120],
  [WX(i) + W / 2, 150],
]
const narrowEq = (i: number): Pt[] => [
  [100, NY(i) + 46],
  [100, NY(i + 1)],
]
const narrowDiff = (i: number): Pt[] => [
  [200, NY(i) + 23],
  [252, NY(i) + 23],
]
</script>

<template>
  <TheoryDiagram
    title="Ключи сортировки выдачи по порядку важности"
    desc="Два товара сравниваются по ключам по очереди: группа, ступень, масштаб, основная линейка вендора, ровно запрошенное количество, итоговое ранжирование. Если по ключу они различаются, порядок решён. Если равны — решает следующий ключ. Поэтому популярный товар не обгонит точное совпадение."
    :wide="[720, 188]"
    :narrow="[328, 396]"
  >
    <template #wide>
      <template v-for="(k, i) in KEYS" :key="k.title">
        <DgNode :x="WX(i)" :y="36" :w="W" :h="84" :title="k.title" :sub="[...k.wide]" />
        <DgArrow v-if="i < KEYS.length - 1" :pts="wideEq(i)" />
        <DgLabel v-if="i < KEYS.length - 1" :x="WX(i) + 115" :y="9" text="равны" />
        <DgArrow
          :pts="wideDiff(i)"
          :label="i < KEYS.length - 1 ? 'разные' : 'решает'"
          :lx="i < KEYS.length - 1 ? WX(i) + W / 2 + 8 : WX(i) + W / 2 - 8"
          :ly="135"
          :anchor="i < KEYS.length - 1 ? 'start' : 'end'"
        />
      </template>
      <DgNode :x="0" :y="150" :w="720" :h="36" kind="term" title="Порядок двух товаров решён" />
    </template>

    <template #narrow>
      <template v-for="(k, i) in KEYS" :key="k.title">
        <DgNode :x="0" :y="NY(i)" :w="200" :h="46" :title="k.title" :sub="k.narrow" />
        <DgArrow
          v-if="i < KEYS.length - 1"
          :pts="narrowEq(i)"
          label="равны"
          :lx="108"
          :ly="NY(i) + 58"
          anchor="start"
        />
        <DgArrow
          :pts="narrowDiff(i)"
          :label="i < KEYS.length - 1 ? 'разные' : 'решает'"
          :lx="226"
          :ly="NY(i) + 13"
        />
      </template>
      <DgNode :x="252" :y="0" :w="76" :h="396" kind="sink" :title="['Порядок', 'решён']" />
    </template>

    <template #caption>
      Следующий ключ смотрится, только если по предыдущим товары равны. Итоговое ранжирование —
      последний ключ: он не переставляет товары между группами.
    </template>
  </TheoryDiagram>
</template>
