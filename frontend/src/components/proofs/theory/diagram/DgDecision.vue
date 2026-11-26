<script setup lang="ts">
// Узел решения: шестиугольник с вопросом. Ветки «да / нет» подписываются на стрелках (DgArrow).
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
  }>(),
  { sub: undefined },
)

const dg = useDg()
const list = (v: string | readonly string[] | undefined) =>
  v === undefined ? [] : typeof v === 'string' ? [v] : [...v]

const points = computed(() => {
  const { x, y, w, h } = props
  const p = Math.min(14, h / 2)
  return [
    [x + p, y],
    [x + w - p, y],
    [x + w, y + h / 2],
    [x + w - p, y + h],
    [x + p, y + h],
    [x, y + h / 2],
  ]
    .map(([a, b]) => `${a},${b}`)
    .join(' ')
})
const lines = computed(() => {
  const all = [
    ...list(props.title).map((text) => ({ text, title: true })),
    ...list(props.sub).map((text) => ({ text, title: false })),
  ]
  const ys = lineCenters(props.y, props.h, all.length, dg.lh)
  return all.map((l, i) => ({ ...l, y: ys[i] ?? 0 }))
})
</script>

<template>
  <g class="dg-dec">
    <polygon :points="points" />
    <text
      v-for="(l, i) in lines"
      :key="i"
      :class="l.title ? 'dg-dec__t' : 'dg-dec__s'"
      :x="props.x + props.w / 2"
      :y="l.y"
      dy="0.35em"
      text-anchor="middle"
    >
      {{ l.text }}
    </text>
  </g>
</template>

<style scoped>
polygon {
  fill: var(--surface-2);
  stroke: var(--text-2);
  stroke-width: 1.25;
  stroke-linejoin: round;
}
.dg-dec__t {
  fill: var(--text);
  font-size: var(--dg-fs);
  font-weight: var(--fw-semibold);
}
.dg-dec__s {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
}
</style>
