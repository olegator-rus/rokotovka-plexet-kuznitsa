<script setup lang="ts">
// Вкладка «Запросы без ответа» («Переделанные экраны»): неудачные запросы за период — запрос,
// частота, исход (пусто, нет клика, переформулировка), причина, если разобран, владелец, статус; кнопка «Разобрать»
// в строке; фильтр по причине и владельцу. Данные — GET /api/analytics (problems). Причина и владелец — с сервера
// (cause, owner), без них — по коду диагноза (lib/causes.ts). «Разобрать» пока ведёт в «Запуски» с запросом в
// параметре (?tab=runs&q=…): запуск на одном запросе подключит трек агента.
import { computed, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { Microscope } from 'lucide-vue-next'
import { getAnalytics } from '@/api/analytics'
import type { ProblemQuery } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
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
  status: { label: string; tone: string | null }
}

function statusOf(p: ProblemQuery): Row['status'] {
  if (p.proposal_status) {
    const s = proposalStatus(p.proposal_status)
    return { label: s.label, tone: s.tone }
  }
  if (p.agent && p.agent.code !== 'unseen') return { label: p.agent.label, tone: null }
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
const filtered = computed(() =>
  rows.value.filter(
    (r) =>
      (causeFilter.value === ALL || r.causeId === causeFilter.value) &&
      (ownerFilter.value === ALL || r.owner === ownerFilter.value),
  ),
)
const filtering = computed(() => causeFilter.value !== ALL || ownerFilter.value !== ALL)

function resetFilters() {
  void router.replace({ query: { ...route.query, cause: undefined, owner: undefined } })
}

function analyse(q: string) {
  void router.push({ path: '/searchops', query: { tab: 'runs', q } })
}
</script>

<template>
  <section class="failed" aria-labelledby="failed-title">
    <div class="failed__head">
      <h2 id="failed-title" class="sr-only">Запросы без ответа</h2>
      <div class="failed__filters">
        <UiSelect v-model="days" :options="DAY_OPTIONS" label="Период" prefix="Период" />
        <UiSelect v-model="causeFilter" :options="CAUSE_SELECT" label="Причина" prefix="Причина" />
        <UiSelect
          v-model="ownerFilter"
          :options="OWNER_SELECT"
          label="Владелец"
          prefix="Владелец"
        />
      </div>
      <span v-if="res.status.value === 'ready'" class="t-sm t-muted t-num">
        {{ formatInt(filtered.length) }}
        {{ plural(filtered.length, ['запрос', 'запроса', 'запросов']) }}
        <template v-if="filtering"> из {{ formatInt(rows.length) }}</template>
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
      <p>По выбранной причине и владельцу запросов нет.</p>
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
            <th scope="col" class="num">Частота</th>
            <th scope="col">Исход</th>
            <th scope="col">Причина</th>
            <th scope="col">Владелец</th>
            <th scope="col">Статус</th>
            <th scope="col"><span class="sr-only">Действие</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in filtered" :key="r.p.query">
            <td>
              <RouterLink
                :to="{ path: '/search', query: { q: r.p.query, trace: '1' } }"
                class="link"
                :title="`Открыть «${r.p.query}» в поиске с трассой`"
              >
                {{ r.p.query }}
              </RouterLink>
            </td>
            <td class="num t-num">
              {{ formatInt(r.p.freq) }}
              <span class="t-xs t-muted failed__sub">неудачных {{ formatInt(r.p.failures) }}</span>
            </td>
            <td>
              <span class="badge">{{ r.p.outcome_label || r.p.outcome }}</span>
            </td>
            <td>
              <template v-if="r.cause">
                <span class="t-sm">{{ r.cause }}</span>
                <span v-if="r.code" class="t-xs t-muted t-mono failed__sub">{{ r.code }}</span>
              </template>
              <span v-else class="t-sm t-muted">не разобран</span>
            </td>
            <td>
              <span v-if="r.owner" class="t-sm">{{ r.owner }}</span>
              <span v-else class="t-sm t-muted">—</span>
            </td>
            <td>
              <RouterLink
                v-if="r.p.proposal_id"
                :to="{
                  path: '/searchops',
                  query: { tab: 'queue', status: 'all', p: r.p.proposal_id },
                }"
                :class="['badge', 'badge--tone', r.status.tone]"
                >{{ r.status.label }}</RouterLink
              >
              <span v-else class="t-sm t-muted">{{ r.status.label }}</span>
            </td>
            <td class="failed__act">
              <button
                type="button"
                class="btn btn--secondary btn--sm"
                :aria-label="`Разобрать «${r.p.query}»`"
                @click="analyse(r.p.query)"
              >
                <Microscope :size="14" aria-hidden="true" /> Разобрать
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
  flex-wrap: wrap;
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
.failed__table td:first-child {
  min-width: 11rem;
}
.failed__act {
  text-align: right;
  white-space: nowrap;
}
</style>
