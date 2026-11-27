<script setup lang="ts">
// Состав спецификации (CONSULTANT.md §6.5): строки с суммами, итог, «Открыть спецификацию». Своей копии в чате нет —
// строки приходят от ядра по спецификации из запроса.
import type { Attachment } from '@/api/typesConsultant'
import { formatMinor } from '@/lib/consultant'
import { plural } from '@/lib/format'
import { useSpecStore } from '@/stores/spec'

defineProps<{ data: Extract<Attachment, { type: 'spec_summary' }> }>()
const spec = useSpecStore()
</script>

<template>
  <section class="ss" data-testid="consultant-spec-summary">
    <header class="ss__head">
      Спецификация · {{ data.count }} {{ plural(data.count, ['позиция', 'позиции', 'позиций']) }}
    </header>
    <ul class="ss__list">
      <li v-for="it in data.items" :key="it.offerId" class="ss__row">
        <span class="ss__name">{{ it.title }} · {{ it.quantity }} шт.</span>
        <span class="t-num ss__sum">{{
          formatMinor(it.totalPriceMinor) || 'Цена по запросу'
        }}</span>
      </li>
    </ul>
    <div v-if="formatMinor(data.totalMinor)" class="ss__row ss__total">
      <span>Итого</span>
      <span class="t-num">{{ formatMinor(data.totalMinor) }}</span>
    </div>
    <button type="button" class="ss__open" @click="spec.open = true">Открыть спецификацию</button>
  </section>
</template>

<style scoped>
.ss {
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
  font-size: var(--fs-sm);
}
.ss__head {
  margin-bottom: var(--space-1);
  font-weight: var(--fw-semibold);
}
.ss__list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.ss__row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-1) 0;
}
.ss__name {
  min-width: 0;
  overflow-wrap: anywhere;
}
.ss__sum {
  white-space: nowrap;
}
.ss__total {
  border-top: 1px solid var(--border);
  font-weight: var(--fw-semibold);
}
.ss__open {
  min-height: 2rem;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
</style>
