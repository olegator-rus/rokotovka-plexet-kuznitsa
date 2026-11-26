<script setup lang="ts">
// Разметка запроса правилами (attributes.go, search_core.go, search_negation.go): под каждым куском запроса —
// скобка, вид куска (название, условие, исключение) и что из него понято. Примеры — запросы раздела.
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

interface Seg {
  text: string
  kind: 'название' | 'условие' | 'исключение'
  value: string
  /** Центр куска в строке запроса (широкая раскладка, колонка 224) и полуширина скобки. */
  cx: number
  hw: number
}

const EXAMPLES: readonly (readonly Seg[])[] = [
  [
    { text: 'касперский', kind: 'название', value: 'вендор Kaspersky', cx: 60, hw: 42 },
    { text: 'на 10 пк', kind: 'условие', value: '10 устройств', cx: 168, hw: 34 },
  ],
  [
    { text: 'kaspersky', kind: 'название', value: 'вендор', cx: 60, hw: 38 },
    { text: 'на 3 года', kind: 'условие', value: 'срок 36 месяцев', cx: 168, hw: 36 },
  ],
  [
    { text: 'антивирус', kind: 'название', value: 'категория', cx: 56, hw: 40 },
    { text: 'кроме dr.web', kind: 'исключение', value: 'чип «Не Dr.Web»', cx: 164, hw: 50 },
  ],
]

/** Скобка под куском: уголки вниз, перекладина и засечка к подписи. */
function bracket(cx: number, hw: number, y: number): string {
  return `M${cx - hw} ${y} V${y + 6} H${cx + hw} V${y} M${cx} ${y + 6} V${y + 13}`
}

const WIDE_COL = 248
const NARROW_ROW = 114
/** В узкой раскладке колонка 328 вместо 224: куски разносятся пропорционально. */
const nx = (cx: number) => Math.round((cx * 328) / 224)

const DESC =
  'Три запроса и их разметка. «касперский» — название, вендор Kaspersky; «на 10 пк» — условие, 10 устройств. «kaspersky» — название, вендор; «на 3 года» — условие, срок 36 месяцев. «антивирус» — название, категория; «кроме dr.web» — исключение, чип «Не Dr.Web».'
</script>

<template>
  <TheoryDiagram
    title="Как правила размечают слова запроса"
    :desc="DESC"
    :wide="[720, 98]"
    :narrow="[328, 326]"
  >
    <template #wide>
      <g v-for="(ex, i) in EXAMPLES" :key="i" :transform="`translate(${i * WIDE_COL} 0)`">
        <rect class="pl-box" x="0" y="0" width="224" height="36" rx="8" />
        <g v-for="s in ex" :key="s.text">
          <text class="pl-q" :x="s.cx" y="18" dy="0.35em" text-anchor="middle">{{ s.text }}</text>
          <path
            :class="['pl-br', { 'pl-br--not': s.kind === 'исключение' }]"
            :d="bracket(s.cx, s.hw, 44)"
          />
          <text class="pl-kind" :x="s.cx" y="70" dy="0.35em" text-anchor="middle">
            {{ s.kind }}
          </text>
          <text class="pl-val" :x="s.cx" y="87" dy="0.35em" text-anchor="middle">
            {{ s.value }}
          </text>
        </g>
      </g>
    </template>

    <template #narrow>
      <g v-for="(ex, i) in EXAMPLES" :key="i" :transform="`translate(0 ${i * NARROW_ROW})`">
        <rect class="pl-box" x="0" y="0" width="328" height="34" rx="8" />
        <g v-for="s in ex" :key="s.text">
          <text class="pl-q" :x="nx(s.cx)" y="17" dy="0.35em" text-anchor="middle">
            {{ s.text }}
          </text>
          <path
            :class="['pl-br', { 'pl-br--not': s.kind === 'исключение' }]"
            :d="bracket(nx(s.cx), s.hw, 42)"
          />
          <text class="pl-kind" :x="nx(s.cx)" y="67" dy="0.35em" text-anchor="middle">
            {{ s.kind }}
          </text>
          <text class="pl-val" :x="nx(s.cx)" y="83" dy="0.35em" text-anchor="middle">
            {{ s.value }}
          </text>
        </g>
      </g>
    </template>

    <template #caption>
      Названия становятся фильтрами вендора, продукта или категории, условия — обязательными
      требованиями к лицензии, исключение — фильтром «кроме» (пунктирная скобка).
    </template>
  </TheoryDiagram>
</template>

<style scoped>
.pl-box {
  fill: var(--surface-2);
  stroke: var(--border-control);
  stroke-width: 1;
}
.pl-q {
  fill: var(--text);
  font-size: var(--dg-fs);
  font-weight: var(--fw-semibold);
}
.pl-br {
  fill: none;
  stroke: var(--text-3);
  stroke-width: 1.4;
}
.pl-br--not {
  stroke-dasharray: 4 3;
}
.pl-kind {
  fill: var(--text-3);
  font-size: var(--dg-fs-sub);
}
.pl-val {
  fill: var(--text);
  font-size: var(--dg-fs);
  font-weight: var(--fw-semibold);
}
</style>
