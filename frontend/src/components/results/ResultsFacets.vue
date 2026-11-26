<script setup lang="ts">
// Фасеты страницы результатов: лицензионные (вендор, тип, метрика, количество,
// срок, реестр, цена с переключателем «С ценой / По запросу / Все», поставка) и фасеты характеристик контура коммерции.
// Счётчики — с сервера, значения с нулём скрыты. Порядок задаёт сервер (категория + подъём названных в запросе),
// но фасет, с которым работают, место не меняет: прошлый порядок сохраняется до смены запроса. Поиск внутри
// фасета — с 15 значений.
import { computed, reactive, ref, watch } from 'vue'
import { SliderRange, SliderRoot, SliderThumb, SliderTrack } from 'reka-ui'
import { Check, ChevronDown, Search, SlidersHorizontal, X } from 'lucide-vue-next'
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
/** Короткие числовые значения (количество, срок, объём) — плитками в строку, а не длинным столбцом. */
const compact = (f: Facet) =>
  (f.values ?? []).length >= 2 &&
  (f.values ?? []).every((v) => /^\d/.test(valueText(f, v)) && valueText(f, v).length <= 8)
/** Граница диапазона: целые — с разрядами («117 000 ₽»). */
const rangeText = (v: number, unit = '') =>
  Number.isInteger(v) && (unit === '₽' || v >= 10000)
    ? [formatInt(v), unit].filter(Boolean).join('\u00a0')
    : formatFacetNumber(v, unit)
</script>

<template>
  <section v-if="ordered.length || filters.length" class="rfacets" aria-label="Фильтры">
    <header class="rfacets__head">
      <h2 class="rfacets__title">
        <SlidersHorizontal :size="16" aria-hidden="true" />
        Фильтры
      </h2>
      <button v-if="filters.length" type="button" class="rfacets__reset" @click="emit('reset')">
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
          <div class="rfacet__range">
            <p class="rfacet__bound">
              <span class="t-xs">от</span>
              <span class="t-num">{{ rangeText(draft[f.key]![0]!, f.unit) }}</span>
            </p>
            <p class="rfacet__bound">
              <span class="t-xs">до</span>
              <span class="t-num">{{ rangeText(draft[f.key]![1]!, f.unit) }}</span>
            </p>
            <button
              v-if="filterOf(f.key)"
              type="button"
              class="rfacet__clear"
              :aria-label="`Снять фильтр «${f.label}»`"
              @click="emit('apply', f.key, null)"
            >
              <X :size="14" aria-hidden="true" />
            </button>
          </div>
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
          <ul :class="['rfacet__values', { 'rfacet__values--compact': compact(f) }]">
            <li v-for="v in values(f)" :key="String(v.value)">
              <button
                type="button"
                class="rfacet__value"
                :aria-pressed="isPicked(f, v.value)"
                @click="pick(f, v.value)"
              >
                <span v-if="!compact(f)" class="rfacet__mark" aria-hidden="true">
                  <Check v-if="isPicked(f, v.value)" :size="12" :stroke-width="3" />
                </span>
                <span class="rfacet__text">{{ valueText(f, v) }}</span>
                <span class="rfacet__count t-num">{{ formatInt(v.count) }}</span>
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
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.rfacets__head {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-height: var(--touch-min);
  padding: var(--space-1) var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
  border-radius: var(--radius-card) var(--radius-card) 0 0;
  background: var(--surface);
}
.rfacets__title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.rfacets__title svg {
  color: var(--accent);
}
.rfacets__reset {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
  cursor: pointer;
}
.rfacets__reset:hover {
  background: var(--accent);
  color: var(--on-accent);
}
.rfacet {
  padding: 0 var(--space-3);
}
.rfacet + .rfacet {
  border-top: var(--border-w) solid var(--border);
}
.rfacet__toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  min-height: var(--touch-min);
  padding: 0 var(--space-1);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-body);
  font-weight: var(--fw-bold);
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
  color: var(--text-3);
  transition: transform var(--dur-fast) var(--ease-standard);
}
.rfacet__chev--open {
  transform: rotate(180deg);
}
.rfacet__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-bottom: var(--space-3);
}

/* Список значений: отметка, название, счётчик в одной колонке */
.rfacet__values {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.rfacet__value {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  box-sizing: border-box;
  width: 100%;
  min-height: var(--control-h);
  padding: var(--space-1) var(--space-1);
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-control);
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--dur-fast) var(--ease-standard),
    border-color var(--dur-fast) var(--ease-standard);
}
.rfacet__value:hover {
  background: var(--surface-2);
}
.rfacet__mark {
  display: grid;
  flex: none;
  place-items: center;
  box-sizing: border-box;
  width: 1.125rem;
  height: 1.125rem;
  border: var(--border-w) solid var(--border-control);
  /* Круглая: в фасете выбирается одно значение, повторное нажатие снимает. */
  border-radius: var(--radius-pill);
  background: var(--surface);
}
.rfacet__value:hover .rfacet__mark {
  border-color: var(--accent);
}
.rfacet__text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.rfacet__count {
  flex: none;
  color: var(--text-3);
  font-size: var(--fs-xs);
}
/* Выбранное: подложка и заполненная отметка с галочкой — не только цвет */
.rfacet__value[aria-pressed='true'] {
  background: var(--accent-soft);
  font-weight: var(--fw-semibold);
}
.rfacet__value[aria-pressed='true'] .rfacet__mark {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}

/* Плитки для коротких чисел */
.rfacet__values--compact {
  flex-direction: row;
  flex-wrap: wrap;
  gap: var(--space-2);
  padding: 0 var(--space-1);
}
.rfacet__values--compact .rfacet__value {
  justify-content: center;
  gap: var(--space-1);
  width: auto;
  min-width: 3.25rem;
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border-color: var(--border);
}
.rfacet__values--compact .rfacet__text {
  flex: none;
  font-weight: var(--fw-semibold);
}
.rfacet__values--compact .rfacet__value:hover {
  border-color: var(--accent);
}
.rfacet__values--compact .rfacet__value[aria-pressed='true'] {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}
.rfacet__values--compact .rfacet__value[aria-pressed='true'] .rfacet__count {
  color: var(--on-accent);
}

/* Диапазон */
.rfacet__range {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-1);
}
.rfacet__bound {
  display: flex;
  flex: 1;
  align-items: baseline;
  gap: var(--space-1);
  min-width: 0;
  min-height: var(--control-h);
  margin: 0;
  padding: var(--space-2) var(--space-2);
  box-sizing: border-box;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.rfacet__bound span:first-child {
  color: var(--text-3);
  font-weight: var(--fw-regular);
}
.rfacet__clear {
  display: inline-flex;
  flex: none;
  padding: var(--space-1);
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
  box-sizing: border-box;
  width: 1.25rem;
  height: 1.25rem;
  border: var(--focus-w) solid var(--accent);
  border-radius: var(--radius-pill);
  background: var(--surface);
  box-shadow: 0 1px 4px var(--scrim);
  cursor: grab;
}
.rslider__thumb:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.rfacet__price-mode {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
}
.rfacet__mode {
  display: inline-flex;
  flex: 1 1 auto;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-1);
  border: 0;
  border-radius: var(--radius-pill);
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-xs);
  white-space: nowrap;
  cursor: pointer;
}
.rfacet__mode[aria-checked='true'] {
  background: var(--surface);
  box-shadow: 0 1px 3px var(--scrim);
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.rfacet__search {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  color: var(--text-2);
}
.rfacet__search:focus-within {
  border-color: var(--focus-ring);
}
.rfacet__input {
  flex: 1;
  min-width: 0;
  min-height: var(--control-h);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  outline: none;
}
.rfacet__none {
  padding: var(--space-1) var(--space-2);
}
</style>
