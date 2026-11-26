<script setup lang="ts">
import type { Category, CategoryId } from '@/lib/catalog'

// Фильтр витрины по категории: «Все» + категории. На узком экране ряд прокручивается вбок.
defineProps<{ categories: readonly Category[] }>()
const selected = defineModel<CategoryId | null>({ required: true })
</script>

<template>
  <div class="chips" role="group" aria-label="Категории">
    <button type="button" class="chip" :aria-pressed="selected === null" @click="selected = null">
      Все
    </button>
    <button
      v-for="c in categories"
      :key="c.id"
      type="button"
      class="chip"
      :aria-pressed="selected === c.id"
      @click="selected = selected === c.id ? null : c.id"
    >
      {{ c.title }}
    </button>
  </div>
</template>

<style scoped>
.chips {
  display: flex;
  gap: 0.5rem;
  overflow-x: auto;
  margin-inline: -1rem;
  padding: 0.25rem 1rem;
  scrollbar-width: none;
}
.chips::-webkit-scrollbar {
  display: none;
}
@media (min-width: 768px) {
  .chips {
    flex-wrap: wrap;
    overflow: visible;
    margin-inline: 0;
    padding-inline: 0;
  }
}
.chip {
  flex: none;
  min-height: 2.5rem;
  padding: 0 1rem;
  border: 1px solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-body-2);
  font-weight: 500;
  cursor: pointer;
  transition:
    background-color 120ms ease-out,
    border-color 120ms ease-out;
}
.chip:hover {
  border-color: var(--border-control);
}
.chip[aria-pressed='true'] {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}
</style>
