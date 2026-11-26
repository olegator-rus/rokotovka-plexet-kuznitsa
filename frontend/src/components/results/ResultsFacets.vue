<script setup lang="ts">
// Фасеты страницы результатов: лицензионные (вендор, тип, метрика, количество,
// срок, реестр, цена с переключателем «С ценой / По запросу / Все», поставка) и фасеты характеристик контура коммерции.
// Счётчики — с сервера, значения с нулём скрыты. Порядок задаёт сервер (категория + подъём названных в запросе),
// но фасет, с которым работают, место не меняет: прошлый порядок сохраняется до смены запроса. Поиск внутри
// фасета — с 15 значений.
import { computed, reactive, ref, watch } from 'vue'
import { SliderRange, SliderRoot, SliderThumb, SliderTrack } from 'reka-ui'
import { ChevronDown, Search, SlidersHorizontal, X } from 'lucide-vue-next'
import type { Facet, SearchFilter } from '@/api/typesP2'
import { facetValueLabel, formatFacetNumber, rangeFilter, rangeOf, rangeStep } from '@/lib/facets'
import { formatInt } from '@/lib/format'
import { DESKTOP_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { FACET_SEARCH_FROM, filterFacetValues, stableFacetOrder } from '@/lib/resultsView'

const props = defineProps<{
  facets: Facet[]
  filters: SearchFilter[]
  /** Смена запроса сбрасывает закреплённый порядок фасетов. */
  queryKey: string
}>()
const emit = defineEmits<{ apply: [key: string, filter: SearchFilter | null]; reset: [] }>()

/** has_price показывается внутри фасета цены переключателем, а не отдельным списком. */
const hasPrice = computed(() => props.facets.find((f) => f.key === 'has_price') ?? null)
const visible = computed(() =>
  props.facets.filter((f) => {
    if (f.key === 'has_price') return !props.facets.some((x) => x.key === 'price_rub')
    if (f.type === 'number')
      return f.min !== null && f.max !== null && (f.max > f.min || Boolean(filterOf(f.key)))
    return (f.values ?? []).some((v) => v.count > 0)
  }),
)

const order = ref<string[]>([])
watch(
  () => props.queryKey,
  () => {
    order.value = []
  },
)
watch(
  visible,
  (next) => {
    order.value = stableFacetOrder(order.value, next)
  },
  { immediate: true },
)
const ordered = computed(() =>
  order.value
    .map((k) => visible.value.find((f) => f.key === k))
    .filter((f): f is Facet => Boolean(f)),
)

/**
 * Свёрнутые фасеты; по умолчанию раскрыты первые пять (на мобильном — ни один: список карточек не уезжает вниз)
 * и все с выбранным значением.
 */
const isDesktop = useMediaQuery(DESKTOP_QUERY)
const collapsed = reactive<Record<string, boolean>>({})
function isOpen(f: Facet, i: number) {
  if (f.key in collapsed) return !collapsed[f.key]
  return (isDesktop.value && i < 5) || Boolean(filterOf(f.key))
}
function toggle(f: Facet, i: number) {
  collapsed[f.key] = isOpen(f, i)
}

const needles = reactive<Record<string, string>>({})
const values = (f: Facet) =>
  filterFacetValues(
    (f.values ?? []).filter((v) => v.count > 0 || isPicked(f, v.value)),
    needles[f.key] ?? '',
  )

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

function filterOf(key: string) {
  return props.filters.find((f) => f.key === key)
}
function isPicked(f: Facet, value: string | number | boolean) {
  const cur = filterOf(f.key)
  return cur?.op === 'eq' && String(cur.value) === String(value)
}
function pick(f: Facet, value: string | number | boolean) {
  emit('apply', f.key, isPicked(f, value) ? null : { key: f.key, op: 'eq', value })
}
function commitRange(f: Facet, v: number[] | undefined) {
  if (!v || v.length < 2) return
  emit('apply', f.key, rangeFilter(f, v[0] as number, v[1] as number))
}
const priceMode = computed(() => String(filterOf('has_price')?.value ?? 'all'))
function setPriceMode(mode: string) {
  emit('apply', 'has_price', mode === 'all' ? null : { key: 'has_price', op: 'eq', value: mode })
}
const priceCount = (mode: string) =>
  hasPrice.value?.values.find((v) => String(v.value) === mode)?.count ?? null
const label = (f: Facet) => (f.unit && f.type === 'number' ? `${f.label}, ${f.unit}` : f.label)
const valueText = (f: Facet, v: { value: string | number | boolean; label?: string }) =>
  v.label || `${facetValueLabel(v.value)}${f.unit ? ` ${f.unit}` : ''}`
</script>

<template>
  <section v-if="ordered.length || filters.length" class="rfacets" aria-label="Фильтры">
    <header class="rfacets__head">
      <h2 class="rfacets__title t-sm">
        <SlidersHorizontal :size="14" aria-hidden="true" />
        Фильтры
      </h2>
      <button v-if="filters.length" type="button" class="link t-xs" @click="emit('reset')">
        Сбросить ({{ filters.length }})
      </button>
    </header>

    <div v-for="(f, i) in ordered" :key="f.key" class="rfacet" :data-facet="f.key">
      <button
        type="button"
        class="rfacet__toggle t-sm"
        :aria-expanded="isOpen(f, i)"
        :aria-controls="`facet-${f.key}`"
        @click="toggle(f, i)"
      >
        <span>{{ label(f) }}</span>
        <span v-if="filterOf(f.key)" class="rfacet__dot" aria-label="выбрано" />
        <ChevronDown
          :size="14"
          aria-hidden="true"
          :class="['rfacet__chev', { 'rfacet__chev--open': isOpen(f, i) }]"
        />
      </button>

      <div v-if="isOpen(f, i)" :id="`facet-${f.key}`" class="rfacet__body">
        <template v-if="f.type === 'number' && draft[f.key]">
          <p class="rfacet__range t-xs t-num">
            {{ formatFacetNumber(draft[f.key]![0]!, f.unit) }} —
            {{ formatFacetNumber(draft[f.key]![1]!, f.unit) }}
            <button
              v-if="filterOf(f.key)"
              type="button"
              class="rfacet__clear"
              :aria-label="`Снять фильтр «${f.label}»`"
              @click="emit('apply', f.key, null)"
            >
              <X :size="12" aria-hidden="true" />
            </button>
          </p>
          <SliderRoot
            v-model="draft[f.key]"
            class="rslider"
            :min="f.min ?? 0"
            :max="f.max ?? 0"
            :step="rangeStep(f)"
            :min-steps-between-thumbs="0"
            @value-commit="commitRange(f, $event)"
          >
            <SliderTrack class="rslider__track">
              <SliderRange class="rslider__range" />
            </SliderTrack>
            <SliderThumb class="rslider__thumb" :aria-label="`${f.label}: от`" />
            <SliderThumb class="rslider__thumb" :aria-label="`${f.label}: до`" />
          </SliderRoot>
          <div
            v-if="f.key === 'price_rub' && hasPrice"
            class="rfacet__price-mode"
            role="radiogroup"
            aria-label="Наличие цены"
          >
            <button
              v-for="m in [
                { v: 'with_price', l: 'С ценой' },
                { v: 'on_request', l: 'По запросу' },
                { v: 'all', l: 'Все' },
              ]"
              :key="m.v"
              type="button"
              role="radio"
              :aria-checked="priceMode === m.v"
              class="rfacet__mode t-xs"
              @click="setPriceMode(m.v)"
            >
              {{ m.l }}
              <span v-if="priceCount(m.v) !== null" class="t-num t-muted">{{
                formatInt(priceCount(m.v) ?? 0)
              }}</span>
            </button>
          </div>
        </template>

        <template v-else>
          <label v-if="(f.values ?? []).length >= FACET_SEARCH_FROM" class="rfacet__search">
            <Search :size="14" aria-hidden="true" />
            <input
              v-model="needles[f.key]"
              type="search"
              class="rfacet__input t-sm"
              :placeholder="`Найти: ${f.label.toLowerCase()}`"
              :aria-label="`Поиск в фильтре «${f.label}»`"
            />
          </label>
          <ul class="rfacet__values">
            <li v-for="v in values(f)" :key="String(v.value)">
              <button
                type="button"
                class="rfacet__value"
                :aria-pressed="isPicked(f, v.value)"
                @click="pick(f, v.value)"
              >
                <span>{{ valueText(f, v) }}</span>
                <span class="t-num t-muted">{{ formatInt(v.count) }}</span>
              </button>
            </li>
            <li v-if="!values(f).length" class="t-xs t-muted rfacet__none">Ничего не найдено</li>
          </ul>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.rfacets {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.rfacets__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.rfacets__title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  font-weight: var(--fw-semibold);
}
.rfacet + .rfacet {
  border-top: var(--border-w) solid var(--border);
  padding-top: var(--space-1);
}
.rfacet__toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  min-height: var(--control-h-sm);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-medium);
  text-align: left;
  cursor: pointer;
}
.rfacet__toggle > span:first-child {
  flex: 1;
}
.rfacet__dot {
  width: 6px;
  height: 6px;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
.rfacet__chev {
  color: var(--text-2);
  transition: transform var(--dur-fast, 120ms);
}
.rfacet__chev--open {
  transform: rotate(180deg);
}
.rfacet__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-bottom: var(--space-2);
}
.rfacet__range {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  color: var(--text-2);
}
.rfacet__clear {
  display: inline-flex;
  padding: 2px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-2);
  cursor: pointer;
}
.rslider {
  position: relative;
  display: flex;
  align-items: center;
  height: var(--control-h-sm);
  margin: 0 var(--space-2);
  touch-action: none;
  user-select: none;
}
.rslider__track {
  position: relative;
  flex: 1;
  height: 4px;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
}
.rslider__range {
  position: absolute;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
.rslider__thumb {
  display: block;
  width: 1rem;
  height: 1rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--accent);
  cursor: grab;
}
.rslider__thumb:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.rfacet__price-mode {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
.rfacet__mode {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-xs);
  cursor: pointer;
}
.rfacet__mode[aria-checked='true'] {
  border-color: var(--border-control);
  background: var(--surface-3);
  color: var(--text);
}
.rfacet__search {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  color: var(--text-2);
}
.rfacet__input {
  flex: 1;
  min-width: 0;
  min-height: var(--control-h-sm);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  outline: none;
}
.rfacet__values {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.rfacet__value {
  display: flex;
  align-items: center;
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
  cursor: pointer;
}
.rfacet__value:hover {
  background: var(--surface-2);
}
.rfacet__value[aria-pressed='true'] {
  border-color: var(--border-control);
  background: var(--surface-3);
  font-weight: var(--fw-medium);
}
.rfacet__none {
  padding: var(--space-1) var(--space-2);
}
</style>
