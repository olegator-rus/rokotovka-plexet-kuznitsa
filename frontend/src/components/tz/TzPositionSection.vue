<script setup lang="ts">
// Блок позиции ТЗ (TZ-UI.md §6): заголовок — ссылка в полный поиск по позиции, статус значком и словом, плашки
// условий без крестиков, цитата из документа (дословно), строка «Не проверено: …» перед карточками — видна без
// раскрытия. Карточки — существующая FamilyCard в плотном виде: расчёт и добавление не меняются. «Найдено» — одна
// карточка и «Ещё N вариантов» на месте; «Похожие» — до четырёх сразу. Незавершённый поиск — заглушка, не «Нет в
// каталоге»; сбой — «Ошибка поиска» с повтором только этой позиции.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import {
  ChevronRight,
  CircleCheck,
  CircleMinus,
  Copy,
  Layers,
  LoaderCircle,
  RotateCcw,
  TriangleAlert,
} from 'lucide-vue-next'
import type { SearchItemBase } from '@/api/types'
import FamilyCard from '@/components/results/FamilyCard.vue'
import { useToast } from '@/components/ui/useToast'
import type { TzRow } from '@/composables/useTzResults'
import {
  TZ_TEXT,
  moreVariantsLabel,
  positionLabel,
  positionPurchase,
  positionQuantity,
  positionSearchQuery,
  shownProducts,
  unverifiedLabel,
} from '@/lib/tz'

const props = defineProps<{
  row: TzRow
  hash: string
  /** Из позиции что-то добавлено в спецификацию. */
  added: boolean
  quoteOpen: boolean
  variantsOpen: boolean
  inSpec: (productId: string) => boolean
}>()

const emit = defineEmits<{
  toggleQuote: []
  toggleVariants: []
  retry: []
  spec: [target: SearchItemBase]
  priceRequest: [target: SearchItemBase]
  manager: []
  open: []
}>()

const toast = useToast()
const p = computed(() => props.row.position)
const ready = computed(() => props.row.state === 'ready')
const status = computed(() => (ready.value ? props.row.status : null))
const label = computed(() => positionLabel(props.row.state, props.row.status))
const shown = computed(() => shownProducts(status.value, props.row.items))
const purchase = computed(() => positionPurchase(p.value))
const searchTo = computed(() => ({
  path: '/search',
  query: positionSearchQuery(props.hash, p.value),
}))
const quoteId = computed(() => `tz-quote-${p.value.id}`)
const moreId = computed(() => `tz-more-${p.value.id}`)
const headingId = computed(() => `tz-pos-title-${p.value.id}`)
/** Абзацы исходного документа: пустые строки между ними не нужны. */
const paragraphs = computed(() =>
  (p.value.source_quote ?? '')
    .split(/\n+/)
    .map((s) => s.trim())
    .filter(Boolean),
)
const noNamed = new Set<string>()

/**
 * Переход в полный поиск по позиции. Ссылка без отметки «текущая страница»: путь тот же (/search), отличается только
 * адрес запроса. Место прокрутки страница ТЗ запоминает до перехода.
 */
function onOpen(e: MouseEvent, navigate: (e?: MouseEvent) => unknown) {
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return
  emit('open')
  void navigate(e)
}

async function copyQuote() {
  try {
    await navigator.clipboard.writeText(p.value.source_quote)
    toast.add({ tone: 'ok', title: 'Цитата скопирована' })
  } catch {
    toast.add({ tone: 'error', title: 'Не удалось скопировать', detail: 'Выделите текст вручную' })
  }
}
</script>

<template>
  <section
    :id="`tz-pos-${p.number}`"
    class="tzp"
    :aria-labelledby="headingId"
    data-testid="tz-position"
    :data-number="p.number"
    :data-state="row.state"
    :data-status="status ?? ''"
  >
    <!-- Что просили в ТЗ — шапка общей рамки позиции; найденные товары — под ней, внутри той же рамки. -->
    <div class="tzp__summary">
      <header class="tzp__head">
        <h3 :id="headingId" class="tzp__title">
          <RouterLink v-slot="{ href, navigate }" :to="searchTo" custom>
            <a :href="href" class="tzp__title-link" @click="onOpen($event, navigate)">
              <span class="tzp__num">{{ p.number }}<span class="sr-only">.</span></span>
              <span class="tzp__name">{{ p.title }}</span>
            </a>
          </RouterLink>
        </h3>
        <span
          :class="['tzp__status', `tzp__status--${row.state === 'ready' ? status : row.state}`]"
          data-testid="tz-position-status"
        >
          <CircleCheck v-if="status === 'found'" :size="16" aria-hidden="true" />
          <Layers v-else-if="status === 'similar'" :size="16" aria-hidden="true" />
          <CircleMinus v-else-if="status === 'not_found'" :size="16" aria-hidden="true" />
          <TriangleAlert v-else-if="row.state === 'error'" :size="16" aria-hidden="true" />
          <LoaderCircle v-else :size="16" aria-hidden="true" class="tzp__spin" />
          {{ label }}
        </span>
      </header>

      <ul v-if="p.attributes.length" class="tzp__attrs" aria-label="Условия позиции">
        <li v-for="a in p.attributes" :key="a.key" class="tzp__attr t-xs" :title="a.label">
          {{ a.value }}
        </li>
      </ul>

      <div v-if="p.source_quote" class="tzp__quote">
        <button
          type="button"
          class="tzp__quote-toggle"
          :aria-expanded="quoteOpen"
          :aria-controls="quoteId"
          data-testid="tz-quote-toggle"
          @click="emit('toggleQuote')"
        >
          <ChevronRight
            :size="14"
            aria-hidden="true"
            :class="['tzp__chev', { 'tzp__chev--open': quoteOpen }]"
          />
          <span class="sr-only"
            >{{ quoteOpen ? TZ_TEXT.quoteCollapse : TZ_TEXT.quoteExpand }}:
          </span>
          <span class="tzp__quote-label" aria-hidden="true">Из документа</span>
          <span v-if="!quoteOpen" class="tzp__quote-line">{{ p.source_quote }}</span>
          <span v-else class="tzp__quote-line" aria-hidden="true">{{ TZ_TEXT.quoteCollapse }}</span>
        </button>
        <div v-if="quoteOpen" :id="quoteId" class="tzp__quote-body" data-testid="tz-quote">
          <blockquote class="tzp__quote-text">
            <p v-for="(para, i) in paragraphs" :key="i">{{ para }}</p>
          </blockquote>
          <button type="button" class="btn btn--ghost btn--sm" @click="copyQuote">
            <Copy :size="14" aria-hidden="true" /> {{ TZ_TEXT.quoteCopy }}
          </button>
        </div>
      </div>

      <p v-if="status === 'similar'" class="tzp__similar t-sm">{{ TZ_TEXT.similarDescription }}</p>
      <p
        v-if="p.unverified_requirements.length"
        class="tzp__unverified t-sm"
        data-testid="tz-unverified"
      >
        <TriangleAlert :size="16" aria-hidden="true" />
        <span>{{ unverifiedLabel(p.unverified_requirements) }}</span>
      </p>
    </div>

    <div class="tzp__body">
      <!-- Подпись связывает товары с позицией: видно, к какой строке ТЗ они подобраны. -->
      <p
        v-if="status === 'found' || status === 'similar'"
        class="tzp__caption t-sm"
        data-testid="tz-position-caption"
      >
        {{ status === 'found' ? 'Подобрано' : 'Похожее' }} по позиции {{ p.number }}
      </p>

      <!-- Заглушка высотой с плотную карточку: готовая карточка заменяет её без прыжка страницы. -->
      <div
        v-if="row.state === 'pending' || row.state === 'searching'"
        class="tzp__placeholder"
        data-testid="tz-placeholder"
      >
        <span class="skeleton tzp__ph-logo" aria-hidden="true" />
        <span class="tzp__ph-lines" aria-hidden="true">
          <span class="skeleton tzp__ph-line" />
          <span class="skeleton tzp__ph-line tzp__ph-line--short" />
        </span>
        <span class="t-sm t-muted tzp__ph-label">{{ label }}</span>
      </div>

      <div v-else-if="row.state === 'error'" class="tzp__error" role="alert">
        <TriangleAlert :size="16" aria-hidden="true" class="tzp__error-icon" />
        <span class="t-sm">{{ TZ_TEXT.errPosition }}</span>
        <button
          type="button"
          class="btn btn--secondary btn--sm"
          data-testid="tz-position-retry"
          @click="emit('retry')"
        >
          <RotateCcw :size="14" aria-hidden="true" /> {{ TZ_TEXT.retry }}
        </button>
      </div>

      <template v-else-if="status === 'found' || status === 'similar'">
        <ul class="tzp__cards">
          <li v-for="(it, i) in shown.first" :key="it.product_id">
            <FamilyCard
              density="compact"
              :item="it"
              :position="i + 1"
              :named="noNamed"
              :in-spec="inSpec"
              :purchase="purchase"
              :quantity="positionQuantity(it, p)"
              :request-id="row.requestId"
              @spec="(t) => emit('spec', t)"
              @price-request="(t) => emit('priceRequest', t)"
            />
          </li>
        </ul>
        <ul v-if="variantsOpen && shown.more.length" :id="moreId" class="tzp__cards">
          <li v-for="(it, i) in shown.more" :key="it.product_id">
            <FamilyCard
              density="compact"
              :item="it"
              :position="i + 2"
              :named="noNamed"
              :in-spec="inSpec"
              :purchase="purchase"
              :quantity="positionQuantity(it, p)"
              :request-id="row.requestId"
              @spec="(t) => emit('spec', t)"
              @price-request="(t) => emit('priceRequest', t)"
            />
          </li>
        </ul>
        <div class="tzp__foot">
          <button
            v-if="shown.more.length"
            type="button"
            class="link tzp__more t-sm"
            :aria-expanded="variantsOpen"
            :aria-controls="moreId"
            data-testid="tz-more"
            @click="emit('toggleVariants')"
          >
            {{ variantsOpen ? TZ_TEXT.variantsCollapse : moreVariantsLabel(shown.more.length) }}
          </button>
          <RouterLink v-slot="{ href, navigate }" :to="searchTo" custom>
            <a :href="href" class="link t-sm tzp__open" @click="onOpen($event, navigate)">
              {{ TZ_TEXT.openSearch }}
            </a>
          </RouterLink>
        </div>
      </template>

      <template v-else>
        <p class="tzp__empty t-sm">{{ TZ_TEXT.empty }}</p>
        <div class="tzp__foot">
          <RouterLink v-slot="{ href, navigate }" :to="searchTo" custom>
            <a :href="href" class="link t-sm" @click="onOpen($event, navigate)">
              {{ TZ_TEXT.openSearch }}
            </a>
          </RouterLink>
          <button
            type="button"
            class="btn btn--secondary btn--sm tzp__manager"
            data-testid="tz-manager"
            @click="emit('manager')"
          >
            {{ TZ_TEXT.manager }}
          </button>
        </div>
      </template>
    </div>
    <span v-if="added" class="sr-only">{{ TZ_TEXT.added }}</span>
  </section>
</template>

<style scoped>
.tzp {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  min-width: 0;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-card);
  background: var(--surface);
  /* Прокрутка к позиции — под закреплённой шапкой и селектором позиций. */
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--tz-sticky, 0px) + 1rem);
}
.tzp__summary {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  padding: var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
  border-radius: calc(var(--radius-card) - var(--border-w))
    calc(var(--radius-card) - var(--border-w)) 0 0;
  background: var(--surface-2);
}
.tzp__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-3) var(--space-4);
}
.tzp__caption {
  margin: 0;
  color: var(--text-2);
  font-weight: var(--fw-semibold);
}
.tzp__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
}
.tzp__title {
  flex: 1 1 20rem;
  min-width: 0;
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.tzp__title-link {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  color: var(--text);
  text-decoration: none;
}
.tzp__num {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-pill);
  background: var(--text);
  color: var(--surface);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.tzp__name {
  min-width: 0;
  /* Первая строка названия — по центру кружка с номером. */
  padding-top: calc((1.75rem - 1lh) / 2);
}
.tzp__title-link:hover .tzp__name {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.tzp__status {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--space-1);
  min-height: 1.75rem;
  padding: 0 var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
.tzp__status--found {
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
}
.tzp__spin {
  animation: tzp-spin 0.9s linear infinite;
}
@keyframes tzp-spin {
  to {
    transform: rotate(360deg);
  }
}
.tzp__attrs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.tzp__attr {
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--surface);
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.tzp__quote {
  min-width: 0;
  border-radius: var(--radius-control);
  background: var(--surface);
}
.tzp__quote-toggle {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  width: 100%;
  min-height: 36px;
  padding: var(--space-2) var(--space-3);
  border: 0;
  border-radius: var(--radius-control);
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: 13px;
  line-height: 20px;
  text-align: left;
  cursor: pointer;
}
.tzp__quote-toggle:hover {
  color: var(--text);
}
.tzp__chev {
  flex: none;
  margin-top: 3px;
  transition: transform 150ms var(--ease-standard);
}
.tzp__chev--open {
  transform: rotate(90deg);
}
.tzp__quote-label {
  flex: none;
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.tzp__quote-line {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tzp__quote-body {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: 0 var(--space-3) var(--space-3) calc(var(--space-3) + 14px + var(--space-2));
}
.tzp__quote-text {
  margin: 0;
  color: var(--text);
  font-size: 13px;
  line-height: 20px;
  overflow-wrap: anywhere;
}
.tzp__quote-text p {
  margin: 0 0 var(--space-2);
}
.tzp__quote-text p:last-child {
  margin-bottom: 0;
}
.tzp__similar,
.tzp__empty {
  margin: 0;
  color: var(--text-2);
}
.tzp__unverified {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--status-pending-bg);
  color: var(--status-pending);
  overflow-wrap: anywhere;
}
.tzp__unverified svg {
  flex: none;
  margin-top: 1px;
}
.tzp__placeholder {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-3);
  box-sizing: border-box;
  min-height: 112px;
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tzp__ph-logo {
  width: 2rem;
  height: 2rem;
  border-radius: var(--radius-control);
}
.tzp__ph-lines {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.tzp__ph-line {
  height: 0.75rem;
  border-radius: var(--radius-sm);
}
.tzp__ph-line--short {
  width: 60%;
}
.tzp__error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tzp__error-icon {
  flex: none;
  color: var(--text-2);
}
.tzp__cards {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.tzp__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
}
.tzp__more {
  padding: var(--space-1) 0;
  border: 0;
  background: none;
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.tzp__open {
  margin-left: auto;
}
/* Узкая колонка: статус — над заголовком позиции. */
@container (max-width: 34rem) {
  .tzp__summary,
  .tzp__body {
    padding: var(--space-3);
  }
  .tzp__status {
    order: -1;
  }
  .tzp__quote-label {
    display: none;
  }
  .tzp__title {
    flex-basis: 100%;
    font-size: var(--fs-h2);
  }
}
@media (prefers-reduced-motion: reduce) {
  .tzp__spin {
    animation: none;
  }
  .tzp__chev {
    transition: none;
  }
}
</style>
