<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import { TooltipProvider } from 'reka-ui'
import AppShell from '@/components/layout/AppShell.vue'
import UiToast from '@/components/ui/UiToast.vue'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
onMounted(() => {
  void app.loadMeta()
})
</script>

<template>
  <TooltipProvider :delay-duration="300">
    <AppShell>
      <RouterView />
    </AppShell>
    <UiToast />
  </TooltipProvider>
</template>
