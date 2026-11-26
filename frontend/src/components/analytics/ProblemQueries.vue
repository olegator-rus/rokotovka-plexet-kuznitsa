<script setup lang="ts">
// Таблица проблемных запросов менеджера e-com: запрос, частота, что произошло, почему,
// что делать. Запрос — ссылка в «Поиск» с открытой трассой; предложение — в очередь SearchOps.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import type { ProblemQuery } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { formatInt, plural } from '@/lib/format'

const props = defineProps<{ problems: ProblemQuery[] }>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

const OUTCOME_HOW: Record<string, string> = {
  empty: 'Чаще всего поиск возвращал ноль результатов.',
  no_click: 'Чаще всего выдача была, но клика по товару не было.',
  reformulated: 'Чаще всего покупатель в течение минуты без клика вводил другой запрос.',
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
            <td :class="{ 't-muted': !p.diagnosis || p.diagnosis === 'не разобрано' }">
              {{ p.diagnosis || 'не разобрано' }}
            </td>
            <td>
              <RouterLink
                v-if="p.proposal_id"
                :to="{ path: '/searchops', query: { proposal: p.proposal_id } }"
                class="link"
              >
                Предложение агента
              </RouterLink>
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
</style>
