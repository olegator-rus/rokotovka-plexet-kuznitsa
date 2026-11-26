<script setup lang="ts">
// Панель трассы: сводка, шаги сверху вниз, разложение скора выбранного товара,
// отсечённые товары с причиной, «Скопировать JSON». Без запроса — подсказка, при загрузке — скелет.
// Сотруднику — «Разобрать»: запуск агента на этом запросе (контур автономии, S4 кадр 3) и переход в ленту.
import { computed } from 'vue'
import { Copy, Microscope, X } from 'lucide-vue-next'
import type { Trace } from '@/api/types'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useToast } from '@/components/ui/useToast'
import { useAnalyseQuery } from '@/composables/useAnalyseQuery'
import { useAuthStore } from '@/stores/auth'
import { formatScore } from '@/lib/format'
import { rejectedLabel, traceItemFor, traceJson } from '@/lib/trace'
import ScoreBreakdown from './ScoreBreakdown.vue'
import TraceStepRow from './TraceStepRow.vue'
import TraceSummary from './TraceSummary.vue'

const props = defineProps<{
  trace: Trace | null
  loading: boolean
  /** Подписи товаров из выдачи: product_id → название. */
  titles: Record<string, string>
  selected: string | null
  /** Показывать крестик (на десктопе панель закрывается переключателем и крестиком). */
  closable?: boolean
  /** Панель внутри окна со своим заголовком: свой заголовок только для скринридера. */
  embedded?: boolean
  /** Запрос выдачи: с ним сотрудник может отдать его агенту на разбор. */
  query?: string | null
}>()

const emit = defineEmits<{ select: [productId: string]; close: [] }>()
const toast = useToast()
const auth = useAuthStore()
const { analyse, busy: analysing } = useAnalyseQuery()
const canAnalyse = computed(() => auth.staff && !!props.query?.trim() && !!props.trace)

const items = computed(() => props.trace?.items ?? [])
const selectedItem = computed(
  () => traceItemFor(props.trace, props.selected) ?? items.value[0] ?? null,
)
const itemOptions = computed(() =>
  items.value.map((it, i) => ({
    value: it.product_id,
    label: `${i + 1}. ${props.titles[it.product_id] ?? it.product_id}`,
    hint: `скор ${formatScore(it.final)}`,
  })),
)
const selectedModel = computed({
  get: () => selectedItem.value?.product_id ?? '',
  set: (v: string) => emit('select', v),
})

async function copyJson() {
  if (!props.trace) return
  try {
    await navigator.clipboard.writeText(traceJson(props.trace))
    toast.add({ tone: 'ok', title: 'JSON трассы скопирован' })
  } catch {
    toast.add({
      tone: 'error',
      title: 'Не удалось скопировать',
      detail: 'Браузер запретил буфер обмена',
    })
  }
}
</script>

<template>
  <section class="trace" aria-labelledby="trace-title">
    <header class="trace__head">
      <h2 id="trace-title" :class="embedded ? 'sr-only' : 't-h2'">Трасса запроса</h2>
      <div class="trace__tools">
        <button
          v-if="canAnalyse"
          type="button"
          class="btn btn--secondary btn--sm"
          :disabled="!!analysing"
          :title="`Агент разберёт «${query}»: план, гипотезы, прогон`"
          @click="analyse(query!)"
        >
          <Microscope :size="14" aria-hidden="true" />
          {{ analysing ? 'Запускаем…' : 'Разобрать' }}
        </button>
        <button type="button" class="btn btn--ghost btn--sm" :disabled="!trace" @click="copyJson">
          <Copy :size="14" aria-hidden="true" />
          Скопировать JSON
        </button>
        <button
          v-if="closable"
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          aria-label="Закрыть трассу"
          @click="emit('close')"
        >
          <X :size="16" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-if="loading" class="trace__loading" aria-busy="true">
      <p class="t-sm t-muted">Шаги появятся вместе с выдачей</p>
      <span v-for="i in 6" :key="i" class="skeleton" :style="{ width: `${90 - i * 8}%` }" />
    </div>

    <p v-else-if="!trace" class="t-muted trace__empty">
      Введите запрос, чтобы увидеть его обработку.
    </p>

    <template v-else>
      <TraceSummary :trace="trace" />

      <div class="trace__block">
        <h3 class="trace__label">Шаги</h3>
        <ol class="trace__steps">
          <TraceStepRow v-for="s in trace.steps ?? []" :key="s.name" :step="s" :titles="titles" />
        </ol>
      </div>

      <div class="trace__block">
        <h3 class="trace__label">Разложение скора</h3>
        <template v-if="selectedItem">
          <UiSelect
            v-if="itemOptions.length > 1"
            v-model="selectedModel"
            :options="itemOptions"
            label="Товар для разложения скора"
          />
          <ScoreBreakdown
            :item="selectedItem"
            :title="titles[selectedItem.product_id] ?? selectedItem.product_id"
          />
        </template>
        <p v-else class="t-sm t-muted">В выдаче нет товаров — раскладывать нечего.</p>
      </div>

      <div v-if="trace.rejected?.length" class="trace__block">
        <h3 class="trace__label">Отсечённые товары</h3>
        <ul class="trace__rejected">
          <li v-for="r in trace.rejected" :key="r.product_id">
            <span class="trace__rejected-title">{{ titles[r.product_id] ?? r.product_id }}</span>
            <span class="t-muted">{{ rejectedLabel(r) }}</span>
          </li>
        </ul>
      </div>

      <div v-if="trace.llm" class="trace__block">
        <h3 class="trace__label">Вызов LLM</h3>
        <dl class="trace__llm">
          <template v-for="(v, k) in trace.llm" :key="k">
            <dt class="t-mono">{{ k }}</dt>
            <dd class="t-mono">{{ typeof v === 'string' ? v : JSON.stringify(v) }}</dd>
          </template>
        </dl>
      </div>
    </template>
  </section>
</template>

<style scoped>
.trace {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
.trace__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.trace__head h2 {
  white-space: nowrap;
}
.trace__tools {
  display: flex;
  margin-left: auto;
  align-items: center;
  gap: var(--space-1);
}
.trace__loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.trace__empty {
  padding: var(--space-6) 0;
}
.trace__block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.trace__label {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-semibold);
}
.trace__steps {
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: var(--border-w) solid var(--border);
}
.trace__rejected {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.trace__rejected li {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0 var(--space-3);
}
.trace__rejected-title {
  min-width: 0;
  overflow-wrap: anywhere;
}
.trace__llm {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 2px var(--space-3);
  margin: 0;
}
.trace__llm dt {
  color: var(--text-2);
}
.trace__llm dd {
  margin: 0;
  overflow-wrap: anywhere;
}
</style>
