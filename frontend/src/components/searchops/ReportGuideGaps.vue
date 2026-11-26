<script setup lang="ts">
// «Вопросы, на которые подбор не ответил» в отчёте SearchOps: вопрос (ПДн замаскированы
// сервером), сколько раз, о каком продукте, чего не хватило, что сделано. «Создать задачу каталогу» — запуск агента
// mode=gaps: код превращает вопросы без факта в базе знаний в кластеры kb_missing с задачей владельцу (без модели);
// ждём конец запуска и перечитываем отчёт. Дат нет.
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowUpRight, ClipboardPlus, Inbox, MessageCircleQuestion } from 'lucide-vue-next'
import { getRun, startRun } from '@/api/agent'
import type { ReportGuide } from '@/api/typesReport'
import { useToast } from '@/components/ui/useToast'
import { isRunActive } from '@/lib/agentFeed'
import { isApiError } from '@/lib/apiError'
import { formatInt, plural } from '@/lib/format'
import { canCreateGapTask, guideProducts, guideStatus, runLink } from '@/lib/report'

const props = defineProps<{ guide: ReportGuide; days: number }>()
const emit = defineEmits<{ reload: [] }>()

const toast = useToast()
const creating = ref(false)

const timesWord = (n: number) => plural(n, ['раз', 'раза', 'раз'])

async function createTasks() {
  if (creating.value) return
  creating.value = true
  try {
    const { run_id } = await startRun({
      mode: 'gaps',
      trigger: 'manual',
      days: props.days,
      max_clusters: 1,
    })
    // Запуск без модели — секунды; ждём итог не дольше 30 с.
    let summary = ''
    for (let i = 0; i < 30; i++) {
      await new Promise((r) => setTimeout(r, 1000))
      const d = await getRun(run_id)
      if (!isRunActive(d.run.status)) {
        summary = d.run.summary ?? ''
        break
      }
    }
    toast.add({
      tone: 'ok',
      title: 'Задачи по пробелам подбора созданы',
      detail: summary || undefined,
    })
    emit('reload')
  } catch (e) {
    toast.add({
      tone: 'error',
      title: 'Задачи не созданы',
      detail: isApiError(e) ? e.message : undefined,
    })
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <section class="panel ggap" aria-labelledby="mrep-sec-guide" data-section="guide">
    <header class="ggap__head">
      <MessageCircleQuestion :size="18" aria-hidden="true" />
      <h3 id="mrep-sec-guide" class="ggap__title">{{ guide.title }}</h3>
      <span class="badge t-num">{{ guide.rows.length }}</span>
    </header>
    <p class="t-sm t-muted ggap__hint">{{ guide.hint }}</p>
    <p class="t-xs t-muted ggap__owner">
      Причина «{{ guide.cause.label }}» · владелец: {{ guide.cause.owner_label }}
    </p>

    <p v-if="!guide.rows.length" class="t-sm t-muted">Подбор отвечал на всё, о чём спрашивали.</p>
    <ul v-else class="ggap__rows">
      <li v-for="r in guide.rows" :key="`${r.code}|${r.question}`" class="ggap__row">
        <div class="ggap__main">
          <p class="ggap__q">«{{ r.question }}»</p>
          <p class="t-sm ggap__meta">
            <span class="t-num">{{ formatInt(r.count) }} {{ timesWord(r.count) }}</span>
            · {{ guideProducts(r) }}
          </p>
          <p class="t-sm ggap__miss">
            Не хватило:
            <span :class="['ggap__label', { 'is-kb': r.kb_missing }]">{{ r.label }}</span>
          </p>
        </div>
        <div class="ggap__side">
          <span :class="['badge', 'badge--tone', guideStatus(r).tone]">{{
            guideStatus(r).label
          }}</span>
          <RouterLink
            v-if="r.proposal"
            :to="{ path: '/searchops', query: { tab: 'queue', status: 'all', p: r.proposal.id } }"
            class="btn btn--secondary btn--sm"
          >
            <Inbox :size="14" aria-hidden="true" />
            Открыть задачу
          </RouterLink>
          <RouterLink v-if="runLink(r)" :to="runLink(r)!" class="btn btn--ghost btn--sm">
            <ArrowUpRight :size="14" aria-hidden="true" />
            Ход агента
          </RouterLink>
          <button
            v-if="canCreateGapTask(r)"
            type="button"
            class="btn btn--primary btn--sm"
            :disabled="creating"
            @click="createTasks"
          >
            <ClipboardPlus :size="14" aria-hidden="true" />
            {{ creating ? 'Создаём…' : 'Создать задачу каталогу' }}
          </button>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.ggap {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.ggap__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.ggap__head svg {
  flex: none;
  color: var(--accent);
}
.ggap__title {
  margin: 0;
  font-size: var(--fs-h2);
  font-weight: var(--fw-bold);
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--space-3));
}
.ggap__hint,
.ggap__owner {
  margin: 0;
  max-width: 70ch;
}
.ggap__rows {
  display: flex;
  flex-direction: column;
  margin: var(--space-2) 0 0;
  padding: 0;
  list-style: none;
  border-top: var(--border-w) solid var(--border);
}
.ggap__row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
  padding: var(--space-3) var(--space-2);
  border-bottom: var(--border-w) solid var(--border);
}
.ggap__main {
  display: flex;
  flex: 1 1 24rem;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.ggap__main p {
  margin: 0;
  overflow-wrap: anywhere;
}
.ggap__q {
  font-weight: var(--fw-semibold);
}
.ggap__meta {
  color: var(--text-2);
}
.ggap__label {
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
}
.ggap__label.is-kb {
  border-color: var(--accent);
  color: var(--accent);
}
.ggap__side {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
}
@media (max-width: 767px) {
  .ggap {
    padding: var(--space-3);
  }
  .ggap__row {
    padding: var(--space-3) 0;
  }
  .ggap__side {
    justify-content: flex-start;
    width: 100%;
  }
}
</style>
