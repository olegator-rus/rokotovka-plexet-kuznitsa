<script setup lang="ts">
// Точка схода или развилки линий.
const props = defineProps<{ x: number; y: number; tone?: 'd' }>()
</script>

<template>
  <circle
    :class="['dg-dot', props.tone ? `dg-dot--${props.tone}` : '']"
    :cx="props.x"
    :cy="props.y"
    r="3"
  />
</template>

<style scoped>
.dg-dot {
  fill: var(--text-3);
}
.dg-dot--d {
  fill: var(--route-d);
}
</style>
