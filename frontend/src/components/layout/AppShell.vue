<script setup lang="ts">
// Оболочка приложения: красная шапка с логотипом и разделами (на узком экране разделы — в выезжающей
// панели, reka Dialog) и содержимое раздела. Ссылка «К содержимому» — первый Tab на странице.
import { ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  DialogClose,
  DialogContent,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { X } from 'lucide-vue-next'
import AppHeader from './AppHeader.vue'
import AppNav from './AppNav.vue'
import BrandMark from './BrandMark.vue'

const drawer = ref(false)
const route = useRoute()

watch(
  () => route.fullPath,
  () => {
    drawer.value = false
  },
)
</script>

<template>
  <a class="skip-link" href="#main">К содержимому</a>
  <div class="shell">
    <AppHeader @open-menu="drawer = true" />
    <main id="main" class="shell__content" tabindex="-1">
      <slot />
    </main>
    <footer class="shell__footer t-xs">
      <span>
        Концепт для хакатона, не сайт компании. Данные вымышлены. На сайте применяются
        рекомендательные технологии.
        <RouterLink class="link" to="/rules">Правила рекомендаций</RouterLink>
      </span>
      <span class="shell__footer-end">
        <!-- Инструменты страницы (на «Поиске» — переключатель «Трасса»). -->
        <span id="footer-tools" />
      </span>
    </footer>
  </div>

  <DialogRoot v-model:open="drawer">
    <DialogPortal>
      <DialogOverlay class="ui-scrim" />
      <DialogContent class="drawer" :aria-describedby="undefined">
        <div class="drawer__head">
          <DialogTitle class="drawer__title"><BrandMark /></DialogTitle>
          <DialogClose class="btn btn--ghost btn--icon" aria-label="Закрыть меню">
            <X :size="18" aria-hidden="true" />
          </DialogClose>
        </div>
        <AppNav @navigate="drawer = false" />
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.skip-link {
  position: absolute;
  left: var(--space-2);
  top: -100px;
  z-index: 100;
  padding: var(--space-2) var(--space-3);
  background: var(--accent);
  color: var(--on-accent);
  border-radius: var(--radius-control);
  text-decoration: none;
  font-weight: var(--fw-medium);
}
.skip-link:focus {
  top: var(--space-2);
}
.shell {
  display: flex;
  flex-direction: column;
  min-height: 100dvh;
}
.shell__content {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  outline: none;
}
.shell__footer-end {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-6);
}
.shell__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
  padding: var(--space-3) var(--gutter);
  border-top: var(--border-w) solid var(--border);
  color: var(--text-3);
}
</style>

<style>
.drawer {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  z-index: 51;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  width: min(var(--drawer-w), 85vw);
  padding: var(--space-3) var(--space-2);
  background: var(--surface);
  border-right: var(--border-w) solid var(--border);
  box-shadow: var(--shadow-overlay);
  animation: ui-slide-right var(--dur-panel) var(--ease-standard);
}
.drawer:focus-visible {
  outline: none;
}
.drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-left: var(--space-3);
}
.drawer__title {
  margin: 0;
}
</style>
