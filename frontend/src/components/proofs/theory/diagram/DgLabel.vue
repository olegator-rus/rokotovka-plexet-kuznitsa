<script setup lang="ts">
// Подпись на схеме: ветка «да / нет», что идёт по стрелке, заголовок рамки. Под текстом — ореол цвета фона,
// чтобы линия под подписью не мешала читать.
import { computed } from 'vue'
import { useDg } from './geometry'

const props = withDefaults(
  defineProps<{
    x: number
    /** Центр первой строки. */
    y: number
    text: string | readonly string[]
    anchor?: 'start' | 'middle' | 'end'
    tone?: 'a' | 'b' | 'c' | 'd' | 'accent' | 'muted'
  }>(),
  { anchor: 'middle', tone: undefined },
)

const dg = useDg()
const lines = computed(() => (typeof props.text === 'string' ? [props.text] : [...props.text]))
</script>

<template>
  <g :class="['dg-label', props.tone ? `dg-label--${props.tone}` : '']">
    <text
      v-for="(l, i) in lines"
      :key="i"
      :x="props.x"
      :y="props.y + i * dg.lh"
      dy="0.35em"
      :text-anchor="props.anchor"
    >
      {{ l }}
    </text>
  </g>
</template>

<style scoped>
text {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
  font-weight: var(--fw-medium);
  paint-order: stroke;
  stroke: var(--bg);
  stroke-width: 4px;
  stroke-linejoin: round;
}
.dg-label--a text {
  fill: var(--route-a);
  font-weight: var(--fw-semibold);
}
.dg-label--b text {
  fill: var(--route-b);
  font-weight: var(--fw-semibold);
}
.dg-label--c text {
  fill: var(--route-c);
  font-weight: var(--fw-semibold);
}
.dg-label--d text {
  fill: var(--route-d);
  font-weight: var(--fw-semibold);
}
.dg-label--accent text {
  fill: var(--accent);
  font-weight: var(--fw-semibold);
}
.dg-label--muted text {
  fill: var(--text-3);
}
</style>
