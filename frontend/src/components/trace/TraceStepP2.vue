<script setup lang="ts">
// Тело шагов контура коммерции в трассе: «Оборудование» — распознанный парт-номер, единицы с
// операторами, модель «для Y»; «Компаньоны» — якорь, рассмотренные кандидаты (класс, источник) и
// отклонённые с причиной. Шаг «Маржа» — отдельно (трек бэк-офиса).
import { computed } from 'vue'
import type { TraceStep } from '@/api/types'
import { classLabel, sourceLabel } from '@/lib/companions'
import {
  type CompanionConsidered,
  type CompanionRejected,
  type HardwareUnit,
  asList,
  companionAnchorOf,
  companionRejectLabel,
  compatTargetOf,
  mpnOf,
  unitLabel,
} from '@/lib/traceP2'

const props = defineProps<{ step: TraceStep; titles: Record<string, string> }>()

const title = (id: string) => props.titles[id] ?? id
const units = computed(() => asList<HardwareUnit>(props.step.units))
const considered = computed(() => asList<CompanionConsidered>(props.step.considered))
const rejected = computed(() => asList<CompanionRejected>(props.step.rejected))
const anchor = computed(() => companionAnchorOf(props.step))
const CONFIDENCE: Record<string, string> = { high: 'уверенный', medium: 'средняя уверенность' }
</script>

<template>
  <template v-if="step.name === 'hardware'">
    <p v-if="mpnOf(step)" class="t-sm">
      Парт-номер: <span class="t-mono">{{ mpnOf(step) }}</span>
      <span class="t-muted"> · исправление опечаток для номера отключено</span>
    </p>
    <ul v-if="units.length" class="p2-list" aria-label="Распознанные единицы">
      <li v-for="(u, i) in units" :key="i">
        <span class="t-mono">{{ unitLabel(u) }}</span>
        <span v-if="u.text" class="t-muted"> · «{{ u.text }}»</span>
      </li>
    </ul>
    <p v-if="compatTargetOf(step)" class="t-sm">
      Совместимость с: <span class="t-mono">{{ compatTargetOf(step) }}</span>
    </p>
  </template>

  <template v-else-if="step.name === 'companions'">
    <p v-if="anchor" class="t-sm">
      Якорь: {{ title(anchor.id) }}
      <span v-if="anchor.confidence" class="t-muted">
        · {{ CONFIDENCE[anchor.confidence] ?? anchor.confidence }}</span
      >
    </p>
    <ul v-if="considered.length" class="p2-list" aria-label="Рассмотренные кандидаты">
      <li v-for="c in considered" :key="c.product_id">
        {{ title(c.product_id) }}
        <span class="t-muted">
          · {{ classLabel({ label: '', class: c.class ?? '' }) }}
          <template v-if="c.source"> · {{ sourceLabel(c.source) }}</template>
        </span>
      </li>
    </ul>
    <template v-if="rejected.length">
      <p class="t-sm">Не показаны:</p>
      <ul class="p2-list" aria-label="Отклонённые кандидаты">
        <li v-for="r in rejected" :key="r.product_id">
          {{ title(r.product_id) }}
          <span class="t-muted"> · {{ companionRejectLabel(r.reason) }}</span>
        </li>
      </ul>
    </template>
  </template>
</template>

<style scoped>
.p2-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding-left: var(--space-4);
}
</style>
