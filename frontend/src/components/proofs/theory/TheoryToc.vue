<script setup lang="ts">
// Содержание «Теоретических основ»: части — блоками друг под другом (их размеры слишком разные для колонок),
// разделы внутри — плитками в сетке. Номера сквозные: разделы идут по пути запроса, это последовательность.
import { THEORY_PARTS } from './sections'

const emit = defineEmits<{ go: [id: string] }>()

let n = 0
const parts = THEORY_PARTS.map((p) => ({
  title: p.title,
  sections: p.sections.map((s) => ({ ...s, n: ++n })),
}))
</script>

<template>
  <nav class="toc panel" aria-labelledby="theory-toc-title">
    <h3 id="theory-toc-title" class="toc__title">Содержание</h3>
    <div v-for="part in parts" :key="part.title" class="toc__part">
      <p class="toc__part-title">
        {{ part.title }}
        <span class="toc__range t-num" aria-hidden="true">
          {{ part.sections[0]?.n
          }}<template v-if="part.sections.length > 1">–{{ part.sections.at(-1)?.n }}</template>
        </span>
      </p>
      <ol class="toc__list" role="list">
        <li v-for="s in part.sections" :key="s.id">
          <button type="button" class="toc__link" @click="emit('go', s.id)">
            <span class="toc__num t-num" aria-hidden="true">{{ s.n }}</span>
            <span>{{ s.title }}</span>
          </button>
        </li>
      </ol>
    </div>
  </nav>
</template>

<style scoped>
.toc {
  container-type: inline-size;
  overflow: hidden;
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--space-4));
}
.toc__title {
  margin: 0;
  padding: var(--space-4) var(--space-4) var(--space-3);
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-bold);
}
.toc__part {
  padding: var(--space-3) var(--space-2) var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.theory .toc__part-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-3);
  margin: 0 0 var(--space-2);
  padding: 0 var(--space-2);
  color: var(--accent);
  font-size: var(--fs-body);
  font-weight: var(--fw-bold);
}
.toc__range {
  color: var(--text-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
}
.toc__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.toc__link {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  height: 100%;
  min-height: var(--touch-min);
  padding: var(--space-1) var(--space-2);
  border: 0;
  border-radius: var(--radius-control);
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-medium);
  text-align: left;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-standard);
}
.toc__link:hover {
  background: var(--accent-soft);
}
.toc__num {
  display: grid;
  flex: none;
  place-items: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: var(--fs-xs);
  font-weight: var(--fw-bold);
}
.toc__link:hover .toc__num {
  background: var(--accent);
  color: var(--on-accent);
}

/* Две колонки — по ширине самого блока: рядом стоит боковое меню «Доказательств». */
@container (min-width: 38rem) {
  .toc__list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
