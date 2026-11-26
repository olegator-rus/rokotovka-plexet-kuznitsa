<script setup lang="ts">
// Экран «Поиск». Адрес — единственный источник состояния запроса
// (lib/searchState.ts): строка, конфигурация, клиент, уточнения и открытая трасса. Любое изменение
// адреса, кроме трассы, — новый POST /api/search; прошлый запрос отменяется.
import { computed, ref, shallowRef, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { Headset, Info } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { postSearch } from '@/api/search'
import type {
  Chip,
  ClarifyOption,
  ConfigId,
  EventType,
  SearchItem,
  SearchItemBase,
  SearchResponse,
} from '@/api/types'
import ConfigSwitch from '@/components/search/ConfigSwitch.vue'
import ManagerDialog from '@/components/search/ManagerDialog.vue'
import ResultCard, { type ItemAction } from '@/components/search/ResultCard.vue'
import ResultSkeleton from '@/components/search/ResultSkeleton.vue'
import SearchBox from '@/components/search/SearchBox.vue'
import SearchError from '@/components/search/SearchError.vue'
import UnderstandingBar from '@/components/search/UnderstandingBar.vue'
import TracePanel from '@/components/trace/TracePanel.vue'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import { useToast } from '@/components/ui/useToast'
import { DESKTOP_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { buildEvent, type EventFields } from '@/lib/events'
import { formatFound } from '@/lib/format'
import { emptyTitle, splitResults } from '@/lib/results'
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
import type { SuggestRow } from '@/lib/suggest'
import { useAppStore } from '@/stores/app'

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
const box = ref<InstanceType<typeof SearchBox> | null>(null)
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
    const body = buildSearchRequest(s, {
      sessionId: app.sessionId,
      defaultConfig: defaultConfig.value,
    })
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

// Клиент: адрес ↔ переключатель в шапке. Без client в адресе берётся выбранный в шапке.
watch(
  () => state.value.client,
  (c) => {
    if (!onSearchRoute()) return
    if (c === null && route.query.client === undefined && clientId.value !== GUEST_CLIENT_ID) {
      void go({ ...state.value, client: clientId.value }, 'replace')
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
  }
  return out
})
const relaxed = computed(() => response.value?.relaxed ?? [])
const related = computed(() => response.value?.related_queries ?? [])
const showManager = computed(
  () => blocks.value.nothing || Boolean(response.value?.empty?.show_manager),
)

function eventContext() {
  return {
    requestId: response.value?.request_id ?? null,
    sessionId: app.sessionId,
    clientId: state.value.client,
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

function onPick(row: SuggestRow) {
  trackQuiet('suggest_pick', {
    position: row.position,
    payload: { type: row.type, text: row.text, id: row.id, position: row.position },
  })
  submit(row.text)
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

function sendManagerRequest() {
  return track('manager_request', { payload: { query: response.value?.query ?? state.value.q } })
}
</script>

<template>
  <div :class="['search-page', { 'search-page--trace': state.trace && isDesktop }]">
    <div class="search-page__main">
      <h1 class="sr-only">Поиск по каталогу</h1>
      <div class="search-page__bar">
        <SearchBox
          ref="box"
          v-model="text"
          :client-id="state.client ?? GUEST_CLIENT_ID"
          @submit="submit"
          @pick="onPick"
        />
        <div class="search-page__controls">
          <ConfigSwitch
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
        <p class="search-page__found t-sm t-muted t-num" role="status">
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

        <ul v-if="relaxed.length" class="search-page__notes">
          <li v-for="r in relaxed" :key="r.field">
            <Info :size="16" aria-hidden="true" />
            <span>{{ r.message }}</span>
          </li>
        </ul>

        <section v-if="blocks.absent || blocks.nothing" class="panel search-page__absent">
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
          <div v-if="showManager">
            <button type="button" class="btn btn--secondary" @click="managerOpen = true">
              <Headset :size="16" aria-hidden="true" />
              Спросить менеджера
            </button>
          </div>
        </section>

        <ol v-if="blocks.main.length" class="search-page__list" aria-label="Результаты">
          <li v-for="p in blocks.main" :key="p.item.product_id">
            <ResultCard
              :item="p.item"
              :position="p.position"
              :selected="state.trace && selected === p.item.product_id"
              @open="onOpen"
              @action="onAction"
              @why="onWhy"
            />
          </li>
        </ol>

        <section
          v-if="blocks.analogs.length"
          class="search-page__section"
          aria-labelledby="analogs-title"
        >
          <h2 id="analogs-title" class="t-h2">
            {{ blocks.absent || !blocks.main.length ? 'Похожие по назначению' : 'Аналоги' }}
          </h2>
          <ol class="search-page__list">
            <li v-for="p in blocks.analogs" :key="p.item.product_id">
              <ResultCard
                :item="p.item"
                :position="p.position"
                :selected="state.trace && selected === p.item.product_id"
                @open="onOpen"
                @action="onAction"
                @why="onWhy"
              />
            </li>
          </ol>
        </section>

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
.search-page__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
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
