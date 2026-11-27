<script setup lang="ts">
// Подтверждение нового разговора (CONSULTANT.md §8): спецификация остаётся.
import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
} from 'reka-ui'

const open = defineModel<boolean>('open', { default: false })
defineEmits<{ confirm: [] }>()
</script>

<template>
  <AlertDialogRoot v-model:open="open">
    <AlertDialogPortal>
      <AlertDialogOverlay class="ui-scrim ncd__scrim" />
      <AlertDialogContent class="ui-dialog ncd" data-testid="consultant-new-dialog">
        <AlertDialogTitle class="t-h2">Начать новый разговор?</AlertDialogTitle>
        <AlertDialogDescription class="t-muted">
          Текущая переписка будет очищена. Товары в спецификации останутся.
        </AlertDialogDescription>
        <div class="ncd__actions">
          <AlertDialogCancel class="btn btn--secondary">Отмена</AlertDialogCancel>
          <AlertDialogAction
            class="btn btn--primary"
            data-testid="consultant-new-confirm"
            @click="$emit('confirm')"
          >
            Начать заново
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>

<style>
.ncd__scrim {
  z-index: 70;
}
.ncd.ui-dialog {
  z-index: 71;
}
.ncd__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
