<script setup lang="ts">
// Лестница «минимум 4 товара» (search_ladder.go answerStage): пять ступеней match_type по порядку, что попадает в
// каждую и когда она включается. Жёсткие условия проверяются на каждой ступени одинаково.
import { THEORY_PARAMS as P, ru } from '@/lib/theoryParams'
import DgArrow from '../diagram/DgArrow.vue'
import DgLabel from '../diagram/DgLabel.vue'
import DgNode from '../diagram/DgNode.vue'
import type { Pt } from '../diagram/geometry'
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const STEPS = [
  {
    title: '1 · Точное совпадение',
    what: 'семейство цели и названное в запросе',
    when: ['всегда'],
    whenNarrow: 'всегда',
    type: 'exact',
  },
  {
    title: '2 · Другие варианты лицензии',
    what: `до ${ru(P.skuVariantsMax)} вариантов того же семейства`,
    when: ['назван точный артикул'],
    whenNarrow: 'назван точный артикул',
    type: 'variant',
  },
  {
    title: '3 · Похожие',
    what: 'найдено поиском, но совпало не всё',
    when: ['в найденном есть', 'неполные совпадения'],
    whenNarrow: 'есть неполные совпадения',
    type: 'related',
  },
  {
    title: '4 · Можно купить сейчас',
    what: `до ${ru(P.maxAlternatives)} продуктов того же класса и назначения`,
    narrow: `до ${ru(P.maxAlternatives)} продуктов того же класса`,
    when: ['ни одну точную карточку', 'нельзя купить сразу'],
    whenNarrow: 'точное нельзя купить сразу',
    type: 'alternative',
  },
  {
    title: '5 · Популярное в наличии',
    what: 'в наличии и с ценой, сначала те же категории',
    narrow: 'в наличии, с ценой, те же категории',
    when: [`разных семейств`, `меньше ${ru(P.ladderMin)}`],
    whenNarrow: `разных семейств меньше ${ru(P.ladderMin)}`,
    type: 'fallback',
  },
] as const

const WY = (i: number) => 28 + i * 84
const NY = (i: number) => i * 92

const wideNext = (i: number): Pt[] => [
  [180, WY(i) + 60],
  [180, WY(i + 1)],
]
const wideLink = (i: number): Pt[] => [
  [360, WY(i) + 30],
  [400, WY(i) + 30],
]
const narrowNext = (i: number): Pt[] => [
  [164, NY(i) + 78],
  [164, NY(i + 1)],
]
</script>

<template>
  <TheoryDiagram
    title="Лестница «минимум 4 товара»: ступени выдачи по порядку"
    :desc="`Пять ступеней по порядку. Точное совпадение — всегда: семейство цели и названное в запросе. Другие варианты лицензии — если назван точный артикул, до ${ru(P.skuVariantsMax)} вариантов того же семейства. Похожие — найденное поиском без полного совпадения. Можно купить сейчас — если ни одну точную карточку нельзя купить сразу, до ${ru(P.maxAlternatives)} продуктов того же класса и назначения с ценой. Популярное в наличии — если разных семейств меньше ${ru(P.ladderMin)}. Жёсткие условия не ослабляются ни на одной ступени.`"
    :wide="[720, 508]"
    :narrow="[328, 528]"
  >
    <template #wide>
      <DgLabel :x="0" :y="8" text="Ступень и что в неё попадает" anchor="start" tone="muted" />
      <DgLabel :x="400" :y="8" text="Когда включается" anchor="start" tone="muted" />
      <template v-for="(s, i) in STEPS" :key="s.type">
        <DgNode :x="0" :y="WY(i)" :w="360" :h="60" :title="s.title" :sub="s.what" />
        <DgArrow :pts="wideLink(i)" kind="skip" :head="false" />
        <DgNode
          :x="400"
          :y="WY(i)"
          :w="320"
          :h="60"
          kind="cond"
          :title="[...s.when]"
          :sub="`match_type: ${s.type}`"
        />
        <DgArrow v-if="i < STEPS.length - 1" :pts="wideNext(i)" />
      </template>
      <DgNode
        :x="0"
        :y="WY(5) + 4"
        :w="720"
        :h="54"
        kind="sink"
        title="Жёсткие условия не ослабляются ни на одной ступени"
        sub="продление, тип организации, платформа, вендор, число мест"
      />
    </template>

    <template #narrow>
      <template v-for="(s, i) in STEPS" :key="s.type">
        <DgNode
          :x="0"
          :y="NY(i)"
          :w="328"
          :h="78"
          :title="s.title"
          :sub="['narrow' in s ? s.narrow : s.what, `когда: ${s.whenNarrow}`, s.type]"
        />
        <DgArrow v-if="i < STEPS.length - 1" :pts="narrowNext(i)" />
      </template>
      <DgNode
        :x="0"
        :y="NY(5) + 4"
        :w="328"
        :h="64"
        kind="sink"
        :title="['Жёсткие условия', 'не ослабляются нигде']"
        sub="продление, организация, число мест"
      />
    </template>

    <template #caption>
      Ступени идут сверху вниз: полное совпадение всегда выше частичного. У каждой ступени указано,
      что в неё попадает и когда она включается. В ответе API ступень карточки — поле
      <code>match_type</code>, на экране «Поиск» — заголовок группы.
    </template>
  </TheoryDiagram>
</template>
