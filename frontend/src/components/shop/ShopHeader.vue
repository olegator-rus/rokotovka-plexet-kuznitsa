<script setup lang="ts">
import { ShoppingBag } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { SHOP_NAME } from '@/lib/brand'
</script>

<template>
  <header class="header">
    <div class="container-page flex h-full items-center">
      <RouterLink to="/" class="brand" :aria-label="`${SHOP_NAME}, на главную`">
        <span class="brand__mark" aria-hidden="true"
          ><ShoppingBag :size="20" :stroke-width="2.25"
        /></span>
        <span class="brand__name">{{ SHOP_NAME }}</span>
      </RouterLink>
    </div>
  </header>
</template>

<style scoped>
.header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: var(--header-h);
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  backdrop-filter: saturate(1.4) blur(8px);
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 0.625rem;
  color: var(--text);
  text-decoration: none;
  border-radius: var(--radius-control);
}
.brand__mark {
  display: grid;
  place-items: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.625rem;
  background: var(--accent);
  color: var(--on-accent);
}
.brand__name {
  font-family: var(--font-display);
  font-weight: 700;
  font-size: 1.0625rem;
  letter-spacing: -0.01em;
}
</style>
