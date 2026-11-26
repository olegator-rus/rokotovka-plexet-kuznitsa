<script setup lang="ts">
// Ошибка внутри блока раздела: что не загрузилось, текст сервера, номер для разбора и повтор.
// Остальные блоки страницы продолжают работать («Состояния экранов»).
import { computed } from 'vue'
import { RouterLink, useRoute, useRouter, type RouteLocationRaw } from 'vue-router'
import { ArrowLeft, CircleAlert, LogIn, RotateCcw } from 'lucide-vue-next'
import { type ApiError, isAuthError } from '@/lib/apiError'
import { LOGIN_PATH } from '@/lib/auth'

const props = defineProps<{
  error: ApiError
  title: string
  /** 404 «нет такого объекта» (запуск, предложение): ссылка назад вместо «Повторить». */
  back?: { label: string; to: RouteLocationRaw }
}>()
defineEmits<{ retry: [] }>()

/** Объекта нет (404 с конвертом сервера): текст сервера и ссылка назад, если она есть. */
const missing = computed(() => props.error.status === 404 && props.error.code !== 'HTTP_404')
/** 404 без конверта сервера — адрес не обслуживается (прокси, старый сервер): так и пишем. */
const notYet = computed(() => props.error.status === 404 && !missing.value)
const text = computed(() => {
  if (notYet.value) return 'Сервер не отдаёт эти данные по этому адресу.'
  // Текст сервера повторяет заголовок («Запуск не найден») — пояснение вместо повтора.
  if (missing.value && props.error.message.trim().replace(/\.$/, '') === props.title.trim())
    return 'Такого объекта нет: ссылка устарела или в ней ошибка.'
  return props.error.message
})
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
        {{ text }}
      </p>
      <p v-if="error.traceId" class="t-xs t-muted">
        Номер для разбора: <span class="t-mono">{{ error.traceId }}</span>
      </p>
      <div>
        <button v-if="needsLogin" type="button" class="btn btn--secondary btn--sm" @click="login">
          <LogIn :size="14" aria-hidden="true" />
          Войти
        </button>
        <RouterLink v-else-if="missing && back" :to="back.to" class="btn btn--secondary btn--sm">
          <ArrowLeft :size="14" aria-hidden="true" />
          {{ back.label }}
        </RouterLink>
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
