<script setup lang="ts">
// Строка под поиском: как поиск работает без языковой модели (вместо плашки «Режим без LLM» в шапке,
// решение владельца). Видна всем, и гостю; при включённой LLM и пока режим неизвестен строки нет.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { Info } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'

const { meta } = storeToRefs(useAppStore())
const shown = computed(() => Boolean(meta.value) && meta.value?.llm_mode !== 'on')
</script>

<template>
  <p v-if="shown" class="llm-note t-sm">
    <Info :size="16" aria-hidden="true" />
    <span>
      Языковая модель (LLM) выключена: сложные запросы разбираются правилами. Поиск по тексту и по
      смыслу, исправление опечаток и фильтры работают как обычно.
    </span>
  </p>
</template>

<style scoped>
.llm-note {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  /* Левый край — вровень с краем поля поиска. */
  margin: var(--space-2) 0 0;
  color: var(--on-hero-2);
}
.llm-note svg {
  flex: none;
  margin-top: 1px;
}
</style>
