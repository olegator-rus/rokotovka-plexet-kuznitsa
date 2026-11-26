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
    <!-- До первого перехода (охрана ждёт ответа о входе) ничего не рисуем: без мигания оболочки на /login. -->
    <template v-if="!route.matched.length" />
    <RouterView v-else-if="route.meta.bare" />
    <AppShell v-else>
      <RouterView />
    </AppShell>
    <UiToast />
  </TooltipProvider>
</template>
