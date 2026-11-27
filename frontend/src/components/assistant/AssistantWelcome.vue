<script setup lang="ts">
// Пустая панель (CHAT.md §2): приветствие и четыре стартовые подсказки по контексту — сразу отправляются.
import { computed } from 'vue'
import { starterPrompts } from '@/lib/assistant'

const props = defineProps<{ context: string }>()
defineEmits<{ pick: [message: string, label: string] }>()
const prompts = computed(() => starterPrompts(props.context))
</script>

<template>
  <div class="as-hello" data-testid="assistant-welcome">
    <h3 class="as-hello__title">Подберём подходящую лицензию</h3>
    <p class="as-hello__text">
      Опишите задачу. Помогу сравнить продукты, выбрать редакцию и рассчитать стоимость.
    </p>
    <p class="as-hello__label">{{ context ? 'По вашему запросу' : 'Можно начать с этого' }}</p>
    <ul class="as-hello__list" data-testid="assistant-starters">
      <li v-for="p in prompts" :key="p.label">
        <button type="button" class="as-hello__btn" @click="$emit('pick', p.message, p.label)">
          {{ p.label }}
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.as-hello {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.as-hello__title {
  margin: 0;
  font-size: 16px;
  line-height: 22px;
  font-weight: var(--fw-semibold);
}
.as-hello__text {
  margin: 0;
  color: var(--text-2);
  font-size: 14px;
  line-height: 21px;
}
.as-hello__label {
  margin: var(--space-3) 0 0;
  color: var(--text-3);
  font-size: 12px;
  line-height: 16px;
}
.as-hello__list {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.as-hello__btn {
  min-height: 36px;
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  text-align: left;
  cursor: pointer;
}
.as-hello__btn:hover {
  border-color: var(--accent);
  color: var(--accent);
}
@media (max-width: 1023px) {
  .as-hello__btn {
    min-height: var(--touch-min);
  }
}
</style>
