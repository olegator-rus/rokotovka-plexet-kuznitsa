<script setup lang="ts">
// Плашка режима из /api/meta: «LLM включена» / «Режим без LLM». Пока ответа нет — «Проверяем API»;
// API не ответил — честно об этом и кнопка повтора.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { Cpu, RotateCcw, Unplug } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const { meta, metaStatus } = storeToRefs(app)
const llmOn = computed(() => meta.value?.llm_mode === 'on')
</script>

<template>
  <span v-if="metaStatus === 'ready'" class="badge badge--strong mode">
    <Cpu :size="14" aria-hidden="true" />
    <span class="mode__long">{{ llmOn ? 'LLM включена' : 'Режим без LLM' }}</span>
    <span class="mode__short" aria-hidden="true">{{ llmOn ? 'LLM вкл.' : 'Без LLM' }}</span>
  </span>
  <span v-else-if="metaStatus === 'error'" class="mode-error">
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
  <span v-else class="badge mode" aria-live="polite">Проверяем API…</span>
</template>

<style scoped>
.mode {
  min-height: var(--control-h-sm);
}
.mode__short {
  display: none;
}
.mode-error {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
@media (max-width: 639px) {
  .mode__long {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }
  .mode__short {
    display: inline;
  }
}
</style>
