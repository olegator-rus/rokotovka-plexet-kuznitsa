<script setup lang="ts">
// Вход в шапке: гость — «Войти»; вошедший — меню с именем, ролью и клиентом, «Сменить демо-аккаунт»
// (если вход демо и сервер включил DEMO_LOGIN_BUTTONS) и «Выйти». Смена аккаунта — диалог с теми же строками, что
// на экране входа; после смены — стартовый экран новой роли.
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from 'reka-ui'
import {
  ChevronDown,
  CircleAlert,
  FileText,
  LogIn,
  LogOut,
  RotateCcw,
  Users,
} from 'lucide-vue-next'
import type { Persona } from '@/api/auth'
import { type MyQuote, getMyQuotes } from '@/api/quotes'
import PersonaList from '@/components/auth/PersonaList.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { formatDateTime, formatRub, plural } from '@/lib/format'
import { LOGIN_PATH, homeFor, isBackofficePath } from '@/lib/auth'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const toast = useToast()

const viewer = computed(() => auth.viewer)
const initials = computed(() =>
  viewer.value.name
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w.charAt(0))
    .join('')
    .slice(0, 2)
    .toUpperCase(),
)
const loginLink = computed(() => ({
  path: LOGIN_PATH,
  query: route.path === LOGIN_PATH ? {} : { returnTo: route.fullPath },
}))
// Демо-сессия действует, только пока сервер включает демо-кнопки: вошёл демо — можно сменить.
const canSwitch = computed(() => auth.demo)

const switchOpen = ref(false)
const busy = ref<string | null>(null)
const switchErr = ref('')

function openSwitch() {
  switchErr.value = ''
  switchOpen.value = true
  void auth.loadConfig(true)
}

async function pick(p: Persona) {
  busy.value = p.id
  switchErr.value = ''
  try {
    await auth.demoSwitch(p.id)
    switchOpen.value = false
    // Новая роль — её стартовый экран (как после входа).
    await router.replace(homeFor(auth.role))
    toast.add({ tone: 'ok', title: `Вы вошли как ${p.name}`, detail: p.role_label })
  } catch (e) {
    switchErr.value = isApiError(e) ? e.message : MSG_UNKNOWN
  } finally {
    busy.value = null
  }
}

// «Мои запросы КП» — у покупателя: запросы его компании (клиент из сессии).
const quotesOpen = ref(false)
const quotes = ref<MyQuote[] | null>(null)
const quotesErr = ref('')
async function openQuotes() {
  quotesOpen.value = true
  quotes.value = null
  quotesErr.value = ''
  try {
    quotes.value = (await getMyQuotes()).items ?? []
  } catch (e) {
    quotesErr.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}
const quoteTotal = (q: MyQuote) =>
  q.total_rub !== null
    ? formatRub(q.total_rub)
    : q.on_request > 0
      ? `цена по запросу у ${q.on_request} ${plural(q.on_request, ['позиции', 'позиций', 'позиций'])}`
      : '—'

async function logout() {
  await auth.logout()
  if (isBackofficePath(route.path) && !auth.staff) await router.replace('/search')
}
</script>

<template>
  <RouterLink v-if="!auth.signedIn" :to="loginLink" class="btn btn--secondary account__login">
    <LogIn :size="16" aria-hidden="true" />
    Войти
  </RouterLink>

  <DropdownMenuRoot v-else>
    <DropdownMenuTrigger
      class="btn btn--ghost account__trigger"
      :aria-label="`Аккаунт: ${viewer.name}, ${viewer.role_label}`"
    >
      <span class="account__avatar" aria-hidden="true">{{ initials }}</span>
      <span class="account__who">
        <span class="account__name">{{ viewer.name }}</span>
        <span class="account__role t-xs">{{ viewer.role_label }}</span>
      </span>
      <ChevronDown :size="16" aria-hidden="true" />
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent class="account__menu" align="end" :side-offset="6">
        <DropdownMenuLabel class="account__label">
          <span class="account__name">{{ viewer.name }}</span>
          <span class="t-xs t-muted">
            {{ viewer.role_label
            }}<template v-if="viewer.client_name"> · {{ viewer.client_name }}</template>
          </span>
          <span v-if="auth.demo" class="t-xs t-muted">Демо-аккаунт, данные вымышлены</span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator class="account__sep" />
        <DropdownMenuItem v-if="auth.buyer" class="account__item" @select="openQuotes">
          <FileText :size="16" aria-hidden="true" /> Мои запросы КП
        </DropdownMenuItem>
        <DropdownMenuItem v-if="canSwitch" class="account__item" @select="openSwitch">
          <Users :size="16" aria-hidden="true" /> Сменить демо-аккаунт
        </DropdownMenuItem>
        <DropdownMenuItem class="account__item" @select="logout">
          <LogOut :size="16" aria-hidden="true" /> Выйти
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>

  <UiDialog v-model:open="quotesOpen" title="Мои запросы КП" wide>
    <p class="t-sm t-muted">
      {{ viewer.client_name ?? 'Ваша компания' }} · запросы коммерческого предложения из
      спецификации
    </p>
    <div v-if="quotesErr" class="account__err" role="alert">
      <CircleAlert :size="16" aria-hidden="true" /> {{ quotesErr }}
      <button type="button" class="btn btn--secondary btn--sm" @click="openQuotes">
        <RotateCcw :size="14" aria-hidden="true" /> Повторить
      </button>
    </div>
    <div v-else-if="quotes === null" class="account__skel" aria-busy="true">
      <span v-for="i in 3" :key="i" class="skeleton" />
    </div>
    <div v-else-if="!quotes.length" class="account__empty">
      <p>Запросов КП пока нет.</p>
      <RouterLink to="/search" class="btn btn--secondary btn--sm" @click="quotesOpen = false">
        Собрать спецификацию в поиске
      </RouterLink>
    </div>
    <ul v-else class="account__quotes">
      <li v-for="q in quotes" :key="q.id" class="account__quote">
        <span class="account__name">{{ formatDateTime(q.created_at) }}</span>
        <span class="t-sm">
          {{ q.positions }} {{ plural(q.positions, ['позиция', 'позиции', 'позиций']) }} ·
          {{ quoteTotal(q) }}
        </span>
        <span class="t-xs t-muted">{{
          q.items.map((i) => i.title || i.product_id).join(', ')
        }}</span>
      </li>
    </ul>
  </UiDialog>

  <UiDialog v-model:open="switchOpen" title="Сменить демо-аккаунт" wide>
    <p v-if="switchErr" class="account__err" role="alert">
      <CircleAlert :size="16" aria-hidden="true" /> {{ switchErr }}
    </p>
    <div v-if="auth.configStatus === 'loading' && !auth.config" class="account__skel">
      <span v-for="i in 4" :key="i" class="skeleton" />
    </div>
    <PersonaList
      v-else-if="auth.config?.personas.length"
      :personas="auth.config.personas"
      :busy="busy"
      :locked="busy !== null"
      :current="viewer.persona"
      @pick="pick"
    />
    <p v-else class="t-sm t-muted">Демо-аккаунты не загрузились.</p>
  </UiDialog>
</template>

<style scoped>
.account__trigger {
  gap: var(--space-2);
  padding: 0 var(--space-2);
  color: var(--text);
}
.account__avatar {
  display: grid;
  flex: none;
  place-items: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 50%;
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
}
.account__who {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.1;
  text-align: left;
}
.account__name {
  font-weight: var(--fw-medium);
}
.account__role {
  color: var(--text-2);
}
.account__err {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--danger-bg);
  color: var(--text);
}
.account__err svg {
  color: var(--danger);
}
.account__empty {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
}
.account__empty p {
  margin: 0;
}
.account__quotes {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.account__quote {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
}
.account__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.account__skel .skeleton {
  height: 3.5rem;
}
@media (max-width: 639px) {
  .account__who {
    display: none;
  }
}
</style>

<style>
.account__menu {
  z-index: 60;
  box-sizing: border-box;
  min-width: 15rem;
  padding: var(--space-1);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  box-shadow: var(--shadow-overlay);
}
.account__label {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
}
.account__sep {
  height: var(--border-w);
  margin: var(--space-1) 0;
  background: var(--border);
}
.account__item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border-radius: var(--radius-sm);
  color: var(--text);
  cursor: pointer;
  outline: none;
  user-select: none;
}
.account__item[data-highlighted] {
  background: var(--surface-3);
}
</style>
