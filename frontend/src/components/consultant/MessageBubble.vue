<script setup lang="ts">
// Пузырь сообщения (CONSULTANT.md §5): текст первичен, вложения — после текста. Текст разбирается на узлы
// (lib/consultant.ts parseRich): полужирное и ссылки на store.softline.ru; сырой текст не идёт в v-html.
import { computed } from 'vue'
import { RotateCcw } from 'lucide-vue-next'
import { parseRich, type ChatMessage } from '@/lib/consultant'
import AttachmentRenderer from './attachments/AttachmentRenderer.vue'

const props = defineProps<{ message: ChatMessage }>()
defineEmits<{ retry: [] }>()

const nodes = computed(() => (props.message.role === 'user' ? null : parseRich(props.message.text)))
</script>

<template>
  <div
    class="mb"
    :class="`mb--${message.role}`"
    data-testid="consultant-message"
    :data-role="message.role"
  >
    <div v-if="message.retry" class="mb__bubble mb__bubble--error" role="alert">
      <p class="mb__text">Не получилось подготовить ответ.</p>
      <button
        type="button"
        class="btn btn--secondary btn--sm mb__retry"
        data-testid="consultant-retry"
        @click="$emit('retry')"
      >
        <RotateCcw :size="14" aria-hidden="true" /> Попробовать ещё раз
      </button>
    </div>
    <div v-else-if="message.text.trim()" class="mb__bubble">
      <p v-if="message.role === 'user'" class="mb__text">{{ message.text }}</p>
      <p v-else class="mb__text" data-testid="consultant-text">
        <template v-for="(n, i) in nodes" :key="i">
          <strong v-if="n.kind === 'bold'" class="mb__bold">{{ n.text }}</strong>
          <a
            v-else-if="n.kind === 'link'"
            class="mb__link"
            :href="n.href"
            target="_blank"
            rel="noopener noreferrer"
            >{{ n.text }}</a
          >
          <template v-else>{{ n.text }}</template>
        </template>
      </p>
    </div>
    <div v-if="message.attachments.length" class="mb__attachments">
      <AttachmentRenderer v-for="a in message.attachments" :key="a.key" :view="a" />
    </div>
  </div>
</template>

<style scoped>
.mb {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--space-1);
}
.mb--user {
  align-items: flex-end;
}
.mb__bubble {
  max-width: 100%;
  padding: var(--space-3);
  border-radius: 0.875rem;
  background: var(--surface-2);
  color: var(--text);
  overflow-wrap: anywhere;
}
.mb--user .mb__bubble {
  max-width: 85%;
  background: var(--accent-soft);
}
.mb__bubble--error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  border: 1px solid var(--border);
  background: var(--surface);
}
.mb__text {
  margin: 0;
  font-size: var(--cw-fs, var(--fs-body));
  line-height: var(--cw-lh, 1.5);
  white-space: pre-line;
}
.mb__bold {
  font-weight: var(--fw-semibold);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.mb__link {
  color: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.mb__retry {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.mb__attachments {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-1);
}
</style>
