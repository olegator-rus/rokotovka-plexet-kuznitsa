<script setup lang="ts">
// Ссылка на раздел магазина (CONSULTANT.md §6.6): строка кликабельна целиком. Адрес — из настроек ядра; не https —
// ссылку не рисуем.
import { computed } from 'vue'
import { ChevronRight } from 'lucide-vue-next'
import type { Attachment } from '@/api/typesConsultant'

const props = defineProps<{ data: Extract<Attachment, { type: 'link' }> }>()
const safe = computed(() => /^https:\/\//.test(props.data.url))
</script>

<template>
  <a
    v-if="safe"
    class="lp"
    :href="data.url"
    target="_blank"
    rel="noopener noreferrer"
    data-testid="consultant-link"
  >
    <span class="lp__body">
      <span class="lp__title">{{ data.title }}</span>
      <span class="lp__sub">Раздел интернет-магазина</span>
    </span>
    <ChevronRight :size="16" aria-hidden="true" />
  </a>
</template>

<style scoped>
.lp {
  display: flex;
  min-height: 3.5rem;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
  color: var(--text);
  text-decoration: none;
}
.lp:hover {
  background: var(--surface-2);
}
.lp__body {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
}
.lp__title {
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
}
.lp__sub {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
</style>
