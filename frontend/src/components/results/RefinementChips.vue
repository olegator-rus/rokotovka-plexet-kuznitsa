<script setup lang="ts">
// Предложенные уточнения над выдачей (RESEARCH Г): со знаком «+» и пунктирной рамкой — отличаются
// от чипов «как поняли запрос» (нейтральные) и применённых фильтров (залитые, с крестиком). Каждое проверено
// сервером числом результатов (hits). На мобильном — первые 4.
import { Plus } from 'lucide-vue-next'
import type { Refinement } from '@/api/typesResults'
import { formatInt, plural } from '@/lib/format'

defineProps<{ items: Refinement[] }>()
const emit = defineEmits<{ apply: [item: Refinement] }>()
</script>

<template>
  <section v-if="items.length" class="refine" aria-label="Уточнить выдачу">
    <span class="t-sm t-muted refine__title">Уточнить:</span>
    <ul class="refine__list">
      <li
        v-for="(r, i) in items"
        :key="`${r.field}:${r.value}:${r.label}`"
        :class="{ refine__extra: i >= 4 }"
      >
        <button
          type="button"
          class="refine__chip"
          :aria-label="`Уточнить: ${r.label} — ${formatInt(r.hits)} ${plural(r.hits, ['продукт', 'продукта', 'продуктов'])}`"
          @click="emit('apply', r)"
        >
          <Plus :size="14" aria-hidden="true" />
          {{ r.label }}
          <span class="t-num t-muted refine__hits">{{ formatInt(r.hits) }}</span>
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.refine {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.refine__list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.refine__chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3) 0 var(--space-2);
  border: var(--border-w) dashed var(--border-control);
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.refine__chip:hover {
  background: var(--surface-2);
  border-style: solid;
}
.refine__hits {
  font-size: var(--fs-xs);
}
@media (max-width: 40rem) {
  .refine__extra {
    display: none;
  }
}
</style>
