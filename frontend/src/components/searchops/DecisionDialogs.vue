<script setup lang="ts">
// Решение по предложению: «Принять» / «Взять в работу» — подтверждение с номером будущей
// версии; «Отклонить» — быстрые причины (reka ToggleGroup) и текст. Причина уходит в память агента.
// Ошибка сервера показана в диалоге, статус предложения не меняется.
import { computed, ref, watch } from 'vue'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { CircleAlert } from 'lucide-vue-next'
import { approveProposal, rejectProposal } from '@/api/agent'
import type { ApproveResponse, Proposal, ProposalRejectReason } from '@/api/types'
import UiDialog from '@/components/ui/UiDialog.vue'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { REJECT_REASONS, isTicket } from '@/lib/proposals'

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
}>()

const busy = ref(false)
const err = ref('')
const reason = ref<ProposalRejectReason | ''>('')
const comment = ref('')

watch(
  () => props.target,
  () => {
    busy.value = false
    err.value = ''
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

const ticket = computed(() => !!props.target && isTicket(props.target.proposal))
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
  try {
    const res = await approveProposal(p.id)
    busy.value = false
    emit('accepted', p, res)
  } catch (e) {
    busy.value = false
    err.value = message(e)
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
    emit('rejected', p)
  } catch (e) {
    busy.value = false
    err.value = message(e)
  }
}
</script>

<template>
  <UiDialog v-model:open="acceptOpen" :title="ticket ? 'Взять в работу?' : 'Принять правку?'">
    <div class="decision">
      <p v-if="target">Кластер «{{ target.proposal.cluster?.label }}».</p>
      <p v-if="ticket">
        Правила поиска не меняются: предложение станет задачей ответственному за ассортимент или
        карточку.
      </p>
      <p v-else>
        Правило начнёт действовать сразу.
        <template v-if="nextVersion">Будет создана версия {{ nextVersion }}</template>
        <template v-else>Будет создана новая версия правил</template>, её можно откатить.
      </p>
      <p v-if="err" class="decision__err" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        Правка не применилась: {{ err }}. Статус предложения не изменился.
      </p>
    </div>
    <template #footer>
      <button type="button" class="btn btn--secondary" :disabled="busy" @click="emit('close')">
        Отмена
      </button>
      <button type="button" class="btn btn--primary" :disabled="busy" @click="accept">
        {{ busy ? 'Применяем…' : ticket ? 'Взять в работу' : 'Принять' }}
      </button>
    </template>
  </UiDialog>

  <UiDialog v-model:open="rejectOpen" title="Отклонить предложение?">
    <form id="reject-form" class="decision" @submit.prevent="reject">
      <p v-if="target">
        Кластер «{{ target.proposal.cluster?.label }}». Причина уйдёт в память агента: похожее он
        больше не предложит.
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
