<script setup lang="ts">
// Карточка-семейство: вендор и категория, название, строка атрибутов (одинаковый набор у карточек
// ПО, нет значения — прочерк; атрибут из запроса выделен), цена «от N ₽» по семейству или точная у позиции,
// действие по состоянию — «Выбрать лицензию» (раскрывает варианты), «В спецификацию» (вариант определён),
// «Запросить цену» (цены нет). Раскрытие — таблица «редакция — срок — метрика и количество — цена» с действием
// у каждой строки. Справа — скор, он же «почему здесь» (трасса). Рейтингов, рассрочки и избранного нет.
import { computed, ref } from 'vue'
import { Check, ChevronDown, ListPlus, Route } from 'lucide-vue-next'
import type { SearchItem, SearchItemBase } from '@/api/types'
import CompatBadge from '@/components/search/CompatBadge.vue'
import { formatScore } from '@/lib/format'
import { isPurchasable, otherVariants, priceLabel } from '@/lib/license'
import {
  attrCells,
  cardAction,
  durationLabel,
  familyPriceLabel,
  type AttrCell,
} from '@/lib/resultsView'

const props = defineProps<{
  item: SearchItem
  position: number
  /** Атрибуты, названные в запросе (namedAttrs по чипам). */
  named: ReadonlySet<string>
  selected?: boolean
  inSpec: (productId: string) => boolean
  showCompat?: boolean
}>()

const emit = defineEmits<{
  expand: [item: SearchItem, position: number]
  spec: [target: SearchItemBase, position: number]
  priceRequest: [target: SearchItemBase, position: number]
  why: [item: SearchItem]
}>()

const expanded = ref(false)
const variants = computed(() => otherVariants(props.item))
const action = computed(() => cardAction(props.item, variants.value.length))
const isSoftware = computed(
  () => (props.item.kind ?? 'software') !== 'hardware' && !!props.item.attrs,
)
const cells = computed<AttrCell[]>(() =>
  props.item.attrs ? attrCells(props.item.attrs, props.named) : [],
)
const rows = computed<SearchItemBase[]>(() => [props.item, ...variants.value])
const detailsId = computed(() => `family-${props.position}-variants`)

function toggle() {
  expanded.value = !expanded.value
  if (expanded.value) emit('expand', props.item, props.position)
}

function rowQuantity(v: SearchItemBase): string {
  const a = v.attrs
  if (!a) return v.license_line || '—'
  const c = attrCells(a, new Set()).find((x) => x.key === 'quantity')
  return c?.value ?? '—'
}
</script>

<template>
  <article
    :class="['fcard', { 'fcard--selected': selected }]"
    :aria-label="item.title"
    :data-product="item.product_id"
  >
    <div class="fcard__row">
      <div class="fcard__main">
        <p class="fcard__meta t-xs">
          <span class="fcard__vendor">{{ item.vendor }}</span>
          <span v-if="item.category" aria-hidden="true"> · </span>
          <span v-if="item.category">{{ item.category }}</span>
        </p>
        <h3 class="fcard__title">{{ item.title }}</h3>

        <dl v-if="isSoftware" class="fcard__attrs t-xs">
          <div
            v-for="c in cells"
            :key="c.key"
            :class="['fcard__attr', { 'fcard__attr--hl': c.highlight }]"
          >
            <dt>{{ c.label }}</dt>
            <dd>
              {{ c.value
              }}<span v-if="c.highlight" class="sr-only"> — атрибут назван в запросе</span>
            </dd>
          </div>
        </dl>
        <p v-else class="fcard__license t-sm">
          <span>{{ item.license_line || (item.mpn ? '' : 'Условия не указаны') }}</span>
          <span v-if="item.mpn" class="t-muted">
            Парт-номер <span class="t-mono">{{ item.mpn }}</span>
          </span>
          <CompatBadge v-if="showCompat" :compat="item.compatibility ?? null" />
        </p>

        <p class="fcard__badges">
          <span v-if="item.matches_query && named.size" class="badge">Подходит под запрос</span>
          <span v-if="item.group === 'analog'" class="badge badge--strong">Аналог</span>
          <span v-if="item.purchased_before" class="badge badge--strong">
            Вы покупали этот продукт
          </span>
        </p>
      </div>

      <div class="fcard__side">
        <p
          :class="[
            'fcard__price t-num',
            { 'fcard__price--request': !isPurchasable(item) && !variants.length },
          ]"
        >
          {{ familyPriceLabel(item, variants.length) }}
        </p>
        <div class="fcard__actions">
          <button
            v-if="action === 'choose'"
            type="button"
            class="btn btn--primary btn--sm"
            :aria-expanded="expanded"
            :aria-controls="detailsId"
            @click="toggle"
          >
            Выбрать лицензию
          </button>
          <template v-else-if="action === 'spec'">
            <span v-if="inSpec(item.product_id)" class="btn btn--ghost btn--sm" role="status">
              <Check :size="14" aria-hidden="true" /> В спецификации
            </span>
            <button
              v-else
              type="button"
              class="btn btn--primary btn--sm"
              @click="emit('spec', item, position)"
            >
              <ListPlus :size="14" aria-hidden="true" /> В спецификацию
            </button>
          </template>
          <button
            v-else
            type="button"
            class="btn btn--secondary btn--sm"
            @click="emit('priceRequest', item, position)"
          >
            Запросить цену
          </button>
          <button
            type="button"
            class="fcard__why t-num"
            :aria-label="`Почему здесь: скор ${formatScore(item.final)}, открыть трассу`"
            :aria-pressed="selected"
            @click="emit('why', item)"
          >
            <Route :size="14" aria-hidden="true" />
            {{ formatScore(item.final) }}
          </button>
        </div>
      </div>
    </div>

    <button
      v-if="variants.length"
      type="button"
      class="fcard__more t-sm"
      :aria-expanded="expanded"
      :aria-controls="detailsId"
      @click="toggle"
    >
      <ChevronDown
        :size="14"
        aria-hidden="true"
        :class="['fcard__chev', { 'fcard__chev--open': expanded }]"
      />
      {{
        expanded
          ? 'Скрыть варианты'
          : `ещё ${variants.length} ${variants.length === 1 ? 'вариант' : variants.length < 5 ? 'варианта' : 'вариантов'}`
      }}
    </button>

    <slot />

    <div v-if="expanded" :id="detailsId" class="fcard__variants">
      <table class="vtable t-sm">
        <caption class="sr-only">
          Варианты лицензий:
          {{
            item.title
          }}
        </caption>
        <thead>
          <tr>
            <th scope="col">Редакция</th>
            <th scope="col">Лицензия</th>
            <th scope="col">Срок</th>
            <th scope="col">Количество</th>
            <th scope="col" class="vtable__num">Цена</th>
            <th scope="col"><span class="sr-only">Действие</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="v in rows" :key="v.product_id" :class="{ vtable__hit: v.matches_query }">
            <td data-label="Редакция">{{ v.attrs?.edition || '—' }}</td>
            <td data-label="Лицензия">
              {{
                v.attrs?.license_type === 'renewal'
                  ? 'Продление'
                  : v.attrs?.license_type
                    ? 'Новая'
                    : '—'
              }}
            </td>
            <td data-label="Срок">
              {{ v.attrs?.license_type ? durationLabel(v.attrs.duration_months) : '—' }}
            </td>
            <td data-label="Количество">{{ rowQuantity(v) }}</td>
            <td data-label="Цена" class="vtable__num t-num">{{ priceLabel(v) }}</td>
            <td class="vtable__act">
              <template v-if="isPurchasable(v)">
                <span v-if="inSpec(v.product_id)" class="t-xs t-muted">В спецификации</span>
                <button
                  v-else
                  type="button"
                  class="btn btn--secondary btn--sm"
                  :aria-label="`В спецификацию: ${v.title}`"
                  @click="emit('spec', v, position)"
                >
                  В спецификацию
                </button>
              </template>
              <button
                v-else
                type="button"
                class="btn btn--ghost btn--sm"
                :aria-label="`Запросить цену: ${v.title}`"
                @click="emit('priceRequest', v, position)"
              >
                Запросить цену
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </article>
</template>

<style scoped>
.fcard {
  container-type: inline-size;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.fcard--selected {
  border-color: var(--border-control);
  background: var(--surface-2);
}
.fcard__row {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
}
.fcard__main {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.fcard__meta {
  margin: 0;
  color: var(--text-2);
}
.fcard__vendor {
  font-weight: var(--fw-medium);
  color: var(--text);
}
.fcard__title {
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.fcard__attrs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 0;
}
.fcard__attr {
  display: inline-flex;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-sm);
}
.fcard__attr dt {
  color: var(--text-3, var(--text-2));
}
.fcard__attr dd {
  margin: 0;
  color: var(--text);
}
.fcard__attr--hl {
  border-color: var(--border-control);
  background: var(--surface-3);
}
.fcard__attr--hl dd {
  font-weight: var(--fw-semibold);
}
.fcard__license {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  margin: 0;
}
.fcard__badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 0;
}
.fcard__badges:empty {
  display: none;
}
.fcard__side {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.fcard__price {
  margin: 0;
  font-weight: var(--fw-semibold);
}
.fcard__price--request {
  color: var(--text-2);
  font-weight: var(--fw-medium);
}
.fcard__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.fcard__why {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: transparent;
  color: var(--text-2);
  font-size: var(--fs-xs);
  cursor: pointer;
}
.fcard__why:hover,
.fcard__why[aria-pressed='true'] {
  color: var(--text);
  border-color: var(--border-control);
}
.fcard__more {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0 var(--space-3) var(--space-2);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.fcard__more:hover {
  color: var(--text);
  text-decoration: underline;
}
.fcard__chev {
  transition: transform var(--dur-fast, 120ms);
}
.fcard__chev--open {
  transform: rotate(180deg);
}
.fcard__variants {
  padding: 0 var(--space-3) var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.vtable {
  width: 100%;
  border-collapse: collapse;
}
.vtable th {
  padding: var(--space-2) var(--space-2) var(--space-1) 0;
  color: var(--text-2);
  font-weight: var(--fw-medium);
  text-align: left;
}
.vtable td {
  padding: var(--space-1) var(--space-2) var(--space-1) 0;
  border-top: var(--border-w) solid var(--border);
}
.vtable__num {
  text-align: right;
}
.vtable__act {
  text-align: right;
  white-space: nowrap;
}
.vtable__hit td:first-child {
  box-shadow: inset 2px 0 0 var(--accent);
  padding-left: var(--space-2);
}

@container (min-width: 40rem) {
  .fcard__row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
  }
  .fcard__side {
    flex-direction: column;
    align-items: flex-end;
  }
  .fcard__price {
    text-align: right;
  }
}

/* Узкая карточка: таблица вариантов — строками-карточками с подписями. */
@container (max-width: 34rem) {
  .vtable thead {
    display: none;
  }
  .vtable,
  .vtable tbody,
  .vtable tr,
  .vtable td {
    display: block;
  }
  .vtable tr {
    padding: var(--space-2) 0;
    border-top: var(--border-w) solid var(--border);
  }
  .vtable td {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    padding: 2px 0;
    border: 0;
    text-align: left;
  }
  .vtable td[data-label]::before {
    content: attr(data-label);
    color: var(--text-2);
  }
  .vtable__hit td:first-child {
    box-shadow: none;
    padding-left: 0;
  }
  .vtable__act {
    justify-content: flex-end;
  }
}
</style>
