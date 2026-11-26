<script setup lang="ts">
// «Доказательства» → «Теоретические основы» (tab=theory): разбор одного запроса, как устроено решение и на
// каких методах держится поиск — от нормализации до выдачи, агент SearchOps, как меряется качество и стоимость.
// Текст статичный, в API не ходит. Параметры алгоритмов — из lib/theoryParams.ts (тест сверяет их с Go-кодом);
// наблюдения на стенде и разбор агента подписаны датой и каталогом. Схемы — SVG на токенах (theory/diagrams/,
// общие узлы и стрелки — theory/diagram/), формулы — нативный MathML (theory/TheoryFormula.vue).
// Содержание прокручивает к разделу, адрес не меняет: вкладка остаётся на tab=theory. Якорь #theory-<id> в
// адресе при открытии прокручивает к разделу (ссылка на раздел снаружи).
import { nextTick, onMounted, useTemplateRef } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowUp } from 'lucide-vue-next'
import TheoryAgent from './theory/TheoryAgent.vue'
import TheoryExample from './theory/TheoryExample.vue'
import TheoryOverview from './theory/TheoryOverview.vue'
import TheoryProof from './theory/TheoryProof.vue'
import TheoryRanking from './theory/TheoryRanking.vue'
import TheoryRetrieval from './theory/TheoryRetrieval.vue'
import TheoryToc from './theory/TheoryToc.vue'
import TheoryUnderstanding from './theory/TheoryUnderstanding.vue'
import { THEORY_PARTS, anchorOf } from './theory/sections'

const route = useRoute()
const toc = useTemplateRef<InstanceType<typeof TheoryToc>>('toc')

function go(id: string) {
  const el = document.getElementById(anchorOf(id))
  if (!el) return
  el.scrollIntoView({ block: 'start' })
  el.querySelector<HTMLElement>('h3')?.focus({ preventScroll: true })
}

function toToc() {
  const el = toc.value?.$el as HTMLElement | undefined
  el?.scrollIntoView({ block: 'start' })
  el?.querySelector<HTMLElement>('button')?.focus({ preventScroll: true })
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
      Здесь описано, как поиск помогает покупателю выбрать товар и вариант лицензии: что он делает с
      запросом от ввода до выдачи и почему. Дальше — как агент SearchOps разбирает неудачные запросы
      и как мы проверяем результат. Поиск работает на фиде магазина store.softline.ru. В конце
      большинства разделов есть ссылки на запрос с трассой: на экране «Поиск» видно, как шаг
      сработал на самом деле.
    </p>
    <div class="theory__kinds">
      <p class="theory__kinds-title">Числа на вкладке четырёх видов</p>
      <ul class="theory__kinds-list" role="list">
        <li>
          <span class="badge">настройка</span> пороги, лимиты и веса из кода, так же помечены
          формулы. Тест сверяет эти числа с константами бэкенда, поэтому они совпадают с тем, что
          работает на стенде.
        </li>
        <li>
          <span class="badge">пример расчёта</span> условные товары и оценки, чтобы показать
          арифметику формулы. Это не выдача поиска.
        </li>
        <li>
          <span class="badge">наблюдение на стенде</span> что поиск ответил на конкретный запрос,
          когда писался раздел. Ссылка «с трассой» рядом покажет текущий ответ.
        </li>
        <li>
          <span class="badge">результат прогона</span> качество, скорость и деньги — на вкладках
          «Метрики», «Вклад компонентов» и «Стоимость». Здесь из прогонов только один разбор агента
          в разделе SearchOps. Прогоны оценки и агента пока шли на mock-каталоге, и это подписано
          рядом с каждым числом.
        </li>
      </ul>
    </div>

    <TheoryToc ref="toc" @go="go" />

    <template v-for="(part, pi) in THEORY_PARTS" :key="part.title">
      <div class="theory__part">
        <p class="theory__part-title">
          <span class="t-num">{{ pi + 1 }}</span> {{ part.title }}
        </p>
        <template v-if="pi === 0">
          <TheoryExample />
          <TheoryOverview />
        </template>
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
/* Длинный текст на всю ширину колонки: на широком экране кегль и интерлиньяж крупнее. */
@media (min-width: 768px) {
  .theory {
    font-size: var(--fs-reading);
    line-height: var(--lh-reading);
  }
}
.theory__lead {
  margin: 0;
}
.theory__kinds {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  container-type: inline-size;
}
.theory .theory__kinds-title {
  font-weight: var(--fw-bold);
}
.theory .theory__kinds-list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.theory__kinds-list li {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-card);
  background: var(--surface-2);
}
.theory__kinds-list .badge {
  display: table;
  margin-bottom: var(--space-2);
  background: var(--surface);
}
@container (min-width: 38rem) {
  .theory .theory__kinds-list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
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
</style>

<style>
/* Общий текст разделов «Теоретических основ»: абзацы, списки, таблицы и пример расчёта. */
.theory p,
.theory ul {
  margin: 0;
}
/* Одна ширина на всё: текст, списки, таблицы и схемы занимают колонку целиком. */
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
