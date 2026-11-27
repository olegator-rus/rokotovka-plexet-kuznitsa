<script setup lang="ts">
// Экран входа: карточка формы «логин + пароль» (аккаунты AUTH_ACCOUNTS) и демо-аккаунты одной
// кнопкой — POST /api/auth/demo-login, пароль в браузер не уходит; демо-блок виден, только если сервер включил
// DEMO_LOGIN_BUTTONS (GET /api/auth/config). Ошибка — одной фразой, 429 — с отсчётом. После входа — returnTo или
// стартовый экран роли. Витрина работает и без входа: «Продолжить как гость».
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  ArrowRight,
  CircleAlert,
  Eye,
  EyeOff,
  Info,
  LoaderCircle,
  LockKeyhole,
  RotateCcw,
  UserRound,
} from 'lucide-vue-next'
import type { Persona } from '@/api/auth'
import PersonaList from '@/components/auth/PersonaList.vue'
import BrandMark from '@/components/layout/BrandMark.vue'
import { type ApiError, MSG_NETWORK, MSG_SERVER, isApiError } from '@/lib/apiError'
import { afterLogin, isBackofficePath, safeReturnTo } from '@/lib/auth'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const login = ref('')
const password = ref('')
const showPassword = ref(false)
const tried = ref(false)
const busy = ref<string | null>(null)
const passwordInput = ref<HTMLInputElement | null>(null)

const error = ref<{ text: string; retry?: () => void } | null>(null)
const waitSeconds = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

const locked = computed(() => busy.value !== null || waitSeconds.value > 0)
const config = computed(() => auth.config)
const personas = computed<Persona[]>(() => config.value?.personas ?? [])
const demoOn = computed(() => !!config.value?.demo_login && personas.value.length > 0)
const returnTo = computed(() => safeReturnTo(route.query.returnTo))
/** Сюда вернули из бэк-офиса: без входа или покупателем. */
const fromBackoffice = computed(() => !!returnTo.value && isBackofficePath(returnTo.value))
const expired = computed(() => route.query.expired === '1')

onMounted(() => void auth.loadConfig(true))
onBeforeUnmount(() => clearInterval(timer))

function startWait(seconds: number) {
  clearInterval(timer)
  waitSeconds.value = seconds
  timer = setInterval(() => {
    waitSeconds.value = Math.max(0, waitSeconds.value - 1)
    if (waitSeconds.value === 0) {
      clearInterval(timer)
      error.value = null
    }
  }, 1000)
}

function showError(e: unknown, retry: () => void) {
  const err: ApiError | null = isApiError(e) ? e : null
  if (!err || err.status === 0) error.value = { text: MSG_NETWORK, retry }
  else if (err.status === 429) {
    error.value = { text: err.message }
    startWait(err.retryAfter ?? 60)
  } else if (err.status >= 500) error.value = { text: MSG_SERVER, retry }
  else error.value = { text: err.message }
}

async function done() {
  await router.replace(afterLogin(auth.role, route.query.returnTo))
}

async function submit() {
  tried.value = true
  if (locked.value || !login.value.trim() || !password.value) return
  error.value = null
  busy.value = 'form'
  try {
    await auth.login(login.value, password.value)
    await done()
  } catch (e) {
    showError(e, submit)
    if (isApiError(e) && e.status === 401) {
      // Одна фраза на любой неуспех: поле пароля очищается, фокус — на нём.
      password.value = ''
      tried.value = false
      await nextTick()
      passwordInput.value?.focus()
    }
  } finally {
    busy.value = null
  }
}

async function demo(p: Persona) {
  if (locked.value) return
  error.value = null
  busy.value = p.id
  try {
    await auth.demoLogin(p.id)
    await done()
  } catch (e) {
    showError(e, () => demo(p))
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <div class="login">
    <main id="main" class="login__layout">
      <section class="panel login__card" aria-labelledby="login-title">
        <BrandMark class="login__brand" />
        <h1 id="login-title" class="login__title">Вход</h1>
        <p class="t-sm t-muted">
          Витрина и «Доказательства» работают без входа. Покупатель видит свою выдачу, сотрудники —
          бэк-офис своей роли.
        </p>

        <p v-if="expired" class="login__note" role="status">
          <Info :size="16" aria-hidden="true" /> Сессия истекла — войдите снова.
        </p>
        <p v-else-if="fromBackoffice" class="login__note" role="status">
          <Info :size="16" aria-hidden="true" />
          {{
            auth.buyer
              ? 'Этот раздел — для сотрудников магазина: войдите под аккаунтом сотрудника.'
              : 'Раздел бэк-офиса — войдите под аккаунтом сотрудника.'
          }}
        </p>

        <form class="login__form" novalidate :aria-busy="busy === 'form'" @submit.prevent="submit">
          <div class="field">
            <label for="login" class="field__label">Логин</label>
            <div class="field__wrap">
              <UserRound class="field__icon" :size="18" aria-hidden="true" />
              <input
                id="login"
                v-model="login"
                class="field__input"
                name="username"
                autocomplete="username"
                autocapitalize="none"
                spellcheck="false"
                :aria-invalid="tried && !login.trim() ? true : undefined"
                :aria-describedby="tried && !login.trim() ? 'login-error' : undefined"
              />
            </div>
            <p v-if="tried && !login.trim()" id="login-error" class="field__error">Введите логин</p>
          </div>
          <div class="field">
            <label for="password" class="field__label">Пароль</label>
            <div class="field__wrap">
              <LockKeyhole class="field__icon" :size="18" aria-hidden="true" />
              <input
                id="password"
                ref="passwordInput"
                v-model="password"
                class="field__input"
                name="password"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="current-password"
                :aria-invalid="(tried && !password) || error ? true : undefined"
                :aria-describedby="
                  tried && !password ? 'password-error' : error ? 'form-error' : undefined
                "
              />
              <button
                type="button"
                class="field__eye"
                :aria-label="showPassword ? 'Скрыть пароль' : 'Показать пароль'"
                :aria-pressed="showPassword"
                @click="showPassword = !showPassword"
              >
                <component :is="showPassword ? EyeOff : Eye" :size="18" aria-hidden="true" />
              </button>
            </div>
            <p v-if="tried && !password" id="password-error" class="field__error">Введите пароль</p>
          </div>

          <div v-if="error" id="form-error" class="login__error" role="alert">
            <p>
              <CircleAlert :size="16" aria-hidden="true" />
              <span>
                {{ error.text }}<template v-if="waitSeconds > 0"> · {{ waitSeconds }} с</template>
              </span>
            </p>
            <button
              v-if="error.retry"
              type="button"
              class="btn btn--secondary btn--sm"
              :disabled="locked"
              @click="error.retry"
            >
              <RotateCcw :size="14" aria-hidden="true" /> Повторить
            </button>
          </div>

          <button type="submit" class="btn btn--primary login__submit" :disabled="locked">
            <LoaderCircle
              v-if="busy === 'form'"
              :size="16"
              class="login__spin"
              aria-hidden="true"
            />
            {{ busy === 'form' ? 'Входим…' : 'Войти' }}
            <ArrowRight v-if="busy !== 'form'" :size="16" aria-hidden="true" />
          </button>
          <p v-if="config && !config.password_login" class="t-xs t-muted login__hint">
            Аккаунты по паролю на этом стенде не заведены — войдите в демо-аккаунт.
          </p>
        </form>

        <RouterLink to="/search" class="link t-sm login__guest">
          Продолжить как гость — к поиску
        </RouterLink>
      </section>

      <section class="panel login__demo" aria-labelledby="demo-title">
        <h2 id="demo-title" class="t-h2">Демо-аккаунты для жюри</h2>
        <p class="t-sm t-muted">Вход в один клик, пароль не нужен.</p>
        <div v-if="auth.configStatus === 'loading' && !config" class="login__skel" aria-busy="true">
          <span v-for="i in 4" :key="i" class="skeleton" />
        </div>
        <div v-else-if="auth.configStatus === 'error'" class="login__error" role="alert">
          <p>
            <CircleAlert :size="16" aria-hidden="true" />
            <span>Список демо-аккаунтов не загрузился.</span>
          </p>
          <button type="button" class="btn btn--secondary btn--sm" @click="auth.loadConfig(true)">
            <RotateCcw :size="14" aria-hidden="true" /> Повторить
          </button>
        </div>
        <PersonaList
          v-else-if="demoOn"
          :personas="personas"
          :busy="busy"
          :locked="locked"
          heading-level="h3"
          @pick="demo"
        />
        <p v-else class="t-sm t-muted">Демо-аккаунты на этом стенде выключены.</p>
      </section>
    </main>
  </div>
</template>

<style scoped>
.login {
  display: flex;
  flex-direction: column;
  min-height: 100dvh;
  background: var(--bg);
}
.login__layout {
  box-sizing: border-box;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-4);
  width: 100%;
  max-width: calc(30rem + 2 * var(--gutter));
  margin: 0 auto;
  padding: var(--space-6) var(--gutter) 0;
  outline: none;
}
.login__card,
.login__demo {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-4);
}
.login__brand {
  margin-bottom: var(--space-2);
}
.login__title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.login__note {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.login__note svg {
  flex: none;
  margin-top: 2px;
}
.login__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-2);
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.field__label {
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.field__wrap {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding-left: var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text-3);
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.field__wrap:focus-within {
  border-color: var(--focus-ring);
  box-shadow: 0 0 0 1px var(--focus-ring);
  color: var(--text);
}
.field__wrap:has(.field__input[aria-invalid='true']) {
  border-color: var(--danger);
}
.field__icon {
  flex: none;
}
.field__input {
  flex: 1;
  align-self: stretch;
  min-width: 0;
  padding: 0 var(--space-3) 0 0;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
  outline: none;
}
.field__eye {
  display: grid;
  flex: none;
  place-items: center;
  width: var(--control-h);
  height: var(--control-h);
  border: 0;
  border-radius: var(--radius-control);
  background: none;
  color: var(--text-2);
  cursor: pointer;
}
.field__eye:hover {
  color: var(--text);
}
.field__eye:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: -2px;
}
.field__error {
  margin: 0;
  color: var(--danger);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.login__error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3);
  border-radius: var(--radius-control);
  background: var(--danger-bg);
}
.login__error p {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0;
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.login__error svg {
  flex: none;
  margin-top: 2px;
  color: var(--danger);
}
.login__submit {
  width: 100%;
  margin-top: var(--space-1);
}
.login__spin {
  animation: login-spin 0.9s linear infinite;
}
@keyframes login-spin {
  to {
    transform: rotate(360deg);
  }
}
.login__hint {
  margin: 0;
}
.login__guest {
  align-self: flex-start;
}
.login__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.login__skel .skeleton {
  height: 3.5rem;
}
@media (min-width: 1024px) {
  .login__layout {
    display: grid;
    flex: none;
    grid-template-columns: minmax(0, 26rem) minmax(0, 34rem);
    justify-content: center;
    align-items: start;
    gap: var(--space-6);
    max-width: none;
    padding-top: var(--space-12);
  }
  .login__card,
  .login__demo {
    padding: var(--space-6);
  }
}
</style>
