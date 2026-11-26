<script setup lang="ts">
// Строка понимания: плашка исправления, чипы распознанных условий, уточняющий
// вопрос. Пустая, если запрос понят буквально. Строгое условие — сплошная обводка, мягкое — пунктир,
// расшифровка в подсказке и в тексте для скринридера.
import { computed } from 'vue'
import { CircleHelp, SpellCheck, X } from 'lucide-vue-next'
import type { Chip, ChipRef, ClarifyOption, SearchResponse } from '@/api/types'
import UiTooltip from '@/components/ui/UiTooltip.vue'
import { chipLabel } from '@/lib/facets'

const props = defineProps<{
  response: SearchResponse
  asTyped: boolean
  removed: ChipRef[]
}>()

const emit = defineEmits<{
  asTyped: []
  tryQuery: [text: string]
  removeChip: [chip: Chip]
  restoreChips: []
  clarify: [attribute: string, option: ClarifyOption]
}>()

const correction = computed(() => {
  const c = props.response.correction
  if (!c || props.asTyped || c.from === c.to) return null
  return c
})
const chips = computed(() => props.response.chips ?? [])
const clarify = computed(() => {
  const c = props.response.clarify
  return c && c.options?.length ? c : null
})
const visible = computed(
  () =>
    Boolean(correction.value) ||
    chips.value.length > 0 ||
    Boolean(clarify.value) ||
    props.removed.length > 0 ||
    props.asTyped,
)
</script>

<template>
  <section v-if="visible" class="understanding" aria-label="Как понят запрос">
    <p v-if="correction && correction.applied" class="understanding__line">
      <SpellCheck :size="16" aria-hidden="true" class="understanding__icon" />
      <span>
        Показаны результаты по запросу <strong>{{ correction.to }}</strong>. Искать как введено:
        <button type="button" class="link" @click="emit('asTyped')">{{ correction.from }}</button>
      </span>
    </p>
    <p v-else-if="correction" class="understanding__line">
      <SpellCheck :size="16" aria-hidden="true" class="understanding__icon" />
      <span>
        Возможно, вы искали:
        <button type="button" class="link" @click="emit('tryQuery', correction.to)">
          {{ correction.to }}
        </button>
      </span>
    </p>
    <p v-else-if="asTyped" class="understanding__line t-muted">
      <SpellCheck :size="16" aria-hidden="true" class="understanding__icon" />
      <span>Показаны результаты по запросу как введено, без исправления</span>
    </p>

    <div v-if="chips.length || removed.length" class="understanding__chips">
      <span class="t-sm t-muted">Мы поняли запрос так:</span>
      <ul class="chips">
        <li v-for="chip in chips" :key="`${chip.field}:${chip.value}`">
          <span :class="['chip', chip.strict ? 'chip--strict' : 'chip--soft']">
            <UiTooltip :text="chip.strict ? 'Учтено строго' : 'Учтено по возможности'">
              <span class="chip__label" tabindex="0">
                {{ chipLabel(chip) }}
                <span class="sr-only">{{
                  chip.strict ? ', учтено строго' : ', по возможности'
                }}</span>
              </span>
            </UiTooltip>
            <button
              type="button"
              class="chip__remove"
              :aria-label="`Снять условие «${chipLabel(chip)}»`"
              @click="emit('removeChip', chip)"
            >
              <X :size="14" aria-hidden="true" />
            </button>
          </span>
        </li>
      </ul>
      <span v-if="removed.length" class="t-sm t-muted">
        Снято условий: {{ removed.length }} ·
        <button type="button" class="link" @click="emit('restoreChips')">вернуть</button>
      </span>
    </div>

    <div v-if="clarify" class="understanding__clarify" role="group" :aria-label="clarify.question">
      <CircleHelp :size="16" aria-hidden="true" class="understanding__icon" />
      <span class="understanding__question">{{ clarify.question }}</span>
      <span class="understanding__options">
        <button
          v-for="o in clarify.options"
          :key="o.value"
          type="button"
          class="btn btn--secondary btn--sm"
          @click="emit('clarify', clarify.attribute, o)"
        >
          {{ o.label }}
        </button>
      </span>
    </div>
  </section>
</template>

<style scoped>
.understanding {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.understanding__line {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}
.understanding__icon {
  flex: none;
  margin-top: 2px;
  color: var(--text-2);
}
.understanding__chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.chip {
  display: inline-flex;
  align-items: center;
  min-height: var(--control-h-sm);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.chip--strict {
  border: var(--border-w) solid var(--border-control);
}
.chip--soft {
  border: var(--border-w) dashed var(--border-control);
  color: var(--text-2);
}
.chip__label {
  padding: 0 var(--space-1) 0 var(--space-3);
  border-radius: var(--radius-pill);
}
.chip__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
}
.chip__remove:hover {
  color: var(--text);
  background: var(--surface-3);
}
.understanding__clarify {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.understanding__question {
  font-weight: var(--fw-medium);
}
.understanding__options {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
</style>
