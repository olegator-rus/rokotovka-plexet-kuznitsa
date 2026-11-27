<script setup lang="ts">
// Плитка позиции: картинка товара из каталога заказчика, если есть; нет или не загрузилась — значок производителя;
// нет и его — общая иконка из слота. Картинка декоративная: название и производитель всегда написаны рядом текстом.
import { computed, ref, watch } from 'vue'
import { Package } from 'lucide-vue-next'
import { vendorLogo } from '@/lib/vendorLogo'

const props = defineProps<{ vendor: string | null | undefined; image?: string | null }>()

const logo = computed(() => vendorLogo(props.vendor))
const imageFailed = ref(false)
const logoFailed = ref(false)
watch(
  () => props.image,
  () => (imageFailed.value = false),
)
watch(logo, () => (logoFailed.value = false))
const photo = computed(() => (props.image && !imageFailed.value ? props.image : null))
</script>

<template>
  <span class="vlogo" :class="{ 'vlogo--photo': photo }" aria-hidden="true">
    <img
      v-if="photo"
      :src="photo"
      alt=""
      loading="lazy"
      referrerpolicy="no-referrer"
      class="vlogo__img"
      @error="imageFailed = true"
    />
    <img
      v-else-if="logo && !logoFailed"
      :src="logo"
      alt=""
      class="vlogo__img"
      @error="logoFailed = true"
    />
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
.vlogo--photo .vlogo__img {
  width: 100%;
  height: 100%;
}
</style>
