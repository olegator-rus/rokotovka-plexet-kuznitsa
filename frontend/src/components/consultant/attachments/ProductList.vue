<script setup lang="ts">
// Список товаров (CONSULTANT.md §6.2): первые четыре позиции видны сразу, остальные — под «Ещё N вариантов».
// Группы: подходят · «Ещё посмотрите» · «Не подойдут» (с причиной, ниже подходящих). Строка раскрывается в полную
// карточку; раскрыта одна.
import { computed, ref } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import type { Attachment, ProductGroup } from '@/api/typesConsultant'
import { plural } from '@/lib/format'
import { productPrice, productTerms, splitVisible, unfitText } from '@/lib/consultant'
import ProductCard from './ProductCard.vue'

const props = defineProps<{ data: Extract<Attachment, { type: 'product_list' }> }>()

const all = ref(false)
const openKey = ref<string | null>(null)

// Порядок групп: подходят → не подойдут → «Ещё посмотрите». Неподходящие, которые покупатель искал сам, входят в
// первые четыре (CONSULTANT.md §6.2, КриптоПро: две серверные и две для рабочего места); добор — после них.
const ORDER: Record<string, number> = { fit: 0, unfit: 1, more: 2 }
const groups = computed<ProductGroup[]>(() =>
  props.data.groups
    .filter((g) => g.items.length)
    .map((g, i) => ({ g, i }))
    .sort((a, b) => (ORDER[a.g.kind] ?? 3) - (ORDER[b.g.kind] ?? 3) || a.i - b.i)
    .map((x) => x.g),
)
const total = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0))
const view = computed(() => splitVisible(groups.value, all.value))

function groupTitle(g: ProductGroup): string {
  if (g.title) return g.title
  if (g.kind === 'more') return 'Ещё посмотрите'
  if (g.kind === 'unfit') return 'Не подойдут'
  return ''
}

function toggle(key: string) {
  openKey.value = openKey.value === key ? null : key
}
</script>

<template>
  <section v-if="total" class="pl" data-testid="consultant-list">
    <header class="pl__head">
      <span class="pl__title">{{ data.title || 'Варианты' }}</span>
      <span class="pl__count"
        >{{ total }} {{ plural(total, ['вариант', 'варианта', 'вариантов']) }}</span
      >
    </header>
    <template v-for="(g, gi) in groups" :key="g.kind + gi">
      <template v-if="view.shown[gi]?.length">
        <div v-if="groupTitle(g)" class="pl__group" :data-kind="g.kind">{{ groupTitle(g) }}</div>
        <div v-for="(p, pi) in view.shown[gi]" :key="p.offerId + pi" class="pl__item">
          <button
            type="button"
            class="pl__row"
            :aria-expanded="openKey === `${gi}:${pi}`"
            data-testid="consultant-list-row"
            :data-kind="g.kind"
            @click="toggle(`${gi}:${pi}`)"
          >
            <span class="pl__main">
              <span class="pl__name">{{ p.title }}</span>
              <span v-if="productTerms(p)" class="pl__terms">{{ productTerms(p) }}</span>
              <span
                v-if="p.compatibility === 'incompatible'"
                class="pl__reason"
                data-testid="consultant-list-reason"
              >
                {{ unfitText(p.reason) }}
              </span>
              <span v-else-if="g.kind === 'more' && p.reason" class="pl__terms">{{
                p.reason
              }}</span>
            </span>
            <span class="pl__price t-num">{{ productPrice(p) }}</span>
            <ChevronDown
              class="pl__chev"
              :class="{ 'pl__chev--open': openKey === `${gi}:${pi}` }"
              :size="16"
              aria-hidden="true"
            />
          </button>
          <div v-if="openKey === `${gi}:${pi}`" class="pl__card">
            <ProductCard :product="p" />
          </div>
        </div>
      </template>
    </template>
    <button
      v-if="view.hidden > 0 || all"
      type="button"
      class="pl__more"
      data-testid="consultant-list-more"
      @click="all = !all"
    >
      {{
        all
          ? 'Свернуть'
          : `Ещё ${view.hidden} ${plural(view.hidden, ['вариант', 'варианта', 'вариантов'])}`
      }}
    </button>
  </section>
</template>

<style scoped>
.pl {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
}
.pl__head {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-sm);
}
.pl__title {
  min-width: 0;
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.pl__count {
  flex-shrink: 0;
  color: var(--text-2);
}
.pl__group {
  padding: var(--space-2) var(--space-3) var(--space-1);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
}
.pl__item + .pl__item {
  border-top: 1px solid var(--border);
}
.pl__row {
  display: flex;
  width: 100%;
  min-height: 4rem;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.pl__row:hover {
  background: var(--surface-2);
}
.pl__row:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: -2px;
}
.pl__main {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}
.pl__name {
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
  line-height: 1.35;
  overflow-wrap: anywhere;
}
.pl__terms {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.pl__reason {
  color: var(--text);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
}
.pl__price {
  flex-shrink: 0;
  font-size: var(--fs-body);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.pl__chev {
  flex-shrink: 0;
  color: var(--text-3);
  transition: transform var(--dur-fast) var(--ease-standard);
}
.pl__chev--open {
  transform: rotate(180deg);
}
.pl__card {
  padding: 0 var(--space-2) var(--space-2);
}
.pl__more {
  display: block;
  width: 100%;
  min-height: 2.75rem;
  border: 0;
  border-top: 1px solid var(--border);
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  cursor: pointer;
}
.pl__more:hover {
  background: var(--surface-2);
}
</style>
