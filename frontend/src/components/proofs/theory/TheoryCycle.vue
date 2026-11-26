<script setup lang="ts">
// Схема «цикл»: шаги по порядку в сетке (на узком экране — столбцом) и строка возврата к шагу from.
// branch — побочный выход у шага (например, «не прошла — следующая гипотеза»).
import { RotateCcw } from 'lucide-vue-next'

export interface CycleStep {
  title: string
  text?: string
  branch?: string
}

const props = defineProps<{ steps: readonly CycleStep[]; back: string; label: string }>()
</script>

<template>
  <figure class="cycle">
    <figcaption class="sr-only">{{ props.label }}</figcaption>
    <ol class="cycle__list" role="list">
      <li v-for="(s, i) in props.steps" :key="s.title" class="cycle__step">
        <div class="cycle__head">
          <span class="cycle__num t-num" aria-hidden="true">{{ i + 1 }}</span>
          <strong>{{ s.title }}</strong>
        </div>
        <p v-if="s.text" class="t-sm cycle__text">{{ s.text }}</p>
        <p v-if="s.branch" class="t-xs cycle__branch">{{ s.branch }}</p>
      </li>
    </ol>
    <p class="cycle__back t-sm">
      <RotateCcw :size="16" aria-hidden="true" />
      <span>{{ props.back }}</span>
    </p>
  </figure>
</template>

<style scoped>
.cycle {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  min-width: 0;
}
.cycle__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
  counter-reset: none;
}
.cycle__step {
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  min-width: 0;
}
.cycle__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.cycle__num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 1.375rem;
  height: 1.375rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--fs-xs);
  font-weight: var(--fw-bold);
}
.cycle__text {
  margin: var(--space-1) 0 0;
  color: var(--text-2);
}
.cycle__branch {
  margin: var(--space-1) 0 0;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--text-2);
}
.cycle__back {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) dashed var(--border-control);
  border-radius: var(--radius-control);
  color: var(--text-2);
}

@media (min-width: 768px) {
  .cycle__list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (min-width: 1280px) {
  .cycle__list {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
