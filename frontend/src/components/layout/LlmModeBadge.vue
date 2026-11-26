<script setup lang="ts">
// Состояние API в шапке: показывается только сбой — «API не отвечает» и кнопка повтора. Плашку режима
// («LLM включена» / «Режим без LLM») убрали по решению владельца.
import { storeToRefs } from 'pinia'
import { RotateCcw, Unplug } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const { metaStatus } = storeToRefs(app)
</script>

<template>
  <span v-if="metaStatus === 'error'" class="mode-error">
    <span class="badge mode">
      <Unplug :size="14" aria-hidden="true" />
      <span>API не отвечает</span>
    </span>
    <button
      type="button"
      class="btn btn--ghost btn--icon btn--sm"
      aria-label="Повторить запрос настроек"
      @click="app.loadMeta(true)"
    >
      <RotateCcw :size="14" aria-hidden="true" />
    </button>
  </span>
</template>

<style scoped>
.mode {
  min-height: var(--control-h-sm);
}
.mode-error {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
</style>
