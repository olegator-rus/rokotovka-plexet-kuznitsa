<script setup lang="ts">
// Строка-приглашение над выдачей (UI 4.1): 72–96 px, без иллюстраций, значка робота и букв «ИИ». Если подбор уже
// был и свёрнут — та же строка возвращает к нему, условия сохранены.
defineProps<{ resume?: string }>()
defineEmits<{ start: [] }>()
</script>

<template>
  <div class="ghint" data-testid="guide-hint">
    <div class="ghint__text">
      <template v-if="resume">
        <p class="ghint__title">Подбор лицензии</p>
        <p class="ghint__sub">{{ resume }} · условия сохранены</p>
      </template>
      <template v-else>
        <p class="ghint__title">Не знаете, какую лицензию выбрать?</p>
        <p class="ghint__sub">Сравните редакции и рассчитайте стоимость</p>
      </template>
    </div>
    <button
      type="button"
      class="btn btn--primary"
      data-testid="guide-start"
      @click="$emit('start')"
    >
      {{ resume ? 'Открыть подбор' : 'Помочь выбрать' }}
    </button>
  </div>
</template>

<style scoped>
.ghint {
  box-sizing: border-box;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3) var(--space-4);
  min-height: 4.5rem;
  max-height: 6rem;
  padding: var(--space-3) var(--space-4) var(--space-3) var(--space-5);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface-2);
}
.ghint__text {
  flex: 1 1 14rem;
  min-width: 0;
}
.ghint__title {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.ghint__sub {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}
@media (max-width: 40rem) {
  .ghint {
    max-height: none;
  }
}
</style>
