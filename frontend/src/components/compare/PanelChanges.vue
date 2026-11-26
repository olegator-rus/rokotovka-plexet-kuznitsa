<script setup lang="ts">
// «Подсказки» → изменения по префиксам: что набрали → что ждали → место цели раньше и сейчас. Сначала одна
// сторона («Лучше» или «Хуже»), фильтр по месту в панели; весь список раскрывается здесь же. Данные — changes
// прогона панели, своих чисел нет.
import { computed, ref, watch } from 'vue'
import { ArrowRight, Search, TrendingDown, TrendingUp } from 'lucide-vue-next'
import type { PanelChange } from '@/api/typesEvalPanel'
import { PREFIX_TYPE_LABELS, rankLabel, SECTION_LABELS, splitChanges } from '@/lib/evalPanel'
import { formatInt } from '@/lib/format'

const props = defineProps<{ changes: PanelChange[] }>()

const SHOWN = 10
type Side = 'better' | 'worse'

const split = computed(() => splitChanges(props.changes))
const side = ref<Side>('better')
const section = ref('all')
const all = ref(false)
watch([side, section], () => (all.value = false))

const SIDES = [
  { id: 'better' as const, label: 'Лучше', icon: TrendingUp },
  { id: 'worse' as const, label: 'Хуже', icon: TrendingDown },
]
/** Фильтр по месту в панели — только те, что есть на выбранной стороне; счётчики по ней же. */
const sections = computed(() => {
  const rows = split.value[side.value]
  const ids = [...new Set(props.changes.map((c) => c.section))]
  return [
    { id: 'all', label: 'Все', count: rows.length },
    ...ids.map((id) => ({
      id,
      label: capital(SECTION_LABELS[id] ?? id),
      count: rows.filter((c) => c.section === id).length,
    })),
  ]
})
const rows = computed(() =>
  split.value[side.value].filter((c) => section.value === 'all' || c.section === section.value),
)
const shown = computed(() => (all.value ? rows.value : rows.value.slice(0, SHOWN)))

function capital(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
</script>

<template>
  <div :class="['pc', `pc--${side}`]">
    <div class="pc__bar">
      <div class="pc__sides" role="group" aria-label="Что показать">
        <button
          v-for="s in SIDES"
          :key="s.id"
          type="button"
          :class="['pc__side', `pc__side--${s.id}`]"
          :aria-pressed="side === s.id"
          @click="side = s.id"
        >
          <component :is="s.icon" :size="16" aria-hidden="true" />
          {{ s.label }}
          <span class="pc__num t-num">{{ formatInt(split[s.id].length) }}</span>
        </button>
      </div>
      <div class="pc__filter" role="group" aria-label="Где в панели">
        <button
          v-for="f in sections"
          :key="f.id"
          type="button"
          class="pc__chip"
          :aria-pressed="section === f.id"
          @click="section = f.id"
        >
          {{ f.label }} <span class="t-num">{{ formatInt(f.count) }}</span>
        </button>
      </div>
    </div>

    <p class="pc__cols t-xs" aria-hidden="true">
      <span>Набрали в строке</span>
      <span>Ждали увидеть</span>
      <span>Место: было → стало</span>
    </p>
    <p v-if="!rows.length" class="pc__empty t-sm t-muted">Под фильтр не попал ни один префикс.</p>
    <ul v-else class="pc__list">
      <li v-for="c in shown" :key="c.id + c.section" class="pc__row">
        <span class="pc__typed">
          <Search :size="14" aria-hidden="true" />
          <span class="sr-only">набрали </span>
          <span>{{ c.prefix }}</span>
        </span>
        <span class="pc__target">
          <span class="pc__target-name"> <span class="sr-only">ждали </span>{{ c.target }} </span>
          <span class="t-xs t-muted">
            {{ capital(SECTION_LABELS[c.section] ?? c.section) }} ·
            {{ PREFIX_TYPE_LABELS[c.type] ?? c.type }}
          </span>
        </span>
        <span class="pc__ranks">
          <span class="pc__rank">
            <span class="sr-only">было: </span>{{ rankLabel(c.old_rank) }}
          </span>
          <ArrowRight :size="14" aria-hidden="true" />
          <span class="pc__rank pc__rank--new">
            <span class="sr-only">стало: </span>{{ rankLabel(c.new_rank) }}
          </span>
        </span>
      </li>
    </ul>

    <button v-if="rows.length > SHOWN" type="button" class="pc__more" @click="all = !all">
      {{ all ? 'Свернуть' : `Показать все ${formatInt(rows.length)}` }}
    </button>
  </div>
</template>

<style scoped>
.pc {
  --tone: var(--delta-up);
  --tone-bg: var(--status-accepted-bg);
  container-type: inline-size;
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.pc--worse {
  --tone: var(--delta-down);
  --tone-bg: var(--danger-bg);
}
.pc__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
}
.pc__sides {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
}
.pc__side {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding: 0 var(--space-4);
  border: 0;
  border-radius: var(--radius-pill);
  background: none;
  color: var(--text-2);
  font: inherit;
  font-weight: var(--fw-semibold);
  cursor: pointer;
}
.pc__side[aria-pressed='true'] {
  background: var(--surface);
  box-shadow: 0 1px 4px var(--hairline-shadow);
}
.pc__side--better[aria-pressed='true'] {
  color: var(--delta-up);
}
.pc__side--worse[aria-pressed='true'] {
  color: var(--delta-down);
}
.pc__num {
  font-size: var(--fs-h2);
  font-weight: var(--fw-bold);
}
.pc__filter {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
.pc__chip {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.pc__chip[aria-pressed='true'] {
  border-color: var(--text);
  background: var(--text);
  color: var(--surface);
}
.pc__cols,
.pc__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) 17rem;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-4);
}
.pc__cols {
  margin: 0;
  border-top: var(--border-w) solid var(--border);
  background: var(--surface-2);
  color: var(--text-2);
  font-weight: var(--fw-semibold);
}
.pc__list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.pc__row {
  min-height: 3.5rem;
  border-top: var(--border-w) solid var(--border);
}
.pc__row:hover {
  background: var(--surface-2);
}
/* Что набрано — как в строке поиска */
.pc__typed {
  display: inline-flex;
  align-items: center;
  justify-self: start;
  gap: var(--space-2);
  max-width: 100%;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  box-sizing: border-box;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.pc__typed svg {
  flex: none;
  color: var(--text-3);
}
.pc__target {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.pc__target-name {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.pc__ranks {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.pc__ranks svg {
  flex: none;
  color: var(--text-3);
}
.pc__rank {
  flex: 1;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-semibold);
  text-align: center;
  white-space: nowrap;
}
.pc__rank--new {
  background: var(--tone-bg);
  color: var(--tone);
}
.pc__empty {
  margin: 0;
  padding: var(--space-5) var(--space-4);
  border-top: var(--border-w) solid var(--border);
}
.pc__more {
  width: 100%;
  min-height: var(--touch-min);
  border: 0;
  border-top: var(--border-w) solid var(--border);
  background: var(--surface);
  color: var(--accent);
  font: inherit;
  font-weight: var(--fw-semibold);
  cursor: pointer;
}
.pc__more:hover {
  background: var(--accent-soft);
}
@container (max-width: 40rem) {
  .pc__cols {
    display: none;
  }
  .pc__row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-2);
    padding-block: var(--space-3);
  }
}
</style>
