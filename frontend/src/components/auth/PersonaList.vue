<script setup lang="ts">
// Демо-аккаунты одной кнопкой: покупатели и сотрудники отдельными группами, в строке — инициалы, имя,
// роль, что покажет вход; у сотрудника — сколько предложений ждёт его решения (число с сервера). Общая для экрана
// входа и смены аккаунта из шапки.
import { computed } from 'vue'
import { ArrowRight, Check, LoaderCircle } from 'lucide-vue-next'
import type { Persona } from '@/api/auth'
import { plural } from '@/lib/format'

const props = defineProps<{
  personas: Persona[]
  /** Аккаунт, в который сейчас входим. */
  busy?: string | null
  /** Все кнопки неактивны (идёт вход, отсчёт 429). */
  locked?: boolean
  /** Текущий аккаунт (смена из шапки): отмечен и не нажимается. */
  current?: string | null
  /** Уровень заголовков групп. */
  headingLevel?: 'h2' | 'h3'
}>()
defineEmits<{ pick: [p: Persona] }>()

const groups = computed(() =>
  [
    { key: 'buyers', title: 'Покупатели', items: props.personas.filter((p) => p.role === 'buyer') },
    {
      key: 'staff',
      title: 'Сотрудники магазина',
      items: props.personas.filter((p) => p.role !== 'buyer'),
    },
  ].filter((g) => g.items.length > 0),
)

/** Покупатель — клиент и кто он; сотрудник — роль (должность совпадает с ролью). */
function roleLine(p: Persona): string {
  if (p.role === 'buyer') return [p.client_name, p.title].filter(Boolean).join(' · ')
  return p.title && p.title !== p.role_label ? `${p.role_label} · ${p.title}` : p.role_label
}

function pendingLine(p: Persona): string | null {
  if (p.pending === null || p.role === 'buyer') return null
  if (p.pending === 0) return 'очередь агента пуста'
  return `ждут решения: ${p.pending} ${plural(p.pending, ['предложение', 'предложения', 'предложений'])}`
}
</script>

<template>
  <div class="personas">
    <section v-for="g in groups" :key="g.key" class="personas__group" :aria-label="g.title">
      <component :is="headingLevel ?? 'h3'" class="personas__title">{{ g.title }}</component>
      <ul class="personas__list">
        <li v-for="p in g.items" :key="p.id">
          <button
            type="button"
            class="persona"
            :data-persona="p.id"
            :aria-label="`Войти как ${p.name}, ${p.role_label.toLowerCase()}`"
            :aria-busy="busy === p.id || undefined"
            :aria-disabled="locked || current === p.id || undefined"
            :aria-current="current === p.id || undefined"
            @click="!locked && current !== p.id && $emit('pick', p)"
          >
            <span class="persona__avatar" aria-hidden="true">{{ p.initials }}</span>
            <span class="persona__text">
              <span class="persona__name">{{ p.name }}</span>
              <span class="persona__role t-sm">{{ roleLine(p) }}</span>
              <span class="persona__note t-xs">{{ p.note }}</span>
              <span v-if="pendingLine(p)" class="persona__pending t-xs">{{ pendingLine(p) }}</span>
            </span>
            <LoaderCircle
              v-if="busy === p.id"
              class="persona__icon persona__spin"
              :size="18"
              aria-hidden="true"
            />
            <Check
              v-else-if="current === p.id"
              class="persona__icon"
              :size="18"
              aria-hidden="true"
            />
            <ArrowRight v-else class="persona__icon" :size="18" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.personas {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.personas__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.personas__title {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.personas__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.persona {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  min-height: 3.5rem;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    border-color var(--dur-fast) var(--ease-standard),
    background-color var(--dur-fast) var(--ease-standard);
}
.persona:hover:not([aria-disabled='true']),
.persona[aria-busy='true'] {
  border-color: var(--border-control);
  background: var(--surface-2);
}
.persona:hover:not([aria-disabled='true']) .persona__icon {
  color: var(--text);
}
.persona[aria-disabled='true'] {
  cursor: not-allowed;
}
.persona[aria-disabled='true']:not([aria-current='true']):not([aria-busy='true']) {
  opacity: 0.6;
}
.persona[aria-current='true'] {
  border-color: var(--border-control);
  background: var(--surface-3);
}
.persona__avatar {
  display: grid;
  flex: none;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: 50%;
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  font-weight: var(--fw-semibold);
  letter-spacing: 0.02em;
}
.persona__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}
.persona__name {
  font-weight: var(--fw-semibold);
}
.persona__role {
  color: var(--text-2);
}
.persona__note {
  color: var(--text-3);
}
.persona__pending {
  color: var(--text-2);
}
.persona__icon {
  flex: none;
  color: var(--text-3);
}
.persona__spin {
  animation: persona-spin 0.9s linear infinite;
}
@keyframes persona-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
