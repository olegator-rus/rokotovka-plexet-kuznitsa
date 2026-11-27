<script setup lang="ts">
// Вопрос подбора (UI 4.2): о количестве — поле со степпером и быстрыми значениями, иначе — варианты списком.
// Свой ответ — текстом в строке поиска. «Пока не знаю — показать варианты» не блокирует подбор.
import { computed, ref, watch } from 'vue'
import { Minus, Plus } from 'lucide-vue-next'
import type { GuideChipAnswer, GuideClarification } from '@/api/typesGuide'
import { chipView } from '@/lib/guide'
import { clampCount, countKind, QUICK_COUNTS } from '@/lib/guideView'

const props = defineProps<{ question: GuideClarification; busy?: boolean }>()
const emit = defineEmits<{ answer: [chip: GuideChipAnswer] }>()

const key = computed(() => props.question.key ?? '')
const isCount = computed(() => Boolean(countKind(key.value)))
const chips = computed(() => (props.question.chips ?? []).map((c) => chipView(c, key.value)))
/** «другое», «ничего из этого», «не знаю» — не вариант, а пропуск. */
const isSkip = (c: { value: string }) => c.value === '' || c.value === 'unknown'
const options = computed(() => chips.value.filter((c) => !isSkip(c)))
const skip = computed(() => {
  const s = chips.value.find((c) => c.value === 'unknown' || (!isCount.value && c.value === ''))
  if (s) return s
  return isCount.value ? { label: '', value: 'unknown', key: key.value } : null
})
const quick = computed(() => {
  const set = new Set<number>(QUICK_COUNTS)
  for (const c of options.value) {
    const n = clampCount(c.value)
    if (n && /^\d+$/.test(c.value)) set.add(n)
  }
  return [...set].sort((a, b) => a - b)
})
const label = (s: string) => {
  const t = s.replace(/^нужн[оа]:\s*/i, 'Нужно ').trim()
  return t ? t[0]!.toUpperCase() + t.slice(1) : t
}

const count = ref<number | null>(null)
watch(
  () => props.question,
  () => (count.value = null),
)
function step(d: number) {
  count.value = clampCount((count.value ?? 0) + d) ?? 1
}
function sendCount(n: number | null) {
  const v = clampCount(n)
  if (v) emit('answer', { key: key.value, value: String(v) })
}
</script>

<template>
  <section
    class="gq"
    role="group"
    aria-labelledby="guide-question-text"
    data-testid="guide-question"
    :data-key="key"
  >
    <h3 id="guide-question-text" class="gq__text">{{ question.question }}</h3>
    <p v-if="isCount" class="gq__hint">От количества зависят цена и набор доступных лицензий.</p>

    <template v-if="isCount">
      <form class="gq__count" @submit.prevent="sendCount(count)">
        <div class="gq__stepper">
          <button
            type="button"
            aria-label="Меньше"
            :disabled="busy || (count ?? 0) <= 1"
            @click="step(-1)"
          >
            <Minus :size="14" aria-hidden="true" />
          </button>
          <label for="gq-count" class="sr-only">{{ question.question }}</label>
          <input
            id="gq-count"
            v-model.number="count"
            class="t-num"
            inputmode="numeric"
            autocomplete="off"
            data-testid="guide-question-count"
          />
          <button type="button" aria-label="Больше" :disabled="busy" @click="step(1)">
            <Plus :size="14" aria-hidden="true" />
          </button>
        </div>
        <button
          type="submit"
          class="btn btn--secondary"
          :disabled="busy || !clampCount(count)"
          data-testid="guide-question-apply"
        >
          Применить
        </button>
      </form>
      <ul class="gq__quick" aria-label="Быстрый выбор">
        <li v-for="n in quick" :key="n">
          <button
            type="button"
            class="gq__chip t-num"
            :disabled="busy"
            data-testid="guide-quick"
            @click="sendCount(n)"
          >
            {{ n }}
          </button>
        </li>
      </ul>
    </template>

    <ul v-else class="gq__options">
      <li v-for="c in options" :key="c.value">
        <button
          type="button"
          class="gq__option"
          :disabled="busy"
          data-testid="guide-option"
          @click="emit('answer', { key: c.key, value: c.value })"
        >
          <span class="gq__radio" aria-hidden="true" />
          {{ label(c.label) }}
        </button>
      </li>
    </ul>

    <p class="gq__foot">
      <button
        v-if="skip"
        type="button"
        class="gq__skip"
        :disabled="busy"
        data-testid="guide-skip"
        @click="emit('answer', { key: skip.key || key, value: skip.value })"
      >
        Пока не знаю — показать варианты
      </button>
    </p>
  </section>
</template>

<style scoped>
.gq {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.gq__text {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.gq__hint {
  margin: calc(-1 * var(--space-2)) 0 0;
  color: var(--text-2);
}
.gq__count {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.gq__stepper {
  display: grid;
  grid-template-columns: 2.75rem 5rem 2.75rem;
  height: 2.75rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  overflow: hidden;
}
.gq__stepper button {
  display: grid;
  place-items: center;
  border: 0;
  background: var(--surface);
  color: var(--text);
  cursor: pointer;
}
.gq__stepper button:disabled {
  color: var(--text-3);
  cursor: default;
}
.gq__stepper input {
  min-width: 0;
  border: 0;
  border-inline: var(--border-w) solid var(--border);
  font: inherit;
  font-size: var(--fs-input);
  font-weight: var(--fw-semibold);
  text-align: center;
}
.gq__count .btn {
  height: 2.75rem;
}
.gq__quick {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gq__chip {
  min-width: 3rem;
  height: 2.5rem;
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-medium);
  cursor: pointer;
}
.gq__chip:hover:not(:disabled),
.gq__option:hover:not(:disabled) {
  border-color: var(--text);
}
.gq__options {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gq__option {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  min-height: 2.75rem;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.gq__radio {
  flex: none;
  width: 1rem;
  height: 1rem;
  border: 1.5px solid var(--border-control);
  border-radius: 50%;
}
.gq__foot {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
}
.gq__skip {
  align-self: flex-start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
</style>
