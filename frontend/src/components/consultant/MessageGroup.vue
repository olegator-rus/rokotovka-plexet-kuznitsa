<script setup lang="ts">
// Группа реплик одного собеседника подряд: 4 px внутри, 16 px между собеседниками (отступ задаёт лента). Значка у
// реплик консультанта нет (он только в шапке окна): ответ и вложения занимают всю ширину ленты.
import type { ChatMessage } from '@/lib/consultant'
import MessageBubble from './MessageBubble.vue'

defineProps<{ role: 'user' | 'assistant'; messages: ChatMessage[] }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="mg" :class="`mg--${role}`">
    <div class="mg__list">
      <MessageBubble v-for="m in messages" :key="m.id" :message="m" @retry="$emit('retry')" />
    </div>
  </div>
</template>

<style scoped>
.mg {
  display: flex;
  min-width: 0;
  gap: var(--space-2);
}
.mg--user {
  justify-content: flex-end;
}
.mg__list {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
}
</style>
