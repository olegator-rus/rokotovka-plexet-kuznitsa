<script setup lang="ts">
// Вкладка «Запросы без ответа» («Переделанные экраны»): неудачные запросы за период — запрос,
// частота, исход (пусто, нет клика, переформулировка), причина, если разобран, владелец, статус; кнопка «Разобрать»
// в строке; фильтр по причине и владельцу. Данные — GET /api/analytics (problems). Причина и владелец — с сервера
// (cause, owner), без них — по коду диагноза (lib/causes.ts). «Разобрать» — запуск агента на одном запросе
// (POST /api/agent/runs {"query"}) и переход в ленту запуска.
import { computed, ref, watch } from 'vue'
import type { Component } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  ChevronDown,
  CircleHelp,
  Microscope,
  MousePointerClick,
  Repeat,
  Search,
  SearchX,
} from 'lucide-vue-next'
import { getAnalytics } from '@/api/analytics'
import type { ProblemQuery } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useAnalyseQuery } from '@/composables/useAnalyseQuery'
import { useResource } from '@/composables/useResource'
import { CAUSE_OPTIONS, OWNER_OPTIONS, causeView, isDiagnosisCode } from '@/lib/causes'
import { formatInt, plural } from '@/lib/format'
import { proposalStatus } from '@/lib/proposals'

const ALL = 'all'
const DAY_OPTIONS = [
  { value: '7', label: '7 дней' },
  { value: '30', label: '30 дней' },
  { value: '90', label: '90 дней' },
]
const CAUSE_SELECT = [{ value: ALL, label: 'Все причины' }, ...CAUSE_OPTIONS]
const OWNER_SELECT = [{ value: ALL, label: 'Все владельцы' }, ...OWNER_OPTIONS]

const route = useRoute()
const router = useRouter()

function queryParam(key: string, allowed: { value: string }[], fallback: string) {
  return computed<string>({
    get: () => {
      const v = typeof route.query[key] === 'string' ? (route.query[key] as string) : ''
      return allowed.some((o) => o.value === v) ? v : fallback
    },
    set: (v) =>
      void router.replace({ query: { ...route.query, [key]: v === fallback ? undefined : v } }),
  })
}
const days = queryParam('days', DAY_OPTIONS, '30')
const causeFilter = queryParam('cause', CAUSE_SELECT, ALL)
const ownerFilter = queryParam('owner', OWNER_SELECT, ALL)

const res = useResource((opts) => getAnalytics(Number(days.value), opts))
watch(days, () => void res.load(), { immediate: true })

/** Код диагноза строки: предложение → вывод агента → диагноз журнала; текст вместо кода — не причина. */
const codeOf = (p: ProblemQuery) =>
  [p.proposal_diagnosis, p.agent?.diagnosis, p.diagnosis].find((c) => isDiagnosisCode(c)) ?? ''

interface Row {
  p: ProblemQuery
  cause: string
  causeId: string
  owner: string
  code: string
  status: { label: string; tone: string | null; note?: string }
}

function statusOf(p: ProblemQuery): Row['status'] {
  if (p.proposal_status) {
    const s = proposalStatus(p.proposal_status)
    return { label: s.label, tone: s.tone }
  }
  if (p.agent && p.agent.code !== 'unseen') {
    // Заметка агента различает исходы под одной подписью: упор в лимит, «поиск уже отвечает», отказ модели.
    const note = (p.agent.note ?? '').trim()
    return {
      label: p.agent.label,
      tone: null,
      note: note && note !== p.agent.label ? note : undefined,
    }
  }
  return { label: 'не разобран', tone: null }
}

const rows = computed<Row[]>(() =>
  (res.data.value?.problems ?? []).map((p) => {
    const code = codeOf(p)
    const known = !!(p.cause || code)
    const v = known ? causeView({ cause: p.cause, owner: p.owner, diagnosis: code }) : null
    return {
      p,
      cause: v?.label ?? '',
      causeId: v?.cause?.id ?? '',
      owner: v?.owner ?? '',
      code,
      status: statusOf(p),
    }
  }),
)
/** Поиск по тексту запроса среди загруженных строк. */
const needle = ref('')
const filtered = computed(() =>
  rows.value.filter(
    (r) =>
      r.p.query.toLowerCase().includes(needle.value.trim().toLowerCase()) &&
      (causeFilter.value === ALL || r.causeId === causeFilter.value) &&
      (ownerFilter.value === ALL || r.owner === ownerFilter.value),
  ),
)
const filtering = computed(
  () => Boolean(needle.value.trim()) || causeFilter.value !== ALL || ownerFilter.value !== ALL,
)

const countText = computed(() => {
  const n = filtered.value.length
  const total = filtering.value ? ` из ${formatInt(rows.value.length)}` : ''
  return `${formatInt(n)} ${plural(n, ['запрос', 'запроса', 'запросов'])}${total}`
})

/** Вид исхода — по подписи сервера: свой значок и цвет, слово всегда рядом. */
function outcomeView(label: string): { icon: Component; tone: string } {
  if (/пуст/i.test(label)) return { icon: SearchX, tone: 'route-d' }
  if (/клик/i.test(label)) return { icon: MousePointerClick, tone: 'route-c' }
  return { icon: Repeat, tone: 'route-b' }
}
/** Ширина полоски: доля неудачных среди поисков этого запроса (оба числа — с сервера). */
const failShare = (p: ProblemQuery) =>
  `${p.freq > 0 ? Math.min(100, (p.failures / p.freq) * 100) : 0}%`

function resetFilters() {
  needle.value = ''
  void router.replace({ query: { ...route.query, cause: undefined, owner: undefined } })
}

const { analyse, busy } = useAnalyseQuery()
</script>

<template>
  <section class="failed" aria-labelledby="failed-title">
    <div class="failed__head">
      <h2 id="failed-title" class="sr-only">Запросы без ответа</h2>
      <div class="failed__filters">
        <label class="failed__search">
          <Search :size="16" aria-hidden="true" />
          <input
            v-model="needle"
            type="search"
            placeholder="Найти запрос"
            aria-label="Найти запрос в таблице"
          />
        </label>
        <UiSelect v-model="days" :options="DAY_OPTIONS" label="Период" prefix="Период" />
        <UiSelect v-model="causeFilter" :options="CAUSE_SELECT" label="Причина" prefix="Причина" />
        <UiSelect
          v-model="ownerFilter"
          :options="OWNER_SELECT"
          label="Владелец"
          prefix="Владелец"
        />
      </div>
      <!-- На широком экране счётчик стоит в шапке таблицы; на телефоне шапки не видно — он здесь. -->
      <span v-if="res.status.value === 'ready'" class="failed__count-top t-sm t-muted t-num">
        {{ countText }}
      </span>
    </div>

    <div
      v-if="res.status.value === 'loading' && !res.data.value"
      class="panel failed__skel"
      aria-busy="true"
    >
      <span v-for="i in 5" :key="i" class="skeleton" :style="{ width: `${94 - i * 8}%` }" />
    </div>

    <BlockError
      v-else-if="res.status.value === 'error' && !res.data.value"
      :error="res.error.value!"
      title="Запросы без ответа не загрузились"
      @retry="res.load()"
    />

    <p v-else-if="res.data.value && !rows.length" class="panel failed__empty">
      Неудачных запросов за период нет.
    </p>

    <div v-else-if="res.data.value && !filtered.length" class="panel failed__empty">
      <p>
        {{
          needle.trim()
            ? 'Под поиск и фильтры не попал ни один запрос.'
            : 'По выбранной причине и владельцу запросов нет.'
        }}
      </p>
      <button type="button" class="btn btn--secondary btn--sm" @click="resetFilters">
        Сбросить фильтры
      </button>
    </div>

    <div
      v-else-if="res.data.value"
      class="data-scroll"
      role="region"
      aria-label="Таблица запросов без ответа"
      tabindex="0"
    >
      <table class="data-table failed__table">
        <thead>
          <tr>
            <th scope="col">Запрос</th>
            <th scope="col" aria-label="Частота" title="Неудачных поисков из всех за период">
              Частота
              <span class="failed__th-sub">неудачных из всех</span>
            </th>
            <th scope="col">Исход</th>
            <th scope="col">Причина</th>
            <th scope="col">Владелец</th>
            <th scope="col">Статус</th>
            <th scope="col" class="failed__count">
              <span class="sr-only">Действие.</span>
              <span class="t-num">{{ countText }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in filtered" :key="r.p.query">
            <td class="failed__query">
              <RouterLink
                :to="{ path: '/search', query: { q: r.p.query, trace: '1' } }"
                class="failed__link"
                :title="`Открыть «${r.p.query}» в поиске с трассировкой`"
              >
                {{ r.p.query }}
              </RouterLink>
            </td>
            <td class="failed__freq" data-label="Неудачных из всех">
              <span class="t-num">
                <strong>{{ formatInt(r.p.failures) }}</strong>
                <span class="t-muted"> из {{ formatInt(r.p.freq) }}</span>
              </span>
              <span class="failed__bar" aria-hidden="true">
                <span :style="{ width: failShare(r.p) }" />
              </span>
            </td>
            <td data-label="Исход">
              <span
                :class="[
                  'badge badge--tone failed__outcome',
                  outcomeView(r.p.outcome_label || r.p.outcome).tone,
                ]"
              >
                <component
                  :is="outcomeView(r.p.outcome_label || r.p.outcome).icon"
                  :size="14"
                  aria-hidden="true"
                />
                {{ r.p.outcome_label || r.p.outcome }}
              </span>
            </td>
            <td data-label="Причина">
              <template v-if="r.cause">
                <span class="t-sm">{{ r.cause }}</span>
                <span v-if="r.code" class="t-xs t-muted t-mono failed__sub">{{ r.code }}</span>
              </template>
              <span v-else class="t-muted">—</span>
            </td>
            <td data-label="Владелец">
              <span v-if="r.owner" class="t-sm">{{ r.owner }}</span>
              <span v-else class="t-sm t-muted">—</span>
            </td>
            <td class="failed__status" data-label="Статус">
              <RouterLink
                v-if="r.p.proposal_id"
                :to="{
                  path: '/searchops',
                  query: { tab: 'queue', status: 'all', p: r.p.proposal_id },
                }"
                :class="['badge', 'badge--tone', r.status.tone]"
                >{{ r.status.label }}</RouterLink
              >
              <!-- Длинное пояснение агента раскрывается по запросу, чтобы строки были одной высоты. -->
              <details v-else-if="r.status.note" class="failed__why">
                <summary>
                  <CircleHelp :size="14" aria-hidden="true" />
                  <span>{{ r.status.label }}</span>
                  <ChevronDown :size="14" aria-hidden="true" class="failed__chev" />
                </summary>
                <p class="t-xs t-muted">{{ r.status.note }}</p>
              </details>
              <span v-else class="badge">{{ r.status.label }}</span>
              <span v-if="r.p.proposal_id && r.status.note" class="failed__note t-xs t-muted">{{
                r.status.note
              }}</span>
            </td>
            <td class="failed__act">
              <button
                type="button"
                class="btn btn--secondary btn--sm failed__go"
                :aria-label="`Разобрать «${r.p.query}»`"
                :disabled="!!busy"
                @click="analyse(r.p.query)"
              >
                <Microscope :size="14" aria-hidden="true" />
                {{ busy === r.p.query.trim() ? 'Запускаем…' : 'Разобрать' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.failed {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.failed__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
}
.failed__filters {
  display: flex;
  flex: 1 1 0;
  flex-wrap: wrap;
  min-width: min(100%, 20rem);
  gap: var(--space-2);
}
.failed__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.failed__empty {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-4);
  color: var(--text-2);
}
.failed__sub {
  display: block;
  margin-top: 2px;
}
.failed__search {
  display: flex;
  flex: 1 1 11rem;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text-3);
}
.failed__search:focus-within {
  border-color: var(--focus-ring);
}
.failed__search input {
  flex: 1;
  min-width: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  outline: none;
}

.failed__table th,
.failed__table td {
  padding: var(--space-3) var(--space-4);
  vertical-align: middle;
}
.failed__table tbody tr {
  height: 4rem;
  transition: background-color var(--dur-fast) var(--ease-standard);
}
.failed__table tbody tr:hover {
  background: var(--surface-2);
}
.failed__table th.failed__count {
  text-align: right;
}
.failed__count-top {
  display: none;
}
.failed__th-sub {
  display: block;
  color: var(--text-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-regular);
}
.failed__query {
  min-width: 12rem;
}
.failed__link {
  color: var(--text);
  font-size: var(--fs-body);
  font-weight: var(--fw-semibold);
  text-decoration: none;
  overflow-wrap: anywhere;
}
.failed__link:hover {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.failed__freq {
  min-width: 7.5rem;
  white-space: nowrap;
}
.failed__freq strong {
  font-size: var(--fs-h2);
  font-weight: var(--fw-bold);
}
.failed__bar {
  display: block;
  width: 6rem;
  height: 4px;
  margin-top: var(--space-1);
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
}
.failed__bar span {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
.failed__outcome {
  gap: var(--space-1);
  min-height: 1.5rem;
  border-radius: var(--radius-pill);
}
.failed__status {
  min-width: 11rem;
}
.failed__why summary {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: var(--control-h-sm);
  color: var(--text-2);
  list-style: none;
  cursor: pointer;
}
.failed__why summary::-webkit-details-marker {
  display: none;
}
.failed__why summary svg {
  flex: none;
}
.failed__why summary:hover {
  color: var(--text);
}
.failed__why[open] .failed__chev {
  transform: rotate(180deg);
}
.failed__why p {
  max-width: 34ch;
  margin: var(--space-1) 0 0;
}
.failed__note {
  display: block;
  margin-top: 2px;
  max-width: 28ch;
}
.failed__act {
  text-align: right;
  white-space: nowrap;
}
.failed__go:hover:not(:disabled) {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}

/* Широкий экран: шапка таблицы остаётся под шапкой сайта, пока листаешь запросы. */
@media (min-width: 1100px) {
  .failed .data-scroll {
    overflow: visible;
  }
  .failed__table thead th:first-child {
    border-top-left-radius: var(--radius-card);
  }
  .failed__table thead th:last-child {
    border-top-right-radius: var(--radius-card);
  }
  .failed__table thead th {
    position: sticky;
    top: var(--header-actual, var(--header-h));
    z-index: 5;
  }
}
/* Телефон: строка — карточка, подписи колонок у значений; прокрутки вбок нет. */
@media (max-width: 767px) {
  .failed__count-top {
    display: inline;
  }
  .failed__table thead {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }
  .failed__table,
  .failed__table tbody {
    display: block;
  }
  .failed__table tbody tr {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: var(--space-2) var(--space-3);
    height: auto;
    padding: var(--space-3);
    border-bottom: var(--border-w) solid var(--border);
  }
  .failed__table tbody tr:last-child {
    border-bottom: 0;
  }
  .failed__table td {
    display: block;
    min-width: 0;
    padding: 0;
    border: 0;
  }
  .failed__table td[data-label]::before {
    content: attr(data-label);
    display: block;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .failed__table .failed__query,
  .failed__table .failed__status,
  .failed__table .failed__act {
    grid-column: 1 / -1;
  }
  .failed__go {
    width: 100%;
  }
}
</style>
