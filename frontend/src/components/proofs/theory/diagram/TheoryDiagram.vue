<script setup lang="ts">
// Схема вкладки «Теоретические основы»: две раскладки одной картинки и подпись под ней.
// Широкая — когда колонка от 670 px (на экранах от 768 px), узкая — на телефоне. Переключает CSS по ширине
// контейнера, поэтому кегль текста в схеме не падает ниже 12 px: широкая ужимается не больше чем до 0,93.
import DgSvg from './DgSvg.vue'

const props = defineProps<{
  title: string
  desc: string
  wide: readonly [number, number]
  narrow: readonly [number, number]
}>()
</script>

<template>
  <figure class="dg">
    <DgSvg
      class="dg__svg dg__svg--wide"
      mode="wide"
      :size="props.wide"
      :title="props.title"
      :desc="props.desc"
    >
      <slot name="wide" />
    </DgSvg>
    <DgSvg
      class="dg__svg dg__svg--narrow"
      mode="narrow"
      :size="props.narrow"
      :title="props.title"
      :desc="props.desc"
    >
      <slot name="narrow" />
    </DgSvg>
    <figcaption class="dg__caption"><slot name="caption" /></figcaption>
  </figure>
</template>

<style scoped>
.dg {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  margin: var(--space-2) 0;
}
.dg__svg--wide {
  display: none;
}
.dg__svg--narrow {
  max-width: 27.5rem;
  margin: 0 auto;
}
.dg__caption {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
@container (min-width: 670px) {
  .dg__svg--wide {
    display: block;
  }
  .dg__svg--narrow {
    display: none;
  }
}
</style>
