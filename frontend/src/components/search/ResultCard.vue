<script setup lang="ts">
// Карточка выдачи — одна строка с раскрытием: название, вендор и категория, строка
// лицензии, пометки «Аналог» / «Вы покупали», цена или «Цена по запросу», действия. Справа — скор,
// он же «почему здесь»: открывает трассу этой позиции. Раскрытие показывает другие лицензии семейства,
// подходящая под запрос — первой и с пометкой.
import VendorLogo from '@/components/common/VendorLogo.vue'
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Check, ChevronDown, ChevronRight, ListPlus, Route } from 'lucide-vue-next'
import type { SearchItem, SearchItemBase } from '@/api/types'
import CompatBadge from '@/components/search/CompatBadge.vue'
import { formatScore } from '@/lib/format'
import {
  isPurchasable,
  joinLicenseParts,
  otherVariants,
  priceLabel,
  splitLicenseLine,
} from '@/lib/license'
import { productHref } from '@/lib/product'

export type ItemAction = 'add_to_cart' | 'order' | 'price_request'

const props = defineProps<{
  item: SearchItem
  position: number
  /** Позиция выбрана в панели трассы. */
  selected?: boolean
  /** Контур коммерции: позиция уже в спецификации; undefined — кнопки «В спецификацию» нет. */
  inSpec?: boolean
  /** Контур коммерции: показывать значок совместимости (флаг hardware). */
  showCompat?: boolean
  /** Номер поиска: уходит в ссылку на страницу товара. */
  requestId?: string | null
}>()

const emit = defineEmits<{
  open: [item: SearchItem, position: number]
  action: [type: ItemAction, target: SearchItemBase, position: number]
  why: [item: SearchItem]
  spec: [item: SearchItem, position: number]
  /** Переход на страницу товара по названию. */
  product: [item: SearchItem, position: number]
}>()

const expanded = ref(false)
const variants = computed(() => otherVariants(props.item))
const license = computed(() => joinLicenseParts(splitLicenseLine(props.item.license_line)))
const detailsId = computed(() => `result-${props.position}-details`)

function toggle() {
  expanded.value = !expanded.value
  if (expanded.value) emit('open', props.item, props.position)
}
</script>

<template>
  <article
    :class="['result', { 'result--selected': selected, 'result--expanded': expanded }]"
    :aria-label="item.title"
  >
    <div class="result__row">
      <div class="result__main">
        <VendorLogo :vendor="item.vendor" class="result__main-logo" />
        <h3 class="result__title">
          <button
            type="button"
            class="result__toggle"
            :aria-expanded="expanded"
            :aria-controls="detailsId"
            :aria-label="`Другие лицензии: ${item.title}`"
            @click="toggle"
          >
            <component
              :is="expanded ? ChevronDown : ChevronRight"
              :size="16"
              aria-hidden="true"
              class="result__chevron"
            />
          </button>
          <RouterLink
            :to="productHref(item.product_id, requestId, position)"
            class="result__title-link"
            data-testid="product-link"
            @click="emit('product', item, position)"
          >
            {{ item.title }}
          </RouterLink>
        </h3>
        <p class="result__meta t-sm">
          <span>{{ item.vendor }}</span>
          <span v-if="item.category" aria-hidden="true"> · </span>
          <span v-if="item.category">{{ item.category }}</span>
        </p>
        <p class="result__license t-sm">
          <span>{{ license || 'Условия лицензии не указаны' }}</span>
          <span v-if="item.group === 'analog'" class="badge badge--strong">Аналог</span>
          <span v-if="item.purchased_before" class="badge badge--strong">
            Вы покупали этот продукт
          </span>
        </p>
        <p v-if="item.mpn || (showCompat && item.compatibility)" class="result__hw t-xs">
          <span v-if="item.mpn" class="t-muted">
            Парт-номер <span class="t-mono">{{ item.mpn }}</span>
          </span>
          <CompatBadge v-if="showCompat" :compat="item.compatibility" />
        </p>
      </div>

      <p :class="['result__price t-num', { 'result__price--request': !isPurchasable(item) }]">
        {{ priceLabel(item) }}
      </p>

      <div class="result__actions">
        <template v-if="isPurchasable(item)">
          <button
            type="button"
            class="btn btn--secondary btn--sm"
            @click="emit('action', 'add_to_cart', item, position)"
          >
            В корзину
          </button>
          <button
            type="button"
            class="btn btn--ghost btn--sm"
            @click="emit('action', 'order', item, position)"
          >
            Заказать
          </button>
        </template>
        <button
          v-else
          type="button"
          class="btn btn--secondary btn--sm"
          @click="emit('action', 'price_request', item, position)"
        >
          Запросить цену
        </button>
        <template v-if="inSpec !== undefined">
          <span
            v-if="inSpec"
            class="btn btn--ghost btn--icon btn--sm result__spec"
            role="img"
            aria-label="В спецификации"
          >
            <Check :size="16" aria-hidden="true" />
          </span>
          <button
            v-else
            type="button"
            class="btn btn--ghost btn--icon btn--sm result__spec"
            :aria-label="`В спецификацию: ${item.title}`"
            title="В спецификацию"
            @click="emit('spec', item, position)"
          >
            <ListPlus :size="16" aria-hidden="true" />
          </button>
        </template>
      </div>

      <button
        type="button"
        class="result__why t-num"
        :aria-label="`Почему здесь: скор ${formatScore(item.final)}, открыть трассу`"
        :aria-pressed="selected"
        @click="emit('why', item)"
      >
        <Route :size="14" aria-hidden="true" />
        {{ formatScore(item.final) }}
      </button>
    </div>

    <slot />

    <div v-if="expanded" :id="detailsId" class="result__details">
      <p v-if="!variants.length" class="t-sm t-muted">
        Других лицензий этого продукта в каталоге нет.
      </p>
      <template v-else>
        <p class="t-sm t-muted">Другие лицензии</p>
        <ul class="variants">
          <li v-for="v in variants" :key="v.product_id" class="variant">
            <span class="variant__license t-sm">
              {{ joinLicenseParts(splitLicenseLine(v.license_line)) || v.title }}
              <span v-if="v.matches_query" class="badge badge--strong">Подходит под запрос</span>
            </span>
            <span class="variant__price t-num t-sm">{{ priceLabel(v) }}</span>
            <span class="variant__actions">
              <button
                v-if="isPurchasable(v)"
                type="button"
                class="btn btn--ghost btn--sm"
                @click="emit('action', 'add_to_cart', v, position)"
              >
                В корзину
              </button>
              <button
                v-else
                type="button"
                class="btn btn--ghost btn--sm"
                @click="emit('action', 'price_request', v, position)"
              >
                Запросить цену
              </button>
            </span>
          </li>
        </ul>
      </template>
    </div>
  </article>
</template>

<style scoped>
.result {
  container-type: inline-size;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.result--selected {
  border-color: var(--border-control);
  background: var(--surface-2);
}
.result__row {
  --indent: calc(var(--space-1) + var(--icon-sm));
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    'main main'
    'price why'
    'actions actions';
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3);
}
.result__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.result__title {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.result__toggle {
  display: inline-flex;
  align-items: flex-start;
  gap: var(--space-1);
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.result__title-link {
  color: var(--text);
  text-decoration: none;
  border-radius: var(--radius-sm);
  overflow-wrap: anywhere;
}
.result__title-link:hover {
  text-decoration: underline;
  text-decoration-color: var(--text-3);
  text-underline-offset: 3px;
}
.result__chevron {
  flex: none;
  margin-top: 2px;
  color: var(--text-2);
}
.result__meta,
.result__license {
  padding-left: var(--indent);
  color: var(--text-2);
}
.result__license {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  color: var(--text);
}
.result__hw {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  padding-left: var(--indent);
}
.result__price {
  grid-area: price;
  padding-left: var(--indent);
  font-weight: var(--fw-semibold);
}
.result__price--request {
  color: var(--text-2);
  font-weight: var(--fw-medium);
}
.result__actions {
  grid-area: actions;
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  padding-left: var(--indent);
}
.result__why {
  grid-area: why;
  justify-self: end;
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
.result__why:hover,
.result__why[aria-pressed='true'] {
  color: var(--text);
  border-color: var(--border-control);
}
.result__details {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-3) var(--space-3) calc(var(--space-4) + var(--icon-sm));
  border-top: var(--border-w) solid var(--border);
}
.variants {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.variant {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-1) var(--space-3);
  padding: var(--space-1) 0;
}
.variant + .variant {
  border-top: var(--border-w) solid var(--border);
}
.variant__license {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.variant__price {
  text-align: right;
}
.variant__actions {
  grid-column: 1 / -1;
}

/* Широкая карточка (по ширине колонки, а не окна: с открытой трассой колонка уже) — одна строка. */
@container (min-width: 36rem) {
  .result__row {
    grid-template-columns: minmax(0, 1fr) 8.5rem 12.5rem auto;
    grid-template-areas: 'main price actions why';
    padding: var(--space-2) var(--space-3);
  }
  .result__price,
  .result__actions {
    padding-left: 0;
  }
  .result__price {
    text-align: right;
  }
  .result__actions {
    flex-wrap: nowrap;
  }
  .variant {
    grid-template-columns: minmax(0, 1fr) 8.5rem 10.5rem;
  }
  .variant__actions {
    grid-column: auto;
  }
}
/* Значок производителя — своя колонка слева от названия и атрибутов. */
.result__main {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-content: start;
  column-gap: var(--space-3);
}
.result__main > * {
  grid-column: 2;
}
.result__main > .result__main-logo {
  grid-column: 1;
  grid-row: 1 / span 3;
  align-self: start;
}
</style>
