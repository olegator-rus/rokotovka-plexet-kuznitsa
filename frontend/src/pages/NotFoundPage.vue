<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <section class="page flex flex-col items-start gap-3">
    <h1 class="t-h1">Такой страницы нет</h1>
    <p class="t-muted">Возможно, ссылка устарела или в адресе опечатка.</p>
    <RouterLink to="/search" class="btn btn--secondary">К поиску</RouterLink>
  </section>
</template>
