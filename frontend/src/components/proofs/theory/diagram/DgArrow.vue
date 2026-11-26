<script setup lang="ts">
// Стрелка схемы: ломаная по точкам со скруглёнными углами и наконечником-треугольником.
// kind: main — основной путь, skip — обход шага (пунктир), back — возврат в цикле. tone — цвет маршрута
// или акцент (решение человека). Подпись — по координатам lx/ly (центр первой строки).
import { computed } from 'vue'
import DgLabel from './DgLabel.vue'
import { parsePts, roundedPath, withHead, type Pt } from './geometry'

const props = withDefaults(
  defineProps<{
    /** Точки: массив или строка «x,y x,y …», как у polyline. */
    pts: string | readonly Pt[]
    kind?: 'main' | 'skip' | 'back'
    tone?: 'a' | 'b' | 'c' | 'd' | 'accent'
    head?: boolean
    label?: string | readonly string[]
    lx?: number
    ly?: number
    anchor?: 'start' | 'middle' | 'end'
  }>(),
  {
    kind: 'main',
    tone: undefined,
    head: true,
    label: undefined,
    lx: 0,
    ly: 0,
    anchor: 'middle',
  },
)

const shape = computed(() => {
  const pts = parsePts(props.pts)
  if (!props.head) return { d: roundedPath(pts), head: '' }
  const { line, head } = withHead(pts)
  return { d: roundedPath(line), head }
})
</script>

<template>
  <g :class="['dg-arrow', `dg-arrow--${props.kind}`, props.tone ? `dg-arrow--${props.tone}` : '']">
    <path :d="shape.d" />
    <polygon v-if="shape.head" :points="shape.head" />
    <DgLabel
      v-if="props.label"
      :x="props.lx"
      :y="props.ly"
      :text="props.label"
      :anchor="props.anchor"
      :tone="props.tone"
    />
  </g>
</template>

<style scoped>
path {
  fill: none;
  stroke: var(--text-3);
  stroke-width: 1.4;
  stroke-linejoin: round;
  stroke-linecap: round;
}
polygon {
  fill: var(--text-3);
}
.dg-arrow--skip path {
  stroke-dasharray: 5 4;
  stroke-linecap: butt;
}
.dg-arrow--a path {
  stroke: var(--route-a);
}
.dg-arrow--a polygon {
  fill: var(--route-a);
}
.dg-arrow--b path {
  stroke: var(--route-b);
}
.dg-arrow--b polygon {
  fill: var(--route-b);
}
.dg-arrow--c path {
  stroke: var(--route-c);
}
.dg-arrow--c polygon {
  fill: var(--route-c);
}
.dg-arrow--d path {
  stroke: var(--route-d);
}
.dg-arrow--d polygon {
  fill: var(--route-d);
}
.dg-arrow--accent path {
  stroke: var(--accent);
}
.dg-arrow--accent polygon {
  fill: var(--accent);
}
</style>
