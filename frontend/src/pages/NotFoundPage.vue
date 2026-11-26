<script setup lang="ts">
import { RouterLink } from 'vue-router'
import ShopHeader from '@/components/shop/ShopHeader.vue'
</script>

<template>
  <div class="flex min-h-dvh flex-col">
    <ShopHeader />
    <main class="container-page flex flex-1 flex-col items-start justify-center gap-3 py-16">
      <h1 class="t-hero m-0">Такой страницы нет</h1>
      <p class="m-0 text-text-2">Возможно, ссылка устарела или в адресе опечатка.</p>
      <RouterLink to="/" class="mt-2 font-semibold text-accent underline underline-offset-2">
        На витрину
      </RouterLink>
    </main>
  </div>
</template>
