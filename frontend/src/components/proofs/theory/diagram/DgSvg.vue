<script setup lang="ts">
// Один <svg> схемы: viewBox, заголовок и пересказ для экранного диктора, шаг строк для узлов внутри.
import { provide, useId } from 'vue'
import { DG_KEY, DG_METRICS, type DgMetrics } from './geometry'

const props = defineProps<{
  mode: DgMetrics['mode']
  size: readonly [number, number]
  title: string
  desc: string
}>()

provide(DG_KEY, DG_METRICS[props.mode])
const id = useId()
</script>

<template>
  <svg
    :class="['dg-svg', `dg-svg--${props.mode}`]"
    :viewBox="`0 0 ${props.size[0]} ${props.size[1]}`"
    :width="props.size[0]"
    :height="props.size[1]"
    role="img"
    :aria-labelledby="`${id}-t ${id}-d`"
  >
    <title :id="`${id}-t`">{{ props.title }}</title>
    <desc :id="`${id}-d`">{{ props.desc }}</desc>
    <slot />
  </svg>
</template>

<style scoped>
.dg-svg {
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
  font-family: var(--font-sans);
  --dg-fs: 13px;
  --dg-fs-sub: 13px;
}
.dg-svg--narrow {
  --dg-fs-sub: 12px;
}
</style>
