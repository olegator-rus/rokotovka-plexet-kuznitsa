<script setup lang="ts">
// Честное пустое состояние раздела, который ещё не подключён к данным: что здесь будет и откуда
// возьмутся числа. Никаких примерных цифр — непосчитанное не показывается (правило 4).
import type { Component } from 'vue'
import { RouterLink } from 'vue-router'

defineProps<{
  title: string
  icon: Component
  lead: string
  /** Что появится в разделе, по пунктам. */
  items: string[]
  /** Откуда данные: эндпоинты API, которые раздел будет читать. */
  source: string
}>()
</script>

<template>
  <section class="page pending" :aria-labelledby="'pending-title'">
    <h1 id="pending-title" class="t-h1">{{ title }}</h1>
    <div class="panel pending__card">
      <component :is="icon" :size="20" aria-hidden="true" class="pending__icon" />
      <div class="pending__body">
        <p class="pending__lead">{{ lead }}</p>
        <ul class="pending__list">
          <li v-for="it in items" :key="it">{{ it }}</li>
        </ul>
        <p class="t-sm t-muted">
          Данные: <span class="t-mono">{{ source }}</span>
        </p>
        <p class="t-sm t-muted">
          Пока раздел пуст: чисел, которые сервер ещё не считает, здесь нет. Поиск и трасса уже
          работают —
          <RouterLink to="/search" class="link">открыть «Поиск»</RouterLink>.
        </p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.pending {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.pending__card {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.pending__icon {
  flex: none;
  margin-top: 2px;
  color: var(--text-2);
}
.pending__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.pending__lead {
  font-weight: var(--fw-medium);
}
.pending__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding-left: var(--space-5);
  color: var(--text-2);
}
</style>
