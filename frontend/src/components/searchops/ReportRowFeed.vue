<script setup lang="ts">
// Раскрытие строки отчёта (кадр 2): что поиск показывает сейчас и лента шагов агента по этому запросу —
// прочитал память, выбрал инструмент, получил ответ, выдвинул причину, проверил правку в песочнице. Шаги — из
// настоящего запуска (GET /api/agent/runs/{id}), отфильтрованы по кластеру строки. Агент не разбирал — «Разобрать».
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowUpRight, Inbox, Microscope } from 'lucide-vue-next'
import { getRun } from '@/api/agent'
import type { ReportRow } from '@/api/typesReport'
import BlockError from '@/components/common/BlockError.vue'
import AgentStepRow from '@/components/searchops/AgentStepRow.vue'
import { useAnalyseQuery } from '@/composables/useAnalyseQuery'
import { useResource } from '@/composables/useResource'
import { hasFeed, rowSteps, runLink } from '@/lib/report'

const props = defineProps<{ row: ReportRow; section: string }>()

const feed = hasFeed(props.row)
const run = useResource((opts) => getRun(props.row.agent!.run_id!, opts))
onMounted(() => {
  if (feed) void run.load()
})
const steps = computed(() =>
  run.data.value ? rowSteps(run.data.value.steps, props.row.agent!.cluster_id!) : [],
)
const link = computed(() => runLink(props.row))
const proposalLink = computed(() =>
  props.row.proposal
    ? { path: '/searchops', query: { tab: 'queue', status: 'all', p: props.row.proposal.id } }
    : null,
)
const foundLine = computed(() => {
  const f = props.row.found
  if (!f) return ''
  const price =
    f.price_state === 'on_request' ? 'только по запросу' : f.price_state ? 'с ценой' : ''
  return `Поиск сейчас: первым «${f.title}»${price ? ` — ${price}` : ''}`
})
const { analyse, busy } = useAnalyseQuery()
</script>

<template>
  <div class="rfeed">
    <p v-if="foundLine" class="t-sm rfeed__now">{{ foundLine }}</p>
    <p v-if="row.origin" class="t-xs t-muted rfeed__origin">Откуда запрос: {{ row.origin }}</p>
    <p v-if="row.examples?.length" class="t-sm rfeed__now">
      Встречается в запросах: {{ row.examples.map((e) => `«${e}»`).join(', ') }}
    </p>
    <p v-if="row.words?.length && section !== 'unknown_words'" class="t-xs t-muted">
      Непонятные поиску слова: {{ row.words.join(', ') }}
    </p>

    <template v-if="feed">
      <div
        v-if="run.status.value === 'loading' && !run.data.value"
        class="rfeed__skel"
        aria-busy="true"
        aria-label="Лента агента загружается"
      >
        <span v-for="i in 3" :key="i" class="skeleton" :style="{ width: `${90 - i * 15}%` }" />
      </div>
      <BlockError
        v-else-if="run.status.value === 'error' && !run.data.value"
        :error="run.error.value!"
        title="Лента агента не загрузилась"
        @retry="run.load()"
      />
      <template v-else-if="run.data.value">
        <p class="t-xs t-muted rfeed__cap">
          Что сделал агент{{
            row.agent?.cluster_label ? ` — кластер «${row.agent.cluster_label}»` : ''
          }}
        </p>
        <ol v-if="steps.length" class="rfeed__steps" aria-label="Шаги агента по запросу">
          <AgentStepRow v-for="v in steps" :key="v.step.id || v.step.seq" :view="v" />
        </ol>
        <p v-else class="t-sm t-muted">Шагов по этому кластеру в запуске нет.</p>
      </template>
    </template>
    <p v-else-if="row.agent && row.agent.code !== 'unseen'" class="t-sm t-muted">
      {{ row.agent.label }}{{ row.agent.note ? ` — ${row.agent.note}` : '' }}
    </p>
    <p v-else class="t-sm t-muted">Агент этот запрос ещё не разбирал.</p>

    <div class="rfeed__acts">
      <RouterLink v-if="proposalLink" :to="proposalLink" class="btn btn--primary btn--sm">
        <Inbox :size="14" aria-hidden="true" />
        {{ row.proposal?.action === 'ticket' ? 'Открыть задачу' : 'Открыть предложение' }}
      </RouterLink>
      <RouterLink v-if="link" :to="link" class="btn btn--secondary btn--sm">
        <ArrowUpRight :size="14" aria-hidden="true" />
        Весь запуск
      </RouterLink>
      <button
        v-if="section !== 'unknown_words'"
        type="button"
        class="btn btn--secondary btn--sm"
        :disabled="!!busy"
        @click="analyse(row.query)"
      >
        <Microscope :size="14" aria-hidden="true" />
        {{ busy === row.query.trim() ? 'Запускаем…' : 'Разобрать' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.rfeed {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-2) var(--space-4);
}
.rfeed p {
  margin: 0;
}
.rfeed__now {
  color: var(--text);
}
.rfeed__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.rfeed__cap {
  margin-top: var(--space-1);
}
.rfeed__steps {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
  border-left: 2px solid var(--border);
}
.rfeed__acts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin-top: var(--space-1);
}
@media (max-width: 767px) {
  .rfeed {
    padding: 0 0 var(--space-3);
  }
  .rfeed__acts > * {
    flex: 1 1 auto;
  }
}
</style>
