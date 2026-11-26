<script setup lang="ts">
// Тело карточки предложения по марже (MARGIN_OPPORTUNITY, action: margin_policy):
// та же пара чисел, что на экране «Маржа» (соответствие NDCG@10 и индекс показанной маржи, до → после), сколько
// запросов проверено. Применяется только на экране «Маржа» — кнопка «Открыть в песочнице» в подвале карточки.
import { computed } from 'vue'
import { CircleCheck } from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import { marginProposalView } from '@/lib/killer'
import { weightLabel } from '@/lib/margin'

const props = defineProps<{ proposal: Proposal }>()
const v = computed(() => marginProposalView(props.proposal))
const notes = computed(() => {
  const ev = (props.proposal.evidence ?? {}) as { technical_note?: string; economic_note?: string }
  return [ev.technical_note, ev.economic_note].filter((x): x is string => !!x)
})
</script>

<template>
  <section class="mp" aria-label="Что предлагается">
    <p class="mp__label">Что предлагается</p>
    <p class="mp__line">
      <template v-if="v.category">Категория «{{ v.category }}»: </template>рычаг маржи
      <strong class="t-num"
        >{{ v.from !== null ? weightLabel(v.from) : '—' }} → {{ weightLabel(v.to) }}</strong
      >; остальные категории — по действующей политике
    </p>
    <dl class="mp__pair">
      <div v-if="v.ndcg">
        <dt>Соответствие, NDCG@10</dt>
        <dd class="t-num">{{ v.ndcg.before }} → {{ v.ndcg.after }}</dd>
      </div>
      <div v-if="v.index">
        <dt>Индекс показанной маржи</dt>
        <dd class="t-num">
          {{ v.index.before }} → {{ v.index.after }}
          <span v-if="v.index.delta" class="t-muted">({{ v.index.delta }})</span>
        </dd>
      </div>
    </dl>
    <p v-if="v.checked" class="t-sm">
      <CircleCheck :size="14" aria-hidden="true" class="mp__ok" />
      {{ v.checked }}<template v-if="v.shifted">; {{ v.shifted }}</template>
    </p>
    <div class="mp__notes">
      <SyntheticBadge v-if="v.synthetic" />
      <p v-for="n in notes" :key="n" class="t-xs t-muted">{{ n }}</p>
    </div>
  </section>
</template>

<style scoped>
.mp {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.mp__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.mp__line {
  overflow-wrap: anywhere;
}
.mp__pair {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-5);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.mp__pair dt {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.mp__pair dd {
  margin: 0;
}
.mp__ok {
  display: inline-block;
  vertical-align: -2px;
  color: var(--status-accepted);
}
.mp__notes {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
}
</style>
