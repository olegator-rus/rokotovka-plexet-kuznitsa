<script setup lang="ts">
// Кривая «потеря качества → индекс показанной маржи»: точки — положения рычага,
// выбранная и действующая отмечены, положения за упором — пунктиром. Клик по точке выбирает положение.
// Таблица для чтения с экрана — рядом (sr-only). Цвета — только нейтральные токены.
import { computed } from 'vue'
import type { MarginSimPoint } from '@/api/typesCommerce'
import { formatDecimal, formatPercent } from '@/lib/format'
import { curveGeometry, type SimBounds, weightLabel } from '@/lib/margin'

const props = defineProps<{
  items: MarginSimPoint[]
  /** Упор: stop_weight и min_weight симуляции. */
  bounds: SimBounds | null
  selected: number
  active: number
}>()
defineEmits<{ select: [w: number] }>()

const W = 360
const H = 200
const PAD = 28
const g = computed(() => curveGeometry(props.items, props.bounds, W, H, PAD))
/** Где на оси «потеря качества» ноль — качество как при весе контура поиска. */
const x0 = computed(() => {
  const { xMin, xMax } = g.value
  return xMax - xMin > 0 ? PAD + ((0 - xMin) / (xMax - xMin)) * (W - 2 * PAD) : PAD
})
/** «0» на оси — только если не налезает на подписи краёв. */
const showZero = computed(() => x0.value - PAD > 70 && W - PAD - x0.value > 110)
const near = (a: number, b: number) => Math.abs(a - b) < 1e-9
const tip = (p: { weight: number; loss: number; index: number }) =>
  `Вес ${weightLabel(p.weight)}: потеря ${formatPercent(p.loss, 2)}, индекс маржи ${formatDecimal(p.index, 3)}`
</script>

<template>
  <figure class="curve">
    <svg
      :viewBox="`0 0 ${W} ${H}`"
      class="curve__svg"
      role="img"
      aria-label="Кривая: потеря качества и индекс показанной маржи по положениям рычага"
    >
      <line :x1="PAD" :y1="H - PAD" :x2="W - PAD / 2" :y2="H - PAD" class="curve__axis" />
      <line :x1="x0" :y1="PAD / 2" :x2="x0" :y2="H - PAD" class="curve__axis" />
      <text :x="W - PAD / 2" :y="H - 8" text-anchor="end" class="curve__tick">
        потеря качества {{ formatPercent(g.xMax, 2) }}
      </text>
      <text v-if="showZero" :x="x0" :y="H - 8" text-anchor="middle" class="curve__tick">0</text>
      <text v-if="g.xMin < 0" :x="PAD" :y="H - 8" class="curve__tick">
        выигрыш {{ formatPercent(-g.xMin, 2) }}
      </text>
      <text :x="W - PAD / 2" :y="10" text-anchor="end" class="curve__tick">
        индекс маржи {{ formatDecimal(g.yMax, 3) }}
      </text>
      <path :d="g.path" class="curve__line" fill="none" />
      <g
        v-for="p in g.points"
        :key="p.weight"
        :class="[
          'curve__pt',
          {
            'curve__pt--sel': near(p.weight, selected),
            'curve__pt--off': p.disabled,
          },
        ]"
        @click="!p.disabled && $emit('select', p.weight)"
      >
        <title>{{ tip(p) }}{{ p.disabled ? ' — за упором' : '' }}</title>
        <circle :cx="p.x" :cy="p.y" r="12" class="curve__hit" />
        <circle :cx="p.x" :cy="p.y" r="5" class="curve__dot" />
        <circle v-if="near(p.weight, active)" :cx="p.x" :cy="p.y" r="9" class="curve__ring" />
        <text
          v-if="p.label"
          :x="p.x"
          :y="p.y - 12"
          :text-anchor="p.x > W - 60 ? 'end' : p.x < 40 ? 'start' : 'middle'"
          class="curve__lbl"
        >
          {{ p.label }}
        </text>
      </g>
    </svg>
    <figcaption class="curve__legend t-xs t-muted">
      <span
        ><span class="curve__key curve__key--sel" aria-hidden="true" /> выбранное положение</span
      >
      <span><span class="curve__key curve__key--ring" aria-hidden="true" /> действует сейчас</span>
      <span><span class="curve__key curve__key--off" aria-hidden="true" /> за упором</span>
    </figcaption>
    <table class="sr-only">
      <caption>
        Положения рычага
      </caption>
      <thead>
        <tr>
          <th scope="col">Вес</th>
          <th scope="col">Потеря качества</th>
          <th scope="col">Индекс маржи</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in g.points" :key="p.weight">
          <th scope="row">{{ weightLabel(p.weight) }}</th>
          <td>{{ formatPercent(p.loss, 1) }}</td>
          <td>{{ formatDecimal(p.index, 3) }}</td>
        </tr>
      </tbody>
    </table>
  </figure>
</template>

<style scoped>
.curve {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
}
.curve__svg {
  width: 100%;
  height: auto;
  max-height: 16rem;
}
.curve__axis {
  stroke: var(--border-control);
  stroke-width: 1;
}
.curve__tick,
.curve__lbl {
  fill: var(--text-2);
  font-size: 10px;
  font-family: var(--font-sans);
}
.curve__line {
  stroke: var(--text-2);
  stroke-width: 2;
  stroke-linejoin: round;
}
.curve__pt {
  cursor: pointer;
}
.curve__hit {
  fill: transparent;
}
.curve__dot {
  fill: var(--surface);
  stroke: var(--text-2);
  stroke-width: 2;
}
.curve__pt--sel .curve__dot {
  fill: var(--accent);
  stroke: var(--accent);
}
.curve__pt--sel .curve__lbl {
  fill: var(--text);
  font-weight: var(--fw-semibold);
}
.curve__pt--off {
  cursor: not-allowed;
}
.curve__pt--off .curve__dot {
  stroke: var(--text-3);
  stroke-dasharray: 2 2;
}
.curve__pt--off .curve__lbl {
  fill: var(--text-3);
}
.curve__ring {
  fill: none;
  stroke: var(--text);
  stroke-width: 1.5;
}
.curve__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
}
.curve__legend > span {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.curve__key {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--text-2);
}
.curve__key--sel {
  background: var(--accent);
  border-color: var(--accent);
}
.curve__key--ring {
  border: 1.5px solid var(--text);
  box-shadow: inset 0 0 0 2px var(--bg);
}
.curve__key--off {
  border-style: dashed;
  border-color: var(--text-3);
}
</style>
