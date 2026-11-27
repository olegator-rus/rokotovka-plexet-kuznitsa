<script setup lang="ts">
// Хост тостов (reka Toast): один на приложение. Внизу справа (на мобильном — снизу по центру),
// ошибки объявляются срочно (foreground), остальное — фоном.
import {
  ToastAction,
  ToastClose,
  ToastDescription,
  ToastProvider,
  ToastRoot,
  ToastTitle,
  ToastViewport,
} from 'reka-ui'
import { CircleAlert, CircleCheck, Info, X } from 'lucide-vue-next'
import {
  TOAST_ACTION_LIFE_MS,
  TOAST_LIFE_MS,
  toastState,
  useToast,
  type ToastTone,
} from './useToast'

const { remove } = useToast()
const icons: Record<ToastTone, typeof Info> = { ok: CircleCheck, error: CircleAlert, info: Info }

function onOpen(id: number, open: boolean) {
  if (open) return
  const item = toastState.items.find((t) => t.id === id)
  if (item) item.open = false
  window.setTimeout(() => remove(id), 300)
}
</script>

<template>
  <ToastProvider label="Уведомление" :duration="TOAST_LIFE_MS">
    <ToastRoot
      v-for="t in toastState.items"
      :key="t.id"
      :open="t.open"
      :type="t.tone === 'error' ? 'foreground' : 'background'"
      :duration="t.action ? TOAST_ACTION_LIFE_MS : TOAST_LIFE_MS"
      :class="['ui-toast', `ui-toast--${t.tone}`]"
      @update:open="onOpen(t.id, $event)"
    >
      <component :is="icons[t.tone]" :size="18" aria-hidden="true" class="ui-toast__icon" />
      <div class="ui-toast__text">
        <ToastTitle class="ui-toast__title">{{ t.title }}</ToastTitle>
        <ToastDescription v-if="t.detail" class="ui-toast__detail">{{ t.detail }}</ToastDescription>
        <ToastAction
          v-if="t.action"
          :alt-text="t.action.label"
          class="btn btn--secondary btn--sm ui-toast__action"
          @click="t.action.run()"
        >
          {{ t.action.label }}
        </ToastAction>
      </div>
      <ToastClose class="btn btn--ghost btn--icon btn--sm" aria-label="Закрыть уведомление">
        <X :size="16" aria-hidden="true" />
      </ToastClose>
    </ToastRoot>
    <ToastViewport class="ui-toast-viewport" />
  </ToastProvider>
</template>

<style>
.ui-toast-viewport {
  position: fixed;
  left: 50%;
  bottom: var(--space-4);
  z-index: 70;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  width: min(24rem, calc(100vw - 2 * var(--gutter)));
  margin: 0;
  padding: 0;
  list-style: none;
  outline: none;
  transform: translateX(-50%);
}
@media (min-width: 1024px) {
  .ui-toast-viewport {
    left: auto;
    right: var(--space-6);
    bottom: var(--space-6);
    transform: none;
  }
}
/* Кнопка помощника и его окно стоят в правом нижнем углу — уведомление их не закрывает. */
@media (min-width: 1024px) {
  body:has(.as-entry) .ui-toast-viewport {
    right: calc(var(--fab-right) + var(--fab-size) + var(--space-4));
    bottom: var(--fab-bottom);
  }
  body:has(.search-page__guide) .ui-toast-viewport {
    right: calc(var(--gutter) + var(--chat-w) + var(--space-4));
  }
}
/* Открыта спецификация — уведомление не должно закрывать итог и «Запросить КП»: на широком экране слева, на
   затемнении, на узком — сверху, под шапкой панели. */
body:has(.spec-sheet) .ui-toast-viewport {
  top: calc(var(--header-h) + var(--space-2));
  bottom: auto;
}
@media (min-width: 1024px) {
  body:has(.spec-sheet) .ui-toast-viewport {
    top: auto;
    right: auto;
    bottom: var(--space-6);
    left: var(--space-6);
  }
}
.ui-toast {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-2) var(--space-3) var(--space-3);
  background: var(--surface-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-card);
  box-shadow: var(--shadow-overlay);
  animation: ui-fade-in var(--dur-panel) var(--ease-standard);
}
.ui-toast:not(:last-of-type) {
  display: none;
}
.ui-toast__icon {
  flex: none;
  margin-top: 1px;
  color: var(--text-2);
}
.ui-toast--error .ui-toast__icon {
  color: var(--danger);
}
.ui-toast__text {
  flex: 1;
  min-width: 0;
}
.ui-toast__title {
  font-weight: var(--fw-medium);
}
.ui-toast__action {
  margin-top: var(--space-2);
}
.ui-toast__detail {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
</style>
