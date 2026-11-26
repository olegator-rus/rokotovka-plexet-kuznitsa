<script setup lang="ts">
// Ответ справки (контур доводки): answer — над выдачей вместо товаров, заголовок,
// 2–3 строки и ссылка на раздел помощи; hint — одна строка рядом с товаром («Как продлить в кабинете»).
import { CircleHelp } from 'lucide-vue-next'
import type { HelpAnswer } from '@/api/types'

defineProps<{ help: HelpAnswer }>()
const emit = defineEmits<{ open: [help: HelpAnswer] }>()
</script>

<template>
  <section
    v-if="help.mode === 'answer'"
    class="panel help-answer"
    aria-labelledby="help-answer-title"
    data-testid="help-answer"
  >
    <h2 id="help-answer-title" class="t-h2 help-answer__title">
      <CircleHelp :size="18" aria-hidden="true" />
      {{ help.title }}
    </h2>
    <p class="help-answer__text">{{ help.text }}</p>
    <p>
      <RouterLink :to="help.url" class="help-answer__link" @click="emit('open', help)">
        {{ help.link_label }}
      </RouterLink>
    </p>
    <p class="t-xs t-muted">Это вопрос о покупке, а не товар — поэтому товаров в ответе нет.</p>
  </section>
  <p v-else class="help-hint t-sm" data-testid="help-hint">
    <CircleHelp :size="16" aria-hidden="true" />
    <RouterLink :to="help.url" class="help-answer__link" @click="emit('open', help)">
      {{ help.link_label }}
    </RouterLink>
  </p>
</template>

<style scoped>
.help-answer {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-4) var(--space-5);
}
.help-answer__title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.help-answer__text {
  max-width: 64ch;
}
.help-answer__link {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.help-hint {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
}
</style>
