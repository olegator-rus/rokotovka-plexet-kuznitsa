<script setup lang="ts">
// Калибровка LLM-судьи на якорях фида (`go run . judge-calib`): эталоны заказчика и их соседи — запросы с известной
// целью. Товар целевого семейства должен получить 3, товар цели другого вендора («запрещённый сосед») — не выше 1.
// Каппы против кликов здесь нет: неявные оценки почти все 2, каппа против них вырождена.
import { computed, ref } from 'vue'
import type { JudgeCalibration } from '@/api/types'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import { formatCost, formatInt, formatPercent } from '@/lib/format'
import { outOf, runTail } from '@/lib/feedSummary'

const props = defineProps<{ calib: JudgeCalibration }>()

const open = ref(false)
const explanation = ref<Explanation | null>(null)

const etalonRate = computed(() =>
  props.calib.etalon_targets > 0 ? props.calib.etalon_hit / props.calib.etalon_targets : null,
)

function grades(g: number[]): string {
  return g.map((n, i) => `${i} — ${formatInt(n)}`).join(', ')
}

type Key = 'targets' | 'etalon' | 'forbidden'

function explain(key: Key) {
  const c = props.calib
  const common = [
    { label: 'Модель судьи', value: c.model },
    { label: 'Промпт', value: c.prompt_version },
    { label: 'Семейство в карточке товара', value: c.with_family ? 'да' : 'нет' },
    { label: 'Прогон калибровки', value: runTail(c.run_id) },
  ]
  if (key === 'forbidden') {
    explanation.value = {
      title: 'Запрещённые соседи не выше 1',
      value: formatPercent(c.forbidden_rate ?? 0),
      what: 'Судья не завышает чужой продукт: к запросу якоря показан товар цели якоря другого вендора.',
      how: 'Доля пар «якорь — цель другого вендора», где судья поставил 0 или 1.',
      rows: [
        { label: 'Пар', value: formatInt(c.forbidden) },
        { label: 'Не выше 1', value: formatInt(c.forbidden_ok) },
        { label: 'Оценки судьи', value: grades(c.forbidden_grades) },
        ...common,
      ],
      source: `GET /api/evaluations → judge_calibration · ${c.run_id}`,
    }
  } else {
    const etalon = key === 'etalon'
    explanation.value = {
      title: etalon ? 'Цели эталонов заказчика: оценка 3' : 'Цели якорей: оценка 3',
      value: formatPercent((etalon ? etalonRate.value : c.target_rate) ?? 0),
      what: etalon
        ? '10 эталонов заказчика: по две основные лицензии целевого семейства (любой вариант продукта — 3).'
        : 'Эталоны заказчика и их соседи (транслит, склонения, опечатки, атрибуты): товар целевого семейства.',
      how: 'Доля пар «якорь — товар целевого семейства», где судья поставил 3.',
      rows: [
        { label: 'Пар', value: formatInt(etalon ? c.etalon_targets : c.targets) },
        { label: 'С оценкой 3', value: formatInt(etalon ? c.etalon_hit : c.targets_hit) },
        ...(etalon ? [] : [{ label: 'Оценки судьи', value: grades(c.target_grades) }]),
        ...common,
      ],
      source: `GET /api/evaluations → judge_calibration · ${c.run_id}`,
    }
  }
  open.value = true
}
</script>

<template>
  <section id="judge" class="panel calib" aria-labelledby="calib-title">
    <header class="calib__head">
      <h2 id="calib-title" class="t-h2">Калибровка LLM-судьи на якорях</h2>
      <p class="t-sm t-muted">
        Судья {{ calib.model }} размечает пары «запрос — товар» вслепую, без конфигурации и позиции.
        Проверка — на запросах с известной целью: эталонах заказчика и их соседях. Согласие с
        кликами не показываем: клики почти все с оценкой 2, и каппа против них ничего не говорит.
      </p>
    </header>
    <div class="calib__tiles">
      <StatTile
        label="Цели якорей получили 3"
        :value="calib.target_rate !== null ? formatPercent(calib.target_rate) : null"
        :sub="outOf(calib.targets_hit, calib.targets)"
        @explain="explain('targets')"
      />
      <StatTile
        label="Из них эталоны заказчика"
        :value="etalonRate !== null ? formatPercent(etalonRate) : null"
        :sub="outOf(calib.etalon_hit, calib.etalon_targets)"
        @explain="explain('etalon')"
      />
      <StatTile
        label="Запрещённые соседи не выше 1"
        :value="calib.forbidden_rate !== null ? formatPercent(calib.forbidden_rate) : null"
        :sub="outOf(calib.forbidden_ok, calib.forbidden)"
        @explain="explain('forbidden')"
      />
    </div>
    <p class="t-xs t-muted">
      {{ formatInt(calib.calls) }} вызовов, {{ formatCost(calib.cost_rub) }} · промпт
      <span class="t-mono">{{ calib.prompt_version }}</span> ·
      <span class="t-mono">go run . judge-calib</span>
    </p>
  </section>
  <ExplainDialog v-model:open="open" :explanation="explanation" />
</template>

<style scoped>
.calib {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  scroll-margin-top: calc(var(--header-h) + var(--space-4));
}
.calib__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 48rem;
}
.calib__tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);
}
@media (max-width: 639px) {
  .calib__tiles {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
