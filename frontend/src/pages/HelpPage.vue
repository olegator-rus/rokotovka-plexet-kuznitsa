<script setup lang="ts">
// Справка магазина (контур доводки): статьи, на которые ведут ответы справки в
// выдаче; якорь — id статьи (/help#renewal). Данные — GET /api/help.
import { nextTick, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getHelp } from '@/api/help'
import SearchError from '@/components/search/SearchError.vue'
import { useResource } from '@/composables/useResource'

const route = useRoute()
const help = useResource((opts) => getHelp(opts))

onMounted(async () => {
  await help.load()
  await nextTick()
  const id = route.hash.replace('#', '')
  if (id) document.getElementById(`help-${id}`)?.scrollIntoView({ block: 'start' })
})
</script>

<template>
  <article class="page help-page">
    <h1 class="t-h1">Помощь покупателю</h1>
    <p class="t-muted">
      Счета, документы, доставка, возврат, продление и активация лицензий. Поиск отвечает отсюда на
      вопросы о покупке.
    </p>
    <p v-if="help.status.value === 'loading'" class="t-muted" role="status">Загружаем справку…</p>
    <SearchError
      v-else-if="help.status.value === 'error' && help.error.value"
      :error="help.error.value"
      @retry="help.load()"
    />
    <p v-else-if="help.data.value && !help.data.value.items.length" class="t-muted">
      Статей справки пока нет — спросите менеджера.
    </p>
    <section
      v-for="a in help.data.value?.items ?? []"
      :id="`help-${a.id}`"
      :key="a.id"
      class="panel help-page__item"
      :aria-labelledby="`help-${a.id}-title`"
    >
      <h2 :id="`help-${a.id}-title`" class="t-h2">{{ a.title }}</h2>
      <p>{{ a.text }}</p>
    </section>
  </article>
</template>

<style scoped>
.help-page {
  display: grid;
  gap: var(--space-4);
  max-width: 72ch;
}
.help-page__item {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-4) var(--space-5);
  scroll-margin-top: var(--space-8);
}
</style>
