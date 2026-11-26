<script setup lang="ts">
// Изменение метрики — одно из трёх мест, где цвет несёт смысл. Стрелка показывает, куда сдвинулось
// значение; цвет — лучше это или хуже с учётом направления метрики; текст со знаком стоит всегда.
import { ArrowDown, ArrowUp, Equal } from 'lucide-vue-next'
import type { MetricDelta } from '@/lib/metrics'

defineProps<{ delta: MetricDelta }>()
</script>

<template>
  <span
    :class="['delta', `delta--${delta.verdict}`]"
    :aria-label="delta.label"
    :title="delta.label"
  >
    <ArrowUp v-if="delta.arrow === 'up'" :size="12" aria-hidden="true" />
    <ArrowDown v-else-if="delta.arrow === 'down'" :size="12" aria-hidden="true" />
    <Equal v-else :size="12" aria-hidden="true" />
    <span aria-hidden="true">{{ delta.verdict === 'same' ? '0' : delta.text }}</span>
  </span>
</template>

<style scoped>
.delta {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.delta--better {
  color: var(--delta-up);
}
.delta--worse {
  color: var(--delta-down);
}
.delta--same {
  color: var(--text-3);
}
</style>
