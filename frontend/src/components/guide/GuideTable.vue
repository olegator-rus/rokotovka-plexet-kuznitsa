<script setup lang="ts">
// Таблица сравнения (SPEC §8): отдельный компонент, не текст. Закреплённые заголовки строки и колонок, четыре вида
// ячейки, значок источника у строки о функции, «только различия». На телефоне — две колонки, остальные
// переключателем; прокручивается сама таблица, не страница.
import { computed, ref, watch } from 'vue'
import { Check, Minus } from 'lucide-vue-next'
import type { GuideClaim, GuideComparison } from '@/api/typesGuide'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { buildTable, claimIndex, visibleColumns } from '@/lib/guide'
import GuideSource from './GuideSource.vue'

const props = defineProps<{ comparison: GuideComparison; claims: GuideClaim[] }>()

const narrow = useMediaQuery('(max-width: 40rem)')
const onlyDiff = ref(false)
const picked = ref(1)
watch(
  () => props.comparison,
  () => (picked.value = 1),
)

const count = computed(() => props.comparison.columns.length)
const shown = computed(() => visibleColumns(count.value, narrow.value, picked.value))
const view = computed(() =>
  buildTable(props.comparison, { onlyDiff: onlyDiff.value, columns: shown.value }),
)
const facts = computed(() => claimIndex(props.claims))

/** Источник строки: первый факт строки, у которого он есть. */
function rowClaim(cells: { factId: string | null }[]) {
  for (const c of cells) {
    const f = c.factId ? facts.value.get(c.factId) : undefined
    if (f?.source) return f
  }
  return null
}
</script>

<template>
  <section class="gtable" aria-label="Сравнение редакций" data-testid="guide-table">
    <div class="gtable__bar">
      <UiSwitch
        v-if="view.total > 1"
        v-model="onlyDiff"
        label="Только различия"
        data-testid="guide-only-diff"
      />
      <div v-if="narrow && count > 2" class="gtable__pick" role="group" aria-label="С чем сравнить">
        <span class="t-xs t-muted">Сравнить с:</span>
        <button
          v-for="i in count - 1"
          :key="i"
          type="button"
          :class="['gtable__pick-btn', { 'gtable__pick-btn--on': shown[1] === i }]"
          :aria-pressed="shown[1] === i"
          @click="picked = i"
        >
          {{ comparison.columns[i]?.title }}
        </button>
      </div>
    </div>

    <div class="gtable__scroll" tabindex="0" aria-label="Таблица сравнения, прокручивается">
      <table class="gtable__table">
        <thead>
          <tr>
            <th scope="col" class="gtable__corner"><span class="sr-only">Параметр</span></th>
            <th v-for="c in view.columns" :key="c.id" scope="col" class="gtable__col">
              <a
                v-if="c.url"
                :href="c.url"
                class="link"
                target="_blank"
                rel="noopener noreferrer"
                >{{ c.title }}</a
              >
              <template v-else>{{ c.title }}</template>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="r in view.rows"
            :key="r.key"
            :class="{ 'gtable__row--diff': r.differs }"
            :data-row="r.key"
          >
            <th scope="row" class="gtable__label">
              <span class="gtable__label-in">
                <span>
                  {{ r.label }}
                  <span v-if="r.note" class="gtable__rnote t-xs t-muted">{{ r.note }}</span>
                </span>
                <GuideSource
                  v-if="rowClaim(r.cells)"
                  :claim="rowClaim(r.cells)!"
                  :label="r.label"
                />
              </span>
            </th>
            <td
              v-for="(cell, ci) in r.cells"
              :key="ci"
              :class="['gtable__cell', `gtable__cell--${cell.tone}`]"
              :data-kind="cell.tone"
            >
              <Check v-if="cell.tone === 'yes'" :size="14" aria-hidden="true" />
              <Minus v-else-if="cell.tone === 'no'" :size="14" aria-hidden="true" />
              <span>{{ cell.text }}</span>
            </td>
          </tr>
          <tr v-if="!view.rows.length">
            <td :colspan="view.columns.length + 1" class="gtable__empty t-sm t-muted">
              Различий в показанных строках нет.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="t-xs t-muted gtable__note">
      Пустая ячейка — «нет сведений», а не «функции нет». Цены и итоги — из каталога магазина.
    </p>
  </section>
</template>

<style scoped>
.gtable {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.gtable__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.gtable__pick {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1);
}
.gtable__pick-btn {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-xs);
  cursor: pointer;
}
.gtable__pick-btn--on {
  border-color: var(--text);
  background: var(--text);
  color: var(--bg);
}
.gtable__scroll {
  max-width: 100%;
  max-height: min(70vh, 32rem);
  overflow: auto;
  overscroll-behavior-x: contain;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
}
.gtable__table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gtable__table th,
.gtable__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
  text-align: left;
  vertical-align: top;
}
.gtable__table tbody tr:last-child > * {
  border-bottom: 0;
}
thead th {
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--surface-2);
  font-weight: var(--fw-semibold);
}
.gtable__corner,
.gtable__label {
  position: sticky;
  left: 0;
  z-index: 1;
  background: var(--surface);
}
thead .gtable__corner {
  z-index: 3;
  background: var(--surface-2);
}
.gtable__col {
  min-width: 8.5rem;
}
.gtable__label {
  width: 34%;
  min-width: 8rem;
  max-width: 14rem;
  border-right: var(--border-w) solid var(--border);
  font-weight: var(--fw-medium);
}
.gtable__label-in {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2);
}
.gtable__rnote {
  display: block;
  margin-top: 2px;
  font-weight: var(--fw-regular);
}
.gtable__row--diff > .gtable__label {
  box-shadow: inset 3px 0 0 var(--accent);
}
.gtable__cell {
  font-variant-numeric: tabular-nums;
}
.gtable__cell > svg {
  display: inline-block;
  margin-right: var(--space-1);
  vertical-align: -2px;
}
@media (max-width: 40rem) {
  .gtable__col {
    min-width: 7rem;
  }
  .gtable__label {
    min-width: 6.5rem;
  }
  .gtable__table th,
  .gtable__table td {
    padding: var(--space-2);
  }
}
.gtable__cell--yes {
  color: var(--route-a);
  font-weight: var(--fw-medium);
}
.gtable__cell--no {
  color: var(--text-2);
}
.gtable__cell--unknown,
.gtable__cell--depends {
  color: var(--text-3);
  font-style: italic;
}
.gtable__empty {
  text-align: center;
}
.gtable__note {
  margin: 0;
}
</style>
