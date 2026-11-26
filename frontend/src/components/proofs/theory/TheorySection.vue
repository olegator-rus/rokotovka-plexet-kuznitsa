<script setup lang="ts">
// Раздел вкладки «Теоретические основы»: заголовок h3 с якорем, текст и ссылки «открыть в поиске с трассой».
// Запросы ссылок — из витрины и демо-набора, которые не зовут модель или уже лежат в кэше разборов.
import { ExternalLink } from 'lucide-vue-next'
import { searchWithTrace } from '@/lib/theoryParams'
import { anchorOf, sectionTitle } from './sections'

const props = defineProps<{ id: string; tries?: readonly string[] }>()
</script>

<template>
  <section
    :id="anchorOf(props.id)"
    class="theory-section"
    :aria-labelledby="`${anchorOf(props.id)}-h`"
  >
    <h3 :id="`${anchorOf(props.id)}-h`" class="theory-section__title" tabindex="-1">
      {{ sectionTitle(props.id) }}
    </h3>
    <slot />
    <p v-if="props.tries?.length" class="theory-section__try t-sm">
      <span class="t-muted">Посмотреть на живом запросе с трассой:</span>
      <template v-for="q in props.tries" :key="q">
        <a :href="searchWithTrace(q)" target="_blank" rel="noopener" class="theory-section__link">
          «{{ q }}»<ExternalLink :size="12" aria-hidden="true" /><span class="sr-only">
            (откроется в новой вкладке)</span
          >
        </a>
      </template>
    </p>
  </section>
</template>

<style scoped>
.theory-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  /* Под прилипшей шапкой: её высота измеряется (на телефоне шапка в две строки). */
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--space-4));
}
.theory-section__title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-bold);
}
.theory-section__try {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-1) var(--space-3);
  margin: 0;
}
.theory-section__link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 3px;
  overflow-wrap: anywhere;
}
.theory-section__link:hover {
  color: var(--accent-hover);
}
</style>
