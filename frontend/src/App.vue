<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import { TooltipProvider } from 'reka-ui'
import AppShell from '@/components/layout/AppShell.vue'
import UiToast from '@/components/ui/UiToast.vue'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const route = useRoute()
onMounted(() => void app.loadMeta())
// Счётчик ждущих предложений — API бэк-офиса (на стенде за входом): только на его страницах. На витрине
// (/search, /compare) закрытые пути не запрашиваются, иначе браузер покажет окно пароля до первого поиска.
watch(
  () => route.meta.backoffice === true,
  (backoffice) => {
    if (backoffice) void app.loadPending()
  },
  { immediate: true },
)
</script>

<template>
  <TooltipProvider :delay-duration="300">
    <AppShell>
      <RouterView />
    </AppShell>
    <UiToast />
  </TooltipProvider>
</template>
