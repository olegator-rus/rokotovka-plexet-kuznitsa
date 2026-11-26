<script setup lang="ts">
// Список разделов: ссылки роутера, активный — светлая подложка и полоса слева (не цвет).
import { RouterLink } from 'vue-router'
import { SECTIONS } from './sections'

defineEmits<{ navigate: [] }>()
</script>

<template>
  <nav aria-label="Разделы">
    <ul class="app-nav">
      <li v-for="s in SECTIONS" :key="s.path">
        <RouterLink :to="s.path" class="app-nav__link" @click="$emit('navigate')">
          <component :is="s.icon" :size="18" aria-hidden="true" class="app-nav__icon" />
          <span>{{ s.label }}</span>
        </RouterLink>
      </li>
    </ul>
  </nav>
</template>

<style scoped>
.app-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.app-nav__link {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border-radius: var(--radius-control);
  color: var(--text-2);
  text-decoration: none;
  font-weight: var(--fw-medium);
}
.app-nav__link:hover {
  background: var(--surface-2);
  color: var(--text);
}
.app-nav__link.router-link-active {
  background: var(--surface-3);
  color: var(--text);
}
.app-nav__link.router-link-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: var(--space-2);
  bottom: var(--space-2);
  width: 2px;
  border-radius: var(--radius-pill);
  background: var(--text);
}
.app-nav__icon {
  flex: none;
}
</style>
