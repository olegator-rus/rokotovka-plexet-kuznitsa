<script setup lang="ts">
// Ошибка поиска: движок или API недоступны — «Поиск временно недоступен» и повтор.
// Ответ 4xx с текстом сервера показывается как есть (например, пустой или слишком длинный запрос).
import { computed } from 'vue'
import { CircleAlert, RotateCcw } from 'lucide-vue-next'
import type { ApiError } from '@/lib/apiError'

const props = defineProps<{ error: ApiError }>()
defineEmits<{ retry: [] }>()

const unavailable = computed(() => props.error.status === 0 || props.error.status >= 500)
</script>

<template>
  <div class="panel search-error" role="alert">
    <CircleAlert :size="20" aria-hidden="true" class="search-error__icon" />
    <div class="search-error__body">
      <p class="t-h2">{{ unavailable ? 'Поиск временно недоступен' : 'Запрос не выполнен' }}</p>
      <p class="t-muted">{{ error.message }}</p>
      <p v-if="error.traceId" class="t-xs t-muted">
        Номер запроса для разбора: <span class="t-mono">{{ error.traceId }}</span>
      </p>
      <div>
        <button type="button" class="btn btn--secondary" @click="$emit('retry')">
          <RotateCcw :size="16" aria-hidden="true" />
          Повторить
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-error {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
  border-color: var(--danger-bg);
}
.search-error__icon {
  flex: none;
  margin-top: 2px;
  color: var(--danger);
}
.search-error__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
</style>
