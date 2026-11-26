<script setup lang="ts">
// Блок «Месяц»: два сценария объёма. LLM и агент — из замеров сервера, сервер и число
// исследований агента — допущения, их можно поправить прямо здесь (пометка «допущение»).
// Без замера строка честно пустая, итог помечен «без …».
import { computed, reactive } from 'vue'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { monthScenario, parseRubInput } from '@/lib/costMonth'
import { formatCost, formatInt, formatRub } from '@/lib/format'

const props = defineProps<{
  llmPerRequestRub: number | null
  agentRunRub: number | null
  pricingVersion: string
}>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

// Допущения по умолчанию — ARCHITECTURE §7: сервер 4 vCPU / 16 ГБ ≈ 7 500 ₽ при 20 тыс.; при 200 тыс. —
// 8 vCPU / 32 ГБ ≈ 15 000 ₽ (следует из «0,79–0,89 ₽ на 8 vCPU при 20 тыс.»); исследований агента —
// 100 на 20 тыс. запросов (нижняя граница 100–500), при 200 тыс. — пропорционально.
const scenarios = reactive([
  { requests: 20_000, server: '7 500', runs: '100', serverNote: '4 vCPU / 16 ГБ' },
  { requests: 200_000, server: '15 000', runs: '1 000', serverNote: '8 vCPU / 32 ГБ' },
])

const results = computed(() =>
  scenarios.map((s) =>
    monthScenario({
      requests: s.requests,
      serverRub: parseRubInput(s.server) ?? 0,
      llmPerRequestRub: props.llmPerRequestRub,
      agentRunRub: props.agentRunRub,
      agentRuns: parseRubInput(s.runs) ?? 0,
    }),
  ),
)

function explainTotal(i: number) {
  const s = scenarios[i]
  const r = results.value[i]
  emit('explain', {
    title: `Месяц при ${formatInt(s.requests)} запросов`,
    value: formatRub(r.total),
    how: 'Сервер (допущение) + онлайн-LLM (средняя измеренная стоимость LLM на запрос × число запросов) + агент (измеренная стоимость исследования × число исследований, допущение).',
    rows: [
      { label: `Сервер, ${s.serverNote} (допущение)`, value: formatRub(r.server) },
      {
        label: 'Онлайн-LLM',
        value:
          r.llm === null
            ? 'нет замера'
            : `${formatCost(props.llmPerRequestRub ?? 0)} × ${formatInt(s.requests)} = ${formatRub(r.llm)}`,
      },
      {
        label: 'Агент',
        value:
          r.agent === null
            ? 'нет замера'
            : `${formatCost(props.agentRunRub ?? 0)} × ${s.runs} = ${formatRub(r.agent)}`,
      },
      { label: 'На запрос', value: r.perRequest === null ? '—' : formatCost(r.perRequest) },
    ],
    source: `GET /api/costs · прайс ${props.pricingVersion}`,
  })
}
</script>

<template>
  <section class="panel month" aria-labelledby="month-title">
    <div class="month__head">
      <h2 id="month-title" class="t-h2">Месяц</h2>
      <span class="t-xs t-muted">поля с пометкой «допущение» можно править</span>
    </div>
    <div class="data-scroll month__scroll">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Статья</th>
            <th v-for="s in scenarios" :key="s.requests" scope="col" class="num">
              {{ formatInt(s.requests) }} запросов
            </th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th scope="row">
              <span class="month__row">Сервер <span class="badge">допущение</span></span>
            </th>
            <td v-for="(s, i) in scenarios" :key="i" class="num">
              <div class="month__cell">
                <span class="month__field">
                  <input
                    v-model="s.server"
                    inputmode="decimal"
                    class="month__input"
                    :aria-label="`Сервер в месяц, рублей, при ${formatInt(s.requests)} запросов`"
                  />
                  <span aria-hidden="true">₽</span>
                </span>
                <span class="t-xs t-muted">{{ s.serverNote }}</span>
              </div>
            </td>
          </tr>
          <tr>
            <th scope="row">
              <span class="month__row">Онлайн-LLM <span class="t-xs t-muted">замер</span></span>
            </th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <span :class="{ 't-muted': r.llm === null }">
                {{ r.llm === null ? 'нет замера' : formatRub(r.llm) }}
              </span>
            </td>
          </tr>
          <tr>
            <th scope="row">
              <span class="month__row">
                Агент <span class="t-xs t-muted">замер × исследований</span>
              </span>
            </th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <div class="month__cell">
                <span :class="{ 't-muted': r.agent === null }">
                  {{ r.agent === null ? 'нет замера' : formatRub(r.agent) }}
                </span>
                <span class="month__field">
                  <input
                    v-model="scenarios[i].runs"
                    inputmode="numeric"
                    class="month__input month__input--sm"
                    :aria-label="`Исследований агента в месяц при ${formatInt(scenarios[i].requests)} запросов, допущение`"
                  />
                  <span class="t-xs t-muted" aria-hidden="true">иссл.</span>
                </span>
              </div>
            </td>
          </tr>
          <tr class="month__total">
            <th scope="row">Итого</th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <div class="month__cell">
                <button
                  type="button"
                  class="num-btn"
                  :aria-label="`Итого ${formatRub(r.total)}. Как посчитано`"
                  @click="explainTotal(i)"
                >
                  {{ formatRub(r.total) }}
                </button>
                <span v-if="r.partial" class="t-xs t-muted">без строк «нет замера»</span>
              </div>
            </td>
          </tr>
          <tr>
            <th scope="row">На запрос</th>
            <td v-for="(r, i) in results" :key="i" class="num">
              {{ r.perRequest === null ? '—' : formatCost(r.perRequest) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.month {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.month__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
}
.month__scroll {
  border-radius: var(--radius-control);
}
.month th[scope='row'] {
  font-weight: var(--fw-regular);
}
.month__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.month__cell {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1);
}
.month__field {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.month__input {
  box-sizing: border-box;
  width: 6.5rem;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
  font: inherit;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.month__input--sm {
  width: 4.5rem;
}
.month__total th,
.month__total td {
  font-weight: var(--fw-semibold);
}
</style>
