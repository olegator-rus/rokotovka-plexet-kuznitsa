<script setup lang="ts">
import { ref } from 'vue'
import { Search, X } from 'lucide-vue-next'

// Поле поиска витрины. v-model — строка запроса; Enter и крестик ничего не отправляют на сервер,
// пока поиск локальный (lib/catalog.ts).
const query = defineModel<string>({ required: true })
const input = ref<{ $el: HTMLInputElement } | null>(null)

function clear(): void {
  query.value = ''
  input.value?.$el.focus()
}
</script>

<template>
  <form class="search" role="search" @submit.prevent>
    <IconField class="w-full">
      <InputIcon class="search__icon"><Search :size="20" aria-hidden="true" /></InputIcon>
      <InputText
        ref="input"
        v-model="query"
        type="search"
        size="large"
        enterkeyhint="search"
        autocomplete="off"
        placeholder="Найти товар"
        aria-label="Поиск товаров"
        class="search__input w-full"
      />
    </IconField>
    <button
      v-if="query"
      type="button"
      class="search__clear"
      aria-label="Очистить поиск"
      @click="clear"
    >
      <X :size="18" aria-hidden="true" />
    </button>
  </form>
</template>

<style scoped>
.search {
  position: relative;
}
.search__icon {
  display: inline-flex;
  color: var(--text-2);
}
/* Справа место под крестик; штатный крестик type=search скрыт — свой понятнее и одинаков везде. */
.search__input {
  padding-right: 3rem;
  box-shadow: var(--shadow-card);
}
.search__input::-webkit-search-cancel-button {
  display: none;
}
.search__clear {
  position: absolute;
  top: 50%;
  right: 0.5rem;
  display: grid;
  place-items: center;
  width: 2.25rem;
  height: 2.25rem;
  transform: translateY(-50%);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
}
.search__clear:hover {
  background: var(--surface-2);
  color: var(--text);
}
</style>
