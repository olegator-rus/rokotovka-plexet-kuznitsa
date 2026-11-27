<script setup lang="ts">
// Выдача по ТЗ: /search?tz=<хеш> (TZ-UI.md §4–§6, §10). Заголовок с числом позиций, слева вместо фильтров список
// позиций (статус, отметка добавления, счётчик, подсветка текущей), на узком экране — закреплённый селектор. Позиции —
// в порядке документа. Фильтров, сортировки, уточнений и группировки нет: всё это — в полном поиске по позиции.
// Раскрытые цитаты и варианты и место прокрутки хранятся на хеш в sessionStorage и восстанавливаются при возврате.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from 'vue'
import {
  SelectContent,
  SelectItem,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectViewport,
} from 'reka-ui'
import { Check, ChevronDown, CircleAlert, FileText, RotateCcw } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import type { SearchItemBase } from '@/api/types'
import type { TzPosition } from '@/api/typesTz'
import NotFoundBlock from '@/components/results/NotFoundBlock.vue'
import ManagerDialog from '@/components/search/ManagerDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { DESKTOP_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { useTzResults } from '@/composables/useTzResults'
import { buildEvent } from '@/lib/events'
import { specPurchase } from '@/lib/procurement'
import { GUEST_CLIENT_ID } from '@/lib/searchState'
import { lineFromTz, tzPositionsInSpec } from '@/lib/spec'
import {
  TZ_TEXT,
  limitNotice,
  mobileNavLabel,
  navProgress,
  parseViewState,
  positionLabel,
  positionPurchase,
  positionQuantity,
  remainingPositions,
  resultsTitle,
  viewKey,
} from '@/lib/tz'
import { useAppStore } from '@/stores/app'
import { useSpecStore } from '@/stores/spec'
import { useTzStore } from '@/stores/tz'
import TzPositionSection from './TzPositionSection.vue'

const props = defineProps<{ hash: string | null }>()
const emit = defineEmits<{ regular: []; file: [name: string | null] }>()

const app = useAppStore()
const spec = useSpecStore()
const tz = useTzStore()
const toast = useToast()
const desktop = useMediaQuery(DESKTOP_QUERY)
const { docState, doc, rows, settled, found, nothingFound, fromCache, retry, reload } =
  useTzResults(toRef(props, 'hash'))

const total = computed(() => rows.value.length)
const remaining = computed(() => (doc.value ? remainingPositions(doc.value) : 0))
const title = computed(() =>
  doc.value ? resultsTitle(doc.value.file_name, total.value, found.value) : '',
)
const addedSet = computed(() =>
  props.hash ? tzPositionsInSpec(spec.lines, props.hash) : new Set<number>(),
)
const addedCount = computed(
  () => rows.value.filter((r) => addedSet.value.has(r.position.number)).length,
)
const inSpec = (id: string) => spec.has(id)

watch(
  () => doc.value?.file_name ?? null,
  (name) => emit('file', name),
  { immediate: true },
)

// ── Раскрытые блоки и место прокрутки ──
const quotes = ref<Set<string>>(new Set())
const variants = ref<Set<string>>(new Set())
let savedScroll = 0
let saving = true

function storage(): Storage | null {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function readView() {
  if (!props.hash) return
  const v = parseViewState(storage()?.getItem(viewKey(props.hash)) ?? null)
  quotes.value = new Set(v?.quotes ?? [])
  variants.value = new Set(v?.variants ?? [])
  savedScroll = v?.scrollY ?? 0
}

function writeView() {
  if (!props.hash || !saving || docState.value !== 'ready') return
  try {
    storage()?.setItem(
      viewKey(props.hash),
      JSON.stringify({
        scrollY: Math.round(window.scrollY),
        quotes: [...quotes.value],
        variants: [...variants.value],
      }),
    )
  } catch {
    // Хранилище недоступно — возврат откроет страницу в исходном виде.
  }
}

function toggled(set: Set<string>, id: string): Set<string> {
  const next = new Set(set)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}
function toggleQuote(id: string) {
  quotes.value = toggled(quotes.value, id)
  writeView()
}
function toggleVariants(id: string) {
  variants.value = toggled(variants.value, id)
  writeView()
}

/** Уход в полный поиск по позиции: место запоминается до того, как страница начнёт меняться. */
function onLeave() {
  writeView()
  saving = false
}

let restored = false
let mounted = false
let restorePending = false
async function restoreScroll() {
  if (restored) return
  if (!mounted) {
    restorePending = true
    return
  }
  restored = true
  if (savedScroll <= 0) return
  await nextTick()
  window.scrollTo({ top: savedScroll })
}
readView()

// ── Текущая позиция ──
const current = ref(1)
const navList = ref<HTMLElement | null>(null)
const stickyBar = ref<HTMLElement | null>(null)
let raf = 0

function stickyOffset(): number {
  const header = document.querySelector<HTMLElement>('.app-header, header')
  const h = header?.getBoundingClientRect().bottom ?? 0
  const bar = !desktop.value ? (stickyBar.value?.getBoundingClientRect().height ?? 0) : 0
  return Math.max(0, h) + bar
}

/** Позиция, выбранная в списке: у конца страницы её заголовок не дойдёт до верха, но подсвечена должна быть она. */
let target: number | null = null
let targetAt = 0

function computeCurrent() {
  raf = 0
  const line = stickyOffset() + 24
  let n = rows.value[0]?.position.number ?? 1
  for (const r of rows.value) {
    const el = document.getElementById(`tz-pos-${r.position.number}`)
    if (!el) continue
    if (el.getBoundingClientRect().top <= line) n = r.position.number
    else break
  }
  if (target !== null && performance.now() - targetAt < 1500) {
    n = target
  } else if (target !== null) {
    const top = document.getElementById(`tz-pos-${target}`)?.getBoundingClientRect().top
    const atBottom =
      window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 2
    if (top !== undefined && top >= 0 && top < window.innerHeight && (atBottom || n === target))
      n = target
    else if (top === undefined || top < 0 || top >= window.innerHeight) target = null
  }
  current.value = n
}

function onScroll() {
  if (!raf) raf = requestAnimationFrame(computeCurrent)
  if (saving) writeView()
}

// Текущая позиция удерживается в видимой части собственной прокрутки колонки (50 позиций).
watch(current, async (n) => {
  await nextTick()
  const list = navList.value
  const item = list?.querySelector<HTMLElement>(`[data-nav="${n}"]`)
  if (!list || !item) return
  const top = item.offsetTop - list.offsetTop
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + item.offsetHeight > list.scrollTop + list.clientHeight)
    list.scrollTop = top + item.offsetHeight - list.clientHeight
})

function goTo(n: number) {
  const el = document.getElementById(`tz-pos-${n}`)
  if (!el) return
  const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  el.style.setProperty(
    '--tz-sticky',
    `${!desktop.value ? (stickyBar.value?.offsetHeight ?? 0) : 0}px`,
  )
  el.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'start' })
  target = n
  targetAt = performance.now()
  current.value = n
  el.querySelector<HTMLElement>('.tzp__title-link')?.focus({ preventScroll: true })
}

const mobileValue = computed({
  get: () => String(current.value),
  set: (v: string) => goTo(Number(v)),
})
const currentRow = computed(() => rows.value.find((r) => r.position.number === current.value))

onMounted(() => {
  mounted = true
  window.addEventListener('scroll', onScroll, { passive: true })
  if (restorePending) {
    restorePending = false
    void restoreScroll()
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  if (raf) cancelAnimationFrame(raf)
  if (props.hash) tz.finishSearching(props.hash)
})

watch(
  () => props.hash,
  () => {
    restored = false
    saving = true
    readView()
  },
)

// Из памяти вкладки — восстанавливаем сразу; после свежей загрузки — когда поиск по позициям закончился, если
// человек ещё не прокрутил страницу сам.
watch(
  [docState, settled, fromCache],
  ([s, done, cached]) => {
    if (s !== 'ready') return
    if (cached) void restoreScroll()
    else if (done && window.scrollY < 8) void restoreScroll()
    else if (done) restored = true
    void nextTick(computeCurrent)
  },
  { immediate: true },
)

// ── Спецификация ──
function addFrom(p: TzPosition, target: SearchItemBase, priceRequest: boolean) {
  if (!props.hash || !doc.value) return
  const purchase = positionPurchase(p)
  spec.add(
    lineFromTz(target, positionQuantity(target, p), purchase ? specPurchase(purchase) : null, {
      hash: props.hash,
      file_name: doc.value.file_name,
      number: p.number,
    }),
  )
  toast.add({
    tone: 'ok',
    title: priceRequest ? 'Добавлено в спецификацию — цена по запросу' : TZ_TEXT.added,
    detail: `${target.title} · по ТЗ, п. ${p.number}`,
    action: { label: 'Открыть', run: () => (spec.open = true) },
  })
}

// ── Менеджер ──
const managerOpen = ref(false)
const managerFor = ref<TzPosition | null>(null)
const managerQuery = computed(() =>
  managerFor.value ? managerFor.value.title : doc.value ? `ТЗ «${doc.value.file_name}»` : '',
)

function openManager(p: TzPosition | null) {
  managerFor.value = p
  managerOpen.value = true
}

function sendManager() {
  const p = managerFor.value
  return postEvent(
    buildEvent(
      'manager_request',
      {
        requestId: rows.value.find((r) => r.position === p)?.requestId ?? null,
        sessionId: app.sessionId,
        clientId: app.clientId || GUEST_CLIENT_ID,
      },
      // В журнал — запрос позиции и номер пункта, без текста документа (TZ-CORE.md §7).
      { payload: { source: 'tz', query: p?.search_query ?? '', tz_position: p?.number ?? null } },
    ),
  )
}

function onRelatedSearch() {
  emit('regular')
}
</script>

<template>
  <div class="tzr" data-testid="tz-results">
    <section
      v-if="docState === 'unavailable'"
      class="tzr__unavailable"
      data-testid="tz-unavailable"
      aria-labelledby="tz-unavailable-title"
    >
      <FileText :size="32" aria-hidden="true" class="tzr__unavailable-icon" />
      <h2 id="tz-unavailable-title" class="tzr__unavailable-title">{{ TZ_TEXT.expiredTitle }}</h2>
      <p class="t-muted">{{ TZ_TEXT.expiredDescription }}</p>
      <p class="t-muted">{{ TZ_TEXT.expiredHint }}</p>
      <button
        type="button"
        class="btn btn--primary"
        data-testid="tz-upload-again"
        @click="tz.openPicker()"
      >
        {{ TZ_TEXT.expiredAction }}
      </button>
      <button type="button" class="link t-sm tzr__link-btn" @click="emit('regular')">
        {{ TZ_TEXT.regularSearch }}
      </button>
    </section>

    <div v-else-if="docState === 'error'" class="tzr__error panel" role="alert">
      <CircleAlert :size="18" aria-hidden="true" class="tzr__error-icon" />
      <p>Не удалось обработать ТЗ. Попробуйте ещё раз.</p>
      <button type="button" class="btn btn--secondary btn--sm" @click="reload">
        <RotateCcw :size="14" aria-hidden="true" /> {{ TZ_TEXT.retry }}
      </button>
    </div>

    <div v-else-if="docState === 'loading'" class="tzr__loading" aria-busy="true">
      <span class="sr-only" role="status">Загружаем позиции ТЗ</span>
      <span class="skeleton tzr__sk-title" aria-hidden="true" />
      <span class="skeleton tzr__sk-block" aria-hidden="true" />
      <span class="skeleton tzr__sk-block" aria-hidden="true" />
    </div>

    <template v-else-if="doc">
      <div role="status">
        <h2 class="tzr__title" data-testid="tz-title">{{ title }}</h2>
      </div>

      <p v-if="remaining > 0" class="tzr__notice t-sm" data-testid="tz-limit">
        <CircleAlert :size="16" aria-hidden="true" />
        <span>{{ limitNotice(remaining) }}</span>
        <button type="button" class="link t-sm tzr__link-btn" @click="tz.openPicker()">
          {{ TZ_TEXT.remainderAction }}
        </button>
      </p>
      <p v-if="nothingFound" class="tzr__notice t-sm" data-testid="tz-no-matches">
        <CircleAlert :size="16" aria-hidden="true" />
        <span
          ><strong>{{ TZ_TEXT.noMatches }}.</strong> {{ TZ_TEXT.noMatchesHint }}</span
        >
      </p>

      <div class="tzr__cols">
        <nav v-if="desktop" class="tzr__nav" aria-labelledby="tz-nav-title" data-testid="tz-nav">
          <div class="tzr__nav-head">
            <h3 id="tz-nav-title" class="tzr__nav-title">{{ TZ_TEXT.navTitle }}</h3>
            <p class="t-xs t-muted t-num" data-testid="tz-progress">
              {{ navProgress(addedCount, total) }}
            </p>
          </div>
          <ol ref="navList" class="tzr__nav-list">
            <li v-for="r in rows" :key="r.position.id">
              <button
                type="button"
                :class="['tzr__nav-item', { 'is-current': current === r.position.number }]"
                :aria-current="current === r.position.number ? 'location' : undefined"
                :data-nav="r.position.number"
                data-testid="tz-nav-item"
                @click="goTo(r.position.number)"
              >
                <span class="tzr__nav-num t-num">{{ r.position.number }}</span>
                <span class="tzr__nav-text">
                  <span class="tzr__nav-name" :title="r.position.title">{{
                    r.position.short_title || r.position.title
                  }}</span>
                  <span
                    :class="[
                      'tzr__nav-status t-xs',
                      { 'tzr__nav-status--found': r.state === 'ready' && r.status === 'found' },
                    ]"
                    >{{ positionLabel(r.state, r.status) }}</span
                  >
                </span>
                <span
                  v-if="addedSet.has(r.position.number)"
                  class="tzr__nav-mark"
                  data-testid="tz-nav-added"
                  title="Из позиции добавлено в спецификацию"
                >
                  <Check :size="16" aria-hidden="true" />
                  <span class="sr-only">в спецификации</span>
                </span>
              </button>
            </li>
          </ol>
        </nav>

        <div v-else ref="stickyBar" class="tzr__mobile" data-testid="tz-mobile-nav">
          <SelectRoot v-model="mobileValue">
            <SelectTrigger class="tzr__select" :aria-label="TZ_TEXT.navTitle">
              <span class="tzr__select-text">
                <span class="tzr__select-head t-xs">
                  {{ mobileNavLabel(current, total) }}
                  <span class="t-muted t-num">· {{ navProgress(addedCount, total) }}</span>
                </span>
                <span class="tzr__select-name">{{
                  currentRow?.position.short_title || currentRow?.position.title
                }}</span>
              </span>
              <ChevronDown :size="18" aria-hidden="true" class="tzr__select-chev" />
            </SelectTrigger>
            <SelectPortal>
              <SelectContent position="popper" :side-offset="4" class="tzr__select-content">
                <SelectViewport class="tzr__select-viewport">
                  <SelectItem
                    v-for="r in rows"
                    :key="r.position.id"
                    :value="String(r.position.number)"
                    :text-value="r.position.short_title || r.position.title"
                    class="tzr__select-item"
                  >
                    <span class="tzr__nav-num t-num">{{ r.position.number }}</span>
                    <span class="tzr__nav-text">
                      <SelectItemText>{{
                        r.position.short_title || r.position.title
                      }}</SelectItemText>
                      <span class="tzr__nav-status t-xs">{{
                        positionLabel(r.state, r.status)
                      }}</span>
                    </span>
                    <span v-if="addedSet.has(r.position.number)" class="tzr__nav-mark">
                      <Check :size="16" aria-hidden="true" />
                      <span class="sr-only">в спецификации</span>
                    </span>
                  </SelectItem>
                </SelectViewport>
              </SelectContent>
            </SelectPortal>
          </SelectRoot>
        </div>

        <div class="tzr__positions">
          <TzPositionSection
            v-for="(r, i) in rows"
            :key="r.position.id"
            :row="r"
            :hash="hash ?? ''"
            :added="addedSet.has(r.position.number)"
            :quote-open="quotes.has(r.position.id)"
            :variants-open="variants.has(r.position.id)"
            :in-spec="inSpec"
            @toggle-quote="toggleQuote(r.position.id)"
            @toggle-variants="toggleVariants(r.position.id)"
            @retry="retry(i)"
            @spec="(t) => addFrom(r.position, t, false)"
            @price-request="(t) => addFrom(r.position, t, true)"
            @manager="openManager(r.position)"
            @open="onLeave"
          />
          <NotFoundBlock
            :related="[]"
            offer-help
            @search="onRelatedSearch"
            @manager="openManager(null)"
          />
        </div>
      </div>
    </template>

    <ManagerDialog
      v-model:open="managerOpen"
      :query="managerQuery"
      :quote="managerFor?.source_quote ?? ''"
      :send="sendManager"
    />
  </div>
</template>

<style scoped>
.tzr {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  /* Запас под кнопку консультанта и безопасную зону: она не перекрывает кнопку последнего товара. */
  padding-bottom: calc(var(--fab-size) + var(--fab-bottom) + env(safe-area-inset-bottom, 0px));
}
.tzr__title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}
.tzr__notice {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--space-1) var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  overflow-wrap: anywhere;
}
.tzr__notice > svg {
  flex: none;
  margin-top: 1px;
  color: var(--text-2);
}
.tzr__notice > span {
  flex: 1 1 16rem;
  min-width: 0;
}
.tzr__link-btn {
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  cursor: pointer;
}
.tzr__cols {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
@media (min-width: 1024px) {
  .tzr__cols {
    display: grid;
    grid-template-columns: 240px minmax(0, 1fr);
    align-items: start;
    gap: var(--space-6);
  }
}
.tzr__nav {
  position: sticky;
  top: calc(var(--header-actual, var(--header-h)) + var(--space-2));
  display: flex;
  flex-direction: column;
  max-height: calc(100dvh - var(--header-actual, var(--header-h)) - 2 * var(--space-2));
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tzr__nav-head {
  flex: none;
  padding: var(--space-3) var(--space-3) var(--space-2);
  border-bottom: var(--border-w) solid var(--border);
}
.tzr__nav-head p {
  margin: 2px 0 0;
}
.tzr__nav-title {
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.tzr__nav-list {
  flex: 1 1 auto;
  min-height: 0;
  margin: 0;
  padding: var(--space-1);
  overflow-y: auto;
  overscroll-behavior: contain;
  list-style: none;
}
.tzr__nav-item {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr) 1.25rem;
  align-items: center;
  gap: var(--space-2);
  box-sizing: border-box;
  width: 100%;
  min-height: 52px;
  padding: var(--space-1) var(--space-2);
  border: 0;
  border-radius: var(--radius-control);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.tzr__nav-item:hover {
  background: var(--surface-2);
}
.tzr__nav-item.is-current {
  background: var(--surface-3);
  box-shadow: inset 2px 0 0 var(--accent);
}
.tzr__nav-num {
  color: var(--text-2);
  font-size: var(--fs-sm);
  text-align: right;
}
.tzr__nav-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.tzr__nav-name {
  overflow: hidden;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tzr__nav-status {
  color: var(--text-2);
}
.tzr__nav-status--found {
  color: var(--status-accepted);
}
.tzr__nav-mark {
  display: inline-flex;
  color: var(--status-accepted);
}
.tzr__mobile {
  position: sticky;
  top: var(--header-actual, var(--header-h));
  z-index: 12;
  margin-inline: calc(-1 * var(--gutter));
  padding: var(--space-2) var(--gutter);
  border-bottom: var(--border-w) solid var(--border);
  background: var(--bg);
}
.tzr__select {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  width: 100%;
  min-height: 56px;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.tzr__select-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.tzr__select-head {
  color: var(--text-2);
}
.tzr__select-name {
  overflow: hidden;
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tzr__select-chev {
  flex: none;
  color: var(--text-2);
}
.tzr__positions {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}
@media (max-width: 767px) {
  .tzr__positions {
    gap: var(--space-5);
  }
}
.tzr__unavailable {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);
  max-width: 32rem;
  margin: var(--space-8) auto;
  text-align: center;
}
.tzr__unavailable p {
  margin: 0;
}
.tzr__unavailable .btn {
  margin-top: var(--space-2);
}
.tzr__unavailable-icon {
  color: var(--text-2);
}
.tzr__unavailable-title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.tzr__error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3) var(--space-4);
}
.tzr__error p {
  flex: 1 1 14rem;
  margin: 0;
}
.tzr__error-icon {
  color: var(--danger);
}
.tzr__loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.tzr__sk-title {
  width: 60%;
  height: 1.75rem;
  border-radius: var(--radius-sm);
}
.tzr__sk-block {
  height: 8rem;
  border-radius: var(--radius-card);
}
</style>

<style>
.tzr__select-content {
  z-index: 60;
  width: var(--reka-select-trigger-width);
  max-height: min(var(--reka-select-content-available-height), 70dvh);
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
}
.tzr__select-viewport {
  padding: var(--space-1);
}
.tzr__select-item {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr) 1.25rem;
  align-items: center;
  gap: var(--space-2);
  min-height: 48px;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-control);
  color: var(--text);
  font-size: var(--fs-sm);
  cursor: pointer;
  outline: none;
  user-select: none;
}
.tzr__select-item[data-highlighted] {
  background: var(--surface-3);
}
.tzr__select-item[data-state='checked'] {
  box-shadow: inset 2px 0 0 var(--accent);
}
.tzr__select-item .tzr__nav-num {
  color: var(--text-2);
  text-align: right;
}
.tzr__select-item .tzr__nav-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.tzr__select-item .tzr__nav-status {
  color: var(--text-2);
}
.tzr__select-item .tzr__nav-mark {
  display: inline-flex;
  color: var(--status-accepted);
}
</style>
