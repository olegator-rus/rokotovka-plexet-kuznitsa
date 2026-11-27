<script setup lang="ts">
// Источник (CHAT.md §3): текстовая ссылка «Документация Kaspersky»; по нажатию — дословный фрагмент из базы знаний
// и «Открыть документ». Дат во фрагментах нет (сервер их вырезает).
import { ref } from 'vue'
import type { GuideScreenSource } from '@/api/typesGuide'

defineProps<{ sources: GuideScreenSource[] }>()
const openIdx = ref(-1)
</script>

<template>
  <ul class="as-src" data-testid="assistant-sources">
    <li v-for="(s, i) in sources" :key="s.url">
      <button
        v-if="s.fragment"
        type="button"
        class="as-src__btn"
        :aria-expanded="openIdx === i"
        @click="openIdx = openIdx === i ? -1 : i"
      >
        {{ s.label }}
      </button>
      <a v-else :href="s.url" target="_blank" rel="noopener" class="as-src__btn">{{ s.label }}</a>
      <div v-if="openIdx === i" class="as-src__frag" data-testid="assistant-fragment">
        <q>{{ s.fragment }}</q>
        <a :href="s.url" target="_blank" rel="noopener" class="link">Открыть документ</a>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.as-src {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.as-src__btn {
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
.as-src__frag {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-top: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border-left: 3px solid var(--border);
  background: var(--surface-2);
  font-size: 12px;
  line-height: 16px;
  overflow-wrap: anywhere;
}
</style>
