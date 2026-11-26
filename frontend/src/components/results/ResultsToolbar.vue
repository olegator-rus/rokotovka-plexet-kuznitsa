<script setup lang="ts">
// Строка управления страницы результатов: сортировка с пояснением,
// группировка «По продукту / Без группировки». Заголовок с числом продуктов и вариантов — на странице.
// «Популярные» и «По вендору» не показываются: популярность в выдаче не измерена отдельно, свёртку по вендору
// сервер не считает (название не обещает метод, которого нет).
import { computed } from 'vue'
import { Info } from 'lucide-vue-next'
import type { SearchResponse } from '@/api/types'
import type { ResultsGroup, ResultsSort } from '@/api/typesResults'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiTooltip from '@/components/ui/UiTooltip.vue'
import { RESULT_SORTS } from '@/lib/searchState'
import { RECOMMENDED_NOTE, SORT_LABELS } from '@/lib/resultsView'

const props = defineProps<{
  response: SearchResponse
  sort: ResultsSort
  group: ResultsGroup
  /** Точный артикул: сервер выключил свёртку — переключатель недоступен. */
  forcedFlat?: boolean
}>()

const emit = defineEmits<{ sort: [value: ResultsSort]; group: [value: ResultsGroup] }>()

const options = RESULT_SORTS.map((v) => ({ value: v, label: SORT_LABELS[v] }))
const note = computed(() => props.response.sort?.note || RECOMMENDED_NOTE)
const sortModel = computed({
  get: () => props.sort as string,
  set: (v: string) => emit('sort', v as ResultsSort),
})
</script>

<template>
  <div class="rtool">
    <div class="rtool__row">
      <div class="rtool__sort">
        <UiSelect v-model="sortModel" :options="options" label="Сортировка" prefix="Сортировка" />
        <UiTooltip :text="note" side="bottom">
          <button type="button" class="rtool__info" :aria-label="`Как устроен порядок: ${note}`">
            <Info :size="16" aria-hidden="true" />
          </button>
        </UiTooltip>
      </div>
      <div class="rtool__group" role="group" aria-label="Группировка">
        <button
          type="button"
          class="rtool__seg"
          :aria-pressed="group === 'family'"
          :disabled="forcedFlat"
          @click="group !== 'family' && emit('group', 'family')"
        >
          По продукту
        </button>
        <button
          type="button"
          class="rtool__seg"
          :aria-pressed="group === 'none'"
          :disabled="forcedFlat"
          @click="group !== 'none' && emit('group', 'none')"
        >
          Без группировки
        </button>
      </div>
    </div>
    <p v-if="forcedFlat" class="t-xs t-muted">
      Точный артикул: показан сам товар, группировка по продукту отключена.
    </p>
  </div>
</template>

<style scoped>
.rtool {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.rtool__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
}
.rtool__sort {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
}
.rtool__info {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: none;
  color: var(--text-2);
  cursor: help;
}
.rtool__info:hover {
  color: var(--text);
  background: var(--surface-2);
}
.rtool__group {
  display: inline-flex;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  overflow: hidden;
}
.rtool__seg {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: 0;
  background: var(--surface);
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.rtool__seg + .rtool__seg {
  border-left: var(--border-w) solid var(--border);
}
.rtool__seg[aria-pressed='true'] {
  background: var(--surface-3);
  color: var(--text);
  font-weight: var(--fw-medium);
}
.rtool__seg:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
</style>
