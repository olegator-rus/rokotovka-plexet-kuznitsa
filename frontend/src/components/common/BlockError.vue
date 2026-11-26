<script setup lang="ts">
// Ошибка внутри блока раздела: что не загрузилось, текст сервера, номер для разбора и повтор.
// Остальные блоки страницы продолжают работать («Состояния экранов»).
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CircleAlert, LogIn, RotateCcw } from 'lucide-vue-next'
import { type ApiError, isAuthError } from '@/lib/apiError'
import { LOGIN_PATH } from '@/lib/auth'

const props = defineProps<{ error: ApiError; title: string }>()
defineEmits<{ retry: [] }>()

/** 404 — эндпоинт ещё не подключён на сервере: так и пишем, без «что-то пошло не так». */
const notYet = computed(() => props.error.status === 404)
/** 401 — экран бэк-офиса без входа сотрудника: на страницу входа с возвратом сюда. */
const needsLogin = computed(() => isAuthError(props.error))
const route = useRoute()
const router = useRouter()
const login = () => void router.push({ path: LOGIN_PATH, query: { returnTo: route.fullPath } })
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
        <button v-if="needsLogin" type="button" class="btn btn--secondary btn--sm" @click="login">
          <LogIn :size="14" aria-hidden="true" />
          Войти
        </button>
        <button v-else type="button" class="btn btn--secondary btn--sm" @click="$emit('retry')">
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
