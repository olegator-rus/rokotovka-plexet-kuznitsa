<script setup lang="ts">
// Список разделов: ссылки роутера, активный — светлая подложка и полоса слева (не цвет).
import { RouterLink, useRoute } from 'vue-router'
import { storeToRefs } from 'pinia'
import { plural } from '@/lib/format'
import { useAppStore } from '@/stores/app'
import { SECTIONS } from './sections'

const { pendingProposals } = storeToRefs(useAppStore())
const route = useRoute()
/** Вложенные страницы раздела (/searchops/runs/:id) подсвечивают свой раздел. */
const inSection = (path: string) => route.path === path || route.path.startsWith(`${path}/`)

defineEmits<{ navigate: [] }>()
</script>

<template>
  <nav aria-label="Разделы">
    <ul class="app-nav">
      <li v-for="s in SECTIONS" :key="s.path">
        <RouterLink
          :to="s.path"
          :class="['app-nav__link', { 'app-nav__link--active': inSection(s.path) }]"
          :aria-current="inSection(s.path) ? 'page' : undefined"
          @click="$emit('navigate')"
        >
          <component :is="s.icon" :size="18" aria-hidden="true" class="app-nav__icon" />
          <span>{{ s.label }}</span>
          <span
            v-if="s.name === 'searchops' && pendingProposals"
            class="app-nav__count t-num"
            :aria-label="`${pendingProposals} ${plural(pendingProposals, ['предложение ждёт', 'предложения ждут', 'предложений ждут'])} решения`"
            >{{ pendingProposals }}</span
          >
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
.app-nav__link--active {
  background: var(--surface-3);
  color: var(--text);
}
.app-nav__link--active::before {
  content: '';
  position: absolute;
  left: 0;
  top: var(--space-2);
  bottom: var(--space-2);
  width: 2px;
  border-radius: var(--radius-pill);
  background: var(--text);
}
.app-nav__count {
  margin-left: auto;
  min-width: 1.25rem;
  padding: 0 var(--space-1);
  border-radius: var(--radius-pill);
  background: var(--status-pending-bg);
  color: var(--status-pending);
  font-size: var(--fs-xs);
  line-height: 1.25rem;
  text-align: center;
}
.app-nav__icon {
  flex: none;
}
</style>
