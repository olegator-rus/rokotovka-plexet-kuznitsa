<script setup lang="ts">
// Стартовый экран поиска (до первого запроса): баннер «Что ищем?» и способы спросить — карточка на каждый,
// нажатие запускает поиск по примеру. Примеры настоящие: каждый даёт выдачу на каталоге.
import type { Component } from 'vue'
import {
  ArrowRight,
  Keyboard,
  ListChecks,
  MessageCircle,
  RefreshCw,
  Replace,
} from 'lucide-vue-next'

defineEmits<{ search: [query: string] }>()

interface Way {
  query: string
  title: string
  hint: string
  icon: Component
  wide?: boolean
}

const WAYS: readonly Way[] = [
  {
    query: 'аналог фотошопа на 20 пользователей',
    title: 'Подобрать замену',
    hint: 'Назовите программу и сколько мест нужно',
    icon: Replace,
  },
  {
    query: 'касперкий',
    title: 'Писать как получается',
    hint: 'Опечатки и раскладку поиск исправит сам',
    icon: Keyboard,
  },
  {
    query: 'винда 11 про',
    title: 'Говорить по-простому',
    hint: 'Сленг и сокращения тоже понятны',
    icon: MessageCircle,
  },
  {
    query: 'мойофис на 30 пользователей на 3 года',
    title: 'Описать задачу целиком',
    hint: 'Продукт, число пользователей и срок лицензии в одной строке',
    icon: ListChecks,
    wide: true,
  },
  {
    query: 'продлить антивирус',
    title: 'Продлить лицензию',
    hint: 'Покажем варианты продления',
    icon: RefreshCw,
  },
]
</script>

<template>
  <section class="start" aria-labelledby="idle-title">
    <div class="start__banner on-hero">
      <h2 id="idle-title" class="start__title">Что ищем?</h2>
      <p class="start__text">
        Название продукта, задача своими словами или артикул. Как поиск понял запрос — видно в
        трассе.
      </p>
    </div>
    <ul class="start__ways" aria-label="Примеры запросов">
      <li v-for="w in WAYS" :key="w.query" :class="{ 'start__item--wide': w.wide }">
        <button
          type="button"
          class="start__way"
          :aria-label="`${w.title}: ${w.query}`"
          @click="$emit('search', w.query)"
        >
          <span class="start__icon" aria-hidden="true"><component :is="w.icon" :size="20" /></span>
          <span class="start__name">{{ w.title }}</span>
          <span class="t-sm t-muted">{{ w.hint }}</span>
          <span class="start__query">
            <span>{{ w.query }}</span>
            <ArrowRight :size="16" aria-hidden="true" />
          </span>
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.start {
  container-type: inline-size;
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-panel);
  background: var(--surface-2);
}
.start__banner {
  position: relative;
  overflow: hidden;
  padding: var(--space-8) var(--space-6);
}
/* Фирменные квадраты из логотипа — тихий фон справа, не несёт смысла. */
.start__banner::after {
  content: '';
  position: absolute;
  right: calc(-1 * var(--space-6));
  top: calc(-1 * var(--space-8));
  width: 14rem;
  height: 14rem;
  background:
    linear-gradient(var(--hero-hover), var(--hero-hover)) 0 0 / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 100% 0 / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 0 100% / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 100% 100% / 48% 48% no-repeat;
  transform: rotate(12deg);
  pointer-events: none;
}
.start__title,
.start__text {
  position: relative;
  z-index: 1;
  margin: 0;
}
.start__title {
  margin-bottom: var(--space-2);
  font-size: var(--fs-banner);
  line-height: 1.2;
  font-weight: var(--fw-bold);
  letter-spacing: -0.02em;
}
.start__text {
  max-width: 52ch;
  color: var(--on-hero-2);
}
.start__ways {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-3);
  margin: 0;
  padding: var(--space-4);
  list-style: none;
}
@container (min-width: 36rem) {
  .start__ways {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@container (min-width: 56rem) {
  .start__ways {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
@container (min-width: 36rem) {
  .start__item--wide {
    grid-column: span 2;
  }
}
.start__way {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  padding: var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    border-color var(--dur-fast) var(--ease-standard),
    box-shadow var(--dur-fast) var(--ease-standard);
}
.start__way:hover {
  border-color: var(--accent);
  box-shadow: 0 6px 18px var(--hairline-shadow);
}
.start__icon {
  display: grid;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.start__name {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
/* Пример запроса — как в строке поиска, прижат к низу карточки */
.start__query {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  box-sizing: border-box;
  width: 100%;
  min-height: var(--control-h);
  margin-top: auto;
  padding: var(--space-1) var(--space-3) var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.start__query svg {
  flex: none;
  color: var(--accent);
}
.start__way:hover .start__query {
  background: var(--accent);
  color: var(--on-accent);
}
.start__way:hover .start__query svg {
  color: var(--on-accent);
}
</style>
