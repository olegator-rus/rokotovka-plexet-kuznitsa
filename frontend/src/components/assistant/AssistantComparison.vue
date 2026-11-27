<script setup lang="ts">
// Таблица сравнения (CHAT.md §3): характеристика и две редакции, три-пять строк; «Нет сведений» не становится «Нет».
// На телефоне — строка характеристики, под ней два значения, без горизонтальной прокрутки.
import type { ChatTable } from '@/api/typesGuide'
import AssistantSources from './AssistantSources.vue'

defineProps<{ table: ChatTable }>()
</script>

<template>
  <div class="as-cmp" data-testid="assistant-table">
    <table class="as-cmp__table">
      <caption class="sr-only">
        {{
          table.title
        }}
      </caption>
      <thead>
        <tr>
          <th scope="col"><span class="sr-only">Характеристика</span></th>
          <th v-for="c in table.columns" :key="c.offer_id" scope="col">{{ c.title }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="r in table.rows"
          :key="r.key"
          :class="{ 'as-cmp__row--sum': r.key === 'total' }"
          data-testid="assistant-table-row"
        >
          <th scope="row">{{ r.label }}</th>
          <td
            v-for="(cell, i) in r.cells"
            :key="i"
            :class="['t-num', `as-cmp__cell--${cell.mark}`]"
            :data-label="table.columns[i]?.title"
          >
            {{ cell.text }}
          </td>
        </tr>
      </tbody>
    </table>
    <AssistantSources v-if="table.source" :sources="[table.source]" />
  </div>
</template>

<style scoped>
.as-cmp {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.as-cmp__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  line-height: 18px;
  table-layout: fixed;
}
.as-cmp__table th,
.as-cmp__table td {
  padding: var(--space-2);
  border-bottom: var(--border-w) solid var(--border);
  text-align: left;
  vertical-align: top;
  overflow-wrap: anywhere;
}
.as-cmp__table thead th {
  color: var(--text-2);
  font-weight: var(--fw-semibold);
}
.as-cmp__table tbody th {
  width: 42%;
  font-weight: var(--fw-medium);
}
.as-cmp__cell--yes {
  color: var(--status-accepted);
  font-weight: var(--fw-semibold);
}
.as-cmp__cell--no {
  color: var(--text-2);
}
.as-cmp__cell--unknown {
  color: var(--text-3);
}
.as-cmp__row--sum th,
.as-cmp__row--sum td {
  font-weight: var(--fw-bold);
}
@media (max-width: 479px) {
  .as-cmp__table thead {
    display: none;
  }
  .as-cmp__table,
  .as-cmp__table tbody,
  .as-cmp__table tr {
    display: block;
  }
  .as-cmp__table tr {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    border-bottom: var(--border-w) solid var(--border);
    padding-block: var(--space-1);
  }
  .as-cmp__table tbody th {
    grid-column: 1 / -1;
    width: auto;
    border: 0;
    padding-bottom: 0;
  }
  .as-cmp__table td {
    border: 0;
  }
  .as-cmp__table td {
    display: block;
    min-width: 0;
  }
  .as-cmp__table td::before {
    content: attr(data-label);
    display: block;
    color: var(--text-3);
    font-size: 12px;
    font-weight: var(--fw-regular);
  }
}
</style>
