<script setup lang="ts">
// Поле ввода (CONSULTANT.md §5): от 44 px до четырёх строк, Enter отправляет, Shift+Enter переносит; пустое не
// отправляется; пока идёт ответ, писать можно, отправить — после завершения. Черновик живёт в состоянии.
import { nextTick, onMounted, ref, watch } from 'vue'
import { SendHorizontal } from 'lucide-vue-next'

const draft = defineModel<string>({ required: true })
const props = defineProps<{ busy: boolean }>()
const emit = defineEmits<{ send: [text: string] }>()

const area = ref<HTMLTextAreaElement | null>(null)

function fit() {
  const el = area.value
  if (!el) return
  el.style.height = 'auto'
  // scrollHeight без рамки: +2 px, иначе у пустого поля появляется полоса прокрутки.
  const h = el.scrollHeight + 2
  el.style.height = `${Math.min(h, 120)}px`
  el.style.overflowY = h > 120 ? 'auto' : 'hidden'
}
watch(draft, () => void nextTick(fit))
onMounted(fit)

function submit() {
  if (props.busy || !draft.value.trim()) return
  emit('send', draft.value)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    submit()
  }
}

defineExpose({ focus: () => area.value?.focus({ preventScroll: true }) })
</script>

<template>
  <form class="mc consultant-composer" @submit.prevent="submit">
    <textarea
      ref="area"
      v-model="draft"
      class="mc__input"
      rows="1"
      placeholder="Напишите сообщение…"
      aria-label="Сообщение консультанту"
      data-testid="consultant-input"
      @keydown="onKey"
    />
    <button
      type="submit"
      class="mc__send"
      aria-label="Отправить"
      title="Отправить"
      :disabled="busy || !draft.trim()"
      data-testid="consultant-send"
    >
      <SendHorizontal :size="18" aria-hidden="true" />
    </button>
  </form>
</template>

<style scoped>
.mc {
  display: flex;
  flex-shrink: 0;
  align-items: flex-end;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--border);
  background: var(--surface);
}
.mc__input {
  flex: 1;
  min-width: 0;
  min-height: 2.75rem;
  max-height: 7.5rem;
  padding: 0.625rem var(--space-3);
  border: 1px solid var(--border-control);
  border-radius: 0.875rem;
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-body);
  line-height: 1.5;
  resize: none;
}
.mc__input::placeholder {
  color: var(--placeholder);
}
.mc__input:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 0;
}
.mc__send {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 2.75rem;
  height: 2.75rem;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  cursor: pointer;
}
.mc__send:hover:not(:disabled) {
  background: var(--accent-hover);
}
.mc__send:disabled {
  background: var(--surface-3);
  color: var(--text-3);
  cursor: default;
}
.mc__send:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
@media (max-width: 640px) {
  .mc {
    padding: var(--space-3);
    padding-bottom: max(var(--space-3), env(safe-area-inset-bottom));
  }
  .mc__input {
    font-size: 16px;
  }
}
</style>
