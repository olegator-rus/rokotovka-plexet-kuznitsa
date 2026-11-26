<script setup lang="ts">
// Подсказка (reka Tooltip) к элементу из слота: по наведению и по фокусу с клавиатуры.
// Провайдер — один на приложение (App.vue).
import { TooltipContent, TooltipPortal, TooltipRoot, TooltipTrigger } from 'reka-ui'

withDefaults(defineProps<{ text: string; side?: 'top' | 'bottom' | 'left' | 'right' }>(), {
  side: 'top',
})
</script>

<template>
  <TooltipRoot>
    <TooltipTrigger as-child><slot /></TooltipTrigger>
    <TooltipPortal>
      <TooltipContent :side="side" :side-offset="6" class="ui-tooltip">{{ text }}</TooltipContent>
    </TooltipPortal>
  </TooltipRoot>
</template>

<style>
.ui-tooltip {
  z-index: 80;
  max-width: 18rem;
  padding: var(--space-1) var(--space-2);
  background: var(--surface-3);
  color: var(--text);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  box-shadow: var(--shadow-overlay);
}
</style>
