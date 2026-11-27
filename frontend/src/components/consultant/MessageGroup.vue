<script setup lang="ts">
// Группа реплик одного собеседника подряд: значок консультанта только у первой реплики, 4 px внутри, 16 px между
// собеседниками (отступ задаёт лента).
import { MessageSquareText } from 'lucide-vue-next'
import type { ChatMessage } from '@/lib/consultant'
import MessageBubble from './MessageBubble.vue'

defineProps<{ role: 'user' | 'assistant'; messages: ChatMessage[] }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="mg" :class="`mg--${role}`">
    <span v-if="role === 'assistant'" class="mg__avatar" aria-hidden="true">
      <MessageSquareText :size="15" />
    </span>
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
.mg__avatar {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.mg__list {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
}
@media (max-width: 640px) {
  .mg__avatar {
    display: none;
  }
}
</style>
