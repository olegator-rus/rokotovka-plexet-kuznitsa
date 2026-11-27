<script setup lang="ts">
// Журнал вызовов модели: время, модель, токены, задержка, стоимость, ссылка на трассу
// поиска или запуск агента. Страницами по COSTS_PAGE; под таблицей — P50/P95 задержки показанных строк.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import type { LlmCall } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { formatCost, formatDateTime, formatInt, formatMs } from '@/lib/format'
import { percentile } from '@/lib/metrics'
import TraceDialog from './TraceDialog.vue'

const props = defineProps<{
  calls: LlmCall[]
  total: number
  loadingMore: boolean
  pricingVersion: string
}>()
const emit = defineEmits<{ more: []; explain: [e: Explanation] }>()

const latency = computed(() => {
  const xs = props.calls.map((c) => c.latency_ms)
  return { p50: percentile(xs, 50), p95: percentile(xs, 95) }
})

const STATUS: Record<string, string> = { ok: 'ok', error: 'ошибка', timeout: 'таймаут' }
const KIND: Record<string, string> = {
  search: 'поиск',
  eval: 'прогон оценки',
  agent: 'агент',
  judge: 'LLM-судья',
  other: 'ручной прогон',
}

/** Трасса есть только у записанного поиска: прогоны оценки и судья трасс не пишут. */
function hasTrace(c: LlmCall): boolean {
  return !!c.request_id && (c.kind === 'search' || !!c.query)
}

const traceOpen = ref(false)
const traceFor = ref<{ id: string; query: string | null } | null>(null)
function openTrace(c: LlmCall) {
  if (!c.request_id) return
  traceFor.value = { id: c.request_id, query: c.query ?? null }
  traceOpen.value = true
}

function explainCost(c: LlmCall) {
  emit('explain', {
    title: `Вызов ${c.model}`,
    value: formatCost(c.cost_rub),
    how: 'Токены входа × цена входа + токены выхода × цена выхода по прайсу, действовавшему в момент вызова; кэшированные токены — по цене кэша, если она есть в прайсе.',
    rows: [
      { label: 'Токены входа', value: formatInt(c.input_tokens) },
      { label: 'Токены выхода', value: formatInt(c.output_tokens) },
      ...(c.cached_tokens
        ? [{ label: 'Из них кэшированных', value: formatInt(c.cached_tokens) }]
        : []),
      { label: 'Задержка', value: formatMs(c.latency_ms) },
      { label: 'Время', value: formatDateTime(c.created_at) },
      { label: 'Откуда', value: KIND[c.kind] ?? c.kind },
      ...(c.request_id ? [{ label: 'request_id', value: c.request_id }] : []),
      ...(c.agent_run_id ? [{ label: 'Запуск агента', value: c.agent_run_id }] : []),
    ],
    source: `llm_usage · прайс ${props.pricingVersion}`,
  })
}
</script>

<template>
  <section id="calls" class="calls" aria-labelledby="calls-title">
    <div class="calls__head">
      <h2 id="calls-title" class="t-h2">Журнал вызовов модели</h2>
      <span v-if="total" class="t-xs t-muted t-num">
        показано {{ formatInt(calls.length) }} из {{ formatInt(total) }}
      </span>
    </div>
    <p v-if="calls.length === 0" class="panel calls__empty">
      Вызовов модели ещё не было: все запросы разобраны правилами или взяты из кэша. Сложный запрос
      в «Поиске» (например, «аналог фотошопа на 20 пользователей») пойдёт по маршруту D и появится
      здесь.
    </p>
    <template v-else>
      <div class="data-scroll" role="region" aria-label="Журнал вызовов модели" tabindex="0">
        <table class="data-table">
          <thead>
            <tr>
              <th scope="col">Время</th>
              <th scope="col">Модель</th>
              <th scope="col" class="num">Вход</th>
              <th scope="col" class="num">Выход</th>
              <th scope="col" class="num">Задержка</th>
              <th scope="col" class="num">Стоимость</th>
              <th scope="col">Откуда</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in calls" :key="c.id">
              <td class="t-num">{{ formatDateTime(c.created_at) }}</td>
              <td>
                <span class="t-mono">{{ c.model }}</span>
                <span v-if="c.status && c.status !== 'ok'" class="badge calls__status">
                  {{ STATUS[c.status] ?? c.status }}
                </span>
              </td>
              <td class="num">{{ formatInt(c.input_tokens) }}</td>
              <td class="num">{{ formatInt(c.output_tokens) }}</td>
              <td class="num">{{ formatMs(c.latency_ms) }}</td>
              <td class="num">
                <button
                  type="button"
                  class="num-btn"
                  :aria-label="`Стоимость ${formatCost(c.cost_rub)}. Как посчитано`"
                  @click="explainCost(c)"
                >
                  {{ formatCost(c.cost_rub) }}
                </button>
              </td>
              <td>
                <RouterLink
                  v-if="c.agent_run_id"
                  :to="`/searchops/runs/${encodeURIComponent(c.agent_run_id)}`"
                  class="link"
                >
                  запуск агента
                </RouterLink>
                <button
                  v-else-if="hasTrace(c)"
                  type="button"
                  class="link calls__trace"
                  :aria-label="`Открыть трассировку поиска ${c.query ? '«' + c.query + '»' : c.request_id}`"
                  @click="openTrace(c)"
                >
                  <template v-if="c.query">поиск «{{ c.query }}»</template>
                  <template v-else>трассировка поиска</template>
                </button>
                <span v-else-if="c.request_id" class="calls__req">
                  {{ KIND[c.kind] ?? 'поиск' }}
                  <span class="t-mono" :title="c.request_id">{{ c.request_id }}</span>
                </span>
                <span v-else class="t-muted">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="calls__foot">
        <span class="t-xs t-muted t-num">
          Задержка показанных вызовов: P50
          {{ latency.p50 === null ? '—' : formatMs(latency.p50) }} · P95
          {{ latency.p95 === null ? '—' : formatMs(latency.p95) }}
        </span>
        <button
          v-if="calls.length < total"
          type="button"
          class="btn btn--secondary btn--sm"
          :disabled="loadingMore"
          @click="$emit('more')"
        >
          {{ loadingMore ? 'Загружаем…' : 'Показать ещё' }}
        </button>
      </div>
    </template>
    <TraceDialog
      v-model:open="traceOpen"
      :request-id="traceFor?.id ?? null"
      :query="traceFor?.query"
    />
  </section>
</template>

<style scoped>
.calls {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  scroll-margin-top: calc(var(--header-h) + var(--space-4));
}
.calls__head,
.calls__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.calls__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.calls__req {
  display: flex;
  flex-direction: column;
}
.calls__trace {
  max-width: 16rem;
  text-align: left;
  overflow-wrap: anywhere;
}
.calls__status {
  margin-left: var(--space-1);
  color: var(--danger);
}
.data-table td:nth-child(1) {
  white-space: nowrap;
}
</style>
