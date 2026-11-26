<script setup lang="ts">
// Логотип «Софтлайн Решения»: белый PNG как маска, цвет берётся из --logo-color (на красной шапке —
// белый, на обычном фоне — фирменный красный).
</script>

<template>
  <span class="brand" role="img" aria-label="Софтлайн Решения" />
</template>

<style scoped>
.brand {
  display: block;
  flex: none;
  width: 8.5rem;
  aspect-ratio: 600 / 133;
  background: var(--logo-color, var(--accent));
  mask: url('@/assets/softline-logo.png') center / contain no-repeat;
}
</style>
