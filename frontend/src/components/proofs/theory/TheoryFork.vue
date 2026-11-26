<script setup lang="ts">
// Схема «развилка»: вопросы по порядку; первый, на который ответ «да», решает исход. Внизу — «иначе».
// Исход — маршрут A–D (бейдж с буквой и подписью) и пояснение.
import { CornerDownRight } from 'lucide-vue-next'
import { ROUTE_LABELS } from '@/lib/trace'

type Route = 'A' | 'B' | 'C' | 'D'

export interface ForkRow {
  ask: string
  route: Route
  note?: string
}

const props = defineProps<{
  rows: readonly ForkRow[]
  otherwise: { route: Route; note?: string }
  label: string
}>()
</script>

<template>
  <figure class="fork">
    <figcaption class="sr-only">{{ props.label }}</figcaption>
    <ol class="fork__list" role="list">
      <li v-for="(r, i) in props.rows" :key="r.ask" class="fork__row">
        <span class="fork__num t-num" aria-hidden="true">{{ i + 1 }}</span>
        <p class="fork__ask">{{ r.ask }}</p>
        <div class="fork__out">
          <CornerDownRight :size="16" aria-hidden="true" class="fork__icon" />
          <span :class="['badge', 'badge--tone', `route-${r.route.toLowerCase()}`]">
            <strong>{{ r.route }}</strong> {{ ROUTE_LABELS[r.route] }}
          </span>
          <span v-if="r.note" class="t-sm t-muted">{{ r.note }}</span>
        </div>
      </li>
      <li class="fork__row fork__row--else">
        <span class="fork__num" aria-hidden="true">·</span>
        <p class="fork__ask">Ничего из этого</p>
        <div class="fork__out">
          <CornerDownRight :size="16" aria-hidden="true" class="fork__icon" />
          <span :class="['badge', 'badge--tone', `route-${props.otherwise.route.toLowerCase()}`]">
            <strong>{{ props.otherwise.route }}</strong> {{ ROUTE_LABELS[props.otherwise.route] }}
          </span>
          <span v-if="props.otherwise.note" class="t-sm t-muted">{{ props.otherwise.note }}</span>
        </div>
      </li>
    </ol>
  </figure>
</template>

<style scoped>
.fork {
  margin: 0;
  min-width: 0;
}
.fork__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.fork__row {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr);
  gap: var(--space-1) var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
}
.fork__row--else {
  background: var(--surface-2);
}
.fork__num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.375rem;
  height: 1.375rem;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
}
.fork__ask {
  margin: 0;
  font-weight: var(--fw-medium);
}
.fork__out {
  grid-column: 2;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.fork__icon {
  color: var(--text-3);
}

@media (min-width: 768px) {
  .fork__row {
    grid-template-columns: 1.5rem minmax(0, 1fr) minmax(0, 1fr);
    align-items: center;
  }
  .fork__out {
    grid-column: 3;
  }
}
</style>
