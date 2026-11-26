<script setup lang="ts">
// Строка сравнения цены за место (contract): факт про пару редакций при условиях запроса —
// текст и условия мелко. Без «лучше» и «выгоднее», без дат.
import { computed } from 'vue'
import { Scale } from 'lucide-vue-next'
import type { Comparison } from '@/api/types'
import { comparisonView } from '@/lib/offerNotes'

const props = defineProps<{ comparison?: Comparison | null }>()
const view = computed(() => comparisonView(props.comparison))
</script>

<template>
  <p v-if="view" class="cmpline" role="note" data-testid="comparison">
    <Scale :size="16" aria-hidden="true" class="cmpline__icon" />
    <span class="cmpline__body">
      <span class="cmpline__text t-sm">{{ view.text }}</span>
      <span v-if="view.conditions" class="cmpline__cond t-xs" data-testid="comparison-conditions">
        {{ view.conditions }}
      </span>
      <span v-if="view.prices" class="cmpline__cond cmpline__prices t-xs">{{ view.prices }}</span>
    </span>
  </p>
</template>

<style scoped>
.cmpline {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0 0 var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.cmpline__icon {
  flex: none;
  margin-top: 2px;
  color: var(--text-2);
}
.cmpline__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  overflow-wrap: anywhere;
}
.cmpline__text {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.cmpline__cond {
  color: var(--text-2);
}
.cmpline__prices {
  font-variant-numeric: tabular-nums;
}
</style>
