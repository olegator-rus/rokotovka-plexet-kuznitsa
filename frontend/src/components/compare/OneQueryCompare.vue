<script setup lang="ts">
// Вкладка «Один запрос»: тот же запрос в двух конфигурациях рядом. Товар, который есть только в одной
// колонке, помечен «только здесь»; у общего — сдвиг позиции относительно соседней колонки.
// Скор и время — ссылки в «Поиск» с открытой трассой этой конфигурации.
import { computed, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ArrowDown, ArrowUp, Equal, Search } from 'lucide-vue-next'
import { postSearch } from '@/api/search'
import type { ConfigId, SearchResponse } from '@/api/types'
import SearchError from '@/components/search/SearchError.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { compareColumn, compareSummary, shiftLabel } from '@/lib/compare'
import { configHint } from '@/lib/configs'
import { formatFound, formatScore } from '@/lib/format'
import { priceLabel } from '@/lib/license'
import { buildSearchRequest, emptyState, isConfigId } from '@/lib/searchState'
import { useAppStore } from '@/stores/app'

/** Метрики считаются по первым 10 позициям — сравниваем их же. */
const COMPARE_LIMIT = 10

const props = defineProps<{ query: string; left: ConfigId; right: ConfigId }>()
const emit = defineEmits<{ change: [query: string, left: ConfigId, right: ConfigId] }>()

const app = useAppStore()
const { configs } = storeToRefs(app)
const options = computed(() =>
  configs.value.map((c) => ({ value: c, label: c, hint: configHint(c) })),
)

const text = ref(props.query)
watch(
  () => props.query,
  (q) => (text.value = q),
)

type Side = {
  status: 'idle' | 'loading' | 'ready' | 'error'
  resp: SearchResponse | null
  error: ApiError | null
}
const sides = shallowRef<Record<'left' | 'right', Side>>({
  left: { status: 'idle', resp: null, error: null },
  right: { status: 'idle', resp: null, error: null },
})
let ctrl: AbortController | null = null

async function runSide(which: 'left' | 'right', config: ConfigId, q: string, signal: AbortSignal) {
  const body = buildSearchRequest(
    { ...emptyState(), q, config, client: app.clientId },
    { sessionId: app.sessionId, defaultConfig: config, limit: COMPARE_LIMIT },
  )
  try {
    const resp = await postSearch(body, { signal })
    if (signal.aborted) return
    sides.value = { ...sides.value, [which]: { status: 'ready', resp, error: null } }
  } catch (e) {
    if (signal.aborted) return
    const error = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
    sides.value = { ...sides.value, [which]: { status: 'error', resp: null, error } }
  }
}

function run() {
  ctrl?.abort()
  const q = props.query.trim()
  if (!q) {
    sides.value = {
      left: { status: 'idle', resp: null, error: null },
      right: { status: 'idle', resp: null, error: null },
    }
    return
  }
  const c = new AbortController()
  ctrl = c
  sides.value = {
    left: { status: 'loading', resp: null, error: null },
    right: { status: 'loading', resp: null, error: null },
  }
  void runSide('left', props.left, q, c.signal)
  void runSide('right', props.right, q, c.signal)
}

watch(() => [props.query, props.left, props.right, app.clientId], run, { immediate: true })

function submit() {
  emit('change', text.value.trim(), props.left, props.right)
}
function setConfig(which: 'left' | 'right', v: string) {
  if (!isConfigId(v)) return
  emit(
    'change',
    props.query,
    which === 'left' ? v : props.left,
    which === 'right' ? v : props.right,
  )
}

const ids = computed(() => ({
  left: (sides.value.left.resp?.items ?? []).map((i) => i.product_id),
  right: (sides.value.right.resp?.items ?? []).map((i) => i.product_id),
}))
const marks = computed(() => ({
  left: compareColumn(ids.value.left, ids.value.right),
  right: compareColumn(ids.value.right, ids.value.left),
}))
const summary = computed(() =>
  sides.value.left.resp && sides.value.right.resp
    ? compareSummary(ids.value.left, ids.value.right)
    : null,
)

function traceLink(config: ConfigId) {
  return { path: '/search', query: { q: props.query, config, trace: '1' } }
}
const columns = computed(() => [
  { key: 'left' as const, config: props.left },
  { key: 'right' as const, config: props.right },
])
</script>

<template>
  <div class="oneq">
    <form class="oneq__form" role="search" @submit.prevent="submit">
      <label for="compare-q" class="sr-only">Запрос для сравнения</label>
      <input
        id="compare-q"
        v-model="text"
        class="oneq__input"
        type="search"
        autocomplete="off"
        placeholder="Запрос, например: касперкий на 10 пк"
      />
      <button type="submit" class="btn btn--primary" :disabled="!text.trim()">
        <Search :size="16" aria-hidden="true" />
        Сравнить
      </button>
    </form>

    <p v-if="summary" class="t-sm t-muted">
      Общих позиций {{ summary.common }} · только в {{ left }}: {{ summary.onlyLeft }} · только в
      {{ right }}: {{ summary.onlyRight }}
    </p>
    <p v-else-if="!query" class="panel oneq__empty">
      Введите запрос — выдачи двух конфигураций появятся рядом. Конфигурации выбираются над
      колонками.
    </p>

    <div class="oneq__cols">
      <section
        v-for="col in columns"
        :key="col.key"
        class="oneq__col"
        :aria-label="`Выдача ${col.config}`"
      >
        <header class="oneq__col-head">
          <UiSelect
            :model-value="col.config"
            :options="options"
            :label="`Конфигурация ${col.key === 'left' ? 'слева' : 'справа'}`"
            prefix="Конфигурация"
            @update:model-value="(v: string) => setConfig(col.key, v)"
          />
          <span class="t-xs t-muted oneq__hint">{{ configHint(col.config) }}</span>
        </header>

        <template v-if="sides[col.key].status === 'loading'">
          <div v-for="i in 5" :key="i" class="panel oneq__row" aria-hidden="true">
            <span class="skeleton" :style="{ width: `${70 - i * 6}%` }" />
          </div>
        </template>
        <SearchError
          v-else-if="sides[col.key].status === 'error' && sides[col.key].error"
          :error="sides[col.key].error!"
          @retry="run"
        />
        <template v-else-if="sides[col.key].resp">
          <RouterLink :to="traceLink(col.config)" class="link t-xs oneq__found">
            {{ formatFound(sides[col.key].resp!.total, sides[col.key].resp!.trace?.total_ms) }} ·
            трасса
          </RouterLink>
          <p v-if="!sides[col.key].resp!.items.length" class="panel oneq__empty">
            {{ sides[col.key].resp!.empty?.message ?? 'Ничего не найдено' }}
          </p>
          <ol v-else class="oneq__list">
            <li
              v-for="(item, i) in sides[col.key].resp!.items"
              :key="item.product_id"
              class="panel oneq__row"
            >
              <span class="oneq__pos t-num">{{ i + 1 }}</span>
              <div class="oneq__main">
                <span class="oneq__title">{{ item.title }}</span>
                <span class="t-xs t-muted"> {{ item.license_line }} · {{ priceLabel(item) }} </span>
              </div>
              <div class="oneq__side">
                <span
                  v-if="marks[col.key][i]?.onlyHere"
                  class="badge badge--strong"
                  :title="`Нет в выдаче ${col.key === 'left' ? right : left}`"
                >
                  только здесь
                </span>
                <span
                  v-else-if="marks[col.key][i]"
                  class="oneq__shift t-xs"
                  :aria-label="`${shiftLabel(marks[col.key][i])}, чем в ${col.key === 'left' ? right : left}`"
                >
                  <ArrowUp
                    v-if="(marks[col.key][i].shift ?? 0) > 0"
                    :size="12"
                    aria-hidden="true"
                  />
                  <ArrowDown
                    v-else-if="(marks[col.key][i].shift ?? 0) < 0"
                    :size="12"
                    aria-hidden="true"
                  />
                  <Equal v-else :size="12" aria-hidden="true" />
                  <span aria-hidden="true">{{ shiftLabel(marks[col.key][i]) }}</span>
                </span>
                <RouterLink
                  :to="traceLink(col.config)"
                  class="link t-xs t-num"
                  :aria-label="`Скор ${formatScore(item.final)} — разложение в трассе`"
                >
                  {{ formatScore(item.final) }}
                </RouterLink>
              </div>
            </li>
          </ol>
        </template>
      </section>
    </div>
  </div>
</template>

<style scoped>
.oneq {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.oneq__form {
  display: flex;
  gap: var(--space-2);
}
.oneq__input {
  box-sizing: border-box;
  flex: 1;
  min-width: 0;
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
}
.oneq__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.oneq__cols {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
}
.oneq__col {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.oneq__col-head {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
}
.oneq__hint {
  min-height: var(--lh-xs);
}
.oneq__found {
  align-self: flex-start;
}
.oneq__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.oneq__row {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr) auto;
  align-items: start;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
}
.oneq__pos {
  color: var(--text-3);
  font-size: var(--fs-sm);
}
.oneq__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.oneq__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.oneq__side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1);
}
.oneq__shift {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--text-2);
  white-space: nowrap;
}
@media (max-width: 767px) {
  .oneq__cols {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
