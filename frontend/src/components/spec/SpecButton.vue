<script setup lang="ts">
// Кнопка «Спецификация» в шапке «Поиска» со счётчиком позиций («Новые постоянные
// элементы»): открывает панель справа.
import { ClipboardList } from 'lucide-vue-next'
import { useSpecStore } from '@/stores/spec'
import SpecPanel from './SpecPanel.vue'

const spec = useSpecStore()
</script>

<template>
  <button
    type="button"
    class="btn btn--secondary btn--sm spec-btn"
    :aria-label="`Спецификация, позиций: ${spec.count}`"
    @click="spec.open = true"
  >
    <ClipboardList :size="16" aria-hidden="true" />
    <span class="spec-btn__label">Спецификация</span>
    <span class="spec-btn__count t-num" aria-hidden="true">{{ spec.count }}</span>
  </button>
  <SpecPanel />
</template>

<style scoped>
.spec-btn__count {
  min-width: 1.25rem;
  padding: 0 var(--space-1);
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  line-height: 1.25rem;
  text-align: center;
}
@media (max-width: 479px) {
  .spec-btn__label {
    display: none;
  }
}
</style>
