<script setup lang="ts">
// Маршрут запроса — одно из трёх мест, где цвет несёт смысл; рядом всегда буква и подпись.
import { computed } from 'vue'
import { ROUTE_LABELS, isRoute } from '@/lib/trace'

const props = defineProps<{ route: string | null | undefined }>()
const known = computed(() => (isRoute(props.route) ? props.route : null))
</script>

<template>
  <span
    v-if="known"
    :class="['badge', 'badge--tone', `route-${known.toLowerCase()}`]"
    :aria-label="`Маршрут ${known}, ${ROUTE_LABELS[known]}`"
  >
    <strong>{{ known }}</strong>
    <span>{{ ROUTE_LABELS[known] }}</span>
  </span>
  <span v-else class="badge">маршрут не указан</span>
</template>
