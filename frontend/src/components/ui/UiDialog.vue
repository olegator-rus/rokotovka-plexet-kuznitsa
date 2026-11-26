<script setup lang="ts">
// Модальный диалог (reka Dialog): заголовок, крестик, фокус заперт внутри, Esc и клик по затемнению
// закрывают. Слоты: default (тело), footer (кнопки).
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { X } from 'lucide-vue-next'

const open = defineModel<boolean>('open', { default: false })

defineProps<{ title: string; description?: string; wide?: boolean }>()
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="ui-scrim" />
      <DialogContent
        v-bind="description ? {} : { 'aria-describedby': undefined }"
        :class="['ui-dialog', { 'ui-dialog--wide': wide }]"
      >
        <header class="ui-dialog__header">
          <DialogTitle class="t-h2">{{ title }}</DialogTitle>
          <DialogClose class="btn btn--ghost btn--icon btn--sm" aria-label="Закрыть">
            <X :size="16" aria-hidden="true" />
          </DialogClose>
        </header>
        <DialogDescription v-if="description" class="t-muted">{{ description }}</DialogDescription>
        <div class="ui-dialog__body"><slot /></div>
        <footer v-if="$slots.footer" class="ui-dialog__footer"><slot name="footer" /></footer>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style>
.ui-scrim {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: var(--scrim);
  animation: ui-fade-in var(--dur-panel) var(--ease-standard);
}
.ui-dialog {
  position: fixed;
  top: 50%;
  left: 50%;
  z-index: 51;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  width: calc(100vw - 2 * var(--gutter));
  max-width: var(--dialog-w);
  max-height: calc(100dvh - 2 * var(--space-6));
  overflow: auto;
  padding: var(--space-5);
  background: var(--surface);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  box-shadow: var(--shadow-overlay);
  transform: translate(-50%, -50%);
  animation: ui-fade-in var(--dur-panel) var(--ease-standard);
}
.ui-dialog--wide {
  max-width: var(--dialog-wide-w);
}
.ui-dialog:focus-visible {
  outline: none;
}
.ui-dialog__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.ui-dialog__footer {
  display: flex;
  flex-wrap: wrap-reverse;
  justify-content: flex-end;
  gap: var(--space-2);
}
@keyframes ui-fade-in {
  from {
    opacity: 0;
  }
}
@keyframes ui-slide-right {
  from {
    transform: translateX(-100%);
  }
}
</style>
