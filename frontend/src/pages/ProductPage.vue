<script setup lang="ts">
// Страница товара: всё, что каталог знает о позиции, — GET /api/products/{id}. Открывается по названию
// карточки выдачи (/product/:id?rid=…&pos=…); rid и pos идут только в события действий. На стенде страница
// демонстрационная: в магазине здесь открывается его карточка товара. Фото, наличия, доставки и отзывов в данных нет —
// их и нет на странице.
import { computed, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ArrowLeft, ExternalLink, ListChecks, Search, ShoppingCart } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { getProduct } from '@/api/product'
import type { EventType } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import VendorLogo from '@/components/common/VendorLogo.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { modelTitle } from '@/lib/companions'
import { buildEvent } from '@/lib/events'
import { isPurchasable, priceLabel } from '@/lib/license'
import { reestrTone } from '@/lib/procurement'
import {
  backTarget,
  compatStateText,
  licenseRows,
  originText,
  parseProductRef,
  productHref,
  specText,
} from '@/lib/product'
import { perPlaceLabel } from '@/lib/offerNotes'
import { useAppStore } from '@/stores/app'

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const toast = useToast()

const id = computed(() => String(route.params.id ?? ''))
/** Откуда пришли: номер поиска и позиция карточки из адреса (null — прямая ссылка). */
const from = computed(() => parseProductRef(route.query))
const product = useResource((opts) => getProduct(id.value, opts))
watch(id, () => void product.load(), { immediate: true })

const p = computed(() => product.data.value)
const notFound = computed(
  () => product.status.value === 'error' && product.error.value?.status === 404,
)
const isSoftware = computed(() => p.value !== null && p.value.kind !== 'hardware')
// у позиции со ступенью количества цена — за одно место («3 070,80 ₽ за узел»), как на карточке выдачи.
const perPlace = computed(() => {
  const v = p.value
  if (
    !v ||
    !isPurchasable(v) ||
    !(typeof v.license.tier_min === 'number' && v.license.tier_min > 0)
  )
    return ''
  return perPlaceLabel(v.license.metric)
})
const license = computed(() =>
  p.value && isSoftware.value ? licenseRows(p.value.license, p.value.license_line) : [],
)
const companions = computed(() =>
  p.value ? [...p.value.companions.required, ...p.value.companions.items] : [],
)

/** «← К результатам»: пришли из выдачи — шаг назад (адрес выдачи на месте), иначе ссылка на /search. */
const back = computed(() => {
  void route.fullPath // history.state меняется с каждым переходом
  return backTarget(window.history.state?.back)
})
function goBack() {
  router.back()
}

const ACTION_TOAST: Record<'add_to_cart' | 'order' | 'price_request', string> = {
  add_to_cart: 'Добавлено в корзину',
  order: 'Заказ записан',
  price_request: 'Запрос цены записан',
}

/** Действие пишет то же событие, что в выдаче, с номером поиска и позицией из адреса. */
function act(type: 'add_to_cart' | 'order' | 'price_request') {
  const item = p.value
  if (!item) return
  const ctx = {
    requestId: from.value.requestId,
    sessionId: app.sessionId,
    clientId: app.clientId,
  }
  postEvent(
    buildEvent(type as EventType, ctx, {
      productId: item.product_id,
      position: from.value.position,
    }),
  )
    .then(() =>
      toast.add({
        tone: 'ok',
        title: ACTION_TOAST[type],
        detail: `${item.title}. Демо-стенд: действие записано событием для аналитики.`,
      }),
    )
    .catch((e: unknown) =>
      toast.add({
        tone: 'error',
        title: 'Действие не записано',
        detail: isApiError(e) ? e.message : MSG_UNKNOWN,
      }),
    )
}

/**
 * Одна таблица характеристик: условия лицензии, затем числовые характеристики и атрибуты. Строка с подписью,
 * которая уже есть выше (например, «Редакция» и в лицензии, и в атрибутах), второй раз не выводится.
 */
const specRows = computed(() => {
  const item = p.value
  if (!item) return []
  const rows: { label: string; value: string; num?: boolean }[] = [...license.value]
  for (const s of item.specs)
    rows.push({ label: s.label, value: specText(s.value, s.unit), num: true })
  for (const a of item.attributes) rows.push({ label: a.label, value: a.value })
  const seen = new Set<string>()
  return rows.filter((r) => (seen.has(r.label) ? false : (seen.add(r.label), true)))
})

// Контур подбора (SPEC §1): вход в подбор со страницы семейства — запрос строки = семейство, вопрос-чип уходит первым ходом.
const GUIDE_QUESTIONS = [
  { label: 'Чем отличаются редакции?', message: 'Чем отличаются редакции?' },
  { label: 'Сколько стоит на N мест?', message: 'Сколько будет стоить на нужное число мест?' },
] as const
function openGuide(message?: string) {
  const item = p.value
  if (!item) return
  const q = item.family_name || item.title
  void router.push({ path: '/search', query: { q, guide: message ?? '1' } })
}

/** priceLabel для строк без price_on_request (компаньоны): нет цены — «Цена по запросу». */
function companionPrice(c: { price_rub: number | null; price_mode: string }) {
  return priceLabel({ ...c, price_on_request: c.price_rub === null })
}
</script>

<template>
  <article class="page product-page" :aria-busy="product.status.value === 'loading'">
    <nav class="product-page__nav" aria-label="Навигация">
      <button
        v-if="back === 'back'"
        type="button"
        class="btn btn--ghost btn--sm"
        data-testid="product-back"
        @click="goBack"
      >
        <ArrowLeft :size="16" aria-hidden="true" /> К результатам
      </button>
      <RouterLink v-else to="/search" class="btn btn--ghost btn--sm" data-testid="product-back">
        <ArrowLeft :size="16" aria-hidden="true" /> К результатам
      </RouterLink>
      <ol v-if="p" class="product-page__crumbs t-sm t-muted" aria-label="Хлебные крошки">
        <li><RouterLink to="/search" class="link">Поиск</RouterLink></li>
        <li v-if="p.category.name">{{ p.category.name }}</li>
        <li aria-current="page">{{ p.title }}</li>
      </ol>
    </nav>

    <div
      v-if="product.status.value === 'loading' || product.status.value === 'idle'"
      class="product-page__skeleton"
      aria-hidden="true"
    >
      <p class="sr-only" role="status">Загружаем товар…</p>
      <div class="panel product-page__block">
        <span class="skeleton" style="width: 30%" />
        <span class="skeleton product-page__skeleton-title" style="width: 70%" />
        <span class="skeleton" style="width: 40%" />
      </div>
      <div v-for="i in 2" :key="i" class="panel product-page__block">
        <span class="skeleton" style="width: 20%" />
        <span class="skeleton" style="width: 90%" />
        <span class="skeleton" style="width: 75%" />
      </div>
    </div>

    <section
      v-else-if="notFound"
      class="panel product-page__block product-page__missing"
      role="alert"
      data-testid="product-not-found"
    >
      <h1 class="t-h1">Товар не найден в каталоге</h1>
      <p class="t-muted">Ссылка устарела или в ней ошибка. Найдите товар поиском.</p>
      <div>
        <RouterLink to="/search" class="btn btn--secondary">
          <Search :size="16" aria-hidden="true" /> К поиску
        </RouterLink>
      </div>
    </section>

    <BlockError
      v-else-if="product.status.value === 'error' && product.error.value"
      :error="product.error.value"
      title="Страница товара не загрузилась"
      @retry="product.load()"
    />

    <template v-else-if="p">
      <div class="pp">
        <!-- Шапка товара: производитель, название, главные факты -->
        <header class="pp__head">
          <div class="pp__brand">
            <VendorLogo :vendor="p.vendor.name" class="pp__logo" />
            <p class="t-sm t-muted">
              <span class="pp__vendor">{{ p.vendor.name }}</span>
              <template v-if="p.category.name"> · {{ p.category.name }}</template>
            </p>
          </div>
          <h1 class="pp__title">{{ p.title }}</h1>
          <ul class="pp__facts" aria-label="Главное о товаре">
            <li v-if="p.license_line" class="pp__fact pp__fact--strong">{{ p.license_line }}</li>
            <li
              v-if="isSoftware && p.reestr"
              :class="['pp__fact', { 'pp__fact--ok': reestrTone(p.reestr) === 'ok' }]"
              data-testid="reestr"
            >
              {{ p.reestr.label }}
            </li>
            <li v-if="isSoftware && originText(p.origin)" class="pp__fact">
              {{ originText(p.origin) }}
            </li>
          </ul>
          <p class="t-sm t-muted pp__codes">
            <template v-if="p.kind === 'hardware'">
              <span v-if="p.mpn">
                Парт-номер <span class="t-mono pp__code">{{ p.mpn }}</span>
              </span>
              <span v-if="p.sku">
                Артикул <span class="t-mono pp__code">{{ p.sku }}</span>
              </span>
            </template>
            <template v-else>
              <span v-if="p.sku">
                Артикул <span class="t-mono pp__code">{{ p.sku }}</span>
              </span>
              <span v-else>Артикул не указан</span>
            </template>
          </p>
        </header>

        <!-- Цена и действия: справа и прилипает при прокрутке на широком экране -->
        <aside class="pp__buy panel" aria-label="Цена и действия">
          <p
            :class="['pp__price t-num', { 'pp__price--request': !isPurchasable(p) }]"
            data-testid="product-price"
          >
            {{ priceLabel(p) }}
          </p>
          <p v-if="perPlace" class="t-sm pp__price-for" data-testid="product-per-place">
            {{ perPlace }}
          </p>
          <p v-if="p.license_line" class="t-sm t-muted pp__price-for">{{ p.license_line }}</p>
          <div class="pp__actions">
            <template v-if="isPurchasable(p)">
              <button type="button" class="btn btn--primary pp__btn" @click="act('add_to_cart')">
                <ShoppingCart :size="16" aria-hidden="true" /> В корзину
              </button>
              <button type="button" class="btn btn--secondary pp__btn" @click="act('order')">
                Заказать
              </button>
            </template>
            <button
              v-else
              type="button"
              class="btn btn--primary pp__btn"
              @click="act('price_request')"
            >
              Запросить цену
            </button>
          </div>
          <div v-if="isSoftware" class="pp__guide" data-testid="product-guide">
            <button
              type="button"
              class="btn btn--secondary pp__btn"
              data-testid="product-guide-start"
              @click="openGuide()"
            >
              <ListChecks :size="16" aria-hidden="true" /> Помочь выбрать лицензию
            </button>
            <ul class="pp__guide-chips" aria-label="Вопросы о лицензии">
              <li v-for="g in GUIDE_QUESTIONS" :key="g.label">
                <button type="button" class="pp__guide-chip" @click="openGuide(g.message)">
                  {{ g.label }}
                </button>
              </li>
            </ul>
          </div>
          <a
            v-if="p.offer_url"
            :href="p.offer_url"
            target="_blank"
            rel="noopener"
            class="pp__site t-sm"
            data-testid="product-site"
          >
            На сайте <ExternalLink :size="14" aria-hidden="true" />
          </a>
          <p class="t-xs t-muted pp__demo" data-testid="product-demo-note">
            Демо-стенд. В магазине здесь открывается карточка товара магазина — поиск передаёт ей
            переход и действия событиями.
          </p>
        </aside>

        <div class="pp__main">
          <!-- Варианты: текущий выделен, остальные — ссылки на свои страницы -->
          <section class="pp__section" aria-labelledby="pp-variants">
            <h2 id="pp-variants" class="pp__h2">Другие варианты этого продукта</h2>
            <p v-if="!p.variants.length" class="t-sm t-muted">
              Других вариантов этого продукта в каталоге нет.
            </p>
            <ul v-else class="pp__variants">
              <li class="pp__variant pp__variant--current" aria-current="true">
                <span class="pp__variant-name">{{ p.license_line || p.title }}</span>
                <span class="pp__variant-price t-num">{{ priceLabel(p) }}</span>
                <span class="pp__variant-note t-xs">Сейчас открыт</span>
              </li>
              <li v-for="v in p.variants" :key="v.product_id">
                <RouterLink
                  :to="productHref(v.product_id)"
                  class="pp__variant"
                  :aria-label="`Открыть: ${v.title}`"
                >
                  <span class="pp__variant-name">{{ v.license_line || v.title }}</span>
                  <span class="pp__variant-price t-num">{{ priceLabel(v) }}</span>
                </RouterLink>
              </li>
            </ul>
          </section>

          <section v-if="p.description" class="pp__section" aria-labelledby="pp-desc">
            <h2 id="pp-desc" class="pp__h2">Описание</h2>
            <p class="pp__desc">{{ p.description }}</p>
          </section>

          <section v-if="specRows.length" class="pp__section" aria-labelledby="pp-attrs">
            <h2 id="pp-attrs" class="pp__h2">Характеристики</h2>
            <dl class="pp__dl t-sm">
              <div v-for="r in specRows" :key="r.label">
                <dt>{{ r.label }}</dt>
                <dd :class="{ 't-num': r.num }">{{ r.value }}</dd>
              </div>
            </dl>
          </section>

          <section v-if="p.compatibility.length" class="pp__section" aria-labelledby="pp-compat">
            <h2 id="pp-compat" class="pp__h2">Совместимость</h2>
            <ul class="pp__list">
              <li
                v-for="c in p.compatibility"
                :key="`${c.target_model}:${c.state}`"
                class="pp__item"
              >
                <span class="t-sm">
                  <strong class="pp__strong">{{ modelTitle(c.target_model) }}</strong>
                  — {{ compatStateText(c.state) }}
                </span>
                <span v-if="c.condition" class="t-xs t-muted">{{ c.condition }}</span>
                <a
                  v-if="c.source_url"
                  :href="c.source_url"
                  class="link t-xs pp__source"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Источник <ExternalLink :size="12" aria-hidden="true" />
                  <span class="sr-only">(откроется в новой вкладке)</span>
                </a>
              </li>
            </ul>
          </section>

          <section v-if="companions.length" class="pp__section" aria-labelledby="pp-companions">
            <h2 id="pp-companions" class="pp__h2">С этим покупают</h2>
            <ul class="pp__cards">
              <li v-for="c in companions" :key="`${c.class}:${c.product_id}`" class="pp__card">
                <span class="t-xs t-muted">{{ c.vendor }}</span>
                <strong class="pp__strong t-sm">{{ c.title }}</strong>
                <span v-if="c.license_line" class="t-xs t-muted">{{ c.license_line }}</span>
                <span v-if="c.reason" class="t-xs pp__reason">{{ c.reason }}</span>
                <span class="pp__card-price t-num">{{ companionPrice(c) }}</span>
              </li>
            </ul>
          </section>

          <section v-if="p.analogs.length" class="pp__section" aria-labelledby="pp-analogs">
            <h2 id="pp-analogs" class="pp__h2">Аналоги</h2>
            <ul class="pp__cards">
              <li v-for="a in p.analogs" :key="a.family_id" class="pp__card">
                <span class="t-xs t-muted">{{ a.vendor }}</span>
                <strong class="pp__strong t-sm">{{ a.title }}</strong>
                <span v-if="a.license_line" class="t-xs t-muted">{{ a.license_line }}</span>
                <span v-if="a.note" class="t-xs pp__reason">{{ a.note }}</span>
                <span class="pp__card-price t-num">{{ priceLabel(a) }}</span>
              </li>
            </ul>
          </section>
        </div>
      </div>
    </template>
  </article>
</template>

<style scoped>
.product-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.product-page__nav {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-4);
}
.product-page__crumbs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}
.product-page__crumbs li {
  min-width: 0;
  overflow-wrap: anywhere;
}
.product-page__crumbs li + li::before {
  content: '›';
  margin-right: var(--space-1);
  color: var(--text-3);
}
.product-page__block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.product-page__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.product-page__skeleton-title {
  height: var(--lh-h1);
}
.product-page__missing {
  align-items: flex-start;
}
/* Страница товара: шапка и разделы слева, цена справа (прилипает); на узком экране — столбиком. */
.pp {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-5);
}
.pp__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.pp__head p {
  margin: 0;
}
.pp__brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.pp__logo {
  --logo-size: 2.25rem;
}
.pp__vendor {
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.pp__title {
  margin: 0;
  font-size: clamp(1.375rem, 3vw, 1.875rem);
  line-height: 1.2;
  font-weight: var(--fw-semibold);
  letter-spacing: -0.01em;
  overflow-wrap: anywhere;
}
.pp__facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pp__fact {
  padding: 0.25rem 0.625rem;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.pp__fact--strong {
  border-color: transparent;
  background: var(--surface-2);
  color: var(--text);
  font-weight: var(--fw-medium);
}
.pp__fact--ok {
  border-color: color-mix(in srgb, var(--delta-up) 30%, transparent);
  background: color-mix(in srgb, var(--delta-up) 8%, transparent);
  color: var(--delta-up);
}
.pp__codes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  overflow-wrap: anywhere;
}
.pp__code {
  color: var(--text);
}

.pp__buy {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
  min-width: 0;
}
.pp__buy p {
  margin: 0;
}
.pp__price {
  font-size: 1.875rem;
  line-height: 1.15;
  font-weight: var(--fw-bold);
  letter-spacing: -0.01em;
}
.pp__price--request {
  color: var(--text-2);
  font-size: var(--fs-h1);
  font-weight: var(--fw-semibold);
}
.pp__actions {
  display: grid;
  gap: var(--space-2);
  margin-top: var(--space-2);
}
.pp__btn {
  justify-content: center;
  width: 100%;
}
.pp__guide {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-2);
  padding-top: var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.pp__guide-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pp__guide-chip {
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: var(--border-w) dashed var(--border-control);
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  text-align: left;
  cursor: pointer;
}
.pp__guide-chip:hover {
  border-style: solid;
  background: var(--surface-2);
}
.pp__demo {
  margin-top: var(--space-2) !important;
  padding-top: var(--space-3);
  border-top: var(--border-w) solid var(--border);
}

.pp__main {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
/* Разделы без рамок: заголовок и линия сверху, чтобы страница не превращалась в стопку коробок */
.pp__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5) 0;
  border-top: var(--border-w) solid var(--border);
}
.pp__section:first-child {
  padding-top: 0;
  border-top: 0;
}
.pp__section p {
  margin: 0;
}
.pp__h2 {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.pp__desc {
  max-width: 70ch;
  color: var(--text-2);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.pp__variants {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 13rem), 1fr));
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pp__variant {
  display: flex;
  height: 100%;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  color: var(--text);
  text-decoration: none;
  transition:
    border-color 0.15s,
    background-color 0.15s;
}
a.pp__variant:hover {
  border-color: var(--border-control);
  background: var(--surface-2);
}
.pp__variant--current {
  border-color: var(--text);
  box-shadow: inset 0 0 0 1px var(--text);
}
.pp__variant-name {
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}
.pp__variant-price {
  font-weight: var(--fw-semibold);
}
.pp__variant-note {
  color: var(--text-3);
}

.pp__dl {
  display: grid;
  margin: 0;
}
.pp__dl > div {
  display: grid;
  grid-template-columns: minmax(0, 14rem) minmax(0, 1fr);
  gap: var(--space-3);
  padding: var(--space-2) 0;
  border-bottom: var(--border-w) solid var(--border);
}
.pp__dl > div:last-child {
  border-bottom: 0;
}
.pp__dl dt {
  color: var(--text-3);
  overflow-wrap: anywhere;
}
.pp__dl dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.pp__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pp__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.pp__source {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.pp__cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 16rem), 1fr));
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pp__card {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-3) var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
}
.pp__reason {
  margin-top: var(--space-1);
  color: var(--text-2);
}
.pp__card-price {
  margin-top: auto;
  padding-top: var(--space-2);
  font-weight: var(--fw-semibold);
}
.pp__strong {
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}

@media (max-width: 30rem) {
  .pp__dl > div {
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }
  .pp__buy {
    padding: var(--space-4);
  }
}

@media (min-width: 64rem) {
  .pp {
    grid-template-columns: minmax(0, 1fr) 20rem;
    column-gap: var(--space-8);
    align-items: start;
  }
  .pp__head,
  .pp__main {
    grid-column: 1;
  }
  .pp__buy {
    grid-column: 2;
    grid-row: 1 / span 2;
    position: sticky;
    top: calc(var(--header-actual, var(--header-h)) + var(--space-4));
  }
}
.pp__site {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  align-self: flex-start;
  margin-top: var(--space-3);
  color: var(--text);
  text-decoration: underline;
  text-underline-offset: 3px;
}
</style>
