<script setup lang="ts">
// Применённые фильтры: залитые чипы «Срок: 1 год ×» — снимаются по одному; «Сбросить фильтры»
// снимает все, запрос в строке остаётся.
import { X } from 'lucide-vue-next'
import type { Facet, SearchFilter } from '@/api/typesP2'
import { filterChipLabel } from '@/lib/resultsView'

defineProps<{ filters: SearchFilter[]; facets: Facet[] }>()
const emit = defineEmits<{ remove: [filter: SearchFilter]; reset: [] }>()
</script>

<template>
  <section v-if="filters.length" class="applied" aria-label="Применённые фильтры">
    <ul class="applied__list">
      <li v-for="f in filters" :key="`${f.key}:${f.op}:${f.value}`">
        <span class="applied__chip">
          <span class="applied__label">{{ filterChipLabel(f, facets) }}</span>
          <button
            type="button"
            class="applied__remove"
            :aria-label="`Снять фильтр «${filterChipLabel(f, facets)}»`"
            @click="emit('remove', f)"
          >
            <X :size="14" aria-hidden="true" />
          </button>
        </span>
      </li>
    </ul>
    <button type="button" class="link t-sm" @click="emit('reset')">Сбросить фильтры</button>
  </section>
</template>

<style scoped>
.applied {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.applied__list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.applied__chip {
  display: inline-flex;
  align-items: center;
  min-height: var(--control-h-sm);
  border: var(--border-w) solid var(--accent);
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}
.applied__label {
  padding: 0 var(--space-1) 0 var(--space-3);
}
.applied__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.applied__remove:hover {
  background: color-mix(in srgb, currentColor 15%, transparent);
}
</style>
