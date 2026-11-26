<script setup lang="ts">
// Экран «Поиск». Адрес — единственный источник состояния запроса
// (lib/searchState.ts): строка, конфигурация, клиент, уточнения и открытая трасса. Любое изменение
// адреса, кроме трассы, — новый POST /api/search; прошлый запрос отменяется.
import { computed, ref, shallowRef, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { Headset, Info, ShieldAlert } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { postSearch } from '@/api/search'
import type {
  Chip,
  ClarifyOption,
  Complement,
  ConfigId,
  EventType,
  SearchItem,
  SearchItemBase,
  SearchResponse,
} from '@/api/types'
import type { Companion, SearchFilter, Upgrade } from '@/api/typesP2'
import CompanionsBlock from '@/components/search/CompanionsBlock.vue'
import FacetsPanel from '@/components/search/FacetsPanel.vue'
import RequiredLine from '@/components/search/RequiredLine.vue'
import ConfigSwitch from '@/components/search/ConfigSwitch.vue'
import ManagerDialog from '@/components/search/ManagerDialog.vue'
import ResultCard, { type ItemAction } from '@/components/search/ResultCard.vue'
import ResultSkeleton from '@/components/search/ResultSkeleton.vue'
import SearchPanel, { type PanelSubmit } from '@/components/panel/SearchPanel.vue'
import SearchError from '@/components/search/SearchError.vue'
import UnderstandingBar from '@/components/search/UnderstandingBar.vue'
import TracePanel from '@/components/trace/TracePanel.vue'
import ComplementsBlock from '@/components/search/ComplementsBlock.vue'
import SolutionCard from '@/components/search/SolutionCard.vue'
import UpgradeCard from '@/components/search/UpgradeCard.vue'
import AppliedFilters from '@/components/results/AppliedFilters.vue'
import FamilyCard from '@/components/results/FamilyCard.vue'
import AnalogOriginal from '@/components/results/AnalogOriginal.vue'
import HelpAnswer from '@/components/results/HelpAnswer.vue'
import RefinementChips from '@/components/results/RefinementChips.vue'
import ResultsFacets from '@/components/results/ResultsFacets.vue'
import ResultsToolbar from '@/components/results/ResultsToolbar.vue'
import type { Refinement, ResultsGroup, ResultsSort } from '@/api/typesResults'
import { namedAttrs, refinementFilter, resultsHeading } from '@/lib/resultsView'
import { newIntentId } from '@/lib/panel'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import { useToast } from '@/components/ui/useToast'
import { DESKTOP_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { buildEvent, type EventFields } from '@/lib/events'
import { formatFound, formatMs } from '@/lib/format'
import { emptyTitle, fillTitle, splitResults } from '@/lib/results'
import {
  GUEST_CLIENT_ID,
  buildSearchRequest,
  parseSearchQuery,
  serializeSearchState,
  withAsTyped,
  withChipRemoved,
  withChipsRestored,
  withClarify,
  withQuery,
  type SearchState,
} from '@/lib/searchState'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useSpecStore } from '@/stores/spec'
import {
  blockItems,
  featureOn,
  firstImpression,
  hasConditional,
  impressionPayload,
} from '@/lib/companions'
import { usableFacet, withFilter } from '@/lib/facets'
import { helpAnswerShown, helpHintShown, specPurchase, supplyShown } from '@/lib/procurement'
import { type SpecLine, lineFromCompanion, lineFromItem, quantityForItem } from '@/lib/spec'
import type { SlotChoice } from '@/lib/solution'

const EXAMPLES = [
  'аналог фотошопа на 20 пользователей',
  'касперкий',
  'винда 11 про',
  'отечественный офис на 30 компов, бессрочка',
  'продлить антивирус',
]

const route = useRoute()
const router = useRouter()
const app = useAppStore()
// Контур доводки: переключатель конфигураций B0–B4 — только сотруднику; гость и покупатель ищут на действующей.
const auth = useAuthStore()
const { clientId, configs, defaultConfig } = storeToRefs(app)
const toast = useToast()
const isDesktop = useMediaQuery(DESKTOP_QUERY)

const state = computed(() => parseSearchQuery(route.query))
const effectiveConfig = computed<ConfigId>(() => state.value.config ?? defaultConfig.value)
const text = ref(state.value.q)
watch(
  () => state.value.q,
  (q) => {
    text.value = q
  },
)

const status = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
const response = shallowRef<SearchResponse | null>(null)
const error = ref<ApiError | null>(null)
const selected = ref<string | null>(null)
const managerOpen = ref(false)
const box = ref<InstanceType<typeof SearchPanel> | null>(null)
/** Намерение (§11.1), из которого отправлен ближайший поиск: уходит в тело POST /api/search один раз. */
let pendingIntent: string | null = null
/**
 * Намерение текущей выдачи (§11.1): из панели — её id; открыта ссылкой — своё. Фильтры, сортировка и раскрытие
 * вариантов того же запроса — то же намерение; новый текст запроса — новое.
 */
let resultsIntent: { q: string; id: string } | null = null
let ctrl: AbortController | null = null

function go(next: SearchState, mode: 'push' | 'replace' = 'push') {
  return router[mode]({ path: '/search', query: serializeSearchState(next) })
}

async function run(s: SearchState) {
  ctrl?.abort()
  ctrl = null
  if (!s.q) {
    status.value = 'idle'
    response.value = null
    error.value = null
    return
  }
  const c = new AbortController()
  ctrl = c
  status.value = 'loading'
  error.value = null
  try {
    // Сессию выдаёт сервер в /api/meta: поиск по ссылке при открытии ждёт её (meta грузится один раз).
    if (!app.sessionId) await app.loadMeta()
    if (ctrl !== c) return
    const body = buildSearchRequest(s, {
      sessionId: app.sessionId,
      defaultConfig: defaultConfig.value,
    })
    if (pendingIntent) body.intent_id = pendingIntent
    if (pendingIntent || resultsIntent?.q !== s.q)
      resultsIntent = { q: s.q, id: pendingIntent ?? newIntentId() }
    pendingIntent = null
    const resp = await postSearch(body, { signal: c.signal })
    if (ctrl !== c) return
    response.value = resp
    selected.value = resp.items?.[0]?.product_id ?? null
    status.value = 'ready'
  } catch (e) {
    if (ctrl !== c) return
    if (e instanceof DOMException && e.name === 'AbortError') return
    error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
    status.value = 'error'
  }
}

// Новый поиск — при смене всего, кроме трассы; конфигурация по умолчанию тоже часть ключа.
const requestKey = computed(() =>
  JSON.stringify({
    ...serializeSearchState({ ...state.value, trace: false }),
    effectiveConfig: effectiveConfig.value,
  }),
)
// Во время ухода на другой раздел route уже чужой, а страница ещё жива — её вотчеры молчат.
const onSearchRoute = () => route.path === '/search'
watch(
  requestKey,
  () => {
    if (onSearchRoute()) void run(state.value)
  },
  { immediate: true },
)

// Клиент: адрес ↔ переключатель в шапке. Без client в адресе берётся выбранный в шапке. Покупатель и гость клиента
// не выбирают: чужой client в адресе заменяется своим.
watch(
  () => state.value.client,
  (c) => {
    if (!onSearchRoute()) return
    const own = clientId.value === GUEST_CLIENT_ID ? null : clientId.value
    if (
      (c === null && route.query.client === undefined && own !== null) ||
      (!app.canPickClient && c !== own)
    ) {
      void go({ ...state.value, client: own }, 'replace')
      return
    }
    const id = c ?? GUEST_CLIENT_ID
    if (clientId.value !== id) app.setClient(id)
  },
  { immediate: true },
)
watch(clientId, (id) => {
  if (!onSearchRoute()) return
  const current = state.value.client ?? GUEST_CLIENT_ID
  if (id !== current)
    void go({ ...state.value, client: id === GUEST_CLIENT_ID ? null : id }, 'replace')
})

const blocks = computed(() => splitResults(response.value))
const titles = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  for (const it of response.value?.items ?? []) {
    out[it.product_id] = it.title
    for (const v of it.variants ?? []) out[v.product_id] ??= v.title
    for (const c of it.required ?? []) out[c.product_id] ??= c.title
  }
  for (const c of response.value?.companions?.items ?? []) out[c.product_id] ??= c.title
  return out
})
const relaxed = computed(() => response.value?.relaxed ?? [])
const related = computed(() => {
  const own = (response.value?.query ?? '').trim().toLowerCase()
  return (response.value?.related_queries ?? []).filter((q) => q.trim().toLowerCase() !== own)
})
// ── Контур коммерции: допродажа, оборудование, спецификация ──
const spec = useSpecStore()
const features = computed(() => app.meta?.features)
const companionsOn = computed(() => featureOn(features.value, 'companions'))
const hardwareOn = computed(() => featureOn(features.value, 'hardware'))
const filters = computed<SearchFilter[]>(() => state.value.filters ?? [])
const fillHeading = computed(() =>
  fillTitle(
    blocks.value.fill,
    filters.value.some((f) => f.key === 'vendor' && f.op === 'eq') &&
      !filters.value.some((f) => f.key === 'category' && f.op === 'eq'),
  ),
)
const facets = computed(() =>
  hardwareOn.value ? (response.value?.facets ?? []).filter(usableFacet) : [],
)
// ── закупка под задачу и старшая редакция ──
const solutionsOn = computed(() => featureOn(features.value, 'solutions'))
const solution = computed(() => (solutionsOn.value ? (response.value?.solution ?? null) : null))
const upgrade = computed(() => (solutionsOn.value ? (response.value?.upgrade ?? null) : null))
const upgradeBase = computed(
  () =>
    response.value?.items.find((i) => i.product_id === upgrade.value?.for_product_id)?.title ?? '',
)

function onSolutionApply(lines: SpecLine[], choice: SlotChoice) {
  for (const l of lines) spec.add(l)
  const sol = solution.value
  trackQuiet('template_apply', {
    payload: {
      template_id: sol?.template_id ?? '',
      positions: lines.length,
      seats: sol?.seats ?? null,
      choice,
    },
  })
  toast.add({
    tone: 'ok',
    title: 'Решение добавлено в спецификацию',
    detail: `${sol?.title ?? ''}: ${lines.length} поз.`,
    action: openSpecAction(),
  })
}

function onUpgradeOpen(u: Upgrade) {
  trackQuiet('companion_click', {
    productId: u.product_id,
    payload: { class: 'upgrade', place: 'upgrade' },
  })
  submit(u.title)
}

function onUpgradeAdd(u: Upgrade) {
  const base = response.value?.items.find((i) => i.product_id === u.for_product_id)
  const qty = base ? quantityForItem(base, response.value?.chips ?? []) : 1
  spec.add(lineFromCompanion(u, qty, upgradeBase.value))
  trackQuiet('companion_add', {
    productId: u.product_id,
    payload: { class: 'upgrade', quantity: qty, place: 'upgrade' },
  })
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: u.title,
    action: openSpecAction(),
  })
}

const showFacets = computed(() => facets.value.length > 0 || filters.value.length > 0)

// ── Контур выдачи: страница результатов. Сервер прислал counts — новая раскладка:
// уточнения, заголовок с сортировкой и группировкой, применённые фильтры, лицензионные фасеты, карточки-семейства.
const resultsOn = computed(
  () => featureOn(features.value, 'results_v3') && Boolean(response.value?.counts),
)
const resultFacets = computed(() => response.value?.facets ?? [])
const refinements = computed(() => (resultsOn.value ? (response.value?.refinements ?? []) : []))
const named = computed(() => namedAttrs(response.value?.chips ?? []))
const sortValue = computed<ResultsSort>(() => state.value.sort ?? 'recommended')
const groupValue = computed<ResultsGroup>(() =>
  response.value?.group === 'none' ? 'none' : (state.value.group ?? 'family'),
)
const forcedFlat = computed(() => response.value?.group === 'none' && state.value.group !== 'none')
const showResultFacets = computed(() => resultFacets.value.length > 0 || filters.value.length > 0)
const inSpec = (id: string) => spec.has(id)
/** Фильтры выдачи сняли всё, хотя запрос находит товары (сервер: empty без менеджера). */
const filteredEmpty = computed(
  () => resultsOn.value && filters.value.length > 0 && (response.value?.items.length ?? 0) === 0,
)

function onRefinement(r: Refinement) {
  const f = refinementFilter(r)
  trackQuiet('refinement_apply', {
    payload: { field: r.field, value: r.value, label: r.label, hits: r.hits, place: 'results' },
  })
  if (!f) {
    submit(r.query)
    return
  }
  void go({ ...state.value, filters: withFilter(filters.value, f.key, f) })
}

function onResultFacet(key: string, filter: SearchFilter | null) {
  const prev = filters.value.find((f) => f.key === key)
  if (filter) trackQuiet('filter_apply', { payload: { key, value: filter.value, op: filter.op } })
  else trackQuiet('filter_remove', { payload: { key, value: prev?.value ?? null } })
  void go({ ...state.value, filters: withFilter(filters.value, key, filter) })
}

function onFilterRemove(f: SearchFilter) {
  trackQuiet('filter_remove', { payload: { key: f.key, value: f.value } })
  void go({ ...state.value, filters: withFilter(filters.value, f.key, null) })
}

function onFiltersReset() {
  trackQuiet('filters_reset', { payload: { count: filters.value.length } })
  void go({ ...state.value, filters: [] })
}

function onSort(v: ResultsSort) {
  if (v === sortValue.value) return
  trackQuiet('sort_change', { payload: { value: v, from: sortValue.value } })
  void go({ ...state.value, sort: v === 'recommended' ? undefined : v })
}

function onGroup(v: ResultsGroup) {
  if (v === groupValue.value) return
  trackQuiet('group_change', { payload: { value: v } })
  void go({ ...state.value, group: v === 'family' ? undefined : v })
}

function onVariantExpand(item: SearchItem, position: number) {
  selected.value = item.product_id
  trackQuiet('variant_expand', {
    productId: item.product_id,
    position,
    payload: { family_id: item.family_id, variants: (item.variants ?? []).length },
  })
}

function onSpecTarget(target: SearchItemBase, position: number) {
  spec.add(
    lineFromItem(
      target,
      quantityForItem(target, response.value?.chips ?? []),
      specPurchase(response.value?.purchase),
    ),
  )
  trackQuiet('click', { productId: target.product_id, position, payload: { action: 'spec' } })
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: target.title,
    action: openSpecAction(),
  })
}
const companionAnchor = computed(() => {
  const a = response.value?.companions?.anchor
  if (!a) return null
  return response.value?.items.find((i) => i.product_id === a.product_id) ?? null
})
const companionItems = computed(() => {
  if (!companionsOn.value) return []
  const inCard = (companionAnchor.value?.required ?? []).map((c) => c.product_id)
  return blockItems(response.value?.companions, inCard)
})
const companionAnchorTitle = computed(
  () => companionAnchor.value?.title ?? response.value?.items[0]?.title ?? '',
)
const requiredOf = (item: SearchItem) => (companionsOn.value ? (item.required ?? []) : [])
const conditionalNote = computed(
  () => hardwareOn.value && hasConditional(response.value?.items ?? []),
)
/** Блок «Дополните решение» встаёт после третьего результата основной выдачи. */
const BLOCK_AFTER = 3
const mainTop = computed(() => blocks.value.main.slice(0, BLOCK_AFTER))
const mainRest = computed(() => blocks.value.main.slice(BLOCK_AFTER))

// Показ блока и строки — одно событие на поиск и место («События»).
const impressions = new Set<string>()
watch(response, (resp) => {
  if (!resp || !companionsOn.value) return
  const rid = resp.request_id
  if (companionItems.value.length && firstImpression(impressions, rid, 'block')) {
    const anchor = resp.companions?.anchor.product_id ?? ''
    trackQuiet('companion_impression', {
      payload: impressionPayload(anchor, companionItems.value, 'block'),
    })
  }
  for (const it of resp.items ?? []) {
    const req = requiredOf(it)
    if (req.length && firstImpression(impressions, rid, `required:${it.product_id}`))
      trackQuiet('companion_impression', {
        payload: impressionPayload(it.product_id, req, 'required'),
      })
  }
})

function openSpecAction() {
  return { label: 'Открыть', run: () => (spec.open = true) }
}

function onSpec(item: SearchItem) {
  spec.add(
    lineFromItem(
      item,
      quantityForItem(item, response.value?.chips ?? []),
      specPurchase(response.value?.purchase),
    ),
  )
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: item.title,
    action: openSpecAction(),
  })
}

function onCompanionOpen(c: Companion, index: number) {
  trackQuiet('companion_click', {
    productId: c.product_id,
    position: index + 1,
    payload: { class: c.class, place: 'block' },
  })
  submit(c.title)
}

function onCompanionAdd(c: Companion, index: number) {
  const qty = c.suggested_quantity ?? 1
  spec.add(lineFromCompanion(c, qty, companionAnchorTitle.value))
  trackQuiet('companion_add', {
    productId: c.product_id,
    position: index + 1,
    payload: { class: c.class, quantity: qty, place: 'block' },
  })
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: c.title,
    action: openSpecAction(),
  })
}

function onFacet(key: string, filter: SearchFilter | null) {
  trackQuiet('facet_apply', {
    payload: {
      key,
      value: filter ? filter.value : null,
      op: filter?.op ?? null,
      value2: filter?.value2 ?? null,
    },
  })
  void go({ ...state.value, filters: withFilter(filters.value, key, filter) })
}

function onFacetsReset() {
  void go({ ...state.value, filters: [] })
}

const complements = computed(() => response.value?.complements ?? [])
const complementAnchor = computed(() => response.value?.items[0]?.title ?? '')
const showManager = computed(
  () => blocks.value.nothing || Boolean(response.value?.empty?.show_manager),
)

function eventContext() {
  return {
    requestId: response.value?.request_id ?? null,
    sessionId: app.sessionId,
    clientId: state.value.client,
    intentId: resultsIntent?.id ?? null,
  }
}

function track(type: EventType, target: EventFields = {}) {
  return postEvent(buildEvent(type, eventContext(), target))
}

function trackQuiet(type: EventType, target: EventFields = {}) {
  // Служебные события не должны мешать поиску: ошибка записи не показывается.
  track(type, target).catch(() => undefined)
}

function submit(raw: string) {
  const q = raw.trim()
  if (!q) {
    box.value?.focus()
    return
  }
  const next = withQuery(state.value, q)
  if (
    JSON.stringify(serializeSearchState(next)) === JSON.stringify(serializeSearchState(state.value))
  ) {
    void run(state.value)
    return
  }
  void go(next)
}

/** Панель поиска (контур выдачи): Enter, подсказка, категория, производитель, товар — с условиями бейджей. */
function onPanelSubmit(p: PanelSubmit) {
  pendingIntent = p.intentId
  const next = { ...withQuery(state.value, p.q), filters: p.filters }
  if (
    JSON.stringify(serializeSearchState(next)) === JSON.stringify(serializeSearchState(state.value))
  ) {
    void run(state.value)
    return
  }
  void go(next)
}

function onAsTyped() {
  const c = response.value?.correction
  if (c) trackQuiet('correction_undo', { payload: { from: c.from, to: c.to } })
  void go(withAsTyped(state.value))
}

function onRemoveChip(chip: Chip) {
  trackQuiet('chip_remove', {
    payload: { field: chip.field, value: chip.value, label: chip.label },
  })
  void go(withChipRemoved(state.value, chip))
}

function onClarify(attribute: string, option: ClarifyOption) {
  trackQuiet('clarify_answer', { payload: { attribute, value: option.value } })
  void go(withClarify(state.value, attribute, option.value))
}

function onConfig(config: ConfigId) {
  if (config !== effectiveConfig.value) void go({ ...state.value, config })
}

function setTrace(open: boolean) {
  if (open !== state.value.trace) void go({ ...state.value, trace: open }, 'replace')
}

function onOpen(item: SearchItem, position: number) {
  selected.value = item.product_id
  trackQuiet('click', { productId: item.product_id, position })
}

function onWhy(item: SearchItem) {
  selected.value = item.product_id
  setTrace(true)
}

const ACTION_TOAST: Record<ItemAction, string> = {
  add_to_cart: 'Добавлено в корзину',
  order: 'Заказ записан',
  price_request: 'Запрос цены записан',
}

function onAction(type: ItemAction, target: SearchItemBase, position: number) {
  track(type, { productId: target.product_id, position })
    .then(() =>
      toast.add({
        tone: 'ok',
        title: ACTION_TOAST[type],
        detail: `${target.title}. Демо-стенд: действие записано событием для аналитики.`,
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

function onComplement(type: 'add_to_cart' | 'price_request', c: Complement, index: number) {
  track(type, { productId: c.product_id, position: index + 1, payload: { block: 'complements' } })
    .then(() =>
      toast.add({
        tone: 'ok',
        title: ACTION_TOAST[type],
        detail: `${c.title}. Демо-стенд: действие записано событием для аналитики.`,
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

function sendManagerRequest() {
  return track('manager_request', { payload: { query: response.value?.query ?? state.value.q } })
}

// Контур доводки: справка, поставка отсутствующего товара, «Вы искали» («События»).
const helpShown = computed(() => helpAnswerShown(response.value))
const helpHint = computed(() => helpHintShown(response.value))
const supply = computed(() => supplyShown(response.value))
const supplySent = ref<string | null>(null)
const reestrUnconfirmed = computed(() => response.value?.reestr_unconfirmed ?? [])
const procurementSeen = new Set<string>()
watch(response, (resp) => {
  if (!resp) return
  const rid = resp.request_id
  if (resp.help && !procurementSeen.has(`help:${rid}`)) {
    procurementSeen.add(`help:${rid}`)
    trackQuiet('help_answer_shown', { payload: { help_id: resp.help.id, mode: resp.help.mode } })
  }
  if (resp.analog_original && !procurementSeen.has(`orig:${rid}`)) {
    procurementSeen.add(`orig:${rid}`)
    trackQuiet('analog_original_shown', { payload: { family_id: resp.analog_original.family_id } })
  }
})

function onHelpOpen(help: { id: string; mode: string }) {
  trackQuiet('help_click', { payload: { help_id: help.id, mode: help.mode } })
}

async function onSupply() {
  const resp = response.value
  if (!resp) return
  try {
    await track('supply_request', {
      payload: { query: resp.query, message: resp.empty?.message ?? '' },
    })
    supplySent.value = resp.request_id
    toast.add({
      tone: 'ok',
      title: 'Запрос поставки отправлен',
      detail: `«${resp.query}» — категорийный менеджер увидит его в SearchOps`,
    })
  } catch {
    toast.add({ tone: 'error', title: 'Запрос не отправлен', detail: 'Повторите через минуту' })
  }
}
</script>

<template>
  <div :class="['search-page', { 'search-page--trace': state.trace && isDesktop }]">
    <div class="search-page__main">
      <h1 class="sr-only">Поиск по каталогу</h1>
      <div class="search-page__bar">
        <SearchPanel
          ref="box"
          v-model="text"
          :client-id="state.client ?? GUEST_CLIENT_ID"
          @submit="onPanelSubmit"
        />
        <div class="search-page__controls">
          <ConfigSwitch
            v-if="auth.staff"
            :configs="configs"
            :model-value="effectiveConfig"
            @update:model-value="onConfig"
          />
          <UiSwitch label="Трасса" :model-value="state.trace" @update:model-value="setTrace" />
        </div>
      </div>

      <ResultSkeleton v-if="status === 'loading'" />

      <SearchError v-else-if="status === 'error' && error" :error="error" @retry="run(state)" />

      <template v-else-if="status === 'ready' && response">
        <h2 v-if="helpShown" class="sr-only" role="status">
          Ответ справки по запросу «{{ response.query }}»
        </h2>
        <h2 v-else-if="resultsOn" class="search-page__heading" role="status">
          {{ resultsHeading(response) }}
          <span v-if="response.trace?.total_ms != null" class="t-sm t-muted">
            · {{ formatMs(response.trace.total_ms) }}
          </span>
        </h2>
        <p v-else class="search-page__found t-sm t-muted t-num" role="status">
          {{ formatFound(response.total, response.trace?.total_ms) }} · {{ response.config }}
        </p>

        <UnderstandingBar
          :response="response"
          :as-typed="state.asTyped"
          :removed="state.removed"
          @as-typed="onAsTyped"
          @try-query="submit"
          @remove-chip="onRemoveChip"
          @restore-chips="go(withChipsRestored(state))"
          @clarify="onClarify"
        />

        <HelpAnswer v-if="helpHint && response.help" :help="response.help" @open="onHelpOpen" />

        <ul v-if="relaxed.length" class="search-page__notes">
          <li v-for="r in relaxed" :key="r.field">
            <Info :size="16" aria-hidden="true" />
            <span>{{ r.message }}</span>
          </li>
        </ul>

        <SolutionCard v-if="solution" :solution="solution" @apply="onSolutionApply" />

        <section v-if="filteredEmpty" class="panel search-page__absent" role="status">
          <h2 class="t-h2">{{ response.empty?.message || 'По выбранным фильтрам позиций нет' }}</h2>
          <p class="t-muted">Снимите один из фильтров или сбросьте все — запрос останется.</p>
          <div>
            <button type="button" class="btn btn--secondary" @click="onFiltersReset">
              Сбросить фильтры
            </button>
          </div>
        </section>

        <HelpAnswer
          v-else-if="helpShown && response.help"
          :help="response.help"
          @open="onHelpOpen"
        />

        <section v-else-if="blocks.absent || blocks.nothing" class="panel search-page__absent">
          <h2 class="t-h2">
            {{
              blocks.absent
                ? emptyTitle(response)
                : `По запросу «${response.query}» ничего не нашлось`
            }}
          </h2>
          <p class="t-muted">
            {{
              blocks.analogs.length
                ? 'Ниже — похожие по назначению. Если не подходят, менеджер подберёт решение.'
                : 'Похожих товаров в каталоге тоже нет. Менеджер подберёт решение или предложит замену.'
            }}
          </p>
          <div v-if="showManager || supply" class="search-page__absent-actions">
            <template v-if="supply">
              <span
                v-if="supplySent === response.request_id"
                class="t-sm t-muted"
                role="status"
                data-testid="supply-sent"
              >
                Запрос поставки отправлен
              </span>
              <button
                v-else
                type="button"
                class="btn btn--primary"
                data-testid="supply-request"
                @click="onSupply"
              >
                Запросить поставку
              </button>
            </template>
            <button
              v-if="showManager"
              type="button"
              class="btn btn--secondary"
              @click="managerOpen = true"
            >
              <Headset :size="16" aria-hidden="true" />
              Спросить менеджера
            </button>
          </div>
        </section>

        <p v-if="conditionalNote" class="search-page__compat t-sm">
          <ShieldAlert :size="16" aria-hidden="true" />
          Для точной проверки укажите конфигурацию сервера
        </p>

        <template v-if="resultsOn">
          <RefinementChips :items="refinements" @apply="onRefinement" />
          <ResultsToolbar
            v-if="response.items.length || filters.length"
            :response="response"
            :sort="sortValue"
            :group="groupValue"
            :forced-flat="forcedFlat"
            @sort="onSort"
            @group="onGroup"
          />
          <AppliedFilters
            :filters="filters"
            :facets="resultFacets"
            @remove="onFilterRemove"
            @reset="onFiltersReset"
          />
        </template>

        <div
          :class="[
            'search-page__body',
            { 'search-page__body--facets': resultsOn ? showResultFacets : showFacets },
          ]"
        >
          <ResultsFacets
            v-if="resultsOn && showResultFacets"
            class="search-page__facets"
            :facets="resultFacets"
            :filters="filters"
            :query-key="state.q"
            @apply="onResultFacet"
            @reset="onFiltersReset"
          />
          <FacetsPanel
            v-else-if="!resultsOn && showFacets"
            class="search-page__facets"
            :facets="facets"
            :filters="filters"
            @apply="onFacet"
            @reset="onFacetsReset"
          />
          <div class="search-page__results">
            <ol v-if="mainTop.length" class="search-page__list" aria-label="Результаты">
              <li v-for="p in mainTop" :key="p.item.product_id">
                <FamilyCard
                  v-if="resultsOn"
                  :item="p.item"
                  :position="p.position"
                  :named="named"
                  :selected="state.trace && selected === p.item.product_id"
                  :in-spec="inSpec"
                  :show-compat="hardwareOn"
                  :purchase="response.purchase"
                  @expand="onVariantExpand"
                  @spec="onSpecTarget"
                  @price-request="(t, pos) => onAction('price_request', t, pos)"
                  @why="onWhy"
                >
                  <RequiredLine
                    v-if="requiredOf(p.item).length"
                    :required="requiredOf(p.item)"
                    :anchor-id="p.item.product_id"
                    :anchor-title="p.item.title"
                    :request-id="response.request_id"
                  />
                </FamilyCard>
                <ResultCard
                  v-else
                  :item="p.item"
                  :position="p.position"
                  :selected="state.trace && selected === p.item.product_id"
                  :in-spec="spec.has(p.item.product_id)"
                  :show-compat="hardwareOn"
                  @open="onOpen"
                  @action="onAction"
                  @why="onWhy"
                  @spec="onSpec"
                >
                  <RequiredLine
                    v-if="requiredOf(p.item).length"
                    :required="requiredOf(p.item)"
                    :anchor-id="p.item.product_id"
                    :anchor-title="p.item.title"
                    :request-id="response.request_id"
                  />
                </ResultCard>
              </li>
            </ol>

            <CompanionsBlock
              v-if="companionItems.length"
              :items="companionItems"
              :anchor="companionAnchorTitle"
              @open="onCompanionOpen"
              @add="onCompanionAdd"
              @search="submit"
            />

            <ol
              v-if="mainRest.length"
              class="search-page__list"
              :start="BLOCK_AFTER + 1"
              aria-label="Продолжение выдачи"
            >
              <li v-for="p in mainRest" :key="p.item.product_id">
                <FamilyCard
                  v-if="resultsOn"
                  :item="p.item"
                  :position="p.position"
                  :named="named"
                  :selected="state.trace && selected === p.item.product_id"
                  :in-spec="inSpec"
                  :show-compat="hardwareOn"
                  :purchase="response.purchase"
                  @expand="onVariantExpand"
                  @spec="onSpecTarget"
                  @price-request="(t, pos) => onAction('price_request', t, pos)"
                  @why="onWhy"
                >
                  <RequiredLine
                    v-if="requiredOf(p.item).length"
                    :required="requiredOf(p.item)"
                    :anchor-id="p.item.product_id"
                    :anchor-title="p.item.title"
                    :request-id="response.request_id"
                  />
                </FamilyCard>
                <ResultCard
                  v-else
                  :item="p.item"
                  :position="p.position"
                  :selected="state.trace && selected === p.item.product_id"
                  :in-spec="spec.has(p.item.product_id)"
                  :show-compat="hardwareOn"
                  @open="onOpen"
                  @action="onAction"
                  @why="onWhy"
                  @spec="onSpec"
                >
                  <RequiredLine
                    v-if="requiredOf(p.item).length"
                    :required="requiredOf(p.item)"
                    :anchor-id="p.item.product_id"
                    :anchor-title="p.item.title"
                    :request-id="response.request_id"
                  />
                </ResultCard>
              </li>
            </ol>

            <UpgradeCard
              v-if="upgrade"
              :upgrade="upgrade"
              :base="upgradeBase"
              @open="onUpgradeOpen"
              @add="onUpgradeAdd"
            />

            <section
              v-if="blocks.fill.length"
              class="search-page__section"
              aria-labelledby="fill-title"
            >
              <h2 id="fill-title" class="t-h2">{{ fillHeading }}</h2>
              <p class="t-sm t-muted">
                Поиск по запросу их не нашёл — популярное по выбранному фильтру, не по соответствию
                запросу.
              </p>
              <ol class="search-page__list">
                <li v-for="p in blocks.fill" :key="p.item.product_id">
                  <FamilyCard
                    v-if="resultsOn"
                    :item="p.item"
                    :position="p.position"
                    :named="named"
                    :selected="state.trace && selected === p.item.product_id"
                    :in-spec="inSpec"
                    :show-compat="hardwareOn"
                    @expand="onVariantExpand"
                    @spec="onSpecTarget"
                    @price-request="(t, pos) => onAction('price_request', t, pos)"
                    @why="onWhy"
                  />
                  <ResultCard
                    v-else
                    :item="p.item"
                    :position="p.position"
                    :selected="state.trace && selected === p.item.product_id"
                    :in-spec="spec.has(p.item.product_id)"
                    :show-compat="hardwareOn"
                    @open="onOpen"
                    @action="onAction"
                    @why="onWhy"
                    @spec="onSpec"
                  />
                </li>
              </ol>
            </section>

            <section
              v-if="blocks.analogs.length"
              class="search-page__section"
              aria-labelledby="analogs-title"
            >
              <h2 id="analogs-title" class="t-h2">
                {{
                  blocks.absent || (!blocks.main.length && !response.analog_original)
                    ? 'Похожие по назначению'
                    : 'Аналоги'
                }}
              </h2>
              <AnalogOriginal
                v-if="response.analog_original"
                :original="response.analog_original"
              />
              <ol class="search-page__list">
                <li v-for="p in blocks.analogs" :key="p.item.product_id">
                  <FamilyCard
                    v-if="resultsOn"
                    :item="p.item"
                    :position="p.position"
                    :named="named"
                    :selected="state.trace && selected === p.item.product_id"
                    :in-spec="inSpec"
                    :show-compat="hardwareOn"
                    @expand="onVariantExpand"
                    @spec="onSpecTarget"
                    @price-request="(t, pos) => onAction('price_request', t, pos)"
                    @why="onWhy"
                  />
                  <ResultCard
                    v-else
                    :item="p.item"
                    :position="p.position"
                    :selected="state.trace && selected === p.item.product_id"
                    :in-spec="spec.has(p.item.product_id)"
                    :show-compat="hardwareOn"
                    @open="onOpen"
                    @action="onAction"
                    @why="onWhy"
                    @spec="onSpec"
                  />
                </li>
              </ol>
            </section>

            <section
              v-if="reestrUnconfirmed.length"
              class="search-page__section"
              aria-labelledby="reestr-unknown-title"
              data-testid="reestr-unconfirmed"
            >
              <h2 id="reestr-unknown-title" class="t-h2">Статус не установлен</h2>
              <p class="t-sm t-muted">
                Запрос требует реестр российского ПО, а подтверждения записи в данных нет —
                проверьте у менеджера.
              </p>
              <ol class="search-page__list">
                <li v-for="(it, i) in reestrUnconfirmed" :key="it.product_id">
                  <FamilyCard
                    :item="it"
                    :position="response.items.length + i + 1"
                    :named="named"
                    :in-spec="inSpec"
                    @expand="onVariantExpand"
                    @spec="onSpecTarget"
                    @price-request="(t, pos) => onAction('price_request', t, pos)"
                    @why="onWhy"
                  />
                </li>
              </ol>
            </section>

            <ComplementsBlock
              v-if="!companionItems.length && complements.length && blocks.main.length"
              :items="complements"
              :anchor="complementAnchor"
              @action="onComplement"
            />
          </div>
        </div>

        <section v-if="related.length" class="search-page__related" aria-labelledby="related-title">
          <h2 id="related-title" class="t-sm t-muted">Похожие запросы</h2>
          <ul class="search-page__chips">
            <li v-for="q in related" :key="q">
              <button type="button" class="btn btn--secondary btn--sm" @click="submit(q)">
                {{ q }}
              </button>
            </li>
          </ul>
        </section>
      </template>

      <section v-else class="search-page__idle" aria-labelledby="idle-title">
        <h2 id="idle-title" class="t-h2">Что ищем?</h2>
        <p class="t-muted">
          Название продукта, задача своими словами («аналог фотошопа на 20 пользователей») или
          артикул. Опечатки, раскладку и сленг поиск исправит сам; как он понял запрос — видно в
          трассе.
        </p>
        <ul class="search-page__chips" aria-label="Примеры запросов">
          <li v-for="q in EXAMPLES" :key="q">
            <button type="button" class="btn btn--secondary btn--sm" @click="submit(q)">
              {{ q }}
            </button>
          </li>
        </ul>
      </section>
    </div>

    <aside v-if="state.trace && isDesktop" class="search-page__trace" aria-label="Трасса запроса">
      <TracePanel
        :trace="response?.trace ?? null"
        :loading="status === 'loading'"
        :titles="titles"
        :selected="selected"
        :query="response?.query ?? null"
        closable
        @select="selected = $event"
        @close="setTrace(false)"
      />
    </aside>

    <DialogRoot
      :open="state.trace && !isDesktop"
      @update:open="(v: boolean) => !v && setTrace(false)"
    >
      <DialogPortal>
        <DialogOverlay class="ui-scrim" />
        <DialogContent class="trace-sheet" :aria-describedby="undefined">
          <DialogTitle class="sr-only">Трасса запроса</DialogTitle>
          <TracePanel
            :trace="response?.trace ?? null"
            :loading="status === 'loading'"
            :titles="titles"
            :selected="selected"
            :query="response?.query ?? null"
            closable
            @select="selected = $event"
            @close="setTrace(false)"
          />
        </DialogContent>
      </DialogPortal>
    </DialogRoot>

    <ManagerDialog
      v-model:open="managerOpen"
      :query="response?.query ?? state.q"
      :send="sendManagerRequest"
    />
  </div>
</template>

<style scoped>
.search-page {
  display: flex;
  flex: 1;
  min-width: 0;
}
.search-page__main {
  container-type: inline-size;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-3);
  box-sizing: border-box;
  min-width: 0;
  max-width: var(--content-max);
  padding: var(--space-4) var(--gutter) var(--space-12);
}
.search-page--trace .search-page__main {
  max-width: none;
}
.search-page__bar {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.search-page__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
}
.search-page__found {
  min-height: var(--lh-sm);
}
.search-page__heading {
  margin: 0;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
  font-size: var(--fs-h2, 1.125rem);
  font-weight: var(--fw-semibold);
}
.search-page__notes {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.search-page__notes li {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  color: var(--text-2);
}
.search-page__notes svg {
  flex: none;
  margin-top: 2px;
}
.search-page__absent {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4) var(--space-5);
}
.search-page__absent-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.search-page__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.search-page__compat {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  color: var(--text-2);
}
.search-page__compat svg {
  flex: none;
  margin-top: 1px;
}
.search-page__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.search-page__results {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
@container (min-width: 52rem) {
  .search-page__body--facets {
    display: grid;
    grid-template-columns: 14rem minmax(0, 1fr);
    align-items: start;
  }
  .search-page__facets {
    position: sticky;
    top: calc(var(--header-h) + var(--space-3));
  }
}
.search-page__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.search-page__related {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.search-page__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.search-page__chips .btn {
  white-space: normal;
  text-align: left;
}
.search-page__idle {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-6) 0;
}
.search-page__trace {
  position: sticky;
  top: var(--header-h);
  flex: none;
  box-sizing: border-box;
  width: var(--trace-w);
  min-width: var(--trace-min-w);
  height: calc(100dvh - var(--header-h));
  overflow-y: auto;
  padding: var(--space-4);
  border-left: var(--border-w) solid var(--border);
  background: var(--surface);
}
</style>

<style>
.trace-sheet {
  position: fixed;
  inset: 0;
  z-index: 51;
  box-sizing: border-box;
  overflow-y: auto;
  padding: var(--space-4) var(--gutter) var(--space-8);
  background: var(--surface);
  animation: ui-fade-in var(--dur-panel) var(--ease-standard);
}
.trace-sheet:focus-visible {
  outline: none;
}
</style>
