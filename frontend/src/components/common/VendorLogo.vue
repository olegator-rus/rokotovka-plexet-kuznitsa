<script setup lang="ts">
// Значок производителя на белой плитке. Нет значка или файл не загрузился — общая иконка из слота.
// Картинка декоративная: имя производителя всегда написано рядом текстом.
import { computed, ref, watch } from 'vue'
import { Package } from 'lucide-vue-next'
import { vendorLogo } from '@/lib/vendorLogo'

const props = defineProps<{ vendor: string | null | undefined }>()

const src = computed(() => vendorLogo(props.vendor))
const failed = ref(false)
watch(src, () => (failed.value = false))
</script>

<template>
  <span class="vlogo" aria-hidden="true">
    <img v-if="src && !failed" :src="src" alt="" class="vlogo__img" @error="failed = true" />
    <slot v-else><Package :size="18" /></slot>
  </span>
</template>

<style scoped>
.vlogo {
  box-sizing: border-box;
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: var(--logo-size);
  height: var(--logo-size);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text-2);
  overflow: hidden;
}
.vlogo__img {
  width: 70%;
  height: 70%;
  object-fit: contain;
}
</style>
