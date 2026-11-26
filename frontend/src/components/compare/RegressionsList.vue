<script setup lang="ts">
// Вкладка «Ухудшения»: запросы, где конфигурация проиграла предыдущей по NDCG@10, с разницей и причиной.
// Показываем открыто — это часть доказательства. Клик по запросу — поиск в проигравшей конфигурации
// с открытой трассой; «Сравнить» — вкладка «Один запрос» с двумя конфигурациями.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Columns2 } from 'lucide-vue-next'
import type { EvaluationView } from '@/api/types'
import DeltaMark from '@/components/common/DeltaMark.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import {
  type RegressionRow,
  changeReasonLabel,
  queryTypeLabel,
  regressionRows,
} from '@/lib/evalView'
import { formatInt, plural } from '@/lib/format'
import { formatMetric, metricDef, metricDelta } from '@/lib/metrics'

const props = defineProps<{ columns: EvaluationView[] }>()
const filter = defineModel<string>('filter', { default: 'all' })
defineEmits<{ compare: [query: string, left: string, right: string] }>()

const NDCG = metricDef('ndcg10')
const all = computed(() => regressionRows(props.columns))
const rows = computed(() =>
  filter.value === 'all' ? all.value : all.value.filter((r) => r.config === filter.value),
)
const options = computed(() => [
  { value: 'all', label: 'Все конфигурации' },
  ...props.columns
    .filter((c) => (c.regressions?.length ?? 0) > 0)
    .map((c) => ({ value: c.config, label: `${c.config} хуже ${c.against || 'предыдущей'}` })),
])

/** Дельту NDCG красим, только если она и есть потеря; иначе ухудшение — по причине (бейдж). */
function delta(r: RegressionRow) {
  const d = metricDelta(r.ndcg_to, r.ndcg_from, NDCG)
  return d && d.verdict === 'worse' ? d : null
}
function searchLink(query: string, config: string) {
  return { path: '/search', query: { q: query, config, trace: '1' } }
}
</script>

<template>
  <div class="regs">
    <div class="regs__head">
      <p class="t-sm t-muted">
        {{ formatInt(rows.length) }}
        {{ plural(rows.length, ['запрос', 'запроса', 'запросов']) }}, где конфигурация проиграла
        предыдущему столбцу. Худшие сверху.
      </p>
      <UiSelect
        v-if="options.length > 2"
        v-model="filter"
        :options="options"
        label="Конфигурация"
      />
    </div>

    <p v-if="all.length === 0" class="panel regs__empty">
      Ухудшений нет: ни одна конфигурация не проиграла предыдущей ни на одном запросе набора.
    </p>
    <p v-else-if="rows.length === 0" class="panel regs__empty">
      У выбранной конфигурации ухудшений нет.
    </p>

    <ul v-else class="regs__list">
      <li v-for="r in rows" :key="`${r.config}:${r.query_id}`" class="panel regs__row">
        <div class="regs__main">
          <RouterLink :to="searchLink(r.text, r.config)" class="link regs__query">
            {{ r.text }}
          </RouterLink>
          <span class="t-xs t-muted">
            <template v-if="r.type">{{ queryTypeLabel(r.type) }} · </template>
            <span class="t-num">{{ formatInt(r.freq) }} в месяц</span> · {{ r.query_id }}
          </span>
          <span v-if="r.reasons?.length" class="regs__reasons">
            <span v-for="why in r.reasons" :key="why" class="badge">{{
              changeReasonLabel(why)
            }}</span>
          </span>
        </div>
        <div class="regs__metric">
          <span class="t-xs t-muted">{{ NDCG.label }}</span>
          <span class="t-num">
            {{ r.against || '—' }} {{ formatMetric(r.ndcg_from, NDCG.format) }} → {{ r.config }}
            {{ formatMetric(r.ndcg_to, NDCG.format) }}
          </span>
          <DeltaMark v-if="delta(r)" :delta="delta(r)!" />
          <span v-else class="t-xs t-muted">NDCG не упал — см. причину</span>
        </div>
        <button
          v-if="r.against"
          type="button"
          class="btn btn--ghost btn--sm"
          :aria-label="`Сравнить «${r.text}» в ${r.against} и ${r.config}`"
          @click="$emit('compare', r.text, r.against, r.config)"
        >
          <Columns2 :size="14" aria-hidden="true" />
          Сравнить
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.regs {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.regs__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.regs__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.regs__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.regs__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
}
.regs__main,
.regs__metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.regs__metric {
  align-items: flex-end;
  font-size: var(--fs-sm);
}
.regs__query {
  overflow-wrap: anywhere;
}
.regs__reasons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin-top: 2px;
}
@media (max-width: 639px) {
  .regs__row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .regs__metric {
    grid-column: 1 / -1;
    grid-row: 2;
    align-items: flex-start;
  }
}
</style>
