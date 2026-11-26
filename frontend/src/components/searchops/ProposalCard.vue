<script setup lang="ts">
// Карточка предложения агента сверху вниз: диагноз словами → запросы кластера с частотой →
// что предлагается (разница правил) → результат прогона двумя строками → ход расследования → решение.
import { computed, ref } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { RouterLink } from 'vue-router'
import {
  ArrowRight,
  ChevronDown,
  ChevronRight,
  CircleCheck,
  CircleX,
  Lock,
  Search,
} from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import { denyMessage } from '@/lib/auth'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime, formatInt, plural } from '@/lib/format'
import {
  clusterQueries,
  diagnosisLabel,
  effectLine,
  evaluationLines,
  hypothesisLines,
  isTicket,
  lostDemandLabel,
  proposalRoleLabel,
  proposalStatus,
  recheckView,
  rejectReasonLabel,
  searchCheckUrl,
  staleReason,
  targetQueryRows,
  ticketLines,
} from '@/lib/proposals'
import {
  PRIORITY_CAPTION,
  isBundle,
  isCopurchase,
  isMarginProposal,
  marginProposalView,
  priorityValue,
} from '@/lib/killer'
import { isRelationPatch } from '@/lib/relationProposal'
import BundleView from './BundleView.vue'
import CopurchaseEvidence from './CopurchaseEvidence.vue'
import MarginProposalView from './MarginProposalView.vue'
import PatchDiff from './PatchDiff.vue'
import RelationPatchView from './RelationPatchView.vue'

const props = defineProps<{ proposal: Proposal; busy?: boolean }>()
defineEmits<{ accept: []; reject: [] }>()

const QUERIES_SHOWN = 6
const p = computed(() => props.proposal)
// Право решения — по роли предложения: чужое — кнопки неактивны с той же фразой, что 403 сервера.
const auth = useAuthStore()
const deny = computed(() => (auth.can(p.value.role) ? '' : denyMessage(p.value.role)))
const denyId = computed(() => `deny-${p.value.id}`)
const status = computed(() => proposalStatus(p.value.status))
const queries = computed(() => clusterQueries(p.value))
const shown = computed(() => queries.value.slice(0, QUERIES_SHOWN))
const more = computed(() => Math.max(0, queries.value.length - QUERIES_SHOWN))
const ev = computed(() => evaluationLines(p.value.evaluation))
const ticket = computed(() => isTicket(p.value))
const checkUrl = computed(() => searchCheckUrl(p.value))
const hypotheses = computed(() => hypothesisLines(p.value))
const rejectedCount = computed(() => hypotheses.value.filter((h) => !h.passed).length)
const targetRows = computed(() => targetQueryRows(p.value.evaluation))
const todo = computed(() => (ticket.value ? ticketLines(p.value) : []))
const effect = computed(() => effectLine(p.value.effect))
const recheck = computed(() => recheckView(p.value))
const stale = computed(() => staleReason(p.value))
const relation = computed(() => isRelationPatch(p.value))
const bundle = computed(() => isBundle(p.value))
const copurchase = computed(() => isCopurchase(p.value))
// Контур коммерции: связь, комплект, маржа — заголовок без «Кластер», без потерянного спроса и запросов кластера.
const commerce = computed(() => relation.value || bundle.value || isMarginProposal(p.value))
// Комплект принимается «в работу»: справочник и выдача не меняются.
const workItem = computed(() => ticket.value || bundle.value)
const priority = computed(() =>
  priorityValue(p.value as Proposal & { priority_score?: number | null }),
)
// Предложение настройки маржи применяется на экране «Маржа», не из очереди:
// /margin?w=<вес>&cat=<категория>&p=<id> — песочница с теми же числами.
const marginLink = computed(() =>
  isMarginProposal(p.value) ? marginProposalView(p.value).link : null,
)
const detailsOpen = ref(false)
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
  <article
    :id="`proposal-${p.id}`"
    class="panel pcard"
    tabindex="-1"
    :aria-labelledby="`p-${p.id}-title`"
  >
    <header class="pcard__head">
      <span :class="['badge', 'badge--tone', status.tone]">{{ status.label }}</span>
      <span class="badge">{{ diagnosisLabel(p.diagnosis) }}</span>
      <span class="t-xs t-muted">{{ proposalRoleLabel(p.role) }}</span>
      <span v-if="priority" class="pcard__priority t-sm" data-testid="priority">
        <strong class="t-num">{{ priority }}</strong>
        <span class="t-xs t-muted">{{ PRIORITY_CAPTION }}</span>
      </span>
      <span
        v-else-if="!commerce || p.cluster?.lost_demand"
        class="pcard__demand t-xs t-muted t-num"
        title="Различные сессии, в которых поиск по запросам кластера не удался"
      >
        {{ lostDemandLabel(p.cluster?.lost_demand) }}
      </span>
    </header>

    <h3 :id="`p-${p.id}-title`" class="t-h2">
      <template v-if="commerce">{{ p.cluster?.label }}</template>
      <template v-else>Кластер «{{ p.cluster?.label }}»</template>
    </h3>
    <p v-if="stale" class="pcard__stale t-sm">
      <span class="pcard__effect-label">Почему устарело:</span> {{ stale }}. Принять нельзя.
    </p>
    <p v-if="p.diagnosis_text" class="pcard__diag">{{ p.diagnosis_text }}</p>

    <section
      v-if="!copurchase && !marginLink && (queries.length || !relation)"
      class="pcard__block"
      aria-label="Запросы кластера"
    >
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

    <MarginProposalView v-if="marginLink" :proposal="p" />

    <BundleView v-else-if="bundle" :proposal="p" />

    <template v-else-if="relation">
      <RelationPatchView :proposal="p" />
      <CopurchaseEvidence v-if="copurchase" :proposal="p" />
    </template>

    <section v-else class="pcard__block" aria-label="Что предлагается">
      <p class="pcard__label">{{ ticket ? 'Что сделать' : 'Что предлагается' }}</p>
      <ul v-if="todo.length" class="pcard__todo">
        <li v-for="t in todo" :key="t">{{ t }}</li>
      </ul>
      <PatchDiff
        v-if="!ticket || !todo.length"
        :patch="p.patch"
        :empty="
          ticket
            ? 'Правила поиска не меняются: задача ответственному за ассортимент или карточку'
            : undefined
        "
      />
      <p v-else class="t-xs t-muted">Правила поиска не меняются</p>
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

      <CollapsibleRoot
        v-if="hypotheses.length > 1 || targetRows.length"
        v-model:open="detailsOpen"
        class="pcard__details"
      >
        <CollapsibleTrigger class="pcard__more link t-sm">
          <component :is="detailsOpen ? ChevronDown : ChevronRight" :size="14" aria-hidden="true" />
          Как проверяли<template v-if="rejectedCount">
            · {{ rejectedCount }}
            {{
              plural(rejectedCount, [
                'гипотеза отклонена',
                'гипотезы отклонены',
                'гипотез отклонено',
              ])
            }}</template
          >
        </CollapsibleTrigger>
        <CollapsibleContent class="pcard__details-body">
          <ol v-if="hypotheses.length > 1" class="pcard__hyps" aria-label="Гипотезы агента">
            <li
              v-for="h in hypotheses"
              :key="h.no"
              :class="['pcard__hyp', h.passed ? 'status-accepted' : 'status-rejected']"
            >
              <component
                :is="h.passed ? CircleCheck : CircleX"
                :size="14"
                aria-hidden="true"
                class="pcard__hyp-icon"
              />
              <span class="pcard__hyp-text">
                <span class="t-muted pcard__hyp-no">№{{ h.no }}</span>
                <s v-if="!h.passed">{{ h.text }}</s>
                <template v-else>{{ h.text }}</template>
                <span v-if="h.target" class="t-num t-muted"> · {{ h.target }}</span>
              </span>
              <span class="pcard__hyp-verdict t-sm">
                {{ h.passed ? 'прошла' : `отклонена: ${h.reason ?? 'не прошла прогон'}` }}
              </span>
            </li>
          </ol>
          <div v-if="targetRows.length" class="data-scroll pcard__tq-scroll">
            <table class="data-table pcard__tq">
              <caption class="sr-only">
                Запросы целевого кластера до и после правки
              </caption>
              <thead>
                <tr>
                  <th scope="col">Запрос</th>
                  <th scope="col" class="num">Частота</th>
                  <th scope="col">До правки</th>
                  <th scope="col">После</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in targetRows" :key="r.text">
                  <th scope="row">{{ r.text }}</th>
                  <td class="num">{{ formatInt(r.freq) }}</td>
                  <td :class="{ 't-muted': !r.before }">{{ r.before ? 'находится' : 'нет' }}</td>
                  <td
                    :class="
                      r.worse
                        ? 'status-rejected pcard__tq-tone'
                        : r.after && !r.before
                          ? 'status-accepted pcard__tq-tone'
                          : r.after
                            ? ''
                            : 't-muted'
                    "
                  >
                    {{ r.after ? 'находится' : 'нет' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </CollapsibleContent>
      </CollapsibleRoot>
    </section>

    <section
      v-if="recheck"
      :class="['pcard__recheck', recheck.passed ? 'status-accepted' : 'status-rejected']"
      aria-label="Перепроверка на действующих правилах"
    >
      <p class="pcard__recheck-title t-sm">
        <component :is="recheck.passed ? CircleCheck : CircleX" :size="14" aria-hidden="true" />
        <span>{{ recheck.title }}</span>
      </p>
      <ul v-if="recheck.reasons.length" class="pcard__eval">
        <li v-for="r in recheck.reasons" :key="r">{{ r }}</li>
      </ul>
      <template v-if="recheck.lines">
        <p class="pcard__label">{{ recheck.linesTitle }}</p>
        <ul class="pcard__eval">
          <li>{{ recheck.lines.target }}</li>
          <li>{{ recheck.lines.regression }}</li>
          <li v-for="x in recheck.lines.extra" :key="x">{{ x }}</li>
        </ul>
      </template>
      <p v-if="recheck.checkedAt" class="t-xs t-muted">
        Перепроверено {{ formatDateTime(recheck.checkedAt) }}
      </p>
    </section>

    <p
      v-if="effect"
      :class="['pcard__effect t-sm', effect.holds ? 'status-accepted' : 'status-rejected']"
    >
      <component :is="effect.holds ? CircleCheck : CircleX" :size="14" aria-hidden="true" />
      <span
        ><span class="pcard__effect-label">Эффект после принятия:</span> {{ effect.text
        }}{{ effect.holds ? ' — держится' : ' — не держится' }}</span
      >
    </p>

    <footer class="pcard__foot">
      <RouterLink v-if="runLink" :to="runLink" class="link t-sm pcard__run">
        Показать ход расследования <ArrowRight :size="14" aria-hidden="true" />
      </RouterLink>

      <div v-if="p.status === 'pending' && marginLink" class="pcard__actions">
        <button
          type="button"
          class="btn btn--secondary"
          :disabled="busy || !!deny"
          :title="deny || undefined"
          :aria-describedby="deny ? denyId : undefined"
          @click="$emit('reject')"
        >
          <Lock v-if="deny" :size="14" aria-hidden="true" />
          Отклонить
        </button>
        <RouterLink :to="marginLink" class="btn btn--primary">Открыть в песочнице</RouterLink>
        <p v-if="deny" :id="denyId" class="pcard__deny t-xs">
          <Lock :size="12" aria-hidden="true" /> {{ deny }}
        </p>
      </div>

      <div v-else-if="p.status === 'pending'" class="pcard__actions">
        <button
          type="button"
          class="btn btn--secondary"
          :disabled="busy || !!deny"
          :title="deny || undefined"
          :aria-describedby="deny ? denyId : undefined"
          @click="$emit('reject')"
        >
          <Lock v-if="deny" :size="14" aria-hidden="true" />
          Отклонить
        </button>
        <button
          type="button"
          class="btn btn--primary"
          :disabled="busy || !!deny"
          :title="deny || undefined"
          :aria-describedby="deny ? denyId : undefined"
          @click="$emit('accept')"
        >
          <Lock v-if="deny" :size="14" aria-hidden="true" />
          {{ workItem ? 'Взять в работу' : 'Принять' }}
        </button>
        <p v-if="deny" :id="denyId" class="pcard__deny t-xs">
          <Lock :size="12" aria-hidden="true" /> {{ deny }}
        </p>
      </div>

      <div v-else class="pcard__decided t-sm">
        <span class="t-muted">
          {{ status.label }}
          <template v-if="p.decided_at">{{ formatDateTime(p.decided_at) }}</template>
          <template v-if="p.config_version"> · версия правил {{ p.config_version }}</template>
          <template v-if="p.decided_reason && p.status !== 'accepted' && p.status !== 'stale'">
            · причина: {{ rejectReasonLabel(p.decided_reason).toLowerCase() }}</template
          >
          <template v-if="p.decided_comment"> · «{{ p.decided_comment }}»</template>
        </span>
        <RouterLink
          v-if="p.status === 'accepted' && checkUrl && !workItem && !marginLink"
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
  scroll-margin-top: calc(var(--header-h) + var(--space-4));
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
.pcard__priority {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0 var(--space-1);
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
.pcard__todo {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  list-style: none;
}
.pcard__more {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  cursor: pointer;
}
.pcard__details-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding-top: var(--space-2);
}
.pcard__hyps {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pcard__hyp {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr);
  gap: 2px var(--space-2);
  align-items: baseline;
}
.pcard__hyp-icon {
  color: var(--tone);
  align-self: center;
}
.pcard__hyp-text {
  overflow-wrap: anywhere;
}
.pcard__hyp-no {
  margin-right: var(--space-1);
}
.pcard__hyp-text s {
  color: var(--text-2);
  text-decoration-color: var(--tone);
}
.pcard__hyp-verdict {
  grid-column: 2;
  color: var(--tone);
}
.pcard__tq-scroll {
  max-width: 40rem;
}
.pcard__tq th[scope='row'] {
  font-weight: var(--fw-regular);
}
.pcard__tq-tone {
  color: var(--tone);
}
.pcard__recheck {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
}
.pcard__recheck-title {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
}
.pcard__recheck-title svg {
  flex: none;
  margin-top: 2px;
  color: var(--tone);
}
.pcard__effect {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
}
.pcard__effect svg {
  flex: none;
  margin-top: 2px;
  color: var(--tone);
}
.pcard__effect-label {
  color: var(--text-2);
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
.pcard__deny {
  display: flex;
  flex-basis: 100%;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-1);
  margin: 0;
  color: var(--text-2);
}
</style>
