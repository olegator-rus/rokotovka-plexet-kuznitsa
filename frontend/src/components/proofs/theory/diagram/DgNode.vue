<script setup lang="ts">
// Узел схемы: прямоугольник со скруглением, как у контролов, заголовок и строки пояснения.
// kind — вид шага: step — обычный, term — начало или итог, hub — центральный узел, sink — куда уходит
// отсечённое или откат, cond — шаг только при условии (пунктир), ext — внешний сервис (пунктир),
// human — решение человека (акцент). tone — маршрут A–D: рамка и заголовок цветом маршрута.
import { computed } from 'vue'
import { lineCenters, useDg } from './geometry'

const props = withDefaults(
  defineProps<{
    x: number
    y: number
    w: number
    h: number
    title: string | readonly string[]
    sub?: string | readonly string[]
    kind?: 'step' | 'term' | 'hub' | 'sink' | 'cond' | 'ext' | 'human'
    tone?: 'a' | 'b' | 'c' | 'd'
    /** Пометка над узлом слева: «только D», «решает человек». */
    tag?: string
    align?: 'middle' | 'start'
  }>(),
  { kind: 'step', sub: undefined, tone: undefined, tag: undefined, align: 'middle' },
)

const dg = useDg()
const list = (v: string | readonly string[] | undefined) =>
  v === undefined ? [] : typeof v === 'string' ? [v] : [...v]

const lines = computed(() => {
  const all = [
    ...list(props.title).map((text) => ({ text, title: true })),
    ...list(props.sub).map((text) => ({ text, title: false })),
  ]
  const ys = lineCenters(props.y, props.h, all.length, dg.lh)
  return all.map((l, i) => ({ ...l, y: ys[i] ?? 0 }))
})
const tx = computed(() => (props.align === 'start' ? props.x + 12 : props.x + props.w / 2))
const rx = computed(() => (props.kind === 'term' ? props.h / 2 : 8))
</script>

<template>
  <g :class="['dg-node', `dg-node--${props.kind}`, props.tone ? `dg-tone-${props.tone}` : '']">
    <rect :x="props.x" :y="props.y" :width="props.w" :height="props.h" :rx="rx" />
    <text v-if="props.tag" class="dg-node__tag" :x="props.x" :y="props.y - 7">
      {{ props.tag }}
    </text>
    <text
      v-for="(l, i) in lines"
      :key="i"
      :class="l.title ? 'dg-node__t' : 'dg-node__s'"
      :x="tx"
      :y="l.y"
      dy="0.35em"
      :text-anchor="props.align"
    >
      {{ l.text }}
    </text>
  </g>
</template>

<style scoped>
rect {
  fill: var(--surface);
  stroke: var(--border-control);
  stroke-width: 1;
}
.dg-node__t {
  fill: var(--text);
  font-size: var(--dg-fs);
  font-weight: var(--fw-semibold);
}
.dg-node__s {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
}
.dg-node__tag {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
  font-weight: var(--fw-semibold);
}
.dg-node--term rect,
.dg-node--hub rect {
  fill: var(--surface-2);
}
.dg-node--hub rect {
  stroke: var(--text-2);
  stroke-width: 1.5;
}
.dg-node--sink rect {
  fill: var(--surface-2);
  stroke: var(--border);
}
.dg-node--cond rect,
.dg-node--ext rect {
  stroke-dasharray: 5 4;
}
.dg-node--human rect {
  fill: var(--accent-soft);
  stroke: var(--accent);
  stroke-width: 2;
}
.dg-node--human .dg-node__t,
.dg-node--human .dg-node__tag {
  fill: var(--accent);
}
.dg-tone-a rect {
  fill: var(--route-a-bg);
  stroke: var(--route-a);
}
.dg-tone-b rect {
  fill: var(--route-b-bg);
  stroke: var(--route-b);
}
.dg-tone-c rect {
  fill: var(--route-c-bg);
  stroke: var(--route-c);
}
.dg-tone-d rect {
  fill: var(--route-d-bg);
  stroke: var(--route-d);
}
.dg-tone-a .dg-node__t,
.dg-tone-a .dg-node__tag {
  fill: var(--route-a);
}
.dg-tone-b .dg-node__t,
.dg-tone-b .dg-node__tag {
  fill: var(--route-b);
}
.dg-tone-c .dg-node__t,
.dg-tone-c .dg-node__tag {
  fill: var(--route-c);
}
.dg-tone-d .dg-node__t,
.dg-tone-d .dg-node__tag {
  fill: var(--route-d);
}
</style>
