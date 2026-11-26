<script setup lang="ts">
// Цель запроса (§1–2) — первый кликабельный элемент выдачи: тип, подпись, ссылка на
// store.softline.ru в новой вкладке, под ней причина выбора мелким шрифтом.
import { ArrowUpRight } from 'lucide-vue-next'
import type { TargetView } from '@/lib/matchGroups'

defineProps<{ target: TargetView }>()
const emit = defineEmits<{ open: [] }>()
</script>

<template>
  <section class="target" aria-label="Куда ведёт запрос" data-testid="search-target">
    <a
      :href="target.url"
      target="_blank"
      rel="noopener noreferrer"
      class="target__link"
      data-testid="target-link"
      @click="emit('open')"
    >
      <span class="target__kind t-xs">{{ target.kind }}</span>
      <span class="target__label">{{ target.label }}</span>
      <span class="target__go t-sm">
        На сайте
        <ArrowUpRight :size="16" aria-hidden="true" />
        <span class="sr-only"> (откроется в новой вкладке)</span>
      </span>
    </a>
    <p v-if="target.reason" class="target__reason t-xs t-muted" data-testid="target-reason">
      {{ target.reason }}
    </p>
  </section>
</template>

<style scoped>
.target {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0 0 var(--space-3);
}
.target__link {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    'kind go'
    'label go';
  align-items: center;
  column-gap: var(--space-3);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-card);
  background: var(--surface);
  color: var(--text);
  text-decoration: none;
}
.target__link:hover {
  background: var(--surface-2);
}
.target__link:hover .target__label {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.target__kind {
  grid-area: kind;
  color: var(--text-2);
}
.target__label {
  grid-area: label;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.target__go {
  grid-area: go;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--text-2);
  white-space: nowrap;
}
.target__reason {
  margin: 0;
  padding: 0 var(--space-3);
}
</style>
