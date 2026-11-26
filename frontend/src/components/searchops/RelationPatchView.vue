<script setup lang="ts">
// Тело карточки `relation_patch`: что сейчас и что предлагается (было/стало связи),
// «Доказательства» с уровнем словом, прогон блока, два раздельных статуса — техническая корректность и эффект.
import { computed } from 'vue'
import {
  CircleCheck,
  CircleHelp,
  CircleX,
  ExternalLink,
  Minus,
  Pencil,
  Plus,
} from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import type { CommerceProposal } from '@/api/typesCommerce'
import { relationProposalView } from '@/lib/relationProposal'

const props = defineProps<{ proposal: Proposal }>()
const v = computed(() => relationProposalView(props.proposal as CommerceProposal))
const icon = (op: string) => (op === 'add' ? Plus : op === 'remove' ? Minus : Pencil)
const word = (op: string) => (op === 'add' ? 'добавить' : op === 'remove' ? 'удалить' : 'изменить')
const statusIcon = (ok: boolean | null) =>
  ok === true ? CircleCheck : ok === false ? CircleX : CircleHelp
const tone = (ok: boolean | null) =>
  ok === true ? 'status-accepted' : ok === false ? 'status-rejected' : 'status-pending'
</script>

<template>
  <section class="rp__block" aria-label="Что предлагается">
    <p class="rp__label">Что сейчас и что предлагается</p>
    <ul v-if="v.diff.length" class="rp__diff">
      <li v-for="d in v.diff" :key="`${d.op}-${d.id}`" class="rp__line">
        <component :is="icon(d.op)" :size="14" aria-hidden="true" />
        <span class="rp__title">
          <span class="sr-only">{{ word(d.op) }}:</span>{{ d.title }}
        </span>
        <span class="rp__ba t-sm t-muted">было: {{ d.before ?? 'связи нет' }}</span>
        <span class="rp__ba t-sm">стало: {{ d.after ?? 'связь удалена' }}</span>
      </li>
    </ul>
    <p v-else class="t-sm t-muted">Справочник связей не меняется</p>
  </section>

  <section class="rp__block" aria-label="Доказательства">
    <p class="rp__label">Доказательства</p>
    <p v-if="v.evidenceLevel" class="t-sm">
      Уровень: <strong>{{ v.evidenceLevel }}</strong>
      <template v-if="v.strength"> · {{ v.strength }}</template>
    </p>
    <ul v-if="v.basis.length" class="rp__basis">
      <li v-for="(b, i) in v.basis" :key="i" class="t-sm">
        {{ b.text }}
        <span v-if="b.evidence_level" class="t-xs t-muted"> · {{ b.evidence_level }}</span>
        <a
          v-if="b.source_url"
          :href="b.source_url"
          target="_blank"
          rel="noopener noreferrer"
          class="link t-xs"
          >основание <ExternalLink :size="12" aria-hidden="true"
        /></a>
      </li>
    </ul>
    <p v-else-if="!v.evidenceLevel" class="t-sm t-muted">Агент не приложил основание</p>
  </section>

  <section
    v-if="v.evalLines.length || v.passed !== null"
    class="rp__block"
    aria-label="Результат прогона"
  >
    <p class="rp__label">Результат прогона в песочнице</p>
    <ul v-if="v.evalLines.length" class="rp__basis rp__eval">
      <li v-for="c in v.evalLines" :key="c" class="t-sm">{{ c }}</li>
    </ul>
    <p v-if="v.passed !== null" :class="['rp__status', tone(v.passed)]">
      <component :is="statusIcon(v.passed)" :size="14" aria-hidden="true" />
      {{ v.passed ? 'Условия допуска выполнены' : 'Условия допуска не выполнены' }}
    </p>
    <ul v-if="v.reasons.length" class="rp__basis">
      <li v-for="r in v.reasons" :key="r" class="t-xs t-muted">{{ r }}</li>
    </ul>
  </section>

  <ul class="rp__statuses" aria-label="Статусы предложения">
    <li v-if="v.technical" :class="['rp__status', tone(v.technical.ok)]">
      <component :is="statusIcon(v.technical.ok)" :size="14" aria-hidden="true" />
      {{ v.technical.label }}
    </li>
    <li :class="['rp__status', tone(v.economic.ok)]">
      <component :is="statusIcon(v.economic.ok)" :size="14" aria-hidden="true" />
      {{ v.economic.label }}
    </li>
    <li v-if="v.priority" class="t-xs t-muted">{{ v.priority }}</li>
    <li v-for="n in v.notes" :key="n" class="t-xs t-muted">{{ n }}</li>
  </ul>
</template>

<style scoped>
.rp__block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.rp__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.rp__diff,
.rp__basis,
.rp__statuses {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.rp__line {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr);
  gap: 0 var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.rp__line > svg {
  align-self: center;
  color: var(--text-2);
}
.rp__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.rp__ba {
  grid-column: 2;
  overflow-wrap: anywhere;
}
.rp__eval {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.rp__status {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--fs-sm);
  color: var(--text);
}
.rp__status svg {
  flex: none;
  color: var(--tone);
}
</style>
