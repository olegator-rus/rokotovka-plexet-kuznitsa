<script setup lang="ts">
// «Доказательства» → «Теоретические основы» (tab=theory): как устроено решение и на каких методах держится
// поиск — от нормализации до выдачи, агент SearchOps, как меряется качество и стоимость. Текст статичный,
// в API не ходит. Параметры алгоритмов — из lib/theoryParams.ts (тест сверяет их с Go-кодом); чисел качества
// и денег здесь нет — только ссылки на соседние вкладки. Схемы — блоки на токенах (theory/Theory*.vue).
// Содержание прокручивает к разделу, адрес не меняет: вкладка остаётся на tab=theory. Якорь #theory-<id> в
// адресе при открытии прокручивает к разделу (ссылка на раздел снаружи).
import { nextTick, onMounted, useTemplateRef } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowUp } from 'lucide-vue-next'
import TheoryAgent from './theory/TheoryAgent.vue'
import TheoryOverview from './theory/TheoryOverview.vue'
import TheoryProof from './theory/TheoryProof.vue'
import TheoryRanking from './theory/TheoryRanking.vue'
import TheoryRetrieval from './theory/TheoryRetrieval.vue'
import TheoryUnderstanding from './theory/TheoryUnderstanding.vue'
import { THEORY_PARTS, anchorOf } from './theory/sections'

const route = useRoute()
const toc = useTemplateRef<HTMLElement>('toc')

function go(id: string) {
  const el = document.getElementById(anchorOf(id))
  if (!el) return
  el.scrollIntoView({ block: 'start' })
  el.querySelector<HTMLElement>('h3')?.focus({ preventScroll: true })
}

function toToc() {
  toc.value?.scrollIntoView({ block: 'start' })
  toc.value?.querySelector<HTMLElement>('button')?.focus({ preventScroll: true })
}

onMounted(async () => {
  const hash = route.hash.replace('#', '')
  if (!hash.startsWith('theory-')) return
  await nextTick()
  document.getElementById(hash)?.scrollIntoView({ block: 'start' })
})
</script>

<template>
  <article class="theory">
    <p class="theory__lead">
      Здесь — что происходит с запросом от ввода до выдачи и почему сделано именно так, а также как
      агент SearchOps чинит поиск и как мы проверяем результат. Названия разделов совпадают с шагами
      трассы на экране «Поиск». Числа в тексте — настройки алгоритмов из кода; измеренное качество и
      деньги — на соседних вкладках, по ссылкам.
    </p>

    <nav ref="toc" class="theory__toc panel" aria-labelledby="theory-toc-title">
      <h3 id="theory-toc-title" class="theory__toc-title">Содержание</h3>
      <div class="theory__toc-parts">
        <div v-for="(part, pi) in THEORY_PARTS" :key="part.title" class="theory__toc-part">
          <p class="theory__toc-part-title">{{ pi + 1 }}. {{ part.title }}</p>
          <ol class="theory__toc-list" role="list">
            <li v-for="s in part.sections" :key="s.id">
              <button type="button" class="theory__toc-link" @click="go(s.id)">
                {{ s.title }}
              </button>
            </li>
          </ol>
        </div>
      </div>
    </nav>

    <template v-for="(part, pi) in THEORY_PARTS" :key="part.title">
      <div class="theory__part">
        <p class="theory__part-title">
          <span class="t-num">{{ pi + 1 }}</span> {{ part.title }}
        </p>
        <TheoryOverview v-if="pi === 0" />
        <template v-else-if="pi === 1">
          <TheoryUnderstanding />
          <TheoryRetrieval />
          <TheoryRanking />
        </template>
        <TheoryAgent v-else-if="pi === 2" />
        <TheoryProof v-else />
        <button type="button" class="link theory__up t-sm" @click="toToc">
          <ArrowUp :size="14" aria-hidden="true" /> К содержанию
        </button>
      </div>
    </template>
  </article>
</template>

<style scoped>
.theory {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}
.theory__lead {
  max-width: 72ch;
  margin: 0;
}
.theory__toc {
  padding: var(--space-4);
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--space-4));
}
.theory__toc-title {
  margin: 0 0 var(--space-3);
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.theory__toc-parts {
  column-gap: var(--space-8);
}
.theory__toc-part + .theory__toc-part {
  margin-top: var(--space-4);
}
.theory__toc-list li {
  break-inside: avoid;
}
.theory__toc-part-title {
  margin: 0 0 var(--space-1);
  break-after: avoid;
  color: var(--text-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-bold);
}
.theory__toc-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.theory__toc-link {
  display: block;
  width: 100%;
  min-height: 2rem;
  padding: var(--space-1) 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  text-align: left;
  cursor: pointer;
}
.theory__toc-link:hover {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.theory__part {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}
.theory__part-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  padding-bottom: var(--space-2);
  border-bottom: 2px solid var(--accent);
  color: var(--accent);
  font-size: var(--fs-sm);
  font-weight: var(--fw-bold);
}
.theory__up {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  align-self: flex-start;
  color: var(--text-2);
}

@media (min-width: 768px) {
  .theory__toc-parts {
    columns: 2;
  }
}
</style>

<style>
/* Общий текст разделов «Теоретических основ»: абзацы, списки, формулы, таблицы и пример расчёта. */
.theory p,
.theory ul {
  margin: 0;
}
.theory-section > p,
.theory-section > ul {
  max-width: 72ch;
}
.theory-section ul {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-left: var(--space-5);
  list-style: disc;
}
.theory-section li::marker {
  color: var(--text-3);
}
.theory-section h4 {
  margin: var(--space-2) 0 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.theory-section code {
  font-family: var(--font-mono);
  font-size: 0.92em;
  overflow-wrap: anywhere;
}
.theory-formula {
  max-width: none;
  padding: var(--space-3) var(--space-4);
  border-left: 3px solid var(--accent);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.theory-formula code {
  white-space: pre-wrap;
  font-size: var(--fs-sm);
  line-height: var(--lh-body);
}
.theory-example {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
}
.theory-example figcaption {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.theory-table th[scope='row'] {
  font-weight: var(--fw-semibold);
}
.theory-example .theory-table th[scope='row'] {
  white-space: nowrap;
}
.theory-table code {
  white-space: nowrap;
}
.theory-link {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.theory-link:hover {
  color: var(--accent-hover);
}
</style>
