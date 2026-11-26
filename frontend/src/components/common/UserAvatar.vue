<script setup lang="ts">
// Аватар демо-аккаунта: фото персоны, а без него (или если файл не загрузился) — инициалы. Декоративный:
// имя всегда написано рядом или есть в подписи кнопки.
import { computed, ref, watch } from 'vue'
import { personaPhoto } from '@/lib/avatar'

const props = defineProps<{ persona: string | null | undefined; initials: string }>()

const src = computed(() => personaPhoto(props.persona))
const failed = ref(false)
watch(src, () => (failed.value = false))
</script>

<template>
  <span class="uava" aria-hidden="true">
    <img v-if="src && !failed" :src="src" alt="" class="uava__img" @error="failed = true" />
    <template v-else>{{ initials }}</template>
  </span>
</template>

<style scoped>
.uava {
  display: grid;
  flex: none;
  place-items: center;
  overflow: hidden;
  border-radius: 50%;
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
  letter-spacing: 0.02em;
}
.uava__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
