<script setup lang="ts">
// Таблица сравнения (SPEC §8): колонки — редакции, строки — различия. По умолчанию только различия; цены — одной
// строкой «Итого» с ценой за единицу под суммой; под колонкой — «Выбрать»: позиция этой редакции становится главной
// карточкой с кнопкой «В спецификацию». Выбранная и рекомендованная колонки помечены.
// Источник — маленький значок у строки о функции. На телефоне — две колонки, вторая выбирается переключателем.
import { computed, ref, watch } from 'vue'
import { Check, Minus } from 'lucide-vue-next'
import type { GuideClaim, GuideComparison, GuideOffer } from '@/api/typesGuide'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { claimIndex, columnOffer, compareView, offerPrice, visibleColumns } from '@/lib/guide'
import GuideSource from './GuideSource.vue'

const props = defineProps<{
  comparison: GuideComparison
  claims: GuideClaim[]
  offers?: GuideOffer[]
  inSpec?: (offerId: string) => boolean
  busy?: boolean
  /** Позиция в главной карточке и рекомендованная ядром. */
  featuredId?: string
  recommendedId?: string
}>()
const emit = defineEmits<{ pick: [offer: GuideOffer] }>()

const narrow = useMediaQuery('(max-width: 40rem)')
const onlyDiff = ref(true)
const picked = ref(1)
watch(
  () => props.comparison,
  () => (picked.value = 1),
)

const count = computed(() => props.comparison.columns.length)
const shown = computed(() => visibleColumns(count.value, narrow.value, picked.value))
const view = computed(() =>
  compareView(props.comparison, { onlyDiff: onlyDiff.value, columns: shown.value }),
)
const facts = computed(() => claimIndex(props.claims))
const buy = computed(() =>
  view.value.columns.map((c) => {
    const o = columnOffer(c.id, props.offers)
    return o && offerPrice(o).known ? o : null
  }),
)
const hasBuy = computed(() => buy.value.some(Boolean))
const isOn = (ci: number) =>
  Boolean(props.featuredId) && buy.value[ci]?.offer_id === props.featuredId
const isRec = (ci: number) =>
  Boolean(props.recommendedId) && buy.value[ci]?.offer_id === props.recommendedId

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
  <section class="gcmp" aria-label="Сравнение редакций" data-testid="guide-table">
    <div v-if="(narrow && count > 2) || view.total > view.differing" class="gcmp__bar">
      <div v-if="narrow && count > 2" class="gcmp__pick" role="group" aria-label="С чем сравнить">
        <button
          v-for="i in count - 1"
          :key="i"
          type="button"
          :class="['gcmp__pick-btn', { 'gcmp__pick-btn--on': shown[1] === i }]"
          :aria-pressed="shown[1] === i"
          @click="picked = i"
        >
          {{ comparison.columns[i]?.title }}
        </button>
      </div>
      <UiSwitch
        v-if="view.total > view.differing"
        v-model="onlyDiff"
        class="gcmp__diff"
        label="Только различия"
        data-testid="guide-only-diff"
      />
    </div>

    <div class="gcmp__scroll">
      <table class="gcmp__table" :style="{ '--gcmp-cols': view.columns.length }">
        <colgroup>
          <col :style="{ width: count > 2 && !narrow ? '22%' : narrow ? '38%' : '42%' }" />
          <col v-for="c in view.columns" :key="c.id" />
        </colgroup>
        <thead>
          <tr>
            <th scope="col"><span class="sr-only">Параметр</span></th>
            <th
              v-for="(c, ci) in view.columns"
              :key="c.id"
              scope="col"
              :class="['gcmp__col', { 'is-on': isOn(ci) }]"
            >
              <a v-if="c.url" :href="c.url" target="_blank" rel="noopener noreferrer">{{
                c.title
              }}</a>
              <template v-else>{{ c.title }}</template>
              <span v-if="isRec(ci)" class="gcmp__rec">Рекомендуем</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in view.features" :key="r.key" :data-row="r.key" data-kind="feature">
            <th scope="row" class="gcmp__label">
              {{ r.label
              }}<GuideSource
                v-if="rowClaim(r.cells)"
                :claim="rowClaim(r.cells)!"
                :label="r.label"
              />
              <span v-if="r.note" class="gcmp__note">{{ r.note }}</span>
            </th>
            <td
              v-for="(cell, ci) in r.cells"
              :key="ci"
              :class="['gcmp__cell', `gcmp__cell--${cell.tone}`, { 'is-on': isOn(ci) }]"
              :data-kind="cell.tone"
            >
              <template v-if="cell.tone === 'yes'"
                ><Check :size="14" :stroke-width="2.25" aria-hidden="true" />{{
                  cell.text
                }}</template
              >
              <template v-else-if="cell.tone === 'no'"
                ><Minus :size="14" aria-hidden="true" /><span>нет</span
                ><span class="gcmp__sub">{{ ' по данным вендора' }}</span></template
              >
              <template v-else>{{ cell.text }}</template>
            </td>
          </tr>
          <tr v-if="!view.features.length && view.total" data-kind="empty">
            <td :colspan="view.columns.length + 1" class="gcmp__empty">
              По показанным строкам редакции не различаются
            </td>
          </tr>
        </tbody>
        <tbody v-if="view.prices.length" class="gcmp__money">
          <tr v-for="p in view.prices" :key="p.key" :data-row="p.key" data-kind="price">
            <th scope="row" class="gcmp__label">
              {{ p.label }}
              <span v-if="p.note" class="gcmp__note">{{ p.note }}</span>
            </th>
            <td
              v-for="(cell, ci) in p.cells"
              :key="ci"
              :class="['gcmp__cell gcmp__price', { 'is-on': isOn(ci) }]"
            >
              <span
                :class="cell.total.tone === 'value' ? 'gcmp__total' : 'gcmp__cell--unknown'"
                data-testid="guide-cell-total"
                >{{ cell.total.text }}</span
              >
              <span v-if="cell.unit" class="gcmp__unit">{{ cell.unit.text }}</span>
            </td>
          </tr>
        </tbody>
        <tfoot v-if="hasBuy">
          <tr>
            <td></td>
            <td v-for="(o, ci) in buy" :key="ci" :class="['gcmp__buy', { 'is-on': isOn(ci) }]">
              <span v-if="o && isOn(ci)" class="gcmp__chosen">
                <Check :size="14" aria-hidden="true" /> Выбрано
              </span>
              <button
                v-else-if="o"
                type="button"
                class="btn btn--secondary btn--sm"
                :disabled="busy"
                data-testid="guide-col-pick"
                @click="emit('pick', o)"
              >
                <Check v-if="inSpec?.(o.offer_id)" :size="14" aria-hidden="true" />
                {{ inSpec?.(o.offer_id) ? 'В спецификации' : 'Выбрать' }}
                <span class="sr-only">: {{ view.columns[ci]?.title }}</span>
              </button>
            </td>
          </tr>
        </tfoot>
      </table>
    </div>
  </section>
</template>

<style scoped>
.gcmp {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.gcmp__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-4);
}
.gcmp__diff {
  margin-left: auto;
}
.gcmp__pick {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 2px;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
}
.gcmp__pick-btn {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gcmp__pick-btn--on {
  background: var(--surface);
  color: var(--text);
  font-weight: var(--fw-medium);
  box-shadow: 0 0 0 var(--border-w) var(--border);
}
.gcmp__scroll {
  max-width: 100%;
  overflow-x: auto;
}
.gcmp__table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gcmp__table th,
.gcmp__table td {
  padding: var(--space-2) var(--space-3) var(--space-2) 0;
  text-align: left;
  vertical-align: top;
}
.gcmp__table tbody tr {
  border-top: var(--border-w) solid var(--border);
}
thead th {
  padding-bottom: var(--space-2);
  font-weight: var(--fw-semibold);
  font-size: var(--fs-body);
  line-height: var(--lh-body);
}
.gcmp__col a {
  color: inherit;
  text-decoration: none;
}
.gcmp__col a:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.gcmp__label {
  font-weight: var(--fw-regular);
  color: var(--text);
}
.gcmp__note {
  display: block;
  margin-top: 2px;
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-regular);
}
.gcmp__cell {
  overflow-wrap: anywhere;
}
.gcmp__cell > svg {
  display: inline-block;
  margin-right: var(--space-1);
  vertical-align: -2px;
}
.gcmp__cell--yes {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.gcmp__cell--yes > svg {
  color: var(--route-a);
}
.gcmp__cell--no {
  color: var(--text-2);
}
.gcmp__cell--no > svg {
  color: var(--text-3);
}
.gcmp__sub {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gcmp__cell--unknown,
.gcmp__cell--depends {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gcmp__money tr {
  border-top: var(--border-w) solid var(--text-3);
}
.gcmp__money .gcmp__label {
  font-weight: var(--fw-medium);
}
.gcmp__price {
  display: table-cell;
}
.gcmp__total {
  display: block;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.gcmp__unit {
  display: block;
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  white-space: nowrap;
}
.gcmp__table .is-on {
  background: var(--surface-2);
}
.gcmp__table th:not(:first-child),
.gcmp__table td:not(:first-child) {
  padding-left: var(--space-2);
}
.gcmp__rec {
  display: block;
  color: var(--route-a);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.gcmp__chosen {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gcmp__empty {
  color: var(--text-3);
}
tfoot td {
  padding-top: var(--space-1);
}
.gcmp__buy .btn {
  white-space: nowrap;
}
/* Телефон: подпись строки — над значениями, значения — по колонкам (две, вторая выбирается переключателем). */
@media (max-width: 40rem) {
  .gcmp__table,
  .gcmp__table thead,
  .gcmp__table tbody,
  .gcmp__table tfoot {
    display: block;
  }
  .gcmp__table colgroup {
    display: none;
  }
  .gcmp__table tr {
    display: grid;
    grid-template-columns: repeat(var(--gcmp-cols), minmax(0, 1fr));
    column-gap: var(--space-3);
  }
  .gcmp__table thead th:first-child,
  .gcmp__table tfoot td:first-child {
    display: none;
  }
  .gcmp__table tbody th,
  .gcmp__empty {
    grid-column: 1 / -1;
    padding-bottom: 0;
    color: var(--text-2);
  }
  .gcmp__table th,
  .gcmp__table td,
  .gcmp__table th:not(:first-child),
  .gcmp__table td:not(:first-child) {
    padding-right: 0;
    padding-left: 0;
  }
  .gcmp__table thead th {
    font-size: var(--fs-sm);
    line-height: var(--lh-sm);
    overflow-wrap: anywhere;
  }
  .gcmp__table th.is-on,
  .gcmp__table td.is-on {
    padding-left: var(--space-2);
  }
  .gcmp__buy .btn {
    white-space: normal;
    text-align: left;
  }
}
</style>
