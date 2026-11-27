<script setup lang="ts">
// Шапка (CONSULTANT.md §3): значок, «Консультант · Виртуальный», спецификация со счётчиком, развернуть, меню
// с «Новым разговором», свернуть. «Онлайн», зелёной точки и фото нет.
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui'
import {
  ClipboardList,
  Maximize2,
  MessageSquareText,
  Minimize2,
  Minus,
  MoreHorizontal,
  RotateCcw,
} from 'lucide-vue-next'

defineProps<{ specCount: number; expanded: boolean }>()
defineEmits<{ spec: []; expand: []; reset: []; close: [] }>()
</script>

<template>
  <header class="ch">
    <span class="ch__avatar" aria-hidden="true"><MessageSquareText :size="18" /></span>
    <div class="ch__name">
      <h2 id="consultant-title" class="ch__title">Консультант</h2>
      <span class="ch__sub">Виртуальный</span>
    </div>
    <button
      type="button"
      class="ch__btn ch__spec"
      :aria-label="`Спецификация: ${specCount}`"
      title="Открыть спецификацию"
      data-testid="consultant-spec"
      @click="$emit('spec')"
    >
      <ClipboardList :size="18" aria-hidden="true" />
      <span class="ch__count t-num" data-testid="consultant-spec-count">{{ specCount }}</span>
    </button>
    <button
      type="button"
      class="ch__btn ch__expand"
      :aria-label="expanded ? 'Уменьшить окно' : 'Развернуть окно'"
      :title="expanded ? 'Уменьшить окно' : 'Развернуть окно'"
      data-testid="consultant-expand"
      @click="$emit('expand')"
    >
      <Minimize2 v-if="expanded" :size="18" aria-hidden="true" />
      <Maximize2 v-else :size="18" aria-hidden="true" />
    </button>
    <DropdownMenuRoot :modal="false">
      <DropdownMenuTrigger class="ch__btn" aria-label="Меню" data-testid="consultant-menu">
        <MoreHorizontal :size="18" aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent class="ch__menu" align="end" :side-offset="4">
          <DropdownMenuItem class="ch__item" data-testid="consultant-new" @select="$emit('reset')">
            <RotateCcw :size="16" aria-hidden="true" /> Новый разговор
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenuRoot>
    <button
      type="button"
      class="ch__btn"
      aria-label="Свернуть консультанта"
      title="Свернуть консультанта"
      data-testid="consultant-close"
      @click="$emit('close')"
    >
      <Minus :size="18" aria-hidden="true" />
    </button>
  </header>
</template>

<style scoped>
.ch {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--space-1);
  height: 4rem;
  padding: 0 var(--space-3) 0 var(--space-4);
  border-bottom: 1px solid var(--border);
}
.ch__avatar {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 2.25rem;
  height: 2.25rem;
  margin-right: var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.ch__name {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  line-height: 1.2;
}
.ch__title {
  margin: 0;
  font-size: 1rem;
  font-weight: var(--fw-semibold);
}
.ch__sub {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.ch__btn {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  min-width: 2.5rem;
  height: 2.5rem;
  padding: 0 var(--space-2);
  border: 0;
  border-radius: var(--radius-control);
  background: transparent;
  color: var(--text);
  cursor: pointer;
}
.ch__btn:hover {
  background: var(--surface-2);
}
.ch__btn:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: -2px;
}
.ch__count {
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
@media (max-width: 640px) {
  .ch {
    height: 3.5rem;
  }
  .ch__expand {
    display: none;
  }
  .ch__btn {
    min-width: 2.75rem;
    height: 2.75rem;
  }
}
</style>

<style>
.ch__menu {
  z-index: 60;
  min-width: 12rem;
  padding: var(--space-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
}
.ch__item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-size: var(--fs-body);
  cursor: pointer;
  outline: none;
}
.ch__item[data-highlighted] {
  background: var(--surface-2);
}
</style>
