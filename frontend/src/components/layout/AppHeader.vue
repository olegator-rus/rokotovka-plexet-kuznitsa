<script setup lang="ts">
// Шапка: на мобильном — кнопка меню и знак; справа — плашка режима LLM (сотруднику), «Глазами клиента» (только сотруднику на
// витрине: роль приходит из входа) и вход: гость — «Войти», вошедший — меню аккаунта. На узком экране
// инструменты уходят второй строкой.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Menu } from 'lucide-vue-next'
import AccountMenu from './AccountMenu.vue'
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
const title = computed(() => (typeof route.meta.section === 'string' ? route.meta.section : ''))
</script>

<template>
  <header class="app-header">
    <div class="app-header__lead">
      <button
        type="button"
        class="btn btn--ghost btn--icon app-header__menu"
        aria-label="Открыть меню разделов"
        @click="$emit('openMenu')"
      >
        <Menu :size="20" aria-hidden="true" />
      </button>
      <BrandMark compact class="app-header__brand" />
      <span v-if="title" class="app-header__title">{{ title }}</span>
    </div>
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
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
  min-height: var(--header-h);
  padding: var(--space-2) var(--gutter);
  box-sizing: border-box;
  background: var(--bg);
  border-bottom: var(--border-w) solid var(--border);
}
.app-header__lead {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}
.app-header__title {
  color: var(--text-2);
  font-weight: var(--fw-medium);
  white-space: nowrap;
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
@media (min-width: 1024px) {
  .app-header__menu,
  .app-header__brand {
    display: none;
  }
}
@media (max-width: 1023px) {
  .app-header__title {
    display: none;
  }
}
@media (max-width: 639px) {
  .app-header__tools {
    width: 100%;
    justify-content: space-between;
  }
  .app-header__tools :deep(.ui-select__prefix) {
    display: none;
  }
}
</style>
