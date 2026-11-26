<script setup lang="ts">
// Формула вкладки «Теоретические основы»: нативный MathML (без библиотек) и расшифровка символов под ней.
// Длинную формулу кладут несколькими <math> подряд: на узком экране они переносятся по знакам «+».
const props = defineProps<{
  /** Пометка вида числа: «настройка» — веса и пороги из кода. */
  kind?: string
  /** Формула словами — для экранного диктора. */
  label: string
}>()
</script>

<template>
  <figure class="tf" role="group" :aria-label="props.label">
    <span v-if="props.kind" class="badge tf__kind">{{ props.kind }}</span>
    <div class="tf__body"><slot /></div>
    <div v-if="$slots.where" class="tf__where">
      <span class="tf__where-lead">где</span>
      <dl class="tf__defs"><slot name="where" /></dl>
    </div>
  </figure>
</template>

<style scoped>
.tf {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-left: 3px solid var(--accent);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.tf__kind {
  align-self: flex-start;
  background: var(--surface);
}
.tf__body {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) 0.25em;
  min-width: 0;
  padding: var(--space-1) 0;
  color: var(--text);
  font-size: 1.125rem;
}
.tf__body :deep(math) {
  font-family: 'STIX Two Math', 'Cambria Math', 'Latin Modern Math', math;
}
/* На телефоне формула на ступень мельче, чтобы дробь с индексами помещалась в строку. */
@container (max-width: 420px) {
  .tf__body {
    font-size: 1rem;
  }
}
/* Перенос строки между двумя формулами одного блока. */
.tf__body :deep(.tf__br) {
  flex-basis: 100%;
  height: 0;
}
.tf__where {
  display: flex;
  gap: var(--space-2);
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-body);
}
.tf__where-lead {
  flex: none;
}
.tf__defs {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  margin: 0;
}
.tf__defs :deep(dt),
.tf__defs :deep(dd) {
  display: inline;
}
.tf__defs :deep(dt) {
  margin-right: 0.3em;
  color: var(--text);
}
.tf__defs :deep(math) {
  font-family: 'STIX Two Math', 'Cambria Math', 'Latin Modern Math', math;
  font-size: 1.1em;
}
.tf__defs :deep(dd) {
  margin: 0;
}
</style>
