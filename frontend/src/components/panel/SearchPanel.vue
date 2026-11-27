<script setup lang="ts">
// Строка поиска с панелью контура выдачи.
// Один ARIA combobox: фокус всегда в строке, активный элемент — aria-activedescendant; панель — один listbox с
// подписанными группами, две колонки — только раскладка, обход стрелками линейный. Бейджи-уточнения — кнопки
// вне списка (Tab). Первый элемент не выделяется: Enter без выделения — страница результатов.
// Сеть: debounce 150 мс, от 2 символов, отмена прежнего запроса + порядковый номер ответа, кэш на сессию.
// Сервер без полей §11.2 (флаг FEATURE_PANEL выключен) — прежний список групп контура коммерции.
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { storeToRefs } from 'pinia'
import {
  ArrowRight,
  CircleHelp,
  Clock,
  FileText,
  Folder,
  Paperclip,
  Plus,
  Search,
  SearchX,
  TrendingUp,
  X,
} from 'lucide-vue-next'
import { getPanelRecs, getPanelSuggest } from '@/api/panel'
import { postEvent } from '@/api/events'
import type { EventType } from '@/api/types'
import type { SearchFilter } from '@/api/typesP2'
import type {
  PanelProduct,
  PanelRecs,
  PanelRefinement,
  PanelSuggestResponse,
  PanelSuggestion,
  PanelTogether,
} from '@/api/typesPanel'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { buildEvent, type EventFields } from '@/lib/events'
import { formatInt, formatPrice } from '@/lib/format'
import { SHOWCASE } from '@/lib/showcase'
import {
  DESKTOP_LIMITS,
  GROUP_LABELS,
  bottomTogether,
  MOBILE_LIMITS,
  PANEL_DEBOUNCE_MS,
  PANEL_MIN_CHARS,
  PANEL_SKELETON_MS,
  type PanelOption,
  type PanelView,
  Sequencer,
  buildOptions,
  cacheKey,
  isEmptyView,
  isStructured,
  loadRecent,
  moveActive,
  newIntentId,
  offeredRefinements,
  refinedQuery,
  refinementFilters,
  saveRecent,
  sameRefinement,
  suggestionParts,
  toPanelView,
  togetherReason,
  withRecent,
  withoutRecent,
} from '@/lib/panel'
import { lineFromCompanion } from '@/lib/spec'
import { toSuggestGroups } from '@/lib/suggest'
import type { SuggestResponse } from '@/api/types'
import type { Companion } from '@/api/typesP2'
import { useAppStore } from '@/stores/app'
import { useSpecStore } from '@/stores/spec'
import { useToast } from '@/components/ui/useToast'
import VendorLogo from '@/components/common/VendorLogo.vue'
import UiTooltip from '@/components/ui/UiTooltip.vue'
import { TZ_TEXT } from '@/lib/tz'
import { useTzStore } from '@/stores/tz'
import PanelProductCard from './PanelProductCard.vue'

const model = defineModel<string>({ required: true })
/** Свой экземпляр текста: значение v-model обновляется только после перерисовки родителя, а запрос панели
 * должен уйти по только что набранному символу. */
const text = ref(model.value)
// Синхронно: родитель всегда держит последний набранный текст, и его эхо в v-model не откатывает строку.
watch(
  text,
  (v) => {
    if (v !== model.value) model.value = v
  },
  { flush: 'sync' },
)
const props = defineProps<{
  clientId: string | null
  /** Контур ТЗ: открыто ТЗ — на месте текста плашка с именем файла (TZ-UI.md §4). */
  tzFile?: string | null
}>()
const emit = defineEmits<{
  submit: [payload: PanelSubmit]
  /** Крестик при плашке ТЗ: выход из режима ТЗ, пустое поле с фокусом. */
  tzExit: []
}>()
const tz = useTzStore()
/** Плашка файла видна, пока человек не начал печатать: начал — обычный поиск. */
const chipShown = computed(() => Boolean(props.tzFile) && !text.value)

const app = useAppStore()
const { sessionId } = storeToRefs(app)
const spec = useSpecStore()
const toast = useToast()
const wide = useMediaQuery('(min-width: 768px)')
const desktop = useMediaQuery('(min-width: 1024px)')
const limits = computed(() => (wide.value ? DESKTOP_LIMITS : MOBILE_LIMITS))

const uid = `sp-${Math.random().toString(36).slice(2, 8)}`
const listId = `${uid}-list`
const inputRef = ref<HTMLInputElement | null>(null)
const rootRef = ref<HTMLElement | null>(null)

// ── Состояние панели ──
const open = ref(false)
const active = ref(-1)
const applied = ref<PanelRefinement[]>([])
const recent = ref<string[]>(loadRecent(storage()))
const popular = ref<PanelSuggestion[]>([])
const response = shallowRef<PanelSuggestResponse | null>(null)
const view = shallowRef<PanelView | null>(null)
const viewQuery = ref('')
const requested = ref('')
const pending = ref(false)
const failed = ref(false)
const skeleton = ref(false)
const recs = shallowRef<PanelRecs | null>(null)

function storage() {
  try {
    return typeof window !== 'undefined' ? window.localStorage : null
  } catch {
    return null
  }
}

const typed = computed(() => text.value.trim())
const idle = computed(() => typed.value.length < PANEL_MIN_CHARS)
const effectiveQuery = computed(() => refinedQuery(text.value, applied.value))
const productsStale = computed(
  () => Boolean(view.value) && cacheKey(viewQuery.value) !== cacheKey(requested.value),
)
const legacyGroups = computed(() =>
  view.value?.legacy && response.value ? toSuggestGroups(response.value as SuggestResponse) : null,
)
const offered = computed(() =>
  idle.value || !view.value ? [] : offeredRefinements(view.value.refinements, applied.value),
)
const products = computed<PanelProduct[]>(() => (idle.value ? [] : (view.value?.products ?? [])))
/** Место под товары держим, пока они могут прийти: ждём ответ или сервер не успел (partial). */
const productsSlot = computed(
  () =>
    !idle.value &&
    !view.value?.legacy &&
    (products.value.length > 0 || pending.value || Boolean(view.value?.partial)),
)
const showSkeleton = computed(
  () =>
    skeleton.value &&
    (productsStale.value || !products.value.length || Boolean(view.value?.partial)),
)
// Контур фида: подборка «по просмотренному» — персонализация по истории; на узкой витрине скрыта.
const recsShown = computed(() =>
  recs.value?.items?.length && (SHOWCASE.personalization || recs.value.basis !== 'viewed')
    ? recs.value
    : null,
)

/** Дополнения к первому товару (§16.8) — в нижнем блоке панели; у товаров прежнего запроса не показываем. */
const together = computed<PanelTogether[]>(() =>
  bottomTogether(
    idle.value || productsStale.value ? [] : (view.value?.together ?? []),
    recsShown.value,
    limits.value.recs,
  ),
)

const options = computed<PanelOption[]>(() =>
  buildOptions({
    idle: idle.value,
    recent: recent.value,
    popular: popular.value,
    view: idle.value ? null : view.value,
    productsStale: productsStale.value,
    recs: recsShown.value,
    recsLimit: limits.value.recs,
    mobile: !wide.value,
    withAll: !idle.value,
    legacyRows: !idle.value && legacyGroups.value ? legacyGroups.value : undefined,
  }),
)
const optIndex = computed(() => new Map(options.value.map((o, i) => [o.id, i])))
const activeOption = computed(() => options.value[active.value] ?? null)
const oid = (kind: string, i: number) => `panel-opt-${kind}-${i}`
const isActive = (id: string) => activeOption.value?.id === id
const has = (id: string) => optIndex.value.has(id)
function pick(id: string) {
  const opt = options.value[optIndex.value.get(id) ?? -1]
  if (opt) activate(opt)
}

const emptyLine = computed(
  () =>
    !idle.value &&
    !pending.value &&
    (failed.value || (view.value ? isEmptyView(view.value) && !legacyGroups.value?.length : false)),
)
/**
 * Ни подсказок, ни товаров (обычно после уточнения, под которое быстрый поиск ничего не нашёл): вместо пустой
 * панели — объяснение и снятие уточнения. Производители, категории и «Все результаты» остаются ниже.
 */
const noMatches = computed(
  () =>
    !idle.value &&
    !pending.value &&
    !failed.value &&
    !productsStale.value &&
    view.value !== null &&
    !view.value.legacy &&
    !view.value.partial &&
    !view.value.suggestions.length &&
    !products.value.length,
)
const appliedNames = computed(() =>
  applied.value.map((r) => `«${r.label.replace(/^\+\s*/, '')}»`).join(', '),
)
function clearRefinements() {
  for (const r of [...applied.value]) onRefinementRemove(r)
}

/** Контур ТЗ: строка «Найти товары по ТЗ» в панели под пустой строкой; при вводе уступает место подсказкам. */
const tzHint = computed(() => typed.value.length === 0 && !chipShown.value)

const hasContent = computed(() => {
  if (idle.value) return tzHint.value || options.value.length > 0
  return Boolean(view.value) || failed.value || applied.value.length > 0
})
const visible = computed(() => open.value && hasContent.value)
const twoColumns = computed(() => wide.value && productsSlot.value)

watch(options, () => {
  if (active.value >= options.value.length) active.value = -1
})

// ── Намерение и события (§11.1) ──
const intent = ref<string | null>(null)
let intentTyped = false
const impressions = new Set<string>()

function ensureIntent(): string {
  if (!intent.value) {
    intent.value = newIntentId()
    intentTyped = false
    impressions.clear()
    track('search_opened', { payload: { state: typed.value ? 'text' : 'empty' } })
  }
  return intent.value
}

function track(type: EventType, fields: EventFields = {}) {
  const ev = buildEvent(
    type,
    {
      requestId: null,
      sessionId: sessionId.value,
      clientId: props.clientId,
      intentId: intent.value,
    },
    fields,
  )
  // Служебное событие не мешает поиску: ошибка записи не показывается.
  postEvent(ev).catch(() => undefined)
}

/** Исход намерения: после него следующее открытие начинает новое намерение. */
function finish(outcome: string, payload: Record<string, unknown> = {}) {
  track('search_commit', { payload: { outcome, prefix_len: typed.value.length, ...payload } })
  intent.value = null
}

function abandon() {
  if (intent.value && intentTyped)
    track('search_abandon', { payload: { prefix_len: typed.value.length } })
  intent.value = null
}

// ── Сеть ──
const seq = new Sequencer()
let timer: ReturnType<typeof setTimeout> | undefined
let skelTimer: ReturnType<typeof setTimeout> | undefined
let retryTimer: ReturnType<typeof setTimeout> | undefined
let ctrl: AbortController | null = null
let startedAt = 0

function stopNetwork() {
  clearTimeout(timer)
  clearTimeout(skelTimer)
  clearTimeout(retryTimer)
  ctrl?.abort()
  ctrl = null
}

function show(resp: PanelSuggestResponse, q: string) {
  response.value = resp
  view.value = toPanelView(resp, limits.value)
  viewQuery.value = q
  pending.value = false
  failed.value = false
  skeleton.value = false
  clearTimeout(skelTimer)
  if (view.value.partial) skeleton.value = true
  impression(q)
}

function impression(q: string) {
  const v = view.value
  if (!v || !intent.value) return
  const key = cacheKey(q)
  if (impressions.has(key)) return
  impressions.add(key)
  const sections = [
    v.refinements.length && 'refinements',
    v.suggestions.length && 'suggestions',
    v.categories.length && 'categories',
    v.vendors.length && 'vendors',
    v.products.length && 'products',
  ].filter(Boolean)
  const started = startedAt
  void nextTick(() =>
    track('suggest_impression', {
      payload: {
        prefix_len: typed.value.length,
        sections,
        items: options.value.length,
        server_ms: v.serverMs,
        render_ms: started ? Math.round(performance.now() - started) : null,
        partial: v.partial,
      },
    }),
  )
}

const cache = panelCache

function schedule() {
  const q = effectiveQuery.value
  requested.value = q
  stopNetwork()
  if (idle.value) {
    pending.value = false
    skeleton.value = false
    return
  }
  startedAt = performance.now()
  const hit = cache.get(cacheKey(q))
  if (hit) {
    show(hit, q)
    return
  }
  pending.value = true
  skelTimer = setTimeout(() => {
    if (pending.value) skeleton.value = true
  }, PANEL_SKELETON_MS)
  timer = setTimeout(() => void fetchPanel(q, false), PANEL_DEBOUNCE_MS)
}

async function fetchPanel(q: string, retry: boolean) {
  const n = seq.next()
  const c = new AbortController()
  ctrl = c
  try {
    if (!sessionId.value) await app.loadMeta()
    const resp = await getPanelSuggest(
      { q, intentId: intent.value ?? '', sessionId: sessionId.value, clientId: props.clientId },
      { signal: c.signal },
    )
    if (!seq.isLatest(n) || cacheKey(requested.value) !== cacheKey(q)) return
    const partial = Boolean(resp?.meta?.partial)
    if (!partial) cache.set(cacheKey(q), resp)
    show(resp ?? {}, q)
    // Товары не успели на сервере: кэш там дописывается в фоне — один дозапрос в то же место.
    if (partial && !retry)
      retryTimer = setTimeout(() => {
        if (cacheKey(requested.value) === cacheKey(q)) void fetchPanel(q, true)
      }, 300)
    else if (partial) skeleton.value = false
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return
    if (!seq.isLatest(n)) return
    // Сбой: подсказки скрыты, строка «Нажмите Enter…»; поиск по Enter работает.
    pending.value = false
    skeleton.value = false
    failed.value = true
    response.value = null
    view.value = null
    viewQuery.value = q
  }
}

let popularLoaded = false
async function loadPopular() {
  if (popularLoaded) return
  popularLoaded = true
  if (popularCache.value) {
    popular.value = popularCache.value
    return
  }
  try {
    if (!sessionId.value) await app.loadMeta()
    const resp = await getPanelSuggest({
      q: '',
      intentId: intent.value ?? '',
      sessionId: sessionId.value,
      clientId: props.clientId,
    })
    const list = Array.isArray(resp?.suggestions)
      ? resp.suggestions.filter((s) => s?.text).slice(0, 5)
      : []
    popularCache.value = list
    popular.value = list
  } catch {
    popular.value = []
  }
}

// ── Рекомендации (§11.3): на сессию, по ключу оснований — не на каждое нажатие ──
const exactProduct = computed(() =>
  !idle.value && !productsStale.value
    ? (view.value?.products.find((p) => p.sku_exact) ?? null)
    : null,
)
const confidentCategory = computed(() => {
  if (idle.value || productsStale.value) return null
  // Уверенная категория (RESEARCH А): доля от 0,65; без доли — единственная показанная.
  const cats = view.value?.categories ?? []
  const top = cats[0]
  if (!top) return null
  if (typeof top.share === 'number') return top.share >= 0.65 ? top.id : null
  return cats.length === 1 ? top.id : null
})
const recsKey = computed(() => {
  const ids = spec.lines.map((l) => l.product_id)
  if (!ids.length && !exactProduct.value && !confidentCategory.value) return ''
  return JSON.stringify({
    ids,
    exact: exactProduct.value?.product_id ?? null,
    cat: confidentCategory.value,
  })
})
let recsCtrl: AbortController | null = null

async function loadRecs(key: string) {
  recsCtrl?.abort()
  recsCtrl = null
  if (!key) {
    recs.value = null
    return
  }
  const hit = recsCache.get(key)
  if (hit !== undefined) {
    recs.value = hit
    recsImpression()
    return
  }
  const c = new AbortController()
  recsCtrl = c
  try {
    const parsed = JSON.parse(key) as { ids: string[]; exact: string | null; cat: string | null }
    const resp = await getPanelRecs(
      {
        sessionId: sessionId.value,
        intentId: intent.value ?? '',
        spec: parsed.ids,
        exact: parsed.exact,
        category: parsed.cat,
      },
      { signal: c.signal },
    )
    if (recsCtrl !== c) return
    const value = resp && Array.isArray(resp.items) && resp.items.length ? resp : null
    recsCache.set(key, value)
    recs.value = value
    recsImpression()
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return
    // Сбой блока рекомендаций не мешает подсказкам: блок скрыт.
    if (recsCtrl === c) recs.value = null
  }
}

function recsImpression() {
  const r = recs.value
  if (!r || !open.value || !intent.value) return
  const ids = r.items.slice(0, limits.value.recs).map((i) => i.product_id)
  const key = `recs:${r.basis}:${ids.join(',')}`
  if (impressions.has(key)) return
  impressions.add(key)
  track('recs_impression', { payload: { basis: r.basis, items: ids } })
}

watch(
  () => [open.value, recsKey.value] as const,
  ([isOpen, key]) => {
    if (isOpen) void loadRecs(key)
  },
)

// ── Ввод и открытие ──
function openPanel() {
  ensureIntent()
  open.value = true
  if (idle.value) void loadPopular()
  else if (!view.value || cacheKey(requested.value) !== cacheKey(effectiveQuery.value)) schedule()
}

// Панель открывается действием человека (нажатие в строке, набор, стрелка), а не автофокусом при загрузке:
// иначе затемнение закрыло бы выдачу сразу после перехода по ссылке.
function onPointerDown() {
  if (!open.value) openPanel()
}

function onInput(e: Event) {
  const el = e.target as HTMLInputElement
  text.value = el.value
  if ((e as InputEvent).isComposing) return
  typedChanged()
}

function typedChanged() {
  ensureIntent()
  intentTyped = true
  open.value = true
  active.value = -1
  if (idle.value) {
    stopNetwork()
    pending.value = false
    skeleton.value = false
    void loadPopular()
    return
  }
  schedule()
}

function close(reason: 'escape' | 'blur' | 'outcome') {
  if (!open.value) return
  open.value = false
  active.value = -1
  if (reason !== 'outcome') abandon()
}

function onFocusOut(e: FocusEvent) {
  const next = e.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  close('blur')
}

function onKeydown(e: KeyboardEvent) {
  if (e.isComposing) return
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowUp': {
      e.preventDefault()
      if (!visible.value) {
        openPanel()
        return
      }
      active.value = moveActive(active.value, options.value.length, e.key === 'ArrowDown' ? 1 : -1)
      void nextTick(() =>
        document.getElementById(activeOption.value?.id ?? '')?.scrollIntoView({ block: 'nearest' }),
      )
      return
    }
    case 'Enter': {
      e.preventDefault()
      const opt = visible.value ? activeOption.value : null
      if (opt) activate(opt)
      else commit('enter')
      return
    }
    case 'Escape': {
      if (visible.value) {
        e.preventDefault()
        close('escape')
      }
      return
    }
    case 'Delete': {
      const opt = activeOption.value
      if (visible.value && opt?.kind === 'recent' && opt.text) {
        e.preventDefault()
        removeRecent(opt.text)
      }
      return
    }
  }
}

// ── Действия ──
function rememberRecent(q: string) {
  recent.value = withRecent(recent.value, q)
  saveRecent(storage(), recent.value)
}

function removeRecent(q: string) {
  recent.value = withoutRecent(recent.value, q)
  saveRecent(storage(), recent.value)
  track('history_clear', { payload: { value: 'one' } })
  inputRef.value?.focus()
}

function clearRecent() {
  recent.value = []
  saveRecent(storage(), [])
  track('history_clear', { payload: { value: 'all' } })
  inputRef.value?.focus()
}

function send(
  q: string,
  source: string,
  filters: SearchFilter[],
  outcome: string,
  extra: Record<string, unknown> = {},
) {
  const query = q.trim()
  if (!query) {
    inputRef.value?.focus()
    return
  }
  const intentId = intent.value ?? ensureIntent()
  stopNetwork()
  rememberRecent(query)
  finish(outcome, { source, ...extra })
  text.value = query
  applied.value = []
  close('outcome')
  emit('submit', { q: query, filters, intentId, source })
}

function commit(source: string) {
  send(text.value, source, refinementFilters(applied.value), 'commit')
}

function activate(opt: PanelOption) {
  const prefixLen = typed.value.length
  switch (opt.kind) {
    case 'recent':
    case 'popular':
    case 'legacy':
      track(opt.kind === 'legacy' ? 'suggest_pick' : 'suggestion_select', {
        position: opt.position,
        payload: {
          position: opt.position,
          prefix_len: prefixLen,
          kind: opt.kind,
          type: opt.legacyType ?? null,
        },
      })
      send(opt.text ?? opt.label, opt.kind === 'legacy' ? 'suggestion' : opt.kind, [], 'suggestion')
      return
    case 'suggestion': {
      const s = opt.suggestion as PanelSuggestion
      track('suggestion_select', {
        position: opt.position,
        payload: {
          position: opt.position,
          prefix_len: prefixLen,
          kind: s.kind,
          source: s.source ?? null,
        },
      })
      if (s.kind === 'category' && s.category_id) {
        const q = s.text.split(' — ')[0] ?? s.text
        send(q, 'suggestion', [{ key: 'category', op: 'eq', value: s.category_id }], 'category')
        return
      }
      send(s.text, 'suggestion', refinementFilters(applied.value), 'suggestion')
      return
    }
    case 'category': {
      const c = opt.category
      if (!c) return
      track('category_select', { position: opt.position, payload: { value: c.id } })
      send(
        text.value,
        'category',
        [...refinementFilters(applied.value), { key: 'category', op: 'eq', value: c.id }],
        'category',
      )
      return
    }
    case 'vendor': {
      const v = opt.vendor
      if (!v) return
      track('vendor_select', { position: opt.position, payload: { value: v.id } })
      send(
        text.value,
        'vendor',
        [...refinementFilters(applied.value), { key: 'vendor', op: 'eq', value: v.id }],
        'vendor',
      )
      return
    }
    case 'product': {
      const p = opt.product as PanelProduct
      track('instant_product_click', {
        productId: p.product_id,
        position: opt.position,
        payload: {
          family_id: p.family_id,
          position: opt.position,
          prefix_len: prefixLen,
          exact: p.sku_exact,
        },
      })
      send(p.kind === 'hardware' && p.mpn ? p.mpn : p.title, 'product', [], 'product', {
        family_id: p.family_id,
      })
      return
    }
    case 'product_add': {
      const p = opt.product as PanelProduct
      addProduct(p, opt.position)
      return
    }
    case 'together': {
      // Как нажатие на товар панели: поиск по названию позиции, событие — с пометкой «вместе с этим».
      const t = opt.together as PanelTogether
      track('instant_product_click', {
        productId: t.product_id,
        position: opt.position,
        payload: {
          family_id: t.family_id,
          position: opt.position,
          prefix_len: prefixLen,
          exact: false,
          source: 'together',
          anchor_family_id: t.anchor_family_id,
        },
      })
      send(t.title || t.family_name, 'product', [], 'product', { family_id: t.family_id })
      return
    }
    case 'rec':
      openRec(opt.rec as Companion, opt.position)
      return
    case 'rec_add':
      addRec(opt.rec as Companion, opt.position)
      return
    case 'all':
      commit('all_results')
      return
  }
}

function addProduct(p: PanelProduct, position: number) {
  spec.add({
    product_id: p.product_id,
    title: p.title,
    vendor: p.vendor,
    category: p.category,
    license_line: p.license_line,
    price_rub: p.price_from_rub,
    price_mode: p.price_mode || 'fixed',
    quantity: 1,
    origin: 'search',
  })
  track('instant_product_click', {
    productId: p.product_id,
    position,
    payload: {
      family_id: p.family_id,
      position,
      prefix_len: typed.value.length,
      action: 'spec_add',
      exact: true,
    },
  })
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: p.title,
    action: { label: 'Открыть', run: () => (spec.open = true) },
  })
  inputRef.value?.focus()
}

function openRec(c: Companion, position: number) {
  track('recs_click', {
    productId: c.product_id,
    position,
    payload: { basis: recs.value?.basis ?? null },
  })
  send(c.title, 'recs', [], 'product')
}

function addRec(c: Companion, position: number) {
  spec.add(lineFromCompanion(c, c.suggested_quantity ?? 1, exactProduct.value?.title ?? ''))
  track('recs_add', {
    productId: c.product_id,
    position,
    payload: { basis: recs.value?.basis ?? null, class: c.class },
  })
  toast.add({
    tone: 'ok',
    title: 'Добавлено в спецификацию',
    detail: c.title,
    action: { label: 'Открыть', run: () => (spec.open = true) },
  })
  inputRef.value?.focus()
}

function onRefinement(r: PanelRefinement) {
  ensureIntent()
  track('refinement_apply', { payload: { field: r.field, value: r.value, place: 'panel' } })
  if (isStructured(r)) applied.value = [...applied.value, r]
  else text.value = r.query || `${text.value.trim()} ${r.label.replace(/^\+\s*/, '')}`
  active.value = -1
  schedule()
  inputRef.value?.focus()
}

function onRefinementRemove(r: PanelRefinement) {
  track('refinement_remove', { payload: { field: r.field, value: r.value, place: 'panel' } })
  applied.value = applied.value.filter((a) => !sameRefinement(a, r))
  active.value = -1
  schedule()
  inputRef.value?.focus()
}

function pickTz() {
  close('outcome')
  tz.openPicker()
}

function clearText() {
  if (chipShown.value) {
    emit('tzExit')
    inputRef.value?.focus()
    return
  }
  text.value = ''
  applied.value = []
  stopNetwork()
  typedChanged()
  inputRef.value?.focus()
}

function focus() {
  inputRef.value?.focus()
}

// Текст из адреса (переход по ссылке, «Назад») — это не набор: панель не открываем, прежний ответ сбрасываем.
watch(model, (v) => {
  if (v === text.value) return
  text.value = v
  if (document.activeElement !== inputRef.value) {
    view.value = null
    viewQuery.value = ''
  }
})

watch(limits, () => {
  if (response.value) view.value = toPanelView(response.value, limits.value)
})

onBeforeUnmount(() => {
  stopNetwork()
  recsCtrl?.abort()
})
defineExpose({ focus })

const allLabel = computed(() => {
  const n = view.value?.totalHint
  return n && n > 0 && !productsStale.value
    ? `${GROUP_LABELS.all} (${formatInt(n)})`
    : GROUP_LABELS.all
})
const parts = (s: PanelSuggestion) => suggestionParts(s.text, typed.value, s.highlight)
</script>

<script lang="ts">
import { ref as vueRef } from 'vue'
import type {
  PanelSuggestResponse as Resp,
  PanelRecs as Recs,
  PanelSuggestion as Sugg,
} from '@/api/typesPanel'
import { ResponseCache as Cache } from '@/lib/panel'
import type { SearchFilter as Filter } from '@/api/typesP2'

export interface PanelSubmit {
  q: string
  filters: Filter[]
  intentId: string | null
  /** enter | suggestion | recent | popular | category | vendor | product | recs | all_results */
  source: string
}

// Кэш ответов — на сессию вкладки: общий для всех экземпляров строки.
const panelCache = new Cache<Resp>(200)
const recsCache = new Map<string, Recs | null>()
const popularCache = vueRef<Sugg[] | null>(null)
</script>

<template>
  <div ref="rootRef" :class="['sp', { 'sp--open': visible }]" @focusout="onFocusOut">
    <form class="sp__form" role="search" @submit.prevent="commit('enter')">
      <div class="sp__field">
        <Search :size="18" aria-hidden="true" class="sp__icon" />
        <span
          v-if="chipShown"
          class="sp__tzchip"
          :title="tzFile ?? ''"
          data-testid="tz-file-chip"
          @click="focus"
        >
          <FileText :size="14" aria-hidden="true" class="sp__tzchip-icon" />
          <span class="sp__tzchip-name">{{ tzFile }}</span>
        </span>
        <input
          ref="inputRef"
          :value="text"
          class="sp__input"
          type="text"
          role="combobox"
          inputmode="search"
          enterkeyhint="search"
          autocomplete="off"
          autocapitalize="off"
          spellcheck="false"
          autofocus
          aria-label="Поиск по каталогу"
          maxlength="300"
          aria-autocomplete="list"
          :aria-expanded="visible"
          :aria-controls="listId"
          :aria-activedescendant="visible && activeOption ? activeOption.id : undefined"
          :placeholder="chipShown ? '' : 'Название, задача своими словами или артикул'"
          @pointerdown="onPointerDown"
          @input="onInput"
          @compositionend="typedChanged"
          @keydown="onKeydown"
        />
        <button
          v-if="text || chipShown"
          type="button"
          class="btn btn--ghost btn--icon sp__clear"
          :aria-label="chipShown ? TZ_TEXT.exit : 'Очистить строку'"
          data-testid="search-clear"
          @click="clearText"
        >
          <!-- Крест в своём поле вдвое меньше рамки значка: крупнее, чтобы на вид был вровень со скрепкой. -->
          <X :size="26" :stroke-width="1.5" absolute-stroke-width aria-hidden="true" />
        </button>
      </div>
      <UiTooltip :text="TZ_TEXT.tooltip" side="bottom">
        <button
          type="button"
          class="btn btn--ghost btn--icon sp__attach"
          :aria-label="TZ_TEXT.accessibleLabel"
          data-testid="tz-attach"
          @click="pickTz"
        >
          <Paperclip :size="18" aria-hidden="true" />
        </button>
      </UiTooltip>
      <button
        type="submit"
        class="btn btn--primary sp__submit"
        aria-label="Найти"
        :disabled="chipShown"
      >
        <span class="sp__submit-text">Найти</span>
        <Search :size="20" aria-hidden="true" class="sp__submit-icon" />
      </button>
    </form>

    <div v-if="visible && desktop" class="sp__scrim" aria-hidden="true" />

    <div v-if="visible" class="sp__pop" @mousedown.prevent>
      <!-- Бейджи-уточнения: кнопки вне списка, до них доходят клавишей Tab. -->
      <div
        v-if="!idle && (applied.length || offered.length)"
        class="sp__badges"
        role="group"
        aria-label="Уточнения запроса"
      >
        <span class="sp__badges-label t-sm" aria-hidden="true">Уточнить</span>
        <button
          v-for="r in applied"
          :key="`a-${r.field}-${String(r.value)}`"
          type="button"
          class="sp__badge sp__badge--on"
          :aria-label="`Снять уточнение: ${r.label.replace(/^\+\s*/, '')}`"
          @click="onRefinementRemove(r)"
        >
          {{ r.label.replace(/^\+\s*/, '') }}
          <X :size="14" aria-hidden="true" />
        </button>
        <button
          v-for="r in offered.slice(0, Math.max(0, limits.refinements - applied.length))"
          :key="`o-${r.label}`"
          type="button"
          class="sp__badge"
          :aria-label="`Уточнить: ${r.label.replace(/^\+\s*/, '')}`"
          @click="onRefinement(r)"
        >
          <Plus :size="14" aria-hidden="true" />
          {{ r.label.replace(/^\+\s*/, '') }}
        </button>
      </div>

      <button
        v-if="tzHint"
        type="button"
        class="sp__tzhint"
        data-testid="tz-suggestion"
        @click="pickTz"
      >
        <Paperclip :size="16" aria-hidden="true" class="sp__opt-icon" />
        <span class="sp__tzhint-text">
          <strong>{{ TZ_TEXT.suggestionTitle }}</strong>
          <span class="t-sm sp__tzhint-desc">{{ TZ_TEXT.suggestionDescription }}</span>
        </span>
      </button>

      <div v-if="noMatches" class="sp__none" role="status" data-testid="panel-no-matches">
        <span class="sp__none-badge" aria-hidden="true"><SearchX :size="22" /></span>
        <div class="sp__none-text">
          <p class="sp__none-title">
            {{
              applied.length
                ? `С уточнением ${appliedNames} быстрых совпадений нет`
                : 'Быстрых совпадений нет'
            }}
          </p>
          <p class="sp__none-hint t-sm t-muted">
            {{
              applied.length
                ? 'Уберите уточнение или посмотрите все результаты по каталогу.'
                : 'Посмотрите все результаты по каталогу или измените запрос.'
            }}
          </p>
        </div>
        <button
          v-if="applied.length"
          type="button"
          class="btn btn--secondary btn--sm sp__none-action"
          @click="clearRefinements"
        >
          {{ applied.length > 1 ? 'Убрать уточнения' : 'Убрать уточнение' }}
        </button>
      </div>

      <div
        :id="listId"
        role="listbox"
        aria-label="Подсказки поиска"
        :class="['sp__card', { 'sp__card--two': twoColumns, 'sp__card--mobile': !wide }]"
      >
        <!-- F32: пустая строка — недавние и популярные -->
        <template v-if="idle">
          <div
            v-if="recent.length"
            class="sp__group"
            role="group"
            :aria-labelledby="`${uid}-g-recent`"
          >
            <div class="sp__label">
              <span :id="`${uid}-g-recent`">{{ GROUP_LABELS.recent }}</span>
              <button
                type="button"
                class="link sp__clear-all t-xs"
                tabindex="-1"
                @click="clearRecent"
              >
                Очистить
              </button>
            </div>
            <div v-for="(q, i) in recent" :key="`r-${q}`" class="sp__row-wrap" role="none">
              <div
                :id="oid('recent', i)"
                role="option"
                :aria-selected="isActive(oid('recent', i))"
                :class="['sp__opt', { 'is-active': isActive(oid('recent', i)) }]"
                @click="pick(oid('recent', i))"
              >
                <Clock :size="16" aria-hidden="true" class="sp__opt-icon" />
                <span class="sp__opt-text">{{ q }}</span>
              </div>
              <button
                type="button"
                class="btn btn--ghost btn--icon btn--sm sp__row-x"
                tabindex="-1"
                :aria-label="`Удалить из недавних: ${q}`"
                @click="removeRecent(q)"
              >
                <X :size="14" aria-hidden="true" />
              </button>
            </div>
          </div>
          <div
            v-if="popular.length"
            class="sp__group"
            role="group"
            :aria-labelledby="`${uid}-g-popular`"
          >
            <div :id="`${uid}-g-popular`" class="sp__label">{{ GROUP_LABELS.popular }}</div>
            <div
              v-for="(s, i) in popular"
              :id="oid('popular', i)"
              :key="`p-${s.text}`"
              role="option"
              :aria-selected="isActive(oid('popular', i))"
              :class="['sp__opt', { 'is-active': isActive(oid('popular', i)) }]"
              @click="pick(oid('popular', i))"
            >
              <TrendingUp :size="16" aria-hidden="true" class="sp__opt-icon" />
              <span class="sp__opt-text">{{ s.text }}</span>
            </div>
          </div>
        </template>

        <!-- Сервер контура коммерции: прежние группы -->
        <template v-else-if="legacyGroups">
          <div
            v-for="(g, gi) in legacyGroups"
            :key="g.type"
            class="sp__group"
            role="group"
            :aria-labelledby="`${uid}-lg-${gi}`"
          >
            <div :id="`${uid}-lg-${gi}`" class="sp__label">{{ g.label }}</div>
            <div
              v-for="row in g.rows"
              :id="oid('legacy', row.position - 1)"
              :key="row.key"
              role="option"
              :aria-selected="isActive(oid('legacy', row.position - 1))"
              :class="['sp__opt', { 'is-active': isActive(oid('legacy', row.position - 1)) }]"
              @click="pick(oid('legacy', row.position - 1))"
            >
              <span class="sp__opt-text">{{ row.text }}</span>
            </div>
          </div>
        </template>

        <template v-else-if="view">
          <p v-if="view.corrected" class="sp__corrected t-sm" role="status">
            Показаны варианты для «<strong>{{ view.corrected.to }}</strong
            >»
          </p>
          <div class="sp__left">
            <div
              v-if="view.suggestions.length"
              class="sp__group sp__sec-sugg"
              role="group"
              :aria-labelledby="`${uid}-g-sugg`"
            >
              <div :id="`${uid}-g-sugg`" class="sp__label">{{ GROUP_LABELS.suggestions }}</div>
              <div
                v-for="(s, i) in view.suggestions"
                :id="oid('suggestion', i)"
                :key="`s-${s.text}`"
                role="option"
                :aria-selected="isActive(oid('suggestion', i))"
                :class="[
                  'sp__opt',
                  {
                    'is-active': isActive(oid('suggestion', i)),
                    'sp__opt--cat': s.kind === 'category',
                  },
                ]"
                @click="pick(oid('suggestion', i))"
              >
                <Folder
                  v-if="s.kind === 'category'"
                  :size="16"
                  aria-hidden="true"
                  class="sp__opt-icon"
                />
                <Search v-else :size="16" aria-hidden="true" class="sp__opt-icon" />
                <span class="sp__opt-text">
                  <template v-for="(p, pi) in parts(s)" :key="pi">
                    <strong v-if="p.continuation" class="sp__cont">{{ p.text }}</strong>
                    <template v-else>{{ p.text }}</template>
                  </template>
                </span>
              </div>
            </div>
            <div
              v-if="view.categories.length"
              class="sp__group sp__sec-cats"
              role="group"
              :aria-labelledby="`${uid}-g-cats`"
            >
              <div :id="`${uid}-g-cats`" class="sp__label">{{ GROUP_LABELS.categories }}</div>
              <div
                v-for="(c, i) in view.categories"
                :id="oid('category', i)"
                :key="`c-${c.id}`"
                role="option"
                :aria-selected="isActive(oid('category', i))"
                :class="['sp__opt', { 'is-active': isActive(oid('category', i)) }]"
                @click="pick(oid('category', i))"
              >
                <Folder :size="16" aria-hidden="true" class="sp__opt-icon" />
                <span class="sp__opt-text">{{ c.name }}</span>
              </div>
            </div>
            <div
              v-if="view.vendors.length"
              class="sp__group sp__sec-vendors"
              role="group"
              :aria-labelledby="`${uid}-g-vendors`"
            >
              <div :id="`${uid}-g-vendors`" class="sp__label">{{ GROUP_LABELS.vendors }}</div>
              <div
                v-for="(v, i) in view.vendors"
                :id="oid('vendor', i)"
                :key="`v-${v.id}`"
                role="option"
                :aria-selected="isActive(oid('vendor', i))"
                :class="['sp__opt', { 'is-active': isActive(oid('vendor', i)) }]"
                @click="pick(oid('vendor', i))"
              >
                <VendorLogo :vendor="v.name" class="sp__vendor-logo" />
                <span class="sp__opt-text">{{ v.name }}</span>
              </div>
            </div>
          </div>

          <div
            v-if="productsSlot"
            class="sp__group sp__right sp__sec-products"
            role="group"
            :aria-labelledby="`${uid}-g-products`"
            :aria-busy="showSkeleton"
          >
            <div
              v-show="products.length || showSkeleton"
              :id="`${uid}-g-products`"
              class="sp__label"
            >
              {{ GROUP_LABELS.products }}
            </div>
            <div
              v-if="showSkeleton && (!products.length || view.partial)"
              class="sp__grid"
              aria-hidden="true"
            >
              <div v-for="n in limits.products" :key="n" class="sp__prod sp__prod--skel">
                <span class="skeleton sp__skel-media" />
                <span class="sp__skel-text">
                  <span class="skeleton sp__skel-line" />
                  <span class="skeleton sp__skel-line sp__skel-line--short" />
                </span>
              </div>
            </div>
            <div v-else :class="['sp__grid', { 'sp__grid--stale': productsStale }]">
              <div
                v-for="(p, i) in products"
                :key="`pr-${p.product_id}`"
                class="sp__prod-wrap"
                role="none"
              >
                <div
                  v-if="has(oid('product', i))"
                  :id="oid('product', i)"
                  role="option"
                  :aria-selected="isActive(oid('product', i))"
                  :class="['sp__prod', { 'is-active': isActive(oid('product', i)) }]"
                  @click="pick(oid('product', i))"
                >
                  <PanelProductCard :product="p" />
                </div>
                <div v-else class="sp__prod sp__prod--stale" aria-disabled="true">
                  <PanelProductCard :product="p" />
                </div>
                <div
                  v-if="p.sku_exact && has(oid('product_add', 0))"
                  :id="oid('product_add', 0)"
                  role="option"
                  :aria-selected="isActive(oid('product_add', 0))"
                  :aria-label="`В спецификацию: ${p.title}`"
                  :class="['sp__add', { 'is-active': isActive(oid('product_add', 0)) }]"
                  @click="pick(oid('product_add', 0))"
                >
                  <Plus :size="16" aria-hidden="true" /> В спецификацию
                </div>
              </div>
            </div>
          </div>
        </template>

        <p v-if="emptyLine && !noMatches" class="sp__empty t-sm t-muted sp__sec-empty">
          Нажмите Enter, чтобы искать по всему каталогу
        </p>

        <!-- Блок рекомендаций: заголовок по основанию, «почему» — ссылка на /rules -->
        <div
          v-if="together.length"
          class="sp__group sp__recs sp__sec-recs"
          role="group"
          :aria-labelledby="`${uid}-g-together`"
          data-testid="panel-together"
        >
          <div class="sp__glabel sp__glabel--strong t-sm" role="presentation">
            <span :id="`${uid}-g-together`">{{ GROUP_LABELS.interest }}</span>
          </div>
          <div class="sp__recs-list sp__recs-list--together">
            <div v-for="(t, j) in together" :key="`tg-${t.product_id}`" class="sp__rec-wrap">
              <VendorLogo :vendor="t.vendor" class="sp__rec-logo" />
              <div
                :id="oid('together', j)"
                role="option"
                :aria-selected="isActive(oid('together', j))"
                :aria-label="`Вместе с этим: ${t.family_name || t.title}`"
                :class="['sp__rec', { 'is-active': isActive(oid('together', j)) }]"
                :data-level="t.level"
                @click="pick(oid('together', j))"
              >
                <span class="sp__rec-title t-sm">{{ t.family_name || t.title }}</span>
                <span class="sp__rec-meta t-xs t-muted">
                  <strong
                    v-if="t.price_rub != null && t.price_rub > 0"
                    class="sp__rec-price t-num"
                    >{{ formatPrice(t.price_rub) }}</strong
                  >
                  <span>{{ togetherReason(t) }}</span>
                </span>
              </div>
            </div>
          </div>
        </div>
        <div
          v-else-if="recsShown"
          class="sp__group sp__recs sp__sec-recs"
          role="group"
          :aria-labelledby="`${uid}-g-recs`"
        >
          <div class="sp__label">
            <span :id="`${uid}-g-recs`">{{ recsShown.title || GROUP_LABELS.recs }}</span>
            <RouterLink
              to="/rules"
              class="sp__why"
              tabindex="-1"
              :title="recsShown.why || 'Почему показано'"
              aria-label="Почему показано: правила рекомендаций"
            >
              <CircleHelp :size="14" aria-hidden="true" />
            </RouterLink>
          </div>
          <p v-if="recsShown.why" class="sp__why-line t-xs t-muted">{{ recsShown.why }}</p>
          <div class="sp__recs-list">
            <div
              v-for="(c, i) in recsShown.items.slice(0, limits.recs)"
              :key="`rec-${c.product_id}`"
              class="sp__rec-wrap"
              role="none"
            >
              <VendorLogo :vendor="c.vendor" class="sp__rec-logo" />
              <div
                :id="oid('rec', i)"
                role="option"
                :aria-selected="isActive(oid('rec', i))"
                :class="['sp__rec', { 'is-active': isActive(oid('rec', i)) }]"
                @click="pick(oid('rec', i))"
              >
                <span class="sp__rec-title t-sm">{{ c.title }}</span>
                <span class="sp__rec-meta t-xs t-muted">
                  <strong
                    v-if="c.price_rub != null && c.price_rub > 0"
                    class="sp__rec-price t-num"
                    >{{ formatPrice(c.price_rub) }}</strong
                  >
                  <!-- Основание «по категории» уже названо над блоком — под позицией не повторяем. -->
                  <span v-if="recsShown.basis !== 'category_popular' && (c.label || c.reason)">{{
                    c.label || c.reason
                  }}</span>
                </span>
              </div>
              <div
                :id="oid('rec_add', i)"
                role="option"
                :aria-selected="isActive(oid('rec_add', i))"
                :aria-label="`Добавить в спецификацию: ${c.title}`"
                :class="['sp__add', { 'is-active': isActive(oid('rec_add', i)) }]"
                @click="pick(oid('rec_add', i))"
              >
                <Plus :size="18" aria-hidden="true" />
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="!idle"
          :id="oid('all', 0)"
          role="option"
          :aria-selected="isActive(oid('all', 0))"
          :class="['sp__all', 'sp__sec-all', { 'is-active': isActive(oid('all', 0)) }]"
          @click="commit('all_results')"
        >
          <Search :size="16" aria-hidden="true" />
          <span class="t-num">{{ allLabel }}</span>
          <ArrowRight :size="16" aria-hidden="true" class="sp__all-arrow" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sp {
  position: relative;
  flex: 1;
  min-width: 0;
  /* Строка стоит на красной полосе: внутри неё и в панели — обычные цвета темы. */
  --text: var(--field-text);
  --text-2: var(--field-text-2);
  --text-3: var(--field-text-3);
  --surface: var(--field-surface);
  --surface-2: var(--field-surface-2);
  --surface-3: var(--field-surface-3);
  --border: var(--field-border);
  --border-control: var(--field-border-control);
  --focus-ring: var(--field-text);
  color: var(--text);
}
.sp--open {
  z-index: 41;
}
/* Без затемнения (узкий экран) панель при прокрутке уходит под прилипшую шапку, а не наезжает на неё. */
@media (max-width: 1023.98px) {
  .sp--open {
    z-index: 19;
  }
}
.sp__form {
  position: relative;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  box-sizing: border-box;
  min-width: 0;
  min-height: var(--search-h);
  padding: var(--space-1) var(--space-1) var(--space-1) var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--surface);
}
.sp__form:focus-within {
  box-shadow: 0 0 0 3px var(--hero-control);
}
.sp__field {
  position: relative;
  display: flex;
  flex: 1;
  min-width: 0;
  align-items: center;
}
.sp__icon {
  flex: none;
  margin-left: var(--space-3);
  color: var(--text-2);
}
.sp__input {
  flex: 1;
  min-width: 0;
  height: var(--control-h);
  padding: 0 var(--space-2);
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
  outline: none;
}
.sp__input::placeholder {
  color: var(--placeholder);
  opacity: 1;
}
.sp__clear {
  flex: none;
  width: 40px;
  height: 40px;
  margin-right: var(--space-1);
  color: var(--text-2);
}
.sp__clear:hover {
  color: var(--text);
}
.sp__submit {
  align-self: stretch;
  min-height: calc(var(--search-h) - 2 * var(--space-1));
  padding: 0 var(--space-6);
  background: var(--hero-button);
  color: var(--on-hero-button);
}
.sp__submit.btn--primary:hover:not(:disabled) {
  background: var(--hero-button-hover);
}
/* Плашка ТЗ без текста: повторный разбор открытого документа не нужен — «Найти» недоступна. */
.sp__submit:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.sp__submit-icon {
  display: none;
}
/* Контур ТЗ: скрепка — серая кнопка-значок 40 × 40 (на телефоне 44 × 44), главным остаётся «Найти». */
.sp__attach {
  flex: none;
  width: 40px;
  height: 40px;
  color: var(--text-2);
}
.sp__attach:hover {
  color: var(--text);
}
.sp__tzchip {
  display: inline-flex;
  flex: 0 1 auto;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
  max-width: 60%;
  margin-left: var(--space-2);
  padding: 2px var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}
.sp__tzchip-icon {
  flex: none;
  color: var(--text-2);
}
.sp__tzchip-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sp__tzhint {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-3) var(--space-4);
  border: 0;
  border-bottom: var(--border-w) solid var(--border);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.sp__tzhint:hover,
.sp__tzhint:focus-visible {
  background: var(--surface-2);
}
.sp__tzhint-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.sp__tzhint-desc {
  color: var(--text-2);
}
@media (max-width: 767px) {
  .sp__attach,
  .sp__clear {
    width: 44px;
    min-width: 44px;
    height: 44px;
  }
  .sp__clear {
    margin-right: 0;
  }
}
/* Телефон: «Найти» заменяется значком лупы 20 px, область нажатия 44 × 44. */
@media (max-width: 479px) {
  .sp__submit {
    width: 44px;
    min-width: 44px;
    padding: 0;
  }
  .sp__submit-text {
    display: none;
  }
  .sp__submit-icon {
    display: block;
  }
}

.sp__scrim {
  position: fixed;
  inset: 0;
  z-index: 0;
  background: var(--scrim);
}

.sp__pop {
  position: absolute;
  top: calc(100% + var(--space-2));
  left: 0;
  z-index: 1;
  box-sizing: border-box;
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--surface);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-panel);
  box-shadow: var(--shadow-overlay);
}

/* Уточнения — полоса над списком */
.sp__badges {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
}
.sp__badges-label {
  flex: none;
  color: var(--text-3);
}
@media (max-width: 767px) {
  .sp__badges {
    flex-wrap: nowrap;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .sp__badges-label {
    display: none;
  }
}
.sp__badge {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transition:
    border-color var(--dur-fast) var(--ease-standard),
    background-color var(--dur-fast) var(--ease-standard);
}
.sp__badge svg {
  color: var(--accent);
}
.sp__badge:hover {
  border-color: var(--accent);
}
.sp__badge--on {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}
.sp__badge--on svg {
  color: var(--on-accent);
}
.sp__badge:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}

/* Список: слева подсказки на серой подложке, справа товары на белом */
.sp__card {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.sp__card > .sp__group,
.sp__card > .sp__corrected {
  padding: var(--space-2);
}
.sp__card--two {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
  grid-template-areas:
    'corr corr'
    'left right'
    'empty empty'
    'recs recs'
    'all all';
}
.sp__card--two .sp__corrected {
  grid-area: corr;
  border-bottom: var(--border-w) solid var(--border);
}
.sp__card--two .sp__left {
  grid-area: left;
  padding: var(--space-1) 0;
  background: var(--surface-2);
}
.sp__card--two .sp__right {
  grid-area: right;
  min-height: 10rem;
  padding: var(--space-3) var(--space-3);
}
.sp__card--two .sp__sec-empty {
  grid-area: empty;
}
.sp__card--two .sp__recs {
  grid-area: recs;
}
.sp__card--two .sp__all {
  grid-area: all;
}
/* Мобильный порядок: подсказки, товары, категории и производители, «Все результаты», рекомендации. */
.sp__card--mobile .sp__left {
  display: contents;
}
.sp__card--mobile .sp__left > .sp__group {
  padding: var(--space-2);
}
.sp__card--mobile .sp__corrected {
  order: 0;
}
.sp__card--mobile .sp__sec-sugg {
  order: 1;
}
.sp__card--mobile .sp__sec-products {
  order: 2;
}
.sp__card--mobile .sp__sec-cats {
  order: 3;
}
.sp__card--mobile .sp__sec-vendors {
  order: 4;
}
.sp__card--mobile .sp__sec-empty {
  order: 5;
}
.sp__card--mobile .sp__sec-all {
  order: 6;
}
.sp__card--mobile .sp__sec-recs {
  order: 7;
}
.sp__left {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.sp__group {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.sp__label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-3);
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-semibold);
}
.sp__corrected {
  margin: 0;
  padding: var(--space-2) var(--space-4);
  color: var(--text-2);
}
.sp__corrected strong {
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.sp__row-wrap {
  display: flex;
  align-items: center;
  min-width: 0;
}
.sp__row-wrap .sp__opt {
  flex: 1;
}
.sp__opt,
.sp__all,
.sp__add,
.sp__rec,
.sp__prod {
  border: var(--border-w) solid transparent;
  cursor: pointer;
}
.sp__opt {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-height: var(--control-h);
  min-width: 0;
  padding: 0 var(--space-3);
  border-radius: var(--radius-control);
  color: var(--text-2);
}
.sp__opt-icon {
  flex: none;
  color: var(--text-3);
}
.sp__opt-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sp__opt--cat {
  font-style: italic;
}
.sp__cont {
  color: var(--text);
  font-weight: var(--fw-bold);
}
/* Производители — пилюли в строку */
.sp__sec-vendors {
  flex-direction: row;
  flex-wrap: wrap;
  align-content: flex-start;
  gap: var(--space-1);
}
.sp__sec-vendors .sp__label {
  width: 100%;
}
.sp__sec-vendors .sp__opt {
  min-height: var(--control-h-sm);
  gap: var(--space-2);
  /* Значок и текст плашки стоят на тех же вертикалях, что у кнопки «Все результаты». */
  padding-inline: var(--space-4) var(--space-4);
  border-color: var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font-size: var(--fs-sm);
}

/* Товары — строки одной высоты, цены в одну колонку */
.sp__grid {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sp__prod-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.sp__prod {
  min-width: 0;
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-control);
}
.sp__prod--stale,
.sp__grid--stale .sp__prod {
  opacity: 0.45;
  cursor: default;
  pointer-events: none;
}
.sp__prod--skel {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  cursor: default;
}
.sp__skel-media {
  flex: none;
  width: 2.25rem;
  height: 2.25rem;
}
.sp__skel-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
}
.sp__skel-line {
  width: 70%;
  height: var(--skeleton-line);
}
.sp__skel-line--short {
  width: 45%;
}

/* Наведение и выбор: не только цветом — у выбранного клавишами есть рамка */
.sp__opt:hover,
.sp__prod:hover,
.sp__rec:hover {
  background: var(--surface-3);
  color: var(--text);
}
.sp__sec-vendors .sp__opt:hover {
  border-color: var(--accent);
}
.is-active {
  background: var(--surface-3);
  color: var(--text);
}
.sp__opt.is-active,
.sp__prod.is-active,
.sp__rec.is-active,
.sp__add.is-active,
.sp__all.is-active,
.sp__sec-vendors .sp__opt.is-active {
  border-color: var(--accent);
}

/* Одна левая линия на всю панель — как у «Уточнить» (space-4): подписи, значки строк списков, края плашек,
   карточек и кнопки «Все результаты». У строк списков подсветка шире текста, поэтому поле группы у них меньше. */
.sp__card .sp__left > .sp__group.sp__sec-sugg,
.sp__card .sp__left > .sp__group.sp__sec-cats {
  padding: var(--space-2) var(--space-1) 0;
}
.sp__card .sp__left > .sp__group.sp__sec-vendors {
  padding: var(--space-2) var(--space-4) var(--space-3);
}
.sp__card .sp__sec-vendors .sp__label {
  padding-inline: 0;
}

/* «В спецификацию» у точного артикула — широкая кнопка под строкой товара */
.sp__add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  min-height: var(--control-h);
  padding: 0 var(--space-4);
  border-radius: var(--radius-pill);
  border-color: var(--border);
  background: var(--surface);
  color: var(--accent);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.sp__add:hover {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}

/* Рекомендации: три компактные карточки, кнопка «+» внутри карточки */
.sp__card > .sp__recs {
  padding: var(--space-3) var(--space-4);
  border-top: var(--border-w) solid var(--border);
}
.sp__why {
  display: inline-flex;
  color: var(--text-3);
}
.sp__why:hover {
  color: var(--text);
}
.sp__why-line {
  margin: 0;
  padding: 0 var(--space-3) var(--space-2);
}
.sp__recs-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-2);
}
.sp__glabel.sp__glabel--strong {
  margin-bottom: var(--space-2);
  color: var(--text);
  font-weight: var(--fw-bold);
}
.sp__recs-list--together {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.sp__recs-list--together .sp__rec-wrap {
  padding-right: 0;
}
@media (max-width: 767px) {
  .sp__recs-list,
  .sp__recs-list--together {
    grid-template-columns: minmax(0, 1fr);
  }
}
.sp__rec-wrap {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
  padding-right: var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
}
.sp__rec {
  display: flex;
  flex: 1;
  align-self: stretch;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
}
.sp__rec-logo {
  margin-left: var(--space-2);
}
.sp__vendor-logo {
  width: var(--logo-size-sm);
  height: var(--logo-size-sm);
  border: 0;
}
.sp__rec-title {
  color: var(--text);
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.sp__rec-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  column-gap: var(--space-2);
}
.sp__rec-price {
  color: var(--text);
  font-size: var(--fs-sm);
  font-weight: var(--fw-bold);
}
.sp__rec-wrap .sp__add {
  flex: none;
  width: var(--control-h);
  height: var(--control-h);
  padding: 0;
}

.sp__none {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3) var(--space-4);
  padding: var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
}
.sp__none-badge {
  display: grid;
  flex: none;
  place-items: center;
  width: var(--touch-min);
  height: var(--touch-min);
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.sp__none-text {
  flex: 1 1 14rem;
  min-width: 0;
}
.sp__none-title {
  margin: 0;
  font-weight: var(--fw-bold);
  overflow-wrap: anywhere;
}
.sp__none-hint {
  margin: 2px 0 0;
}
.sp__none-action {
  flex: none;
}
.sp__empty {
  margin: 0;
}
.sp__card > .sp__empty {
  padding: var(--space-3) var(--space-4);
}
/* «Все результаты» — главный переход панели */
.sp__all {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-height: var(--touch-min);
  margin: 0 var(--space-4) var(--space-3);
  padding: 0 var(--space-4);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.sp__card--two .sp__all {
  margin-top: var(--space-1);
}
.sp__all-arrow {
  margin-left: auto;
}
.sp__all:hover,
.sp__all.is-active {
  background: var(--accent);
  color: var(--on-accent);
}
.sp__all.is-active {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 1px;
}
.sp__clear-all {
  border: 0;
  background: none;
  padding: 0;
  cursor: pointer;
}
.sp__row-x {
  flex: none;
}
</style>
