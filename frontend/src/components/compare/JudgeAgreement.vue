<script setup lang="ts">
// Согласие LLM-судьи с ручной разметкой. Судья — модель другого семейства, оценивает пары
// «запрос — товар» вслепую. Рядом NDCG по обеим оценкам и честное объяснение, где они расходятся.
import { computed } from 'vue'
import type { JudgeAgreement } from '@/api/types'
import { judgeView } from '@/lib/evalView'
import {
  formatCost,
  formatDateTime,
  formatDecimal,
  formatInt,
  formatPercent,
  formatSigned,
} from '@/lib/format'

const props = defineProps<{ judge: JudgeAgreement }>()

const view = computed(() => judgeView(props.judge))
const swappedText = computed(() =>
  (view.value?.swapped ?? []).map(([a, b]) => `${a} и ${b}`).join(', '),
)
// Начало лестницы до первого расхождения: «B0 < B1 < B2».
const agreedOrder = computed(() => {
  const rows = view.value?.rows ?? []
  const first = view.value?.swapped[0]
  const end = first ? rows.findIndex((r) => r.config === first[0]) + 1 : rows.length
  return end > 1
    ? rows
        .slice(0, end)
        .map((r) => r.config)
        .join(' < ')
    : ''
})
</script>

<template>
  <section id="judge" class="panel judge" aria-labelledby="judge-title">
    <header class="judge__head">
      <h2 id="judge-title" class="t-h2">Проверка LLM-судьёй</h2>
      <p class="t-xs t-muted">
        {{ judge.model }} — другое семейство, чем модель разбора; оценивал вслепую, без конфигурации
        и позиции · {{ formatInt(judge.judged) }} пар из топ-{{ judge.depth }} ·
        {{ formatDateTime(judge.created_at) }}
      </p>
    </header>

    <dl class="judge__tiles">
      <div>
        <dt>Точное согласие с разметкой</dt>
        <dd class="t-num">{{ formatPercent(judge.exact) }}</dd>
      </div>
      <div>
        <dt>Расхождение не больше 1 балла</dt>
        <dd class="t-num">{{ formatPercent(judge.within1) }}</dd>
      </div>
      <div>
        <dt>Взвешенная каппа</dt>
        <dd class="t-num">{{ formatDecimal(judge.kappa, 3) }}</dd>
      </div>
    </dl>

    <div
      v-if="view"
      class="data-scroll judge__scroll"
      role="region"
      aria-label="NDCG по разметке и по судье"
    >
      <table class="data-table judge__table">
        <thead>
          <tr>
            <th scope="col">Конфигурация</th>
            <th scope="col" class="num">NDCG@{{ judge.depth }}, разметка</th>
            <th scope="col" class="num">NDCG@{{ judge.depth }}, судья</th>
            <th scope="col" class="num">Разница</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in view.rows" :key="r.config">
            <th scope="row">{{ r.config }}</th>
            <td class="num">{{ formatDecimal(r.labels, 3) }}</td>
            <td class="num">{{ formatDecimal(r.judge, 3) }}</td>
            <td class="num t-muted">{{ formatSigned(r.diff, 3) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="judge.same_order" class="t-sm">Порядок конфигураций у судьи и у разметки совпадает.</p>
    <p v-else-if="view?.swapped.length" class="t-sm judge__note">
      <template v-if="agreedOrder">Порядок {{ agreedOrder }} совпадает. </template>
      {{ swappedText }} судья ставит наоборот: он даёт баллы близким заменам отсутствующих товаров,
      которые показывает B2, а B3 в таких случаях честно отвечает «нет в каталоге». Для закупщика
      замена без пометки хуже честного «нет», поэтому решение принимаем по ручной разметке.
    </p>

    <p class="t-xs t-muted">
      {{ formatInt(judge.calls) }} вызовов, {{ formatCost(judge.cost_rub) }} · набор
      {{ judge.set }}, промпт <span class="t-mono">{{ judge.prompt_version }}</span>
    </p>
  </section>
</template>

<style scoped>
.judge {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  scroll-margin-top: calc(var(--header-h) + var(--space-4));
}
.judge__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.judge__tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);
  margin: 0;
}
.judge__tiles div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.judge__tiles dt {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.judge__tiles dd {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.judge__scroll {
  max-width: 40rem;
}
.judge__table th[scope='row'] {
  font-weight: var(--fw-medium);
}
.judge__note {
  max-width: 48rem;
  color: var(--text-2);
}
@media (max-width: 639px) {
  .judge__tiles {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
