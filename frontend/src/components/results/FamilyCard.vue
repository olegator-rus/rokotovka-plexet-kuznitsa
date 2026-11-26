<script setup lang="ts">
// Карточка-семейство: вендор и категория, название, строка атрибутов (одинаковый набор у карточек
// ПО, нет значения — прочерк; атрибут из запроса выделен), цена «от N ₽» по семейству или точная у позиции,
// действие по состоянию — «Выбрать лицензию» (раскрывает варианты), «В спецификацию» (вариант определён),
// «Запросить цену» (цены нет). Раскрытие — таблица «редакция — срок — метрика и количество — цена» с действием
// у каждой строки. Справа — скор, он же «почему здесь» (трасса). Рейтингов, рассрочки и избранного нет.
import VendorLogo from '@/components/common/VendorLogo.vue'
import { computed, nextTick, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Check, ChevronDown, ExternalLink, ListPlus, Route } from 'lucide-vue-next'
import type { Purchase, SearchItem, SearchItemBase } from '@/api/types'
import CompatBadge from '@/components/search/CompatBadge.vue'
import PromoLine from '@/components/results/PromoLine.vue'
import { hasTier, perPlaceLabel } from '@/lib/offerNotes'
import { formatScore } from '@/lib/format'
import { reestrTone, selectedOffer } from '@/lib/procurement'
import { productHref } from '@/lib/product'
import { isPurchasable, licenseKindLabel, otherVariants, priceLabel } from '@/lib/license'
import { ALTERNATIVE_NOTE, cardPrice, storeLink } from '@/lib/matchGroups'
import { SHOWCASE } from '@/lib/showcase'
import {
  attrCells,
  cardAction,
  durationLabel,
  type AttrCell,
  type NamedAttrs,
} from '@/lib/resultsView'

const props = defineProps<{
  item: SearchItem
  position: number
  /** Атрибуты, названные в запросе (namedAttrs по чипам). */
  named: NamedAttrs | ReadonlySet<string>
  selected?: boolean
  inSpec: (productId: string) => boolean
  showCompat?: boolean
  /** Контур доводки: параметры закупки из запроса — карточка открывается с выбранным по ним вариантом. */
  purchase?: Purchase | null
  /** Номер поиска: уходит в ссылку на страницу товара. */
  requestId?: string | null
}>()

const emit = defineEmits<{
  expand: [item: SearchItem, position: number]
  spec: [target: SearchItemBase, position: number]
  priceRequest: [target: SearchItemBase, position: number]
  why: [item: SearchItem]
  /** Переход на страницу товара по названию. */
  product: [item: SearchItem, position: number]
}>()

const expanded = ref(false)
const offer = computed(() => selectedOffer(props.item, props.purchase))
const variants = computed(() => otherVariants(props.item))
const action = computed(() => cardAction(props.item, variants.value.length))
// Контур фида: «от N ₽» / старая цена и скидка / «Цена по запросу»; 0 ₽ не выводится.
const price = computed(() => cardPrice(props.item, variants.value.length))
const store = computed(() => storeLink(props.item))
// у позиции со ступенью количества цена — за одно место («3 070,80 ₽ за узел»).
const perPlace = computed(() =>
  hasTier(props.item) && price.value.state === 'priced'
    ? perPlaceLabel(props.item.attrs?.license_metric || props.purchase?.metric)
    : '',
)
const isAlternative = computed(() => props.item.match_type === 'alternative')
// У альтернативы одна подпись: пояснение сервера, если оно уже говорит о совместимости, иначе общее.
const altNote = computed(() =>
  /совместим/i.test(props.item.explain ?? '') ? (props.item.explain as string) : ALTERNATIVE_NOTE,
)
const explainShown = computed(() => (isAlternative.value ? '' : (props.item.explain ?? '')))
// Одна позиция на одно место: цена и «В спецификацию» уже справа — в блоке выбранного не повторяем.
const offerRepeatsSide = computed(() => action.value === 'spec' && offer.value?.quantity === 1)
const isSoftware = computed(
  () => (props.item.kind ?? 'software') !== 'hardware' && !!props.item.attrs,
)
// Контур доводки: статус реестра — бейджем в трёх состояниях (item.reestr), прежняя ячейка из in_reestr не дублирует его.
const cells = computed<AttrCell[]>(() =>
  props.item.attrs
    ? attrCells(props.item.attrs, props.named, licenseKindLabel(props.item)).filter(
        (c) => !(props.item.reestr && c.key === 'reestr'),
      )
    : [],
)
const rows = computed<SearchItemBase[]>(() => [props.item, ...variants.value])
const detailsId = computed(() => `family-${props.position}-variants`)

function toggle() {
  expanded.value = !expanded.value
  if (expanded.value) emit('expand', props.item, props.position)
}

const root = ref<HTMLElement | null>(null)

/**
 * В спецификацию (или «Запросить цену» — тоже кладёт позицию с ценой по запросу). Кнопка сменяется пометкой
 * «В спецификации», фокус переходит на неё, а не в body.
 */
async function addToSpec(target: SearchItemBase, place: 'offer' | 'side' | 'row', price = false) {
  if (price) emit('priceRequest', target, props.position)
  else emit('spec', target, props.position)
  await nextTick()
  root.value
    ?.querySelector<HTMLElement>(`[data-spec-done="${place}:${target.product_id}"]`)
    ?.focus()
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
    ref="root"
    :class="['fcard', { 'fcard--selected': selected }]"
    :aria-label="item.title"
    :data-product="item.product_id"
  >
    <div class="fcard__row">
      <div class="fcard__main">
        <VendorLogo :vendor="item.vendor" class="fcard__main-logo" />
        <p class="fcard__meta t-xs">
          <span class="fcard__vendor">{{ item.vendor }}</span>
          <span v-if="item.category" aria-hidden="true"> · </span>
          <span v-if="item.category">{{ item.category }}</span>
        </p>
        <h3 class="fcard__title">
          <RouterLink
            :to="productHref(item.product_id, requestId, position)"
            class="fcard__title-link"
            data-testid="product-link"
            @click="emit('product', item, position)"
          >
            {{ item.title }}
          </RouterLink>
        </h3>

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
          <span v-if="item.group === 'analog' && !item.match_type" class="badge badge--strong">
            Аналог
          </span>
          <span
            v-if="price.discount"
            class="badge badge--tone fcard__discount"
            data-testid="discount-badge"
          >
            {{ price.discount }}
          </span>
          <span
            v-if="SHOWCASE.personalization && item.purchased_before"
            class="badge badge--strong"
          >
            Вы покупали этот продукт
          </span>
          <span
            v-if="item.reestr"
            :class="['badge', { 'badge--strong': reestrTone(item.reestr) === 'ok' }]"
            data-testid="reestr"
          >
            {{ item.reestr.label }}
          </span>
        </p>
        <PromoLine :promos="item.promos" />
        <p v-if="item.analog_note" class="fcard__note t-xs t-muted">{{ item.analog_note }}</p>
        <p v-if="isAlternative" class="fcard__alt t-xs" data-testid="alternative-note">
          {{ altNote }}
        </p>
        <p v-if="explainShown" class="fcard__note t-xs t-muted" data-testid="item-explain">
          {{ explainShown }}
        </p>
        <p v-if="store" class="fcard__store t-xs">
          <a
            :href="store.url"
            target="_blank"
            rel="noopener noreferrer"
            class="fcard__store-link"
            data-testid="store-link"
          >
            {{ store.kind === 'family' ? 'Все варианты на сайте' : 'На сайте' }}
            <ExternalLink :size="12" aria-hidden="true" />
            <span class="sr-only"> (откроется в новой вкладке)</span>
          </a>
        </p>
        <div v-if="offer" class="fcard__offer t-sm" data-testid="selected-offer">
          <p>
            <span class="t-muted">Выбрано по запросу:</span>&nbsp;<strong>{{ offer.label }}</strong>
          </p>
          <template v-if="!offerRepeatsSide">
            <p class="t-num">{{ offer.sum }}</p>
            <span
              v-if="inSpec(item.product_id)"
              class="t-xs t-muted"
              role="status"
              tabindex="-1"
              :data-spec-done="`offer:${item.product_id}`"
            >
              В спецификации
            </span>
            <button
              v-else
              type="button"
              class="btn btn--primary btn--sm"
              data-testid="offer-spec"
              @click="addToSpec(item, 'offer')"
            >
              <ListPlus :size="14" aria-hidden="true" /> В спецификацию
            </button>
          </template>
        </div>
      </div>

      <div class="fcard__side">
        <p
          :class="['fcard__price t-num', { 'fcard__price--request': price.state === 'on_request' }]"
          data-testid="card-price"
          :data-price-state="price.state"
        >
          <s v-if="price.old" class="fcard__old t-sm" data-testid="old-price">
            <span class="sr-only">Старая цена </span>{{ price.old }}
          </s>
          {{ price.label }}
          <span v-if="perPlace" class="fcard__unit t-xs" data-testid="per-place">{{
            perPlace
          }}</span>
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
          <span
            v-else-if="inSpec(item.product_id)"
            class="btn btn--ghost btn--sm"
            role="status"
            tabindex="-1"
            :data-spec-done="`side:${item.product_id}`"
          >
            <Check :size="14" aria-hidden="true" /> В спецификации
          </span>
          <button
            v-else-if="action === 'spec'"
            type="button"
            class="btn btn--primary btn--sm"
            @click="addToSpec(item, 'side')"
          >
            <ListPlus :size="14" aria-hidden="true" /> В спецификацию
          </button>
          <button
            v-else
            type="button"
            class="btn btn--secondary btn--sm"
            @click="addToSpec(item, 'side', true)"
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
              {{ licenseKindLabel(v) || '—' }}
            </td>
            <td data-label="Срок">
              {{ v.attrs?.license_type ? durationLabel(v.attrs.duration_months) : '—' }}
            </td>
            <td data-label="Количество">{{ rowQuantity(v) }}</td>
            <td data-label="Цена" class="vtable__num t-num">{{ priceLabel(v) }}</td>
            <td class="vtable__act">
              <span
                v-if="inSpec(v.product_id)"
                class="t-xs t-muted"
                tabindex="-1"
                :data-spec-done="`row:${v.product_id}`"
              >
                В спецификации<span class="sr-only">: {{ v.title }}</span>
              </span>
              <button
                v-else-if="isPurchasable(v)"
                type="button"
                class="btn btn--secondary btn--sm"
                :aria-label="`В спецификацию: ${v.title}`"
                @click="addToSpec(v, 'row')"
              >
                В спецификацию
              </button>
              <button
                v-else
                type="button"
                class="btn btn--ghost btn--sm"
                :aria-label="`Запросить цену: ${v.title}`"
                @click="addToSpec(v, 'row', true)"
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
.fcard__note {
  margin: var(--space-1) 0 0;
}
.fcard__alt {
  margin: var(--space-1) 0 0;
  padding-left: var(--space-2);
  border-left: 2px solid var(--border-control);
  color: var(--text-2);
}
.fcard__store {
  margin: 0;
}
.fcard__store-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--text-2);
  text-underline-offset: 3px;
}
.fcard__store-link:hover {
  color: var(--text);
}
.fcard__discount {
  --tone: var(--status-accepted);
  --tone-bg: var(--status-accepted-bg);
}
.fcard__unit {
  display: block;
  color: var(--text-2);
  font-weight: var(--fw-regular);
}
.fcard__old {
  margin-right: var(--space-1);
  color: var(--text-2);
  font-weight: var(--fw-regular);
}
.fcard__offer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-3);
  margin-top: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-card);
  background: var(--surface-2);
}
.fcard__offer p {
  margin: 0;
}
.fcard__offer p.t-num {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-bold);
  white-space: normal;
}
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
.fcard__title-link {
  color: var(--text);
  text-decoration: none;
  border-radius: var(--radius-sm);
  overflow-wrap: anywhere;
}
.fcard__title-link:hover {
  text-decoration: underline;
  text-decoration-color: var(--text-3);
  text-underline-offset: 3px;
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
/* Значок производителя — своя колонка слева от названия и атрибутов. */
.fcard__main {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-content: start;
  column-gap: var(--space-3);
}
.fcard__main > * {
  grid-column: 2;
}
.fcard__main > .fcard__main-logo {
  grid-column: 1;
  grid-row: 1 / span 3;
  align-self: start;
}
</style>
