<script setup lang="ts">
// Поле ввода и строка условий (CHAT.md §2): «Учитываю: 20 ПК · 1 год · новая · Сбросить». Условия меняют репликой;
// «Сбросить» очищает условия, но не историю и не спецификацию.
import { nextTick, ref } from 'vue'
import { SendHorizontal } from 'lucide-vue-next'

defineProps<{ conditions: string; busy: boolean; hasConversation: boolean }>()
const model = defineModel<string>({ required: true })
const emit = defineEmits<{ send: [text: string]; reset: [] }>()
const input = ref<HTMLTextAreaElement | null>(null)

function submit() {
  const t = model.value.trim()
  if (!t) return
  emit('send', t)
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    submit()
  }
}
function focus() {
  void nextTick(() => input.value?.focus({ preventScroll: true }))
}
defineExpose({ focus })
</script>

<template>
  <div class="as-comp">
    <p v-if="conditions" class="as-comp__cond" data-testid="assistant-conditions">
      <span class="as-comp__cond-text">{{ conditions }}</span>
      <button
        type="button"
        class="as-comp__reset"
        data-testid="assistant-reset"
        @click="emit('reset')"
      >
        Сбросить
      </button>
    </p>
    <form class="as-comp__form" @submit.prevent="submit">
      <label class="sr-only" for="assistant-input">Сообщение помощнику</label>
      <textarea
        id="assistant-input"
        ref="input"
        v-model="model"
        class="as-comp__input"
        rows="1"
        maxlength="500"
        :placeholder="hasConversation ? 'Уточнить условия…' : 'Напишите, что нужно подобрать'"
        data-testid="assistant-input"
        @keydown="onKey"
      />
      <button
        type="submit"
        class="as-comp__send"
        :disabled="busy || !model.trim()"
        aria-label="Отправить"
        data-testid="assistant-send"
      >
        <SendHorizontal :size="18" aria-hidden="true" />
      </button>
    </form>
  </div>
</template>

<style scoped>
.as-comp {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2) 16px calc(var(--space-3) + env(safe-area-inset-bottom, 0px));
  border-top: var(--border-w) solid var(--border);
  background: var(--surface);
}
.as-comp__cond {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  margin: 0;
  color: var(--text-2);
  font-size: 12px;
  line-height: 16px;
}
.as-comp__cond-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.as-comp__reset {
  flex: none;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  cursor: pointer;
}
.as-comp__form {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}
.as-comp__input {
  flex: 1;
  min-height: 40px;
  max-height: 120px;
  padding: 9px var(--space-3);
  border: var(--border-w) solid var(--field-border, var(--border-control));
  border-radius: var(--radius-control);
  background: var(--field-surface, var(--surface));
  color: var(--field-text, var(--text));
  font: inherit;
  font-size: 14px;
  line-height: 20px;
  resize: none;
  field-sizing: content;
}
.as-comp__input:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 1px;
}
.as-comp__send {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 0;
  border-radius: var(--radius-control);
  background: var(--accent);
  color: var(--on-accent);
  cursor: pointer;
}
.as-comp__send:disabled {
  background: var(--surface-3);
  color: var(--text-3);
  cursor: default;
}
@media (max-width: 1023px) {
  .as-comp__input {
    font-size: 16px;
    min-height: var(--touch-min);
  }
  .as-comp__send {
    width: var(--touch-min);
    height: var(--touch-min);
  }
  .as-comp__reset {
    min-height: var(--touch-min);
  }
}
</style>
