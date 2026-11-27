<script setup lang="ts">
// Таблица сравнения (CONSULTANT.md §6.3): строки от ядра, строка «Стоимость» — из columns[].totalPriceMinor.
// «—» — подтверждено, что нет; неизвестное — «Нет данных». Название столбца раскрывает карточку с «В спецификацию».
// Телефон: характеристика строкой, под ней пары «столбец — значение», без горизонтальной прокрутки.
import { computed, ref } from 'vue'
import type { Attachment, ProductReference } from '@/api/typesConsultant'
import { comparisonCostRow, productTerms } from '@/lib/consultant'
import ProductCard from './ProductCard.vue'

const props = defineProps<{ data: Extract<Attachment, { type: 'comparison' }> }>()

/** Общая часть названий («Kaspersky Endpoint Security») — в заголовок, в столбцы — отличие. */
const split = computed(() => {
  const titles = props.data.columns.map((c) => c.title)
  const heads = titles.map((t) => t.split(' — ')[0] ?? t)
  const same = titles.every((t) => t.includes(' — ')) && heads.every((h) => h === heads[0])
  return {
    caption: same ? (heads[0] ?? '') : '',
    names: titles.map((t) => (same ? t.slice(t.indexOf(' — ') + 3) : t)),
  }
})
const terms = computed(() => {
  const first = props.data.columns[0]
  return first ? productTerms(first) : ''
})
const rows = computed(() => [
  ...props.data.rows.map((r) => ({
    label: r.label,
    values: props.data.columns.map((_, i) => cell(r.values[i])),
    cost: false,
  })),
  { label: 'Стоимость', values: comparisonCostRow(props.data.columns), cost: true },
])

function cell(v: string | undefined): string {
  const s = (v ?? '').trim()
  return s || 'Нет данных'
}

const picked = ref<number | null>(null)
const pickedProduct = computed<ProductReference | null>(() =>
  picked.value === null ? null : (props.data.columns[picked.value] ?? null),
)
function pick(i: number) {
  picked.value = picked.value === i ? null : i
}
</script>

<template>
  <section class="ct" data-testid="consultant-comparison">
    <header v-if="split.caption || terms" class="ct__head">
      <span v-if="split.caption" class="ct__caption">{{ split.caption }}</span>
      <span v-if="terms" class="ct__terms">{{ terms }}</span>
    </header>

    <table class="ct__table">
      <thead>
        <tr>
          <th scope="col"><span class="sr-only">Характеристика</span></th>
          <th v-for="(n, i) in split.names" :key="i" scope="col">
            <button
              type="button"
              class="ct__col"
              :aria-expanded="picked === i"
              data-testid="consultant-comparison-col"
              @click="pick(i)"
            >
              {{ n }}
            </button>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.label" :class="{ ct__cost: r.cost }">
          <th scope="row">{{ r.label }}</th>
          <td v-for="(v, i) in r.values" :key="i" class="t-num">
            <span v-if="v === '✓'" aria-label="есть">✓</span>
            <span v-else-if="v === '—'" aria-label="нет">—</span>
            <template v-else>{{ v }}</template>
          </td>
        </tr>
      </tbody>
    </table>

    <dl class="ct__pairs">
      <div v-for="r in rows" :key="r.label" class="ct__pair" :class="{ ct__cost: r.cost }">
        <dt>{{ r.label }}</dt>
        <dd v-for="(v, i) in r.values" :key="i">
          <span class="ct__pname">{{ split.names[i] }}</span>
          <span class="t-num">{{ v }}</span>
        </dd>
      </div>
      <div class="ct__pick">
        <button
          v-for="(n, i) in split.names"
          :key="i"
          type="button"
          class="ct__col"
          :aria-expanded="picked === i"
          @click="pick(i)"
        >
          {{ n }}
        </button>
      </div>
    </dl>

    <div v-if="pickedProduct" class="ct__card"><ProductCard :product="pickedProduct" /></div>
  </section>
</template>

<style scoped>
.ct {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
}
.ct__head {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border);
}
.ct__caption {
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
.ct__terms {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.ct__table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: var(--fs-sm);
}
.ct__table th,
.ct__table td {
  height: 2.5rem;
  padding: var(--space-1) var(--space-2);
  border-bottom: 1px solid var(--border);
  overflow-wrap: break-word;
  hyphens: auto;
  text-align: left;
  vertical-align: middle;
}
.ct__table th:first-child,
.ct__table td:first-child {
  padding-left: var(--space-3);
}
.ct__table th:first-child {
  width: 36%;
  font-weight: var(--fw-regular);
  color: var(--text-2);
}
.ct__table tbody tr:last-child > * {
  border-bottom: 0;
}
.ct__cost td {
  font-size: var(--cw-fs, var(--fs-body));
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.ct__col {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-semibold);
  text-align: left;
  text-decoration: underline dotted;
  text-underline-offset: 3px;
  cursor: pointer;
}
.ct__col:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.ct__pairs {
  display: none;
  margin: 0;
}
.ct__card {
  padding: var(--space-2);
  border-top: 1px solid var(--border);
}
@media (max-width: 640px) {
  .ct__table {
    display: none;
  }
  .ct__pairs {
    display: block;
  }
  .ct__pair {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border);
    font-size: var(--fs-sm);
  }
  .ct__pair dt {
    margin-bottom: 2px;
    color: var(--text-2);
  }
  .ct__pair dd {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    margin: 0;
  }
  .ct__pair.ct__cost dd .t-num {
    font-weight: var(--fw-semibold);
  }
  .ct__pname {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .ct__pick {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
  }
  .ct__pick .ct__col {
    min-height: 2.75rem;
  }
}
</style>
