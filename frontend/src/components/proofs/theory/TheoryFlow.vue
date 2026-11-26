<script setup lang="ts">
// Схема «шаги по порядку»: блоки сверху вниз со стрелками; с row — строкой на широком экране (от 1024 px).
// tone — маршрут A–D (цвет и буква) или muted (необязательный шаг); tag — короткая пометка справа.
import { ArrowDown, ArrowRight } from 'lucide-vue-next'

export interface FlowStep {
  title: string
  text?: string
  tone?: 'a' | 'b' | 'c' | 'd' | 'muted'
  tag?: string
}

const props = defineProps<{
  steps: readonly FlowStep[]
  label: string
  row?: boolean
  numbered?: boolean
}>()
</script>

<template>
  <figure class="flow" :class="{ 'flow--row': props.row }">
    <figcaption class="sr-only">{{ props.label }}</figcaption>
    <ol class="flow__list" role="list">
      <li v-for="(s, i) in props.steps" :key="s.title" class="flow__item">
        <div class="flow__step" :class="s.tone ? `flow__step--${s.tone}` : ''">
          <div class="flow__head">
            <span v-if="props.numbered" class="flow__num t-num" aria-hidden="true">{{
              i + 1
            }}</span>
            <strong class="flow__title">{{ s.title }}</strong>
            <span
              v-if="s.tag"
              :class="[
                'badge',
                s.tone && s.tone !== 'muted' ? ['badge--tone', `route-${s.tone}`] : '',
              ]"
              >{{ s.tag }}</span
            >
          </div>
          <p v-if="s.text" class="flow__text t-sm">{{ s.text }}</p>
        </div>
        <span v-if="i < props.steps.length - 1" class="flow__arrow" aria-hidden="true">
          <ArrowDown :size="16" class="flow__down" />
          <ArrowRight :size="16" class="flow__right" />
        </span>
      </li>
    </ol>
  </figure>
</template>

<style scoped>
.flow {
  margin: 0;
  min-width: 0;
}
.flow__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.flow__item {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  min-width: 0;
}
.flow__step {
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-left: 3px solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  min-width: 0;
}
.flow__step--a {
  border-left-color: var(--route-a);
}
.flow__step--b {
  border-left-color: var(--route-b);
}
.flow__step--c {
  border-left-color: var(--route-c);
}
.flow__step--d {
  border-left-color: var(--route-d);
  background: var(--route-d-bg);
}
.flow__step--muted {
  border-style: dashed;
  border-left-style: solid;
  border-left-color: var(--border);
  background: var(--surface-2);
}
.flow__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.flow__num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.375rem;
  height: 1.375rem;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
}
.flow__title {
  font-weight: var(--fw-semibold);
}
.flow__text {
  margin: var(--space-1) 0 0;
  color: var(--text-2);
}
.flow__arrow {
  display: flex;
  justify-content: center;
  padding: 2px 0;
  color: var(--text-3);
}
.flow__right {
  display: none;
}

@media (min-width: 1024px) {
  .flow--row .flow__list {
    flex-direction: row;
    align-items: stretch;
  }
  .flow--row .flow__item {
    flex: 1 1 0;
    flex-direction: row;
    align-items: center;
  }
  .flow--row .flow__step {
    flex: 1;
    align-self: stretch;
  }
  .flow--row .flow__arrow {
    padding: 0 2px;
  }
  .flow--row .flow__down {
    display: none;
  }
  .flow--row .flow__right {
    display: block;
  }
}
</style>
