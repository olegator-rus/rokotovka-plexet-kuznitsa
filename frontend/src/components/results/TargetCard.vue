<script setup lang="ts">
// Цель запроса (§1–2) — первый кликабельный элемент выдачи: тип, подпись, ссылка на
// store.softline.ru в новой вкладке, под ней причина выбора мелким шрифтом. Категория без страницы на сайте —
// заголовок и чипы производителей (`clarifications`): нажатие перезапускает поиск. Сервисная цель без справки —
// кнопка «Помощь менеджера».
import { ArrowUpRight, Filter, Headset } from 'lucide-vue-next'
import type { Refinement } from '@/api/typesResults'
import type { TargetView } from '@/lib/matchGroups'

withDefaults(
  defineProps<{
    target: TargetView
    chips?: Refinement[]
    /** Категория без страницы на сайте есть в фасете — цель включает фильтр выдачи по ней. */
    filterable?: boolean
  }>(),
  { chips: () => [], filterable: false },
)
const emit = defineEmits<{ open: []; manager: []; filter: []; chip: [item: Refinement] }>()
</script>

<template>
  <section class="target" aria-label="Куда ведёт запрос" data-testid="search-target">
    <a
      v-if="target.action === 'link'"
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
    <button
      v-else-if="target.action === 'manager'"
      type="button"
      class="target__link target__btn"
      data-testid="target-manager"
      @click="emit('manager')"
    >
      <span class="target__kind t-xs">{{ target.kind }}</span>
      <span class="target__label">{{ target.label }}</span>
      <span class="target__go t-sm">
        Написать
        <Headset :size="16" aria-hidden="true" />
      </span>
    </button>
    <button
      v-else-if="filterable"
      type="button"
      class="target__link target__btn"
      data-testid="target-filter"
      @click="emit('filter')"
    >
      <span class="target__kind t-xs">{{ target.kind }}</span>
      <span class="target__label">{{ target.label }}</span>
      <span class="target__go t-sm">
        Только эта категория
        <Filter :size="16" aria-hidden="true" />
      </span>
    </button>
    <div v-else class="target__link target__static" data-testid="target-static">
      <span class="target__kind t-xs">{{ target.kind }}</span>
      <span class="target__label">{{ target.label }}</span>
    </div>
    <p v-if="target.reason" class="target__reason t-xs t-muted" data-testid="target-reason">
      {{ target.reason }}
    </p>
    <ul
      v-if="chips.length"
      class="target__chips"
      aria-label="Производители"
      data-testid="target-chips"
    >
      <li v-for="c in chips" :key="`${c.field}:${c.value}:${c.label}`">
        <button
          type="button"
          class="target__chip"
          :aria-label="`Искать: ${c.query}`"
          @click="emit('chip', c)"
        >
          {{ c.label }}
          <span v-if="c.hits" class="t-num t-muted target__hits">{{ c.hits }}</span>
        </button>
      </li>
    </ul>
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
.target__btn {
  width: 100%;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.target__static {
  grid-template-areas:
    'kind'
    'label';
  grid-template-columns: minmax(0, 1fr);
  border-color: var(--border);
  background: var(--surface-2);
}
.target__static:hover {
  background: var(--surface-2);
}
.target__static:hover .target__label {
  text-decoration: none;
}
.target__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0 var(--space-3);
  list-style: none;
}
.target__chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.target__chip:hover {
  background: var(--surface-2);
}
.target__hits {
  font-size: var(--fs-xs);
}
.target__reason {
  margin: 0;
  padding: 0 var(--space-3);
}
</style>
