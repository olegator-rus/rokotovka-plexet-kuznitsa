<script setup lang="ts">
// Конец выдачи, один блок в колонке товаров: красная часть «Не нашли то, что искали?» (одна кнопка — менеджер) и
// под ней похожие запросы — следующий шаг к другим товарам. Запросы — related_queries выдачи с сервера.
import { ArrowRight, Headset, Search } from 'lucide-vue-next'

defineProps<{
  related: string[]
  /** false — на экране уже есть свой блок «ничего не нашлось» с кнопкой менеджера. */
  offerHelp: boolean
}>()
defineEmits<{ search: [query: string]; manager: [] }>()
</script>

<template>
  <div v-if="offerHelp || related.length" class="nf">
    <section v-if="offerHelp" class="nf__banner on-hero" aria-labelledby="nf-title">
      <span class="nf__badge" aria-hidden="true"><Headset :size="28" /></span>
      <div class="nf__lead">
        <h2 id="nf-title" class="nf__title">Не нашли то, что искали?</h2>
        <p class="nf__text">Менеджер подберёт решение по вашему запросу или предложит замену.</p>
      </div>
      <div class="nf__actions">
        <button type="button" class="btn nf__primary" @click="$emit('manager')">
          Спросить менеджера
          <ArrowRight :size="16" aria-hidden="true" />
        </button>
      </div>
    </section>

    <section v-if="related.length" class="nf__more" aria-labelledby="related-title">
      <h2 id="related-title" class="t-h2">Похожие запросы</h2>
      <ul class="nf__related">
        <li v-for="q in related" :key="q">
          <button type="button" class="nf__query" @click="$emit('search', q)">
            <Search :size="16" aria-hidden="true" />
            <span>{{ q }}</span>
            <ArrowRight :size="16" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.nf {
  container-type: inline-size;
  /* Отступ сверху и скругление — как у карточек товаров: расстояние задаёт колонка выдачи. */
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.nf__banner {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-4) var(--space-6);
  padding: var(--space-8) var(--space-6);
  overflow: hidden;
}
/* Фирменные квадраты из логотипа — тихий фон справа, не несёт смысла. */
.nf__banner::after {
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
.nf__banner > * {
  position: relative;
  z-index: 1;
}
.nf__badge {
  display: grid;
  flex: none;
  place-items: center;
  width: 3.5rem;
  height: 3.5rem;
  border-radius: var(--radius-pill);
  background: var(--on-hero);
  color: var(--hero);
}
.nf__lead {
  flex: 1 1 18rem;
  min-width: 0;
}
.nf__title {
  margin: 0 0 var(--space-1);
  font-size: var(--fs-banner);
  line-height: 1.2;
  font-weight: var(--fw-bold);
  letter-spacing: -0.02em;
  text-wrap: balance;
}
.nf__text {
  margin: 0;
  color: var(--on-hero-2);
}
.nf__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.nf__actions .btn {
  min-height: var(--touch-min);
  padding-inline: var(--space-5);
}
.nf__actions .nf__primary {
  background: var(--on-hero);
  color: var(--hero);
}
.nf__actions .nf__primary:hover {
  background: var(--on-hero-2);
}

.nf__more {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-6) var(--space-6);
}
.nf__related {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 17rem), 1fr));
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.nf__query {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  min-height: var(--touch-min);
  padding: var(--space-2) var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-medium);
  text-align: left;
  cursor: pointer;
  transition:
    border-color var(--dur-fast) var(--ease-standard),
    background-color var(--dur-fast) var(--ease-standard);
}
.nf__query svg {
  flex: none;
  color: var(--accent);
}
.nf__query span {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.nf__query:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
}
</style>
