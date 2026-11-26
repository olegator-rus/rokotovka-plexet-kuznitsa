<script setup lang="ts">
// Ошибка внутри блока раздела: что не загрузилось, текст сервера, номер для разбора и повтор.
// Остальные блоки страницы продолжают работать («Состояния экранов»).
import { computed } from 'vue'
import { CircleAlert, RotateCcw } from 'lucide-vue-next'
import type { ApiError } from '@/lib/apiError'

const props = defineProps<{ error: ApiError; title: string }>()
defineEmits<{ retry: [] }>()

/** 404 — эндпоинт ещё не подключён на сервере: так и пишем, без «что-то пошло не так». */
const notYet = computed(() => props.error.status === 404)
</script>

<template>
  <div class="panel block-error" role="alert">
    <CircleAlert :size="18" aria-hidden="true" class="block-error__icon" />
    <div class="block-error__body">
      <p class="block-error__title">{{ title }}</p>
      <p class="t-sm t-muted">
        {{
          notYet ? 'Сервер пока не отдаёт эти данные — эндпоинт ещё не подключён.' : error.message
        }}
      </p>
      <p v-if="error.traceId" class="t-xs t-muted">
        Номер для разбора: <span class="t-mono">{{ error.traceId }}</span>
      </p>
      <div>
        <button type="button" class="btn btn--secondary btn--sm" @click="$emit('retry')">
          <RotateCcw :size="14" aria-hidden="true" />
          Повторить
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.block-error {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-4);
  border-color: var(--danger-bg);
}
.block-error__icon {
  flex: none;
  margin-top: 1px;
  color: var(--danger);
}
.block-error__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.block-error__title {
  font-weight: var(--fw-medium);
}
</style>
