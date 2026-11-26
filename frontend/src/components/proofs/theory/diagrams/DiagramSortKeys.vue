<script setup lang="ts">
// Порядок выдачи (rank.go): пять ключей сравниваются по очереди. Следующий ключ решает, только если по
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
    wide: ['названо целиком', 'выше частичного'],
    narrow: 'названо целиком — выше',
  },
  {
    title: '3 · Масштаб',
    wide: ['бизнес-запрос:', 'домашние ниже'],
    narrow: 'бизнес-запрос: домашние ниже',
  },
  {
    title: '4 · Ровно N',
    wide: ['ровно на N —', 'выше поштучной'],
    narrow: 'ровно на N — выше поштучной',
  },
  { title: '5 · Скор', wide: ['итоговый,', 'формула ниже'], narrow: 'итоговый, формула ниже' },
] as const

const WX = (i: number) => i * 150
const NY = (i: number) => i * 76

/** Широкая: дуга «равны» к следующему ключу и стрелка «разные» вниз. Узкая: «равны» вниз, «разные» вправо. */
const wideEq = (i: number): Pt[] => [
  [WX(i) + 90, 36],
  [WX(i) + 90, 18],
  [WX(i + 1) + 30, 18],
  [WX(i + 1) + 30, 36],
]
const wideDiff = (i: number): Pt[] => [
  [WX(i) + 60, 120],
  [WX(i) + 60, 150],
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
    desc="Два товара сравниваются по ключам по очереди: группа, ступень, масштаб, ровно запрошенное количество, итоговый скор. Если по ключу они различаются, порядок решён. Если равны — решает следующий ключ. Поэтому популярный товар не обгонит точное совпадение."
    :wide="[720, 188]"
    :narrow="[328, 352]"
  >
    <template #wide>
      <template v-for="(k, i) in KEYS" :key="k.title">
        <DgNode :x="WX(i)" :y="36" :w="120" :h="84" :title="k.title" :sub="[...k.wide]" />
        <DgArrow v-if="i < KEYS.length - 1" :pts="wideEq(i)" />
        <DgLabel v-if="i < KEYS.length - 1" :x="WX(i) + 135" :y="9" text="равны" />
        <DgArrow
          :pts="wideDiff(i)"
          :label="i < KEYS.length - 1 ? 'разные' : 'решает'"
          :lx="WX(i) + 68"
          :ly="135"
          anchor="start"
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
          :ly="NY(i) + 61"
          anchor="start"
        />
        <DgArrow
          :pts="narrowDiff(i)"
          :label="i < KEYS.length - 1 ? 'разные' : 'решает'"
          :lx="226"
          :ly="NY(i) + 13"
        />
      </template>
      <DgNode :x="252" :y="0" :w="76" :h="350" kind="sink" :title="['Порядок', 'решён']" />
    </template>

    <template #caption>
      Следующий ключ смотрится, только если по предыдущим товары равны. Итоговый скор — последний
      ключ: он не переставляет товары между группами.
    </template>
  </TheoryDiagram>
</template>
