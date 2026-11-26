<script setup lang="ts">
// «Спросить менеджера»: на стенде настоящей отправки нет — пишется событие
// manager_request, и только после ответа сервера диалог говорит «Запрос передан».
import { ref, watch } from 'vue'
import { CircleAlert, CircleCheck, RotateCcw } from 'lucide-vue-next'
import UiDialog from '@/components/ui/UiDialog.vue'

const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ query: string; send: () => Promise<unknown> }>()

const status = ref<'sending' | 'sent' | 'error'>('sending')

async function run() {
  status.value = 'sending'
  try {
    await props.send()
    status.value = 'sent'
  } catch {
    status.value = 'error'
  }
}

watch(open, (v) => {
  if (v) void run()
})
</script>

<template>
  <UiDialog v-model:open="open" title="Спросить менеджера">
    <div class="manager" aria-live="polite">
      <p v-if="status === 'sending'" class="t-muted">Передаём запрос…</p>
      <template v-else-if="status === 'sent'">
        <p class="manager__line">
          <CircleCheck :size="18" aria-hidden="true" class="manager__icon" />
          <span
            ><strong>Запрос передан.</strong> Менеджер подберёт решение по запросу «{{
              query
            }}».</span
          >
        </p>
        <p class="t-sm t-muted">
          Это демонстрационный стенд: письмо не уходит, запрос записан событием и попадёт в разбор
          SearchOps как неудачный поиск.
        </p>
      </template>
      <template v-else>
        <p class="manager__line">
          <CircleAlert :size="18" aria-hidden="true" class="manager__icon manager__icon--error" />
          <span>Не удалось передать запрос. Проверьте соединение и повторите.</span>
        </p>
      </template>
    </div>
    <template #footer>
      <button v-if="status === 'error'" type="button" class="btn btn--secondary" @click="run">
        <RotateCcw :size="16" aria-hidden="true" />
        Повторить
      </button>
      <button type="button" class="btn btn--primary" @click="open = false">Понятно</button>
    </template>
  </UiDialog>
</template>

<style scoped>
.manager {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.manager__line {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}
.manager__icon {
  flex: none;
  margin-top: 1px;
  color: var(--text-2);
}
.manager__icon--error {
  color: var(--danger);
}
</style>
