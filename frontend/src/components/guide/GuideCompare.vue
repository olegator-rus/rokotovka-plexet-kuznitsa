<script setup lang="ts">
// Сравнение редакций (UI 6.3): 2–3 колонки и 4–6 строк в порядке сервера, четыре значения ячейки с легендой,
// остальное — в «Все характеристики». Под таблицей — источник, строка разницы и переключатель «Выбрано», который
// меняет итоговую строку. На телефоне — строки по две редакции: название характеристики, под ним два значения.
import { computed, ref, watch } from 'vue'
import { ChevronDown, ChevronUp } from 'lucide-vue-next'
import type { GuideScreenRow, GuideScreenTable } from '@/api/typesGuide'
import { LEGEND, MARK_SIGN, phoneColumns } from '@/lib/guideView'
import GuideSource from './GuideSource.vue'

const props = defineProps<{
  table: GuideScreenTable
  selectedId: string
  narrow?: boolean
  busy?: boolean
}>()
const emit = defineEmits<{ select: [offerId: string] }>()

const more = ref(false)
const second = ref(1)
watch(
  () => props.table,
  () => {
    more.value = false
    second.value = 1
  },
)
const cols = computed(() =>
  props.narrow
    ? phoneColumns(props.table.columns.length, second.value)
    : props.table.columns.map((_, i) => i),
)
const moreRows = computed(() => props.table.more_rows ?? [])
const shownRows = computed<GuideScreenRow[]>(() =>
  more.value ? [...props.table.rows, ...moreRows.value] : props.table.rows,
)
const marks = computed(() => {
  const seen = new Set<string>()
  for (const r of shownRows.value) for (const c of r.cells) seen.add(c.mark)
  return LEGEND.filter((l) => seen.has(l.mark))
})
const canExpand = computed(() => moreRows.value.length > 0)
const selectable = computed(() => props.table.columns.filter((c) => c.offer_id).length > 1)
</script>

<template>
  <section class="gc" aria-labelledby="guide-compare-title" data-testid="guide-table">
    <div class="gc__head">
      <h3 id="guide-compare-title" class="gc__title">{{ table.title || 'Различия редакций' }}</h3>
      <label v-if="narrow && table.columns.length > 2" class="gc__pick">
        <span class="sr-only">Вторая колонка</span>
        <select v-model.number="second">
          <option v-for="(c, i) in table.columns.slice(1)" :key="c.offer_id" :value="i + 1">
            {{ c.title }}
          </option>
        </select>
      </label>
    </div>

    <table v-if="!narrow" class="gc__table">
      <thead>
        <tr>
          <th scope="col"><span class="sr-only">Характеристика</span></th>
          <th
            v-for="i in cols"
            :key="table.columns[i]!.offer_id || i"
            scope="col"
            :class="{ 'is-picked': table.columns[i]!.offer_id === selectedId }"
          >
            {{ table.columns[i]!.title }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="r in shownRows"
          :key="r.key"
          :class="{ 'is-asked': r.asked }"
          :data-row="r.key"
          data-testid="guide-row"
        >
          <th scope="row">{{ r.label }}</th>
          <td
            v-for="i in cols"
            :key="i"
            :class="['gc__cell', `gc__cell--${r.cells[i]?.mark ?? 'unknown'}`]"
            :data-mark="r.cells[i]?.mark"
          >
            <span v-if="MARK_SIGN[r.cells[i]?.mark ?? '']" class="gc__sign" aria-hidden="true">{{
              MARK_SIGN[r.cells[i]!.mark]
            }}</span>
            <span class="gc__val">{{ r.cells[i]?.text || 'Нет сведений' }}</span>
          </td>
        </tr>
      </tbody>
    </table>

    <dl v-else class="gc__rows">
      <div
        v-for="r in shownRows"
        :key="r.key"
        :class="['gc__prow', { 'is-asked': r.asked }]"
        :data-row="r.key"
        data-testid="guide-row"
      >
        <dt>{{ r.label }}</dt>
        <dd v-for="i in cols" :key="i" :class="['gc__cell', `gc__cell--${r.cells[i]?.mark}`]">
          <span class="gc__col">{{ table.columns[i]!.title }}</span>
          <span>
            <span v-if="MARK_SIGN[r.cells[i]?.mark ?? '']" class="gc__sign" aria-hidden="true">{{
              MARK_SIGN[r.cells[i]!.mark]
            }}</span>
            <span class="gc__val">{{ r.cells[i]?.text || 'Нет сведений' }}</span>
          </span>
        </dd>
      </div>
    </dl>

    <p v-if="marks.length" class="gc__legend" data-testid="guide-legend">
      <span v-for="l in marks" :key="l.mark" :title="l.hint">
        <span class="gc__sign" aria-hidden="true">{{ l.sign }}</span> {{ l.text }}
      </span>
    </p>

    <p class="gc__links">
      <GuideSource v-if="table.source" :source="table.source" />
      <button
        v-if="canExpand"
        type="button"
        class="gc__more"
        :aria-expanded="more"
        data-testid="guide-more-rows"
        @click="more = !more"
      >
        {{ more ? 'Скрыть характеристики' : 'Все характеристики' }}
        <ChevronUp v-if="more" :size="14" aria-hidden="true" />
        <ChevronDown v-else :size="14" aria-hidden="true" />
      </button>
    </p>
    <p v-if="table.hidden_label && (more || !canExpand)" class="gc__note">
      {{ table.hidden_label }}
    </p>
    <p v-if="table.vendors_note" class="gc__note">{{ table.vendors_note }}</p>
    <p v-if="table.difference" class="gc__diff" data-testid="guide-difference">
      {{ table.difference }}
    </p>

    <fieldset v-if="selectable" class="gc__selected" :disabled="busy">
      <legend>Выбрано</legend>
      <label v-for="c in table.columns.filter((x) => x.offer_id)" :key="c.offer_id">
        <input
          type="radio"
          name="guide-selected"
          :value="c.offer_id"
          :checked="c.offer_id === selectedId"
          data-testid="guide-selected"
          @change="emit('select', c.offer_id)"
        />
        {{ c.title }}
      </label>
    </fieldset>
  </section>
</template>

<style scoped>
.gc {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.gc__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.gc__title {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.gc__pick select {
  height: 2rem;
  max-width: 12rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  font: inherit;
  font-size: var(--fs-sm);
}
.gc__table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.gc__table th,
.gc__table td {
  padding: var(--space-2) var(--space-2);
  border-bottom: var(--border-w) solid var(--border);
  text-align: left;
  vertical-align: top;
  overflow-wrap: anywhere;
}
.gc__table thead th {
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
.gc__table thead th.is-picked {
  color: var(--text);
  box-shadow: inset 0 -2px 0 var(--accent);
}
.gc__table tbody th {
  width: 38%;
  font-weight: var(--fw-regular);
}
.is-asked > th,
.is-asked > dt {
  font-weight: var(--fw-semibold) !important;
}
.gc__cell {
  white-space: normal;
}
.gc__val,
.gc__diff {
  font-variant-numeric: tabular-nums;
}
.gc__sign {
  display: inline-block;
  width: 1em;
  margin-right: var(--space-1);
  font-weight: var(--fw-semibold);
  text-align: center;
}
.gc__cell--yes .gc__sign {
  color: var(--status-accepted);
}
.gc__cell--no,
.gc__cell--unknown {
  color: var(--text-2);
}
.gc__cell--conditional .gc__sign {
  color: var(--status-pending);
}
.gc__rows {
  display: flex;
  flex-direction: column;
  margin: 0;
}
.gc__prow {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-1) var(--space-3);
  padding: var(--space-2) 0;
  border-bottom: var(--border-w) solid var(--border);
}
.gc__prow dt {
  grid-column: 1 / -1;
}
.gc__prow dd {
  display: flex;
  flex-direction: column;
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}
.gc__col {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gc__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  margin: 0;
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gc__links {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-4);
  margin: 0;
}
.gc__more {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gc__note {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gc__diff {
  margin: 0;
  font-weight: var(--fw-medium);
}
.gc__selected {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-4);
  margin: var(--space-1) 0 0;
  padding: 0;
  border: 0;
}
.gc__selected legend {
  float: left;
  margin-right: var(--space-2);
  color: var(--text-2);
}
.gc__selected label {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: 2rem;
  cursor: pointer;
}
.gc__selected input {
  width: 1rem;
  height: 1rem;
  accent-color: var(--accent);
}
</style>
