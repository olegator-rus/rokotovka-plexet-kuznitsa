<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { TooltipProvider } from 'reka-ui'
import AppShell from '@/components/layout/AppShell.vue'
import UiToast from '@/components/ui/UiToast.vue'
import { LOGIN_PATH } from '@/lib/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const app = useAppStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
onMounted(() => void app.loadMeta())
// Счётчик ждущих предложений — API бэк-офиса: только сотруднику и только на его страницах. На витрине
// закрытые пути не запрашиваются.
watch(
  () => [route.meta.backoffice === true && auth.staff, auth.generation] as const,
  ([backoffice]) => {
    if (backoffice) void app.loadPending()
  },
  { immediate: true },
)
// Смена входа (вход, смена демо-аккаунта, выход): выбор клиента и «Смотреть как» прежнего не переносятся.
watch(
  () => auth.generation,
  () => app.resetForViewer(),
)
// Сессия кончилась посреди работы в бэк-офисе — на вход с возвратом сюда же.
watch(
  () => auth.expired,
  (expired) => {
    if (expired && route.meta.backoffice)
      void router.replace({ path: LOGIN_PATH, query: { returnTo: route.fullPath, expired: '1' } })
  },
)
</script>

<template>
  <TooltipProvider :delay-duration="300">
    <!-- До первого перехода охрана ждёт ответа о входе. Оболочку не рисуем (без мигания на /login), а при медленном
         API через 0,3 с показываем индикатор загрузки вместо белого экрана. -->
    <div v-if="!route.matched.length" class="app-boot" role="status" aria-live="polite">
      <span class="app-boot__dot" aria-hidden="true" />
      <span>Загрузка…</span>
    </div>
    <RouterView v-else-if="route.meta.bare" />
    <AppShell v-else>
      <RouterView />
    </AppShell>
    <UiToast />
  </TooltipProvider>
</template>

<style scoped>
.app-boot {
  display: flex;
  min-height: 100vh;
  min-height: 100dvh;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  color: var(--text-2);
  font-size: var(--fs-sm);
  opacity: 0;
  animation: app-boot-in 0.2s ease-out 0.3s forwards;
}
.app-boot__dot {
  width: 0.75rem;
  height: 0.75rem;
  border: 2px solid var(--border);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: app-boot-spin 0.8s linear infinite;
}
@keyframes app-boot-in {
  to {
    opacity: 1;
  }
}
@keyframes app-boot-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .app-boot {
    animation-duration: 0s;
  }
  .app-boot__dot {
    animation: none;
  }
}
</style>
