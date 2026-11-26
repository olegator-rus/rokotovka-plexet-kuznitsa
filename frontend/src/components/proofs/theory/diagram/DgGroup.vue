<script setup lang="ts">
// Рамка группы: граница (свой сервер, один вызов модели) или набор узлов под одной подписью.
// Подпись — в левом верхнем углу внутри рамки.
import { useDg } from './geometry'

const props = withDefaults(
  defineProps<{
    x: number
    y: number
    w: number
    h: number
    label?: string
  }>(),
  { label: undefined },
)

const dg = useDg()
</script>

<template>
  <g class="dg-group">
    <rect :x="props.x" :y="props.y" :width="props.w" :height="props.h" rx="14" />
    <text v-if="props.label" :x="props.x + 10" :y="props.y + dg.lh - 2" dy="0.35em">
      {{ props.label }}
    </text>
  </g>
</template>

<style scoped>
rect {
  fill: none;
  stroke: var(--border-control);
  stroke-width: 1;
  stroke-dasharray: 2 4;
  stroke-linecap: round;
}
text {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
  font-weight: var(--fw-semibold);
}
</style>
