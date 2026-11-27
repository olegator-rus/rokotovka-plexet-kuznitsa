<script setup lang="ts">
// Список товаров (CONSULTANT.md §6.2): первые четыре позиции видны сразу, остальные — под «Ещё N вариантов».
// Группы: подходят · «Ещё посмотрите» · «Не подойдут» (с причиной, ниже подходящих). Всё нужное — в строке, без
// раскрытия: название (ссылка в магазин), цена с расчётом и кнопка «в спецификацию». Условия, общие для всего
// списка, названы один раз в шапке.
import { computed, ref } from 'vue'
import { Check, ExternalLink, ListPlus } from 'lucide-vue-next'
import type { Attachment, ProductGroup, ProductReference } from '@/api/typesConsultant'
import VendorLogo from '@/components/common/VendorLogo.vue'
import { plural } from '@/lib/format'
import {
  isStoreUrl,
  productCalc,
  productOldUnit,
  productPrice,
  productTerms,
  splitVisible,
  unfitText,
} from '@/lib/consultant'
import { useSpecStore } from '@/stores/spec'
import { useConsultant } from '../composables/useConsultant'

const props = defineProps<{ data: Extract<Attachment, { type: 'product_list' }> }>()

const c = useConsultant()
const spec = useSpecStore()
const all = ref(false)

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

/** Условия, одинаковые у всех позиций списка, — в шапке; пусто, если различаются. */
const shared = computed(() => {
  const terms = groups.value.flatMap((g) => g.items.map((p) => productTerms(p)))
  return terms.length > 1 && terms.every((t) => t === terms[0]) ? (terms[0] ?? '') : ''
})

const storeUrl = (p: ProductReference) => (isStoreUrl(p.productUrl) ? p.productUrl : '')
/** Есть настоящая цена; «Цена по запросу» набирается тише. */
const priced = (p: ProductReference) => /\d/.test(productPrice(p))
</script>

<template>
  <section v-if="total" class="pl" data-testid="consultant-list">
    <header class="pl__head">
      <div class="pl__headline">
        <span class="pl__title">{{ data.title || 'Варианты' }}</span>
        <span class="pl__count"
          >{{ total }} {{ plural(total, ['вариант', 'варианта', 'вариантов']) }}</span
        >
      </div>
      <p v-if="shared" class="pl__shared">У всех: {{ shared }}</p>
    </header>
    <template v-for="(g, gi) in groups" :key="g.kind + gi">
      <template v-if="view.shown[gi]?.length">
        <div v-if="groupTitle(g)" class="pl__group" :data-kind="g.kind">{{ groupTitle(g) }}</div>
        <ul class="pl__list">
          <li
            v-for="(p, pi) in view.shown[gi]"
            :key="p.offerId + pi"
            class="pl__row"
            data-testid="consultant-list-row"
            :data-kind="g.kind"
          >
            <VendorLogo class="pl__logo" :vendor="p.vendor" :image="p.imageUrl" />
            <div class="pl__main">
              <a
                v-if="storeUrl(p)"
                class="pl__name pl__name--link"
                :href="storeUrl(p)"
                target="_blank"
                rel="noopener noreferrer"
              >
                {{ p.title }}<ExternalLink :size="12" aria-hidden="true" class="pl__ext" />
                <span class="sr-only"> (в магазине, в новой вкладке)</span>
              </a>
              <span v-else class="pl__name">{{ p.title }}</span>
              <span v-if="!shared && productTerms(p)" class="pl__terms">{{ productTerms(p) }}</span>
              <span :class="['pl__price t-num', { 'pl__price--request': !priced(p) }]">
                {{ productPrice(p) }}
              </span>
              <span v-if="productCalc(p)" class="pl__terms t-num">{{ productCalc(p) }}</span>
              <span v-if="productOldUnit(p)" class="pl__terms t-num">
                <s>{{ productOldUnit(p) }}</s> · Цена по акции
              </span>
              <span
                v-if="p.compatibility === 'incompatible'"
                class="pl__reason"
                data-testid="consultant-list-reason"
              >
                {{ unfitText(p.reason) }}
              </span>
              <span v-else-if="p.compatibility === 'unverified'" class="pl__terms">
                Совместимость не подтверждена
              </span>
              <span v-else-if="p.reason" class="pl__terms">{{ p.reason }}</span>
            </div>
            <button
              v-if="p.compatibility !== 'incompatible'"
              type="button"
              :class="['pl__add', { 'pl__add--done': spec.has(p.offerId) }]"
              :disabled="spec.has(p.offerId)"
              :aria-label="
                spec.has(p.offerId) ? `В спецификации: ${p.title}` : `В спецификацию: ${p.title}`
              "
              :title="spec.has(p.offerId) ? 'В спецификации' : 'В спецификацию'"
              data-testid="consultant-list-add"
              @click="c.addProduct(p)"
            >
              <Check v-if="spec.has(p.offerId)" :size="18" aria-hidden="true" />
              <ListPlus v-else :size="18" aria-hidden="true" />
            </button>
          </li>
        </ul>
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
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-sm);
}
.pl__headline {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
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
.pl__shared {
  margin: 2px 0 0;
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.pl__group {
  padding: var(--space-2) var(--space-3) var(--space-1);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
}
.pl__list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.pl__row {
  display: grid;
  grid-template-columns: 2rem minmax(0, 1fr) 2.5rem;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
}
.pl__row + .pl__row {
  border-top: 1px solid var(--border);
}
.pl__row[data-kind='unfit'] {
  grid-template-columns: 2rem minmax(0, 1fr);
}
.pl__logo {
  width: 2rem;
  height: 2rem;
}
.pl__main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}
.pl__name {
  color: var(--text);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  line-height: 1.35;
  overflow-wrap: anywhere;
}
.pl__name--link {
  text-decoration: none;
}
.pl__name--link:hover {
  text-decoration: underline;
  text-underline-offset: 2px;
}
.pl__ext {
  display: inline;
  margin-left: 0.3em;
  color: var(--text-3);
  vertical-align: -1px;
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
  font-size: var(--cw-fs, var(--fs-body));
  font-weight: var(--fw-bold);
}
.pl__price--request {
  color: var(--text-2);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
}
.pl__add {
  display: grid;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  padding: 0;
  border: 1px solid var(--accent);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--accent);
  cursor: pointer;
}
.pl__add:hover:not(:disabled) {
  background: var(--accent);
  color: var(--on-accent);
}
.pl__add:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.pl__add--done {
  border-color: transparent;
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
  cursor: default;
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
