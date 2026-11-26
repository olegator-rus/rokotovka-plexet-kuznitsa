<script setup lang="ts">
// Разница правил списком: добавленное и удалённое словами, не сырой JSON.
// Цвета нет: знак операции — иконка и подпись.
import { computed } from 'vue'
import { Minus, Plus } from 'lucide-vue-next'
import type { RulePatch } from '@/api/types'
import { diffLines } from '@/lib/proposals'

const props = defineProps<{ patch: RulePatch | null; empty?: string }>()
const lines = computed(() => diffLines(props.patch))
</script>

<template>
  <ul v-if="lines.length" class="diff">
    <li v-for="(l, i) in lines" :key="i" :class="['diff__line', `diff__line--${l.op}`]">
      <component :is="l.op === 'add' ? Plus : Minus" :size="14" aria-hidden="true" />
      <span class="sr-only">{{ l.op === 'add' ? 'Добавить:' : 'Удалить:' }}</span>
      <span class="diff__text">{{ l.text }}</span>
      <span class="diff__note t-xs"
        >{{ l.op === 'add' ? 'добавить' : 'удалить' }} · {{ l.note }}</span
      >
    </li>
  </ul>
  <p v-else class="t-sm t-muted">{{ empty ?? 'Правила поиска не меняются' }}</p>
</template>

<style scoped>
.diff {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.diff__line {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr);
  align-items: baseline;
  gap: 0 var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.diff__line > svg {
  align-self: center;
  color: var(--text-2);
}
.diff__text {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.diff__line--remove .diff__text {
  text-decoration: line-through;
  text-decoration-color: var(--text-3);
}
.diff__note {
  grid-column: 2;
  color: var(--text-2);
}
</style>
