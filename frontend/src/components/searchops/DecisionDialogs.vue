<script setup lang="ts">
// Решение по карточке очереди (контур доводки): «Утвердить» правку поиска — подтверждение с номером будущей
// версии; «Отправить» задачу владельцу; «Отклонить с причиной» — быстрые причины (reka ToggleGroup) и текст. Причина уходит в память агента.
// Ошибка сервера показана в диалоге, статус предложения не меняется.
import { computed, ref, watch } from 'vue'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { CircleAlert } from 'lucide-vue-next'
import { approveProposal, rejectProposal } from '@/api/agent'
import { postEvent } from '@/api/events'
import type { ApproveResponse, Proposal, ProposalRejectReason } from '@/api/types'
import UiDialog from '@/components/ui/UiDialog.vue'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { REJECT_REASONS, isTicket } from '@/lib/proposals'
import { buildEvent } from '@/lib/events'
import { causeView } from '@/lib/causes'
import { isRelationPatch, proposalSubject, relationIds } from '@/lib/relationProposal'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  /** Предложение и что с ним делаем; null — диалоги закрыты. */
  target: { proposal: Proposal; mode: 'accept' | 'reject' } | null
  /** Действующая версия правил: новая будет N+1. */
  activeVersion: number | null
}>()
const emit = defineEmits<{
  close: []
  accepted: [p: Proposal, res: ApproveResponse]
  rejected: [p: Proposal]
  /** 409 при «Принять»: правила сменились и перепроверка не прошла — в карточке появился evidence.recheck. */
  conflict: [p: Proposal]
}>()

const busy = ref(false)
const err = ref('')
const conflict = ref(false)
const reason = ref<ProposalRejectReason | ''>('')
const comment = ref('')

watch(
  () => props.target,
  () => {
    busy.value = false
    err.value = ''
    conflict.value = false
    reason.value = ''
    comment.value = ''
  },
)

const acceptOpen = computed({
  get: () => props.target?.mode === 'accept',
  set: (v) => {
    if (!v && !busy.value) emit('close')
  },
})
const rejectOpen = computed({
  get: () => props.target?.mode === 'reject',
  set: (v) => {
    if (!v && !busy.value) emit('close')
  },
})

const cause = computed(() => (props.target ? causeView(props.target.proposal) : null))
// Комплект, как задача, берётся в работу: справочник и выдача не меняются.
const ticket = computed(
  () => !!props.target && (isTicket(props.target.proposal) || cause.value?.kind === 'owner_task'),
)
const relation = computed(() => !!props.target && isRelationPatch(props.target.proposal))
const app = useAppStore()

/** relation_decision: по событию на каждую связь правки. */
function logRelationDecision(p: Proposal, decision: string, reason = '') {
  if (!isRelationPatch(p)) return
  for (const id of relationIds(p))
    void postEvent(
      buildEvent(
        'relation_decision',
        { requestId: null, sessionId: app.sessionId, clientId: null },
        { payload: { relation_id: id, decision, reason, proposal_id: p.id } },
      ),
    ).catch(() => undefined)
}
const nextVersion = computed(() => (props.activeVersion ? props.activeVersion + 1 : null))
const canReject = computed(
  () => !!reason.value && (reason.value !== 'other' || comment.value.trim().length > 0),
)

function message(e: unknown): string {
  return isApiError(e) ? e.message : MSG_UNKNOWN
}

async function accept() {
  const p = props.target?.proposal
  if (!p) return
  busy.value = true
  err.value = ''
  conflict.value = false
  try {
    const res = await approveProposal(p.id)
    busy.value = false
    logRelationDecision(p, 'accepted')
    emit('accepted', p, res)
  } catch (e) {
    busy.value = false
    err.value = message(e)
    conflict.value = isApiError(e) && e.status === 409
    if (conflict.value) emit('conflict', p)
  }
}

async function reject() {
  const p = props.target?.proposal
  if (!p || !reason.value) return
  busy.value = true
  err.value = ''
  try {
    await rejectProposal(p.id, reason.value, comment.value.trim())
    busy.value = false
    logRelationDecision(p, 'rejected', reason.value)
    emit('rejected', p)
  } catch (e) {
    busy.value = false
    err.value = message(e)
  }
}
</script>

<template>
  <UiDialog
    v-model:open="acceptOpen"
    :title="
      ticket
        ? 'Отправить задачу владельцу?'
        : relation
          ? 'Утвердить правку справочника связей?'
          : 'Утвердить правку поиска?'
    "
  >
    <div class="decision">
      <p v-if="target">{{ proposalSubject(target.proposal) }}.</p>
      <p v-if="ticket">
        Правила поиска не меняются: задача уйдёт владельцу причины — {{ cause?.owner }}.
      </p>
      <p v-else-if="relation">
        Связь начнёт действовать в блоке «Дополните решение» сразу. Будет создана новая версия
        справочника связей, её можно откатить в «Изменениях» → «Связи».
      </p>
      <p v-else>
        Правило начнёт действовать сразу.
        <template v-if="nextVersion">Будет создана версия {{ nextVersion }}</template>
        <template v-else>Будет создана новая версия правил</template>, её можно откатить.
      </p>
      <p v-if="err && conflict" class="decision__err" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        <span>{{ err }}. Результат перепроверки — в карточке предложения.</span>
      </p>
      <p v-else-if="err" class="decision__err" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        Правка не применилась: {{ err }}. Статус предложения не изменился.
      </p>
    </div>
    <template #footer>
      <button type="button" class="btn btn--secondary" :disabled="busy" @click="emit('close')">
        {{ conflict ? 'Закрыть' : 'Отмена' }}
      </button>
      <button
        v-if="!conflict"
        type="button"
        class="btn btn--primary"
        :disabled="busy"
        @click="accept"
      >
        {{ busy ? 'Применяем…' : ticket ? 'Отправить' : 'Утвердить' }}
      </button>
    </template>
  </UiDialog>

  <UiDialog v-model:open="rejectOpen" title="Отклонить предложение?">
    <form id="reject-form" class="decision" @submit.prevent="reject">
      <p v-if="target">
        {{ proposalSubject(target.proposal) }}. Причина уйдёт в память агента: похожее он больше не
        предложит.
      </p>
      <ToggleGroupRoot
        v-model="reason"
        type="single"
        class="decision__reasons"
        aria-label="Причина отклонения"
      >
        <ToggleGroupItem
          v-for="r in REJECT_REASONS"
          :key="r.id"
          :value="r.id"
          class="decision__reason"
        >
          {{ r.label }}
        </ToggleGroupItem>
      </ToggleGroupRoot>
      <label class="decision__field">
        <span class="t-xs t-muted">
          Комментарий{{ reason === 'other' ? ' (обязателен для «Другое»)' : ', если нужно' }}
        </span>
        <textarea v-model="comment" rows="3" class="decision__text" maxlength="500" />
      </label>
      <p v-if="err" class="decision__err" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        Не удалось отклонить: {{ err }}
      </p>
    </form>
    <template #footer>
      <button type="button" class="btn btn--secondary" :disabled="busy" @click="emit('close')">
        Отмена
      </button>
      <button
        type="submit"
        form="reject-form"
        class="btn btn--primary"
        :disabled="busy || !canReject"
      >
        {{ busy ? 'Отклоняем…' : 'Отклонить' }}
      </button>
    </template>
  </UiDialog>
</template>

<style scoped>
.decision {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.decision__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.decision__err svg {
  flex: none;
  margin-top: 1px;
}
.decision__reasons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.decision__reason {
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font-weight: var(--fw-medium);
  cursor: pointer;
}
.decision__reason:hover {
  background: var(--surface-2);
}
.decision__reason[data-state='on'] {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--on-accent);
}
.decision__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.decision__text {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
</style>
