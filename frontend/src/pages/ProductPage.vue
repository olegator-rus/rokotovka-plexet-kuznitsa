<script setup lang="ts">
// Страница товара: всё, что каталог знает о позиции, — GET /api/products/{id}. Открывается по названию
// карточки выдачи (/product/:id?rid=…&pos=…); rid и pos идут только в события действий. Цена, скидка, акции и строка
// лицензии — те же, что на карточке выдачи; «На сайте» ведёт на карточку позиции в магазине (offer_url). Наличия,
// доставки и отзывов в данных нет — их и нет на странице.
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { computed, ref, watch } from 'vue'
import { ArrowLeft, Check, ExternalLink, ListChecks, ListPlus, Search } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { getProduct } from '@/api/product'
import type { EventType } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import PromoLine from '@/components/results/PromoLine.vue'
import VendorLogo from '@/components/common/VendorLogo.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { modelTitle } from '@/lib/companions'
import { buildEvent } from '@/lib/events'
import { plural } from '@/lib/format'
import { isPurchasable, priceLabel } from '@/lib/license'
import { reestrTone } from '@/lib/procurement'
import {
  backTarget,
  compatStateText,
  descriptionText,
  licenseRows,
  originText,
  parseProductRef,
  productHref,
  specText,
} from '@/lib/product'
import { perPlaceLabel } from '@/lib/offerNotes'
import {
  collapsedRows,
  titleTails,
  variantColumns,
  discountView,
  guideQuestions,
  specQuantity,
  variantTable,
} from '@/lib/productVariants'
import { useAppStore } from '@/stores/app'
import { useSpecStore } from '@/stores/spec'

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const spec = useSpecStore()
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

const discount = computed(() => (p.value ? discountView(p.value) : { old: '', badge: '' }))
/** Хост ссылки «На сайте» (store.softline.ru) — подпись честно говорит, куда ведёт ссылка. */
const siteHost = computed(() => {
  try {
    return p.value?.offer_url ? new URL(p.value.offer_url).host : ''
  } catch {
    return ''
  }
})
const inSpec = computed(() => (p.value ? spec.has(p.value.product_id) : false))

/** Спецификация открывается в шапке «Поиска»: возвращаемся к выдаче (или на /search) и открываем панель. */
async function openSpec() {
  if (back.value === 'back') router.back()
  else await router.push('/search')
  spec.open = true
}

/**
 * «В спецификацию» и «Запросить цену» — как в выдаче: позиция ложится в спецификацию (цена по запросу — с этим
 * режимом цены), событие — с номером поиска и позицией из адреса. У ступени цены за место количество — нижняя
 * граница ступени (100–149 устройств → 100): по цене за место меньше не купить.
 */
function act(type: 'spec' | 'price_request') {
  const item = p.value
  if (!item) return
  spec.add({
    product_id: item.product_id,
    title: item.title,
    vendor: item.vendor.name,
    category: item.category.name,
    license_line: item.license_line,
    price_rub: item.price_rub,
    price_mode: item.price_mode || (item.price_on_request ? 'quote_required' : 'fixed'),
    quantity: specQuantity(item.license),
    origin: 'search',
  })
  const ctx = { requestId: from.value.requestId, sessionId: app.sessionId, clientId: app.clientId }
  const ev =
    type === 'spec'
      ? buildEvent('click' as EventType, ctx, {
          productId: item.product_id,
          position: from.value.position,
          payload: { action: 'spec' },
        })
      : buildEvent('price_request' as EventType, ctx, {
          productId: item.product_id,
          position: from.value.position,
        })
  postEvent(ev).catch(() => {})
  toast.add({
    tone: 'ok',
    title: type === 'spec' ? 'Добавлено в спецификацию' : 'Запрос цены — в спецификации',
    detail: `${item.title}. Спецификация и запрос КП — на странице поиска.`,
    action: { label: 'Открыть', run: () => void openSpec() },
  })
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

/** Варианты: текущая позиция и остальные одной таблицей; колонки — только различающиеся условия. */
const table = computed(() =>
  p.value ? variantTable(p.value, p.value.variants, p.value.vendor.name) : null,
)
const showAllVariants = ref(false)
watch(id, () => (showAllVariants.value = false))
const shownRows = computed(() => {
  const t = table.value
  if (!t) return []
  return showAllVariants.value ? t.rows : collapsedRows(t.rows)
})
const shownColumns = computed(() => variantColumns(shownRows.value))
const shownTitles = computed(() => titleTails(shownRows.value))
const hiddenCount = computed(() =>
  table.value ? table.value.rows.length - shownRows.value.length : 0,
)

// Контур подбора (SPEC §1): вход в подбор со страницы семейства — запрос строки = семейство, вопрос-чип уходит первым ходом.
// Консультант (контур консультанта, по умолчанию) и помощник (контур помощника, ?consultant=0) оба принимают /search?q=…&guide=<вопрос>.
const guide = computed(() => (p.value ? guideQuestions(p.value, p.value.variants) : []))
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
            <VendorLogo :vendor="p.vendor.name" :image="p.image_url" class="pp__logo" />
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
          <p v-if="discount.badge" class="pp__deal">
            <s class="t-sm t-muted t-num" data-testid="product-old-price">
              <span class="sr-only">Старая цена </span>{{ discount.old }}
            </s>
            <span class="badge badge--tone pp__discount" data-testid="product-discount">
              {{ discount.badge }}
            </span>
          </p>
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
          <PromoLine :promos="p.promos" />
          <div class="pp__actions">
            <template v-if="inSpec">
              <p class="pp__in-spec t-sm" role="status" data-testid="product-in-spec">
                <Check :size="16" aria-hidden="true" /> В спецификации
              </p>
              <button type="button" class="btn btn--secondary pp__btn" @click="openSpec">
                Открыть спецификацию
              </button>
            </template>
            <button
              v-else-if="isPurchasable(p)"
              type="button"
              class="btn btn--primary pp__btn"
              @click="act('spec')"
            >
              <ListPlus :size="16" aria-hidden="true" /> В спецификацию
            </button>
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
              <ListChecks :size="16" aria-hidden="true" /> Спросить про лицензии
            </button>
            <ul class="pp__guide-chips" aria-label="Вопросы о лицензии">
              <li v-for="g in guide" :key="g.label">
                <button
                  type="button"
                  class="pp__guide-chip"
                  data-testid="product-guide-chip"
                  @click="openGuide(g.message)"
                >
                  {{ g.label }}
                </button>
              </li>
            </ul>
          </div>
          <p v-if="p.offer_url" class="pp__site-row t-sm">
            <a
              :href="p.offer_url"
              target="_blank"
              rel="noopener noreferrer"
              class="pp__site"
              data-testid="product-site"
            >
              {{ siteHost ? `Открыть на ${siteHost}` : 'На сайте магазина' }}
              <ExternalLink :size="14" aria-hidden="true" />
              <span class="sr-only"> (откроется в новой вкладке)</span>
            </a>
          </p>
          <p class="t-xs t-muted pp__demo" data-testid="product-demo-note">
            Цена и условия — из каталога магазина.
          </p>
        </aside>

        <div class="pp__main">
          <!-- Варианты: одной таблицей с текущим; колонки — только различающиеся условия -->
          <section
            v-if="p.variants.length && table"
            class="pp__section"
            aria-labelledby="pp-variants"
            data-testid="product-variants"
          >
            <h2 id="pp-variants" class="pp__h2">Варианты этого продукта</h2>
            <template v-if="table">
              <p class="t-sm t-muted">
                В каталоге {{ table.rows.length }}
                {{ plural(table.rows.length, ['вариант', 'варианта', 'вариантов']) }}, открытый
                отмечен.
                <template v-if="hiddenCount > 0">
                  Показаны ближайшие к нему: тот же вид лицензии и та же организация.
                </template>
              </p>
              <table class="vt t-sm">
                <caption class="sr-only">
                  Варианты:
                  {{
                    p.family_name || p.title
                  }}
                </caption>
                <thead>
                  <tr>
                    <th v-for="c in shownColumns" :key="c.key" scope="col">{{ c.label }}</th>
                    <th scope="col" class="vt__num">Цена</th>
                    <th scope="col"><span class="sr-only">Открыть</span></th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="r in shownRows"
                    :key="r.productId"
                    :class="{ vt__current: r.current }"
                    :aria-current="r.current ? 'true' : undefined"
                    data-testid="variant-row"
                  >
                    <td
                      v-for="c in shownColumns"
                      :key="c.key"
                      :data-label="c.label"
                      :class="{ vt__title: c.key === 'title' }"
                    >
                      <template v-if="c.key === 'title'">
                        <span :title="r.cells.title">{{ shownTitles.get(r.productId) }}</span>
                      </template>
                      <template v-else>{{ r.cells[c.key] }}</template>
                    </td>
                    <td data-label="Цена" class="vt__num t-num">
                      <span
                        >{{ r.price
                        }}<span v-if="r.perPlace" class="vt__unit t-xs">
                          {{ r.perPlace }}</span
                        ></span
                      >
                    </td>
                    <td class="vt__act">
                      <span v-if="r.current" class="vt__mark t-xs">Открыт</span>
                      <RouterLink
                        v-else
                        :to="productHref(r.productId)"
                        class="link t-sm"
                        :aria-label="`Открыть вариант: ${r.cells.title}`"
                      >
                        Открыть
                      </RouterLink>
                    </td>
                  </tr>
                </tbody>
              </table>
              <button
                v-if="hiddenCount > 0 || showAllVariants"
                type="button"
                class="btn btn--ghost btn--sm pp__more"
                :aria-expanded="showAllVariants"
                data-testid="variants-toggle"
                @click="showAllVariants = !showAllVariants"
              >
                {{
                  showAllVariants
                    ? 'Свернуть'
                    : `Показать все ${table.rows.length} ${plural(table.rows.length, ['вариант', 'варианта', 'вариантов'])}`
                }}
              </button>
            </template>
          </section>

          <section v-if="p.description" class="pp__section" aria-labelledby="pp-desc">
            <h2 id="pp-desc" class="pp__h2">Описание</h2>
            <p class="pp__desc">{{ descriptionText(p.description) }}</p>
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
                <RouterLink :to="productHref(c.product_id)" class="pp__strong pp__card-link t-sm">
                  {{ c.title }}
                </RouterLink>
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
                <RouterLink :to="productHref(a.product_id)" class="pp__strong pp__card-link t-sm">
                  {{ a.title }}
                </RouterLink>
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
  --logo-size: 4.5rem;
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

.vt {
  width: 100%;
  border-collapse: collapse;
}
.vt th {
  padding: var(--space-2) var(--space-3) var(--space-2) 0;
  color: var(--text-2);
  font-weight: var(--fw-medium);
  text-align: left;
  vertical-align: bottom;
}
.vt td {
  padding: var(--space-2) var(--space-3) var(--space-2) 0;
  border-top: var(--border-w) solid var(--border);
  vertical-align: top;
}
.vt .vt__num {
  text-align: right;
  white-space: nowrap;
}
.vt__unit {
  display: block;
  color: var(--text-2);
}
.vt__title {
  max-width: 22rem;
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.vt .vt__act {
  text-align: right;
  white-space: nowrap;
}
.vt__current td {
  background: var(--surface-2);
  font-weight: var(--fw-semibold);
}
.vt__current td:first-child {
  box-shadow: inset 3px 0 0 var(--text);
  padding-left: var(--space-3);
}
.vt__mark {
  color: var(--text-2);
}
.pp__more {
  align-self: flex-start;
}
.pp__deal {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.pp__discount {
  --tone: var(--status-accepted);
  --tone-bg: var(--status-accepted-bg);
}
.pp__in-spec {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  color: var(--text-2);
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
.pp__card-link {
  color: var(--text);
  text-decoration: underline;
  text-decoration-color: var(--border-control);
  text-underline-offset: 3px;
}
.pp__card-link:hover {
  text-decoration-color: currentColor;
}
.pp__strong {
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}

@media (max-width: 40rem) {
  .vt thead {
    display: none;
  }
  .vt,
  .vt tbody,
  .vt tr,
  .vt td {
    display: block;
  }
  .vt tr {
    padding: var(--space-2) var(--space-3);
    border-top: var(--border-w) solid var(--border);
  }
  .vt td {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    padding: 2px 0;
    border: 0;
    text-align: right;
  }
  .vt td[data-label]::before {
    content: attr(data-label);
    flex: none;
    color: var(--text-2);
    font-weight: var(--fw-regular);
    text-align: left;
  }
  .vt__current {
    background: var(--surface-2);
    box-shadow: inset 3px 0 0 var(--text);
  }
  .vt__current td,
  .vt__current td:first-child {
    background: transparent;
    box-shadow: none;
    padding-left: 0;
  }
  .vt .vt__num {
    white-space: normal;
  }
  .vt__unit {
    display: inline;
    margin-left: 0.25em;
  }
  /* Название — заголовок карточки варианта: слева, без подписи «Позиция». */
  .vt td.vt__title {
    display: block;
    max-width: none;
    color: var(--text);
    text-align: left;
  }
  .vt td.vt__title::before {
    content: none;
  }
  .vt td.vt__act {
    justify-content: flex-end;
  }
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
.pp__site-row {
  margin-top: var(--space-3) !important;
}
.pp__site {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--text);
  text-decoration: underline;
  text-underline-offset: 3px;
}
</style>
