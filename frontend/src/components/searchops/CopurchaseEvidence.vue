<script setup lang="ts">
// Доказательства предложения по статистике покупок: сила доказательства словом,
// таблица доказательств (аккаунты, support, confidence, lift, p/q, повтор на позднем периоде), ступени проверки,
// вердикт судьи и отклонённые гипотезы с причиной. Все числа — готовые строки сервера (evidence.table).
import { computed, ref } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import {
  ChevronDown,
  ChevronRight,
  CircleCheck,
  CircleMinus,
  CircleX,
  Gavel,
  ShieldAlert,
  ShieldCheck,
} from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import { evidenceView, type RungState } from '@/lib/killer'

const props = defineProps<{ proposal: Proposal }>()
const v = computed(() => evidenceView(props.proposal))
const open = ref(false)
const icon = (s: RungState) => (s === 'ok' ? CircleCheck : s === 'skipped' ? CircleMinus : CircleX)
const tone = (s: RungState) =>
  s === 'ok' ? 'status-accepted' : s === 'skipped' ? 'status-pending' : 'status-rejected'
const failed = computed(() => v.value.ladder.filter((r) => r.state === 'failed').length)
</script>

<template>
  <section class="ce" aria-label="Доказательства по покупкам">
    <p class="ce__label">Доказательства по покупкам</p>
    <p
      v-if="v.strength"
      :class="['ce__strength', v.strong ? 'status-accepted' : 'status-pending']"
      data-testid="strength"
    >
      <component :is="v.strong ? ShieldCheck : ShieldAlert" :size="14" aria-hidden="true" />
      <span
        >Сила доказательства: <strong>{{ v.strength }}</strong></span
      >
    </p>

    <div v-if="v.table.length" class="data-scroll ce__scroll">
      <table class="data-table ce__table">
        <caption class="sr-only">
          Таблица доказательств
        </caption>
        <tbody>
          <tr v-for="r in v.table" :key="r.label">
            <th scope="row">{{ r.label }}</th>
            <td>{{ r.value }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="v.judge" :class="['ce__judge t-sm', v.judge.objection ? 'status-rejected' : '']">
      <Gavel :size="14" aria-hidden="true" />
      <span><span class="ce__muted">Судья:</span> {{ v.judge.text }}</span>
    </p>

    <CollapsibleRoot v-if="v.ladder.length || v.rejected.length" v-model:open="open">
      <CollapsibleTrigger class="ce__more link t-sm">
        <component :is="open ? ChevronDown : ChevronRight" :size="14" aria-hidden="true" />
        Ступени проверки · {{ v.ladder.length
        }}<template v-if="failed"> · не пройдено {{ failed }}</template
        ><template v-if="v.rejected.length">
          · отклонённые гипотезы {{ v.rejected.length }}</template
        >
      </CollapsibleTrigger>
      <CollapsibleContent class="ce__body">
        <ol v-if="v.ladder.length" class="ce__ladder" aria-label="Ступени проверки">
          <li v-for="r in v.ladder" :key="r.no" :class="['ce__rung', tone(r.state)]">
            <component :is="icon(r.state)" :size="14" aria-hidden="true" class="ce__icon" />
            <span class="ce__rung-head">
              <span class="t-muted">{{ r.no }}.</span> {{ r.name }} —
              <span class="ce__tone">{{ r.stateLabel }}</span>
            </span>
            <span class="ce__rung-text t-sm t-muted">{{ r.text }}</span>
          </li>
        </ol>
        <ul v-if="v.rejected.length" class="ce__ladder" aria-label="Отклонённые гипотезы">
          <li v-for="h in v.rejected" :key="h.no" class="ce__rung status-rejected">
            <CircleX :size="14" aria-hidden="true" class="ce__icon" />
            <s class="ce__rung-head">{{ h.text }}</s>
            <span class="ce__rung-text t-sm ce__tone">отклонена: {{ h.reason }}</span>
          </li>
        </ul>
      </CollapsibleContent>
    </CollapsibleRoot>
  </section>
</template>

<style scoped>
.ce {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ce__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.ce__strength,
.ce__judge {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
  overflow-wrap: anywhere;
}
.ce__strength svg,
.ce__judge svg {
  flex: none;
  margin-top: 3px;
  color: var(--tone, var(--text-2));
}
.ce__muted {
  color: var(--text-2);
}
.ce__scroll {
  max-width: 48rem;
}
.ce__table th[scope='row'] {
  font-weight: var(--fw-regular);
  color: var(--text-2);
  width: 45%;
}
.ce__table td {
  overflow-wrap: anywhere;
}
.ce__more {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  cursor: pointer;
}
.ce__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-top: var(--space-2);
}
.ce__ladder {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ce__rung {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr);
  gap: 2px var(--space-2);
  align-items: baseline;
}
.ce__icon {
  color: var(--tone);
  align-self: center;
}
.ce__rung-head {
  overflow-wrap: anywhere;
}
.ce__rung-text {
  grid-column: 2;
  overflow-wrap: anywhere;
}
.ce__tone {
  color: var(--tone);
}
</style>
