<script setup lang="ts">
// Полоса разбора ТЗ и ошибки загрузки (TZ-UI.md §3, §10): сразу под строкой поиска, не модальное окно. В полосе —
// имя файла, настоящие этапы сервера (без процентов; «Распознаю текст» — только для скана) и «Отменить». Ошибка
// стоит здесь же и сама не исчезает: закрыть её можно крестиком или новой загрузкой.
import { computed } from 'vue'
import { CircleAlert, FileText, LoaderCircle, Check, X } from 'lucide-vue-next'
import { TZ_ERROR_TEXT, TZ_TEXT, errorAction, stageSteps } from '@/lib/tz'
import { useTzStore } from '@/stores/tz'

const emit = defineEmits<{ regular: [] }>()
const tz = useTzStore()

const steps = computed(() => stageSteps(tz.step, tz.sawOcr))
const busy = computed(() => tz.phase === 'processing' || tz.phase === 'searching')
const action = computed(() => (tz.error ? errorAction(tz.error) : null))
const activeLabel = computed(() => steps.value.find((s) => s.state === 'active')?.label ?? '')

function onAction() {
  const a = action.value
  if (!a) return
  if (a.kind === 'search') {
    tz.clearError()
    emit('regular')
    return
  }
  if (tz.error === 'general') tz.retry()
  else tz.openPicker()
}
</script>

<template>
  <div v-if="busy || tz.error" class="tzs">
    <div
      v-if="busy"
      class="tzs__bar"
      data-testid="tz-processing"
      :data-step="tz.step"
      role="group"
      aria-label="Разбор технического задания"
    >
      <FileText :size="18" aria-hidden="true" class="tzs__file-icon" />
      <span class="tzs__file" :title="tz.fileName" data-testid="tz-processing-file">{{
        tz.fileName
      }}</span>
      <ol class="tzs__steps" aria-label="Этапы разбора">
        <li
          v-for="s in steps"
          :key="s.key"
          :class="['tzs__step', `tzs__step--${s.state}`]"
          :data-testid="`tz-step-${s.key}`"
          :data-state="s.state"
          :aria-current="s.state === 'active' ? 'step' : undefined"
        >
          <LoaderCircle
            v-if="s.state === 'active'"
            :size="14"
            aria-hidden="true"
            class="tzs__spin"
          />
          <Check v-else-if="s.state === 'done'" :size="14" aria-hidden="true" />
          <span v-else class="tzs__dot" aria-hidden="true" />
          <span>{{ s.label }}</span>
        </li>
      </ol>
      <span class="sr-only" role="status">{{ activeLabel }}</span>
      <button
        v-if="tz.phase === 'processing'"
        type="button"
        class="btn btn--ghost btn--sm tzs__cancel"
        data-testid="tz-cancel"
        @click="tz.cancel()"
      >
        {{ TZ_TEXT.cancel }}
      </button>
    </div>
    <p v-if="busy && tz.step === 'ocr'" class="tzs__note t-sm" data-testid="tz-long-ocr">
      {{ TZ_TEXT.longOcr }}
    </p>

    <div
      v-if="tz.error"
      class="tzs__error"
      role="alert"
      data-testid="tz-upload-error"
      :data-error="tz.error"
      :data-place="tz.errorPlace"
    >
      <CircleAlert :size="18" aria-hidden="true" class="tzs__error-icon" />
      <p class="tzs__error-text">{{ TZ_ERROR_TEXT[tz.error] }}</p>
      <div class="tzs__error-actions">
        <button
          v-if="action"
          type="button"
          class="btn btn--secondary btn--sm"
          data-testid="tz-error-action"
          @click="onAction"
        >
          {{ action.label }}
        </button>
        <button
          type="button"
          class="btn btn--ghost btn--icon btn--sm tzs__close"
          aria-label="Закрыть сообщение"
          @click="tz.clearError()"
        >
          <X :size="16" aria-hidden="true" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tzs {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-2);
  /* Полоса может стоять на красной полосе строки поиска: внутри — обычные цвета темы, как у строки. */
  --text: var(--field-text);
  --text-2: var(--field-text-2);
  --surface: var(--field-surface);
  --border: var(--field-border);
  --border-control: var(--field-border-control);
  --focus-ring: var(--field-text);
  color: var(--text);
}
.tzs__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  box-sizing: border-box;
  min-height: 48px;
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tzs__file-icon {
  flex: none;
  color: var(--text-2);
}
.tzs__file {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 18rem;
  overflow: hidden;
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tzs__steps {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
}
.tzs__step {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
}
.tzs__step--active {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.tzs__step--done {
  color: var(--status-accepted);
}
.tzs__dot {
  width: 8px;
  height: 8px;
  margin: 0 3px;
  border: 1.5px solid var(--border-control);
  border-radius: 50%;
}
.tzs__spin {
  animation: tzs-spin 0.9s linear infinite;
}
@keyframes tzs-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .tzs__spin {
    animation: none;
  }
}
.tzs__cancel {
  margin-left: auto;
}
.tzs__note {
  margin: 0;
  color: var(--text-2);
}
.tzs__error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  border: var(--border-w) solid var(--danger);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tzs__error-icon {
  flex: none;
  color: var(--danger);
}
.tzs__error-text {
  flex: 1 1 14rem;
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}
.tzs__error-actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  margin-left: auto;
}
@media (max-width: 767px) {
  .tzs__file {
    max-width: calc(100% - 2rem);
  }
  .tzs__close {
    min-width: 44px;
    min-height: 44px;
  }
}
</style>
