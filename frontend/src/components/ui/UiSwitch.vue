<script setup lang="ts">
// Переключатель (reka Switch, role="switch") с подписью слева. Пробел/Enter переключают.
import { useId } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'

const model = defineModel<boolean>({ default: false })
defineProps<{ label: string }>()
const id = useId()
</script>

<template>
  <span class="ui-switch-row">
    <label :for="id" class="ui-switch-row__label">{{ label }}</label>
    <SwitchRoot :id="id" v-model="model" class="ui-switch">
      <SwitchThumb class="ui-switch__thumb" />
    </SwitchRoot>
  </span>
</template>

<style>
.ui-switch-row {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
}
.ui-switch-row__label {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  cursor: pointer;
  user-select: none;
}
.ui-switch {
  position: relative;
  box-sizing: border-box;
  flex: none;
  width: 2.25rem;
  height: 1.25rem;
  padding: 0;
  background: var(--surface-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  cursor: pointer;
  transition: background-color var(--dur-fast) var(--ease-standard);
}
.ui-switch[data-state='checked'] {
  background: var(--accent);
  border-color: var(--accent);
}
.ui-switch__thumb {
  display: block;
  width: 0.875rem;
  height: 0.875rem;
  border-radius: var(--radius-pill);
  background: var(--text-2);
  transform: translateX(0.125rem);
  transition: transform var(--dur-fast) var(--ease-standard);
}
.ui-switch__thumb[data-state='checked'] {
  background: var(--on-accent);
  transform: translateX(1.125rem);
}
</style>
