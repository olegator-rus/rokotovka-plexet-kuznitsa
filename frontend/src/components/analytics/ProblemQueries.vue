<script setup lang="ts">
// Таблица проблемных запросов менеджера e-com. «За период» — исторические
// журналы старого поиска магазина; «Сейчас» — что отвечает действующий поиск (GET /api/analytics/current,
// поиск без записи в журнал); «Вывод агента» — из предложения или итога кластера последних запусков (сервер).
// Запрос — ссылка в «Поиск» с открытой трассой; предложение — в очередь SearchOps, итог — в ленту запуска.
import { computed, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { getProblemsNow } from '@/api/analytics'
import type { ProblemNow, ProblemQuery } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { formatInt, plural } from '@/lib/format'
import { diagnosisLabel, proposalStatus } from '@/lib/proposals'

const props = defineProps<{ problems: ProblemQuery[]; days: number; periodLabel: string }>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

const OUTCOME_HOW: Record<string, string> = {
  empty: 'Чаще всего поиск возвращал ноль результатов.',
  no_click: 'Чаще всего выдача была, но клика по товару не было.',
  reformulated: 'Чаще всего покупатель в течение минуты без клика вводил другой запрос.',
}

const FIRST = 15
const showAll = ref(false)
const rows = computed(() => (showAll.value ? props.problems : props.problems.slice(0, FIRST)))

// «Сейчас»: таблица не ждёт поисков — колонка заполняется отдельным запросом.
const now = shallowRef(new Map<string, ProblemNow>())
const nowStatus = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
const nowRules = ref<number | null>(null)
let ctrl: AbortController | null = null
async function loadNow() {
  const limit = rows.value.length
  if (!limit) return
  ctrl?.abort()
  const c = new AbortController()
  ctrl = c
  nowStatus.value = 'loading'
  try {
    const res = await getProblemsNow(props.days, limit, { signal: c.signal })
    if (ctrl !== c) return
    now.value = new Map(res.items.map((i) => [i.query, i]))
    nowRules.value = res.rules_version
    nowStatus.value = 'ready'
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return
    nowStatus.value = 'error'
  }
}
watch([() => props.days, () => props.problems, showAll], () => void loadNow(), { immediate: true })

function nowText(n: ProblemNow): string {
  if (n.error) return 'поиск не ответил'
  if (n.total === 0) return 'пусто'
  const items = `${formatInt(n.total)} ${plural(n.total, ['товар', 'товара', 'товаров'])}`
  if (n.not_in_catalog) return `нет в каталоге · ${items} аналогов`
  return n.exact ? `${items} · точное первым` : items
}
const nowTone = (n: ProblemNow) =>
  n.error || n.total === 0 ? 'problems__bad' : n.not_in_catalog ? 'problems__warn' : 'problems__ok'

function verdictText(p: ProblemQuery): string {
  const a = p.agent
  if (!a) return p.diagnosis || 'не разобрано'
  if (a.code === 'proposal' && a.diagnosis) return diagnosisLabel(a.diagnosis)
  return a.label
}

function runLink(p: ProblemQuery) {
  const a = p.agent
  if (!a?.run_id) return null
  return {
    path: `/searchops/runs/${a.run_id}`,
    query: a.cluster_id ? { cluster: a.cluster_id } : {},
  }
}

function proposalLink(p: ProblemQuery) {
  return p.proposal_id
    ? { path: '/searchops', query: { tab: 'queue', status: 'all', p: p.proposal_id } }
    : null
}

function explainFreq(p: ProblemQuery) {
  emit('explain', {
    title: `«${p.query}»`,
    value: `${formatInt(p.freq)} ${plural(p.freq, ['поиск', 'поиска', 'поисков'])} за ${props.periodLabel}`,
    what: OUTCOME_HOW[p.outcome] ?? `Чаще всего: ${p.outcome_label}.`,
    how: 'Частота и неудачи — по журналам за период (в основном это старый поиск магазина до внедрения). Неудача — пустая выдача, поиск без клика или переформулировка. Колонка «Сейчас» — тот же запрос через действующий поиск сегодня, без записи в журнал.',
    rows: [
      { label: 'Неудачных поисков', value: formatInt(p.failures) },
      { label: 'Пустая выдача', value: formatInt(p.empty) },
      { label: 'Без клика', value: formatInt(p.no_click) },
      { label: 'Переформулировка', value: formatInt(p.reformulated) },
      { label: 'Кликов', value: formatInt(p.clicks) },
      { label: 'Заказов', value: formatInt(p.orders) },
    ],
  })
}
</script>

<template>
  <section class="problems" aria-labelledby="problems-title">
    <div class="problems__head">
      <h2 id="problems-title" class="t-h2">Проблемные запросы</h2>
      <span class="t-xs t-muted">
        «За период» — журналы старого поиска; «Сейчас» — действующий поиск<template
          v-if="nowRules !== null"
        >
          (правила v{{ nowRules }})</template
        >; клик по запросу — поиск с трассой
      </span>
    </div>
    <p v-if="problems.length === 0" class="panel problems__empty">
      Проблемных запросов за период нет: пустых выдач, поисков без клика и переформулировок не было.
    </p>
    <div v-else class="data-scroll" role="region" aria-label="Проблемные запросы" tabindex="0">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Запрос</th>
            <th scope="col" class="num">Поисков</th>
            <th scope="col">За период</th>
            <th scope="col">Сейчас</th>
            <th scope="col">Вывод агента</th>
            <th scope="col">Что делать</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in rows" :key="p.query">
            <td>
              <RouterLink :to="{ path: '/search', query: { q: p.query, trace: '1' } }" class="link">
                {{ p.query }}
              </RouterLink>
            </td>
            <td class="num">
              <button
                type="button"
                class="num-btn"
                :aria-label="`Поисков ${formatInt(p.freq)}. Как посчитано`"
                @click="explainFreq(p)"
              >
                {{ formatInt(p.freq) }}
              </button>
            </td>
            <td>
              <span class="badge">{{ p.outcome_label || p.outcome }}</span>
              <span class="t-xs t-muted t-num problems__sub">
                неудачных {{ formatInt(p.failures) }} из {{ formatInt(p.freq) }} за период
              </span>
            </td>
            <td class="problems__now">
              <template v-if="now.get(p.query)">
                <span :class="['t-sm', 't-num', nowTone(now.get(p.query)!)]">{{
                  nowText(now.get(p.query)!)
                }}</span>
                <span
                  v-if="now.get(p.query)!.top_title"
                  class="t-xs t-muted problems__sub problems__clip"
                  :title="now.get(p.query)!.top_title"
                  >{{ now.get(p.query)!.top_title }}</span
                >
              </template>
              <span v-else-if="nowStatus === 'loading'" class="t-xs t-muted">проверяем…</span>
              <button
                v-else-if="nowStatus === 'error'"
                type="button"
                class="link t-xs"
                @click="loadNow()"
              >
                не проверено — повторить
              </button>
              <span v-else class="t-xs t-muted">—</span>
            </td>
            <td :class="{ 't-muted': !p.agent || p.agent.code === 'unseen' }">
              <span class="t-sm">{{ verdictText(p) }}</span>
              <span
                v-if="p.agent?.note"
                class="t-xs t-muted problems__sub problems__clip"
                :title="p.agent.note"
                >{{ p.agent.note }}</span
              >
            </td>
            <td>
              <span v-if="proposalLink(p)" class="problems__todo">
                <RouterLink :to="proposalLink(p)!" class="link">Предложение агента</RouterLink>
                <span
                  v-if="p.proposal_status"
                  :class="['badge', 'badge--tone', proposalStatus(p.proposal_status).tone]"
                  >{{ proposalStatus(p.proposal_status).label }}</span
                >
              </span>
              <RouterLink v-else-if="runLink(p)" :to="runLink(p)!" class="link t-sm">
                {{ p.agent?.code === 'queued' ? 'в очереди запуска' : 'Лента агента' }}
              </RouterLink>
              <span v-else-if="p.agent?.code === 'mostly_ok'" class="t-muted t-sm">
                не требуется
              </span>
              <span v-else class="t-muted t-sm">ждёт разбора агентом</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="problems.length > FIRST">
      <button type="button" class="btn btn--secondary btn--sm" @click="showAll = !showAll">
        {{ showAll ? `Только первые ${FIRST}` : `Показать все ${formatInt(problems.length)}` }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.problems {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.problems__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
}
.problems__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.problems__sub {
  display: block;
  margin-top: 2px;
}
.problems__clip {
  max-width: 13rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.problems__ok {
  color: var(--status-accepted);
}
.problems__warn {
  color: var(--status-pending);
}
.problems__bad {
  color: var(--danger);
}
.data-table td:first-child {
  min-width: 12rem;
}
.problems__todo {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
</style>
