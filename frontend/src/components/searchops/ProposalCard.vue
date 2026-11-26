<script setup lang="ts">
// Карточка предложения агента сверху вниз: диагноз словами → запросы кластера с частотой →
// что предлагается (разница правил) → результат прогона двумя строками → ход расследования → решение.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowRight, CircleCheck, CircleX, Search } from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import { formatDateTime, formatInt, plural } from '@/lib/format'
import {
  clusterQueries,
  diagnosisLabel,
  evaluationLines,
  isTicket,
  proposalRoleLabel,
  proposalStatus,
  rejectReasonLabel,
  searchCheckUrl,
} from '@/lib/proposals'
import PatchDiff from './PatchDiff.vue'

const props = defineProps<{ proposal: Proposal; busy?: boolean }>()
defineEmits<{ accept: []; reject: [] }>()

const QUERIES_SHOWN = 6
const p = computed(() => props.proposal)
const status = computed(() => proposalStatus(p.value.status))
const queries = computed(() => clusterQueries(p.value))
const shown = computed(() => queries.value.slice(0, QUERIES_SHOWN))
const more = computed(() => Math.max(0, queries.value.length - QUERIES_SHOWN))
const ev = computed(() => evaluationLines(p.value.evaluation))
const ticket = computed(() => isTicket(p.value))
const checkUrl = computed(() => searchCheckUrl(p.value))
const runLink = computed(() =>
  p.value.run_id
    ? {
        path: `/searchops/runs/${encodeURIComponent(p.value.run_id)}`,
        query: { cluster: p.value.cluster?.id },
      }
    : null,
)
</script>

<template>
  <article class="panel pcard" :aria-labelledby="`p-${p.id}-title`">
    <header class="pcard__head">
      <span :class="['badge', 'badge--tone', status.tone]">{{ status.label }}</span>
      <span class="badge">{{ diagnosisLabel(p.diagnosis) }}</span>
      <span class="t-xs t-muted">{{ proposalRoleLabel(p.role) }}</span>
      <span class="pcard__demand t-xs t-muted t-num">
        потерянный спрос {{ formatInt(p.cluster?.lost_demand ?? 0) }}
      </span>
    </header>

    <h3 :id="`p-${p.id}-title`" class="t-h2">Кластер «{{ p.cluster?.label }}»</h3>
    <p v-if="p.diagnosis_text" class="pcard__diag">{{ p.diagnosis_text }}</p>

    <section class="pcard__block" aria-label="Запросы кластера">
      <p class="pcard__label">
        Запросы кластера · {{ formatInt(queries.length) }}
        {{ plural(queries.length, ['запрос', 'запроса', 'запросов']) }}
      </p>
      <ul class="pcard__queries">
        <li v-for="q in shown" :key="q.text">
          <RouterLink
            :to="{ path: '/search', query: { q: q.text, trace: '1' } }"
            class="pcard__query"
            :title="`Открыть «${q.text}» в поиске с трассой`"
          >
            {{ q.text }}
          </RouterLink>
          <span class="t-num t-muted">× {{ formatInt(q.freq) }}</span>
        </li>
        <li v-if="more" class="t-muted t-xs">и ещё {{ more }}</li>
      </ul>
    </section>

    <section class="pcard__block" aria-label="Что предлагается">
      <p class="pcard__label">{{ ticket ? 'Что сделать' : 'Что предлагается' }}</p>
      <PatchDiff
        :patch="p.patch"
        :empty="
          ticket
            ? 'Правила поиска не меняются: задача ответственному за ассортимент или карточку'
            : undefined
        "
      />
    </section>

    <section v-if="ev" class="pcard__block" aria-label="Результат прогона">
      <p class="pcard__label">
        Результат прогона
        <template v-if="p.hypotheses_tried > 1">
          · проверено {{ p.hypotheses_tried }}
          {{ plural(p.hypotheses_tried, ['гипотеза', 'гипотезы', 'гипотез']) }}
        </template>
      </p>
      <ul class="pcard__eval">
        <li>{{ ev.target }}</li>
        <li>{{ ev.regression }}</li>
        <li v-for="x in ev.extra" :key="x">{{ x }}</li>
      </ul>
      <p :class="['pcard__verdict', ev.passed ? 'status-accepted' : 'status-rejected']">
        <component :is="ev.passed ? CircleCheck : CircleX" :size="14" aria-hidden="true" />
        {{ ev.passed ? 'Условия допуска выполнены' : 'Условия допуска не выполнены' }}
      </p>
    </section>

    <footer class="pcard__foot">
      <RouterLink v-if="runLink" :to="runLink" class="link t-sm pcard__run">
        Показать ход расследования <ArrowRight :size="14" aria-hidden="true" />
      </RouterLink>

      <div v-if="p.status === 'pending'" class="pcard__actions">
        <button type="button" class="btn btn--secondary" :disabled="busy" @click="$emit('reject')">
          Отклонить
        </button>
        <button type="button" class="btn btn--primary" :disabled="busy" @click="$emit('accept')">
          {{ ticket ? 'Взять в работу' : 'Принять' }}
        </button>
      </div>

      <div v-else class="pcard__decided t-sm">
        <span class="t-muted">
          {{ status.label }}
          <template v-if="p.decided_at">{{ formatDateTime(p.decided_at) }}</template>
          <template v-if="p.config_version"> · версия правил {{ p.config_version }}</template>
          <template v-if="p.decided_reason">
            · причина: {{ rejectReasonLabel(p.decided_reason) }}</template
          >
        </span>
        <RouterLink
          v-if="p.status === 'accepted' && checkUrl && !ticket"
          :to="checkUrl"
          class="btn btn--secondary btn--sm"
        >
          <Search :size="14" aria-hidden="true" /> Проверить в поиске
        </RouterLink>
      </div>
    </footer>
  </article>
</template>

<style scoped>
.pcard {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.pcard__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.pcard__demand {
  margin-left: auto;
}
.pcard__diag {
  color: var(--text);
}
.pcard__block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.pcard__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.pcard__queries {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pcard__queries li {
  display: inline-flex;
  max-width: 100%;
  box-sizing: border-box;
  overflow-wrap: anywhere;
  align-items: baseline;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-sm);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.pcard__queries li.t-muted {
  border: 0;
}
.pcard__query {
  color: var(--text);
  text-decoration: none;
}
.pcard__query:hover {
  text-decoration: underline;
}
.pcard__eval {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  list-style: none;
}
.pcard__verdict {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--tone);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.pcard__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding-top: var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.pcard__run {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.pcard__actions,
.pcard__decided {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
</style>
