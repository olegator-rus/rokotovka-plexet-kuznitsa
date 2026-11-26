<script setup lang="ts">
// Красная шапка: логотип и разделы (на узком экране — кнопка меню и логотип); справа — плашка режима LLM (сотруднику), «Глазами клиента» (только сотруднику на
// витрине: роль приходит из входа) и вход: гость — «Войти», вошедший — меню аккаунта. На узком экране
// инструменты уходят второй строкой.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { Menu } from 'lucide-vue-next'
import AccountMenu from './AccountMenu.vue'
import AppNav from './AppNav.vue'
import BrandMark from './BrandMark.vue'
import ClientSwitch from './ClientSwitch.vue'
import LlmModeBadge from './LlmModeBadge.vue'
import SpecButton from '@/components/spec/SpecButton.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

defineEmits<{ openMenu: [] }>()

const route = useRoute()
const app = useAppStore()
// Контур доводки: плашка режима LLM — только сотруднику; гость и покупатель её не видят.
const auth = useAuthStore()
const backoffice = computed(() => route.meta.backoffice === true)
// Контур коммерции: «Спецификация» — в шапке раздела «Поиск».
const storefront = computed(() => route.path === '/search')

// Настоящая высота шапки: на узком экране и у сотрудника она в два ряда. По ней прилипают строка
// сортировки, фильтры и трасса.
const header = ref<HTMLElement | null>(null)
let observer: ResizeObserver | null = null
onMounted(() => {
  const el = header.value
  if (!el) return
  observer = new ResizeObserver(() =>
    document.documentElement.style.setProperty('--header-actual', `${el.offsetHeight}px`),
  )
  observer.observe(el)
})
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <header ref="header" class="app-header on-hero">
    <div class="app-header__lead">
      <button
        type="button"
        class="btn btn--ghost btn--icon app-header__menu"
        aria-label="Открыть меню разделов"
        @click="$emit('openMenu')"
      >
        <Menu :size="20" aria-hidden="true" />
      </button>
      <RouterLink to="/search" class="app-header__brand" aria-label="Софтлайн Решения — к поиску">
        <BrandMark />
      </RouterLink>
    </div>
    <AppNav horizontal class="app-header__nav" />
    <div class="app-header__tools">
      <SpecButton v-if="storefront" />
      <LlmModeBadge v-if="auth.staff" />
      <ClientSwitch v-if="app.canPickClient && !backoffice" />
      <AccountMenu />
    </div>
  </header>
</template>

<style scoped>
.app-header {
  --logo-color: var(--on-hero);
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
  min-height: var(--header-h);
  padding: var(--space-2) max(var(--gutter), (100% - var(--content-max)) / 2 + var(--gutter));
  box-sizing: border-box;
  border-bottom: var(--border-w) solid var(--hero-line);
}
.app-header__lead {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}
.app-header__brand {
  display: block;
  border-radius: var(--radius-sm);
}
.app-header__tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  min-width: 0;
  margin-left: auto;
}
/* Всё в правой части шапки — одной высоты и одним кеглем: кнопки, плашка режима, выбор клиента. */
.app-header__tools :deep(.btn),
.app-header__tools :deep(.badge),
.app-header__tools :deep(.ui-select) {
  box-sizing: border-box;
  height: var(--control-h);
  min-height: var(--control-h);
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  border-radius: var(--radius-pill);
}
.app-header__tools :deep(.btn) {
  padding-inline: var(--space-3);
}
.app-header__tools :deep(.ui-select__prefix) {
  display: none;
}
.app-header__tools :deep(.btn--icon) {
  width: var(--control-h);
  padding-inline: 0;
}
@media (min-width: 1024px) {
  .app-header__menu {
    display: none;
  }
}
/* Воздух между логотипом и переключателем разделов. */
.app-header__nav {
  margin-left: var(--space-4);
}
@media (max-width: 1023px) {
  .app-header__nav {
    display: none;
  }
}
@media (max-width: 639px) {
  .app-header__tools {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
