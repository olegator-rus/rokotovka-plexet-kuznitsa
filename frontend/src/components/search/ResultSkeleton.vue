<script setup lang="ts">
// Скелет выдачи: пять карточек-строк. Через паузу — подпись «Разбираем запрос» (долгий разбор, маршрут D).
import { onBeforeUnmount, onMounted, ref } from 'vue'

const SLOW_MS = 700
const slow = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
  timer = setTimeout(() => (slow.value = true), SLOW_MS)
})
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="result-skeleton" aria-busy="true">
    <p class="t-sm t-muted result-skeleton__caption" role="status">
      {{ slow ? 'Разбираем запрос…' : 'Ищем…' }}
    </p>
    <div v-for="i in 5" :key="i" class="panel result-skeleton__row" aria-hidden="true">
      <div class="result-skeleton__main">
        <span class="skeleton" :style="{ width: `${60 - i * 4}%` }" />
        <span class="skeleton" style="width: 30%" />
      </div>
      <span class="skeleton result-skeleton__price" />
    </div>
  </div>
</template>

<style scoped>
.result-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.result-skeleton__caption {
  min-height: var(--lh-sm);
}
.result-skeleton__row {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-4);
}
.result-skeleton__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
}
.result-skeleton__price {
  width: 5rem;
}
</style>
