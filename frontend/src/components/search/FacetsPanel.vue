<script setup lang="ts">
// Фасеты категории: числовые — диапазоном ползунка (reka Slider, два бегунка),
// перечислимые и да/нет — списком с числом товаров. Применение — новый поиск с `filters` (адрес `f=`).
// Нет атрибутов у категории — фасетов нет, компонент не рисуется (решает страница).
import { reactive, watch } from 'vue'
import { SliderRange, SliderRoot, SliderThumb, SliderTrack } from 'reka-ui'
import { SlidersHorizontal, X } from 'lucide-vue-next'
import type { Facet, SearchFilter } from '@/api/typesP2'
import {
  facetValueLabel,
  formatFacetNumber,
  rangeFilter,
  rangeOf,
  rangeStep,
  usableFacet,
} from '@/lib/facets'
import { formatInt } from '@/lib/format'

const props = defineProps<{ facets: Facet[]; filters: SearchFilter[] }>()
const emit = defineEmits<{ apply: [key: string, filter: SearchFilter | null]; reset: [] }>()

const draft = reactive<Record<string, number[]>>({})
watch(
  () => [props.facets, props.filters] as const,
  ([facets, filters]) => {
    for (const f of facets) {
      if (f.type !== 'number') continue
      draft[f.key] = [
        ...rangeOf(
          f,
          filters.find((x) => x.key === f.key),
        ),
      ]
    }
  },
  { immediate: true },
)

const filterOf = (key: string) => props.filters.find((f) => f.key === key)

function commitRange(f: Facet, v: number[] | undefined) {
  if (!v || v.length < 2) return
  emit('apply', f.key, rangeFilter(f, v[0] as number, v[1] as number))
}

function pickValue(f: Facet, value: string | number | boolean) {
  const cur = filterOf(f.key)
  const same = cur?.op === 'eq' && String(cur.value) === String(value)
  emit('apply', f.key, same ? null : { key: f.key, op: 'eq', value })
}

const label = (f: Facet) => (f.unit && f.type === 'number' ? `${f.label}, ${f.unit}` : f.label)
</script>

<template>
  <section class="facets" aria-label="Фильтры">
    <header class="facets__head">
      <h2 class="facets__title t-sm">
        <SlidersHorizontal :size="14" aria-hidden="true" />
        Фильтры
      </h2>
      <button v-if="filters.length" type="button" class="link t-xs" @click="emit('reset')">
        Сбросить ({{ filters.length }})
      </button>
    </header>

    <template v-for="f in facets" :key="f.key">
      <fieldset v-if="usableFacet(f)" class="facet">
        <legend class="facet__label t-sm">{{ label(f) }}</legend>

        <template v-if="f.type === 'number' && draft[f.key]">
          <p class="facet__range t-xs t-num">
            {{ formatFacetNumber(draft[f.key]![0]!, f.unit) }} —
            {{ formatFacetNumber(draft[f.key]![1]!, f.unit) }}
            <button
              v-if="filterOf(f.key)"
              type="button"
              class="facet__clear"
              :aria-label="`Снять фильтр «${f.label}»`"
              @click="emit('apply', f.key, null)"
            >
              <X :size="12" aria-hidden="true" />
            </button>
          </p>
          <SliderRoot
            v-model="draft[f.key]"
            class="slider"
            :min="f.min ?? 0"
            :max="f.max ?? 0"
            :step="rangeStep(f)"
            :min-steps-between-thumbs="0"
            @value-commit="commitRange(f, $event)"
          >
            <SliderTrack class="slider__track">
              <SliderRange class="slider__range" />
            </SliderTrack>
            <SliderThumb class="slider__thumb" :aria-label="`${f.label}: от`" />
            <SliderThumb class="slider__thumb" :aria-label="`${f.label}: до`" />
          </SliderRoot>
        </template>

        <ul v-else class="facet__values">
          <li v-for="v in f.values" :key="String(v.value)">
            <button
              type="button"
              class="facet__value"
              :aria-pressed="
                filterOf(f.key)?.op === 'eq' && String(filterOf(f.key)?.value) === String(v.value)
              "
              @click="pickValue(f, v.value)"
            >
              <span
                >{{ facetValueLabel(v.value)
                }}<template v-if="f.unit"> {{ f.unit }}</template></span
              >
              <span class="t-num t-muted">{{ formatInt(v.count) }}</span>
            </button>
          </li>
        </ul>
      </fieldset>
    </template>
  </section>
</template>

<style scoped>
.facets {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.facets__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.facets__title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  font-weight: var(--fw-semibold);
}
.facet {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
.facet__label {
  padding: 0;
  margin-bottom: var(--space-1);
  font-weight: var(--fw-medium);
}
.facet__range {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
}
.facet__clear {
  display: inline-flex;
  padding: 2px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-2);
  cursor: pointer;
}
.slider {
  position: relative;
  display: flex;
  align-items: center;
  height: var(--control-h-sm);
  margin: 0 var(--space-2);
  touch-action: none;
  user-select: none;
}
.slider__track {
  position: relative;
  flex: 1;
  height: 4px;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
}
.slider__range {
  position: absolute;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
.slider__thumb {
  display: block;
  width: 1rem;
  height: 1rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--accent);
  cursor: grab;
}
.slider__thumb:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.facet__values {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.facet__value {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
  width: 100%;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-control);
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  text-align: left;
  align-items: center;
  cursor: pointer;
}
.facet__value:hover {
  background: var(--surface-2);
}
.facet__value[aria-pressed='true'] {
  border-color: var(--border-control);
  background: var(--surface-3);
}
</style>
