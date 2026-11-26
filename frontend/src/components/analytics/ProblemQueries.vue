<script setup lang="ts">
// Таблица проблемных запросов менеджера e-com: запрос, частота, что произошло, почему,
// что делать. Запрос — ссылка в «Поиск» с открытой трассой; предложение — в очередь SearchOps.
import { computed, onMounted, ref, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'
import { getProposals, getRuns } from '@/api/agent'
import type { Proposal, ProblemQuery } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { formatInt, plural } from '@/lib/format'
import {
  diagnosisLabel,
  findProposalForQuery,
  proposalStatus,
  proposalsByQuery,
} from '@/lib/proposals'

const props = defineProps<{ problems: ProblemQuery[] }>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

const OUTCOME_HOW: Record<string, string> = {
  empty: 'Чаще всего поиск возвращал ноль результатов.',
  no_click: 'Чаще всего выдача была, но клика по товару не было.',
  reformulated: 'Чаще всего покупатель в течение минуты без клика вводил другой запрос.',
}

// Предложения агента по запросам: сервер аналитики пока не связывает их сам — сводим по запросам кластера.
const index = shallowRef(new Map<string, Proposal>())
/** Кластеры, которые агент разобрал и закрыл без предложения: подпись кластера → диагноз. */
const closed = shallowRef(new Map<string, string>())
onMounted(async () => {
  const [queue, runs] = await Promise.allSettled([getProposals(), getRuns()])
  // Без очереди или списка запусков таблица остаётся как есть: «ждёт разбора агентом».
  if (queue.status === 'fulfilled') index.value = proposalsByQuery(queue.value.items ?? [])
  if (runs.status === 'fulfilled') {
    const m = new Map<string, string>()
    const ordered = [...(runs.value.items ?? [])].sort((a, b) =>
      String(b.started_at).localeCompare(String(a.started_at)),
    )
    for (const r of ordered)
      for (const c of r.episode?.clusters ?? [])
        if (c.outcome === 'closed' && !m.has(c.label.toLowerCase()))
          m.set(c.label.toLowerCase(), c.diagnosis)
    closed.value = m
  }
})

const closedOf = (p: ProblemQuery) => closed.value.get(p.query.trim().toLowerCase()) ?? null

function proposalOf(p: ProblemQuery): Proposal | null {
  const byQuery = findProposalForQuery(index.value, p.query)
  if (p.proposal_id) return byQuery?.id === p.proposal_id ? byQuery : null
  return byQuery
}

function proposalLink(p: ProblemQuery) {
  const id = p.proposal_id ?? proposalOf(p)?.id
  return id ? { path: '/searchops', query: { tab: 'queue', status: 'all', p: id } } : null
}

const FIRST = 15
const showAll = ref(false)
const rows = computed(() => (showAll.value ? props.problems : props.problems.slice(0, FIRST)))

function explainFreq(p: ProblemQuery) {
  emit('explain', {
    title: `«${p.query}»`,
    value: `${formatInt(p.freq)} ${plural(p.freq, ['поиск', 'поиска', 'поисков'])}`,
    what: OUTCOME_HOW[p.outcome] ?? `Чаще всего: ${p.outcome_label}.`,
    how: 'Частота — поиски за период с этим нормализованным текстом. Неудача — пустая выдача, поиск без клика или переформулировка; в таблицу попадают запросы с неудачами, частые сверху.',
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
      <span class="t-xs t-muted">клик по запросу — поиск с трассой</span>
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
            <th scope="col">Что произошло</th>
            <th scope="col">Почему</th>
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
              <span class="t-xs t-muted t-num problems__fail">
                неудачных {{ formatInt(p.failures) }} из {{ formatInt(p.freq) }}
              </span>
            </td>
            <td v-if="proposalOf(p)">{{ diagnosisLabel(proposalOf(p)!.diagnosis) }}</td>
            <td v-else-if="closedOf(p)">{{ diagnosisLabel(closedOf(p)!) }}</td>
            <td v-else :class="{ 't-muted': !p.diagnosis || p.diagnosis === 'не разобрано' }">
              {{ p.diagnosis || 'не разобрано' }}
            </td>
            <td>
              <span v-if="proposalLink(p)" class="problems__todo">
                <RouterLink :to="proposalLink(p)!" class="link">Предложение агента</RouterLink>
                <span
                  v-if="proposalOf(p)"
                  :class="['badge', 'badge--tone', proposalStatus(proposalOf(p)!.status).tone]"
                  >{{ proposalStatus(proposalOf(p)!.status).label }}</span
                >
              </span>
              <span v-else-if="closedOf(p)" class="t-muted">
                агент разобрал: правкой правил не решается
              </span>
              <span v-else class="t-muted">ждёт разбора агентом</span>
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
.problems__fail {
  display: block;
  margin-top: 2px;
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
