<script setup lang="ts">
// Смета (вторая очередь, CONSULTANT.md §6.8): пока есть нерассчитанные позиции, известную сумму итогом не называем.
import { computed } from 'vue'
import type { Attachment } from '@/api/typesConsultant'
import { formatMinor } from '@/lib/consultant'

const props = defineProps<{ data: Extract<Attachment, { type: 'estimate' }> }>()
const partial = computed(() => props.data.lines.some((l) => l.status !== 'known'))
</script>

<template>
  <section class="es" data-testid="consultant-estimate">
    <ul class="es__list">
      <li v-for="(l, i) in data.lines" :key="i" class="es__row">
        <span class="es__name">{{ l.label }}</span>
        <span class="t-num es__sum">
          {{
            l.status === 'known'
              ? formatMinor(l.totalPriceMinor) || 'Цена по запросу'
              : 'Не рассчитано'
          }}
        </span>
      </li>
    </ul>
    <div v-if="formatMinor(data.knownTotalMinor)" class="es__row es__total">
      <span>{{ partial ? 'Стоимость рассчитанных позиций' : 'Итого' }}</span>
      <span class="t-num">{{ formatMinor(data.knownTotalMinor) }}</span>
    </div>
    <p v-if="data.note" class="es__note">{{ data.note }}</p>
  </section>
</template>

<style scoped>
.es {
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
  font-size: var(--fs-sm);
}
.es__list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.es__row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-1) 0;
}
.es__name {
  min-width: 0;
  overflow-wrap: anywhere;
}
.es__sum {
  white-space: nowrap;
}
.es__total {
  border-top: 1px solid var(--border);
  font-weight: var(--fw-semibold);
}
.es__note {
  margin: var(--space-1) 0 0;
  color: var(--text-2);
  font-size: var(--fs-xs);
}
</style>
