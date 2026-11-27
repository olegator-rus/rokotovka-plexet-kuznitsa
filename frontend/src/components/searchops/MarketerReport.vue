<script setup lang="ts">
// Первый экран SearchOps — отчёт для маркетолога (контур фида, кадр 1): четыре раздела —
// спрос без предложения; товар есть, но не найден; слова клиента, которых нет в каталоге; сервисные запросы.
// Строка раскрывается в ленту шагов агента по этому запросу (ReportRowFeed). Поток подписан: тестовый, три слоя,
// частоты модельные. Данные — GET /api/agent/report; раздел и доказательство считает сервер. Рублей и дат нет.
import { computed, ref } from 'vue'
import {
  ChevronDown,
  CircleHelp,
  MessageCircleQuestion,
  PackageX,
  SearchX,
  SpellCheck,
  Store,
} from 'lucide-vue-next'
import type { Component } from 'vue'
import { getReport } from '@/api/agent'
import type { ReportRow, ReportSection } from '@/api/typesReport'
import BlockError from '@/components/common/BlockError.vue'
import ReportGuideGaps from '@/components/searchops/ReportGuideGaps.vue'
import ReportRowFeed from '@/components/searchops/ReportRowFeed.vue'
import { useResource } from '@/composables/useResource'
import { formatInt, plural } from '@/lib/format'
import { failLine, layerShort, rowStatus } from '@/lib/report'

const emit = defineEmits<{ start: [] }>()

const REPORT_DAYS = 7
/** Строк раздела без «Показать все». */
const PAGE = 4

const res = useResource((opts) => getReport(REPORT_DAYS, opts))
void res.load()

const SECTION_ICONS: Record<string, Component> = {
  no_offer: PackageX,
  not_found: SearchX,
  unknown_words: SpellCheck,
  service: Store,
}

const report = computed(() => res.data.value)
const sections = computed<ReportSection[]>(() => report.value?.sections ?? [])
const failedTotal = computed(() => report.value?.totals.failed_queries ?? 0)

const expanded = ref<Set<string>>(new Set())
const showAll = ref<Set<string>>(new Set())
const rowKey = (code: string, r: ReportRow) => `${code}|${r.query}`

function toggle(key: string) {
  const s = new Set(expanded.value)
  if (s.has(key)) s.delete(key)
  else s.add(key)
  expanded.value = s
}
function toggleAll(code: string) {
  const s = new Set(showAll.value)
  if (s.has(code)) s.delete(code)
  else s.add(code)
  showAll.value = s
}
const visible = (sec: ReportSection) =>
  showAll.value.has(sec.code) ? sec.rows : sec.rows.slice(0, PAGE)

const pendingOpen = ref(false)
const pendingRows = computed(() => report.value?.pending ?? [])

/** Переход к разделу из сводки (без смены адреса: вкладка в адресе остаётся). */
function jump(code: string) {
  document
    .getElementById(`mrep-sec-${code}`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
const capital = (t: string) => t.charAt(0).toUpperCase() + t.slice(1)
const searchesWord = (n: number) => plural(n, ['поиск', 'поиска', 'поисков'])
const queriesWord = (n: number) => plural(n, ['запрос', 'запроса', 'запросов'])
</script>

<template>
  <section class="mrep" aria-labelledby="mrep-title">
    <h2 id="mrep-title" class="sr-only">Отчёт для маркетолога</h2>

    <div
      v-if="res.status.value === 'loading' && !report"
      class="panel mrep__skel"
      aria-busy="true"
      aria-label="Отчёт загружается"
    >
      <span v-for="i in 6" :key="i" class="skeleton" :style="{ width: `${96 - i * 9}%` }" />
    </div>

    <BlockError
      v-else-if="res.status.value === 'error' && !report"
      :error="res.error.value!"
      title="Отчёт не загрузился"
      @retry="res.load()"
    />

    <template v-else-if="report">
      <div class="panel mrep__stream">
        <p class="mrep__stream-label">{{ capital(report.stream.label) }}</p>
        <ul class="mrep__layers" aria-label="Слои потока">
          <li v-for="l in report.stream.layers" :key="l.code" class="mrep__layer">
            <span class="mrep__layer-name">{{ l.label }}</span>
            <span class="t-num t-sm"
              >{{ formatInt(l.queries) }} {{ queriesWord(l.queries) }} · {{ formatInt(l.searches) }}
              {{ searchesWord(l.searches) }}</span
            >
          </li>
        </ul>
        <p class="t-xs t-muted">
          Частоты модельные. Неудачных запросов —
          <span class="t-num">{{ formatInt(failedTotal) }}</span> из
          <span class="t-num">{{ formatInt(report.totals.queries) }}</span>, неудачных поисков —
          <span class="t-num">{{ formatInt(report.totals.failed_searches) }}</span> из
          <span class="t-num">{{ formatInt(report.totals.searches) }}</span>. Версия правил поиска
          <span class="t-num">{{ report.rules_version }}</span>.
        </p>
      </div>

      <nav v-if="failedTotal" class="mrep__nav" aria-label="Разделы отчёта">
        <button
          v-for="sec in sections"
          :key="sec.code"
          type="button"
          class="mrep__nav-item"
          @click="jump(sec.code)"
        >
          <component :is="SECTION_ICONS[sec.code] ?? CircleHelp" :size="16" aria-hidden="true" />
          <span class="mrep__nav-title">{{ sec.title }}</span>
          <strong class="t-num">{{ sec.rows.length }}</strong>
        </button>
        <button v-if="report.guide" type="button" class="mrep__nav-item" @click="jump('guide')">
          <MessageCircleQuestion :size="16" aria-hidden="true" />
          <span class="mrep__nav-title">{{ report.guide.title }}</span>
          <strong class="t-num">{{ report.guide.rows.length }}</strong>
        </button>
      </nav>

      <div v-if="!failedTotal" class="panel mrep__empty">
        <p>В журнале текущего каталога неудачных поисков нет — разбирать нечего.</p>
        <button type="button" class="btn btn--secondary btn--sm" @click="res.load()">
          Обновить
        </button>
      </div>

      <section
        v-for="sec in sections"
        :key="sec.code"
        class="panel mrep__sec"
        :aria-labelledby="`mrep-sec-${sec.code}`"
        :data-section="sec.code"
      >
        <header class="mrep__sec-head">
          <component :is="SECTION_ICONS[sec.code] ?? CircleHelp" :size="18" aria-hidden="true" />
          <h3 :id="`mrep-sec-${sec.code}`" class="mrep__sec-title">{{ sec.title }}</h3>
          <span class="badge t-num mrep__sec-count">{{ sec.rows.length }}</span>
        </header>
        <p class="t-sm t-muted mrep__hint">{{ sec.hint }}</p>

        <p v-if="!sec.rows.length" class="t-sm t-muted mrep__none">В этом разделе пусто.</p>
        <ul v-else class="mrep__rows">
          <li
            v-for="r in visible(sec)"
            :key="rowKey(sec.code, r)"
            class="mrep__row"
            :class="{ 'is-open': expanded.has(rowKey(sec.code, r)) }"
          >
            <button
              type="button"
              class="mrep__row-btn"
              :aria-expanded="expanded.has(rowKey(sec.code, r))"
              @click="toggle(rowKey(sec.code, r))"
            >
              <span class="mrep__q">
                <span class="mrep__query">{{ r.query }}</span>
                <span class="mrep__chips">
                  <span v-for="l in r.layers" :key="l" class="mrep__chip">{{ layerShort(l) }}</span>
                  <span class="mrep__kind">{{ r.kind_label }}</span>
                </span>
              </span>
              <span class="mrep__freq t-num" title="Неудачных поисков из всех, частота модельная">
                <strong>{{ failLine(r) }}</strong>
                <span class="t-xs t-muted">неудачных</span>
              </span>
              <span :class="['badge', 'badge--tone', 'mrep__status', rowStatus(r).tone]">
                {{ rowStatus(r).label }}
              </span>
              <ChevronDown :size="16" aria-hidden="true" class="mrep__chev" />
              <span class="mrep__evidence t-sm">{{ r.evidence }}</span>
            </button>
            <ReportRowFeed v-if="expanded.has(rowKey(sec.code, r))" :row="r" :section="sec.code" />
          </li>
        </ul>
        <button
          v-if="sec.rows.length > PAGE"
          type="button"
          class="btn btn--ghost btn--sm mrep__more"
          :aria-expanded="showAll.has(sec.code)"
          @click="toggleAll(sec.code)"
        >
          {{ showAll.has(sec.code) ? 'Свернуть' : `Показать все ${sec.rows.length}` }}
        </button>
      </section>

      <ReportGuideGaps
        v-if="report.guide"
        :guide="report.guide"
        :days="report.days"
        @reload="res.load({ quiet: true })"
      />
      <section
        v-if="pendingRows.length"
        class="panel mrep__sec mrep__pending"
        data-section="pending"
      >
        <header class="mrep__sec-head">
          <CircleHelp :size="18" aria-hidden="true" />
          <h3 class="mrep__sec-title">Ещё не разобраны</h3>
          <span class="badge t-num mrep__sec-count">{{ pendingRows.length }}</span>
        </header>
        <p class="t-sm t-muted mrep__hint">
          Неудачные запросы, причина которых пока не доказана: ни агент, ни ответ поиска не
          показали, есть товар или нет. Агент разберёт их по расписанию; любой можно разобрать
          сейчас.
        </p>
        <button
          type="button"
          class="btn btn--secondary btn--sm mrep__more"
          :aria-expanded="pendingOpen"
          @click="pendingOpen = !pendingOpen"
        >
          {{ pendingOpen ? 'Свернуть' : `Показать ${pendingRows.length}` }}
        </button>
        <ul v-if="pendingOpen" class="mrep__rows">
          <li
            v-for="r in pendingRows"
            :key="rowKey('pending', r)"
            class="mrep__row"
            :class="{ 'is-open': expanded.has(rowKey('pending', r)) }"
          >
            <button
              type="button"
              class="mrep__row-btn"
              :aria-expanded="expanded.has(rowKey('pending', r))"
              @click="toggle(rowKey('pending', r))"
            >
              <span class="mrep__q">
                <span class="mrep__query">{{ r.query }}</span>
                <span class="mrep__chips">
                  <span v-for="l in r.layers" :key="l" class="mrep__chip">{{ layerShort(l) }}</span>
                </span>
              </span>
              <span class="mrep__freq t-num">
                <strong>{{ failLine(r) }}</strong>
                <span class="t-xs t-muted">неудачных</span>
              </span>
              <span :class="['badge', 'badge--tone', 'mrep__status', rowStatus(r).tone]">
                {{ rowStatus(r).label }}
              </span>
              <ChevronDown :size="16" aria-hidden="true" class="mrep__chev" />
              <span class="mrep__evidence t-sm">{{ r.evidence }}</span>
            </button>
            <ReportRowFeed v-if="expanded.has(rowKey('pending', r))" :row="r" section="pending" />
          </li>
        </ul>
      </section>

      <p v-if="report.truncated" class="t-xs t-muted">
        Показаны неудачные запросы с наибольшим потерянным спросом; остальные — во вкладке «Запросы
        без ответа».
      </p>
      <p class="t-xs t-muted mrep__foot">
        Раздел строки — по доказательству: вывод агента, ответ поиска сейчас или переформулировка
        покупателей.
        <button type="button" class="link" @click="emit('start')">Запустить разбор</button>
      </p>
    </template>
  </section>
</template>

<style scoped>
.mrep {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.mrep__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.mrep__empty {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-4);
  color: var(--text-2);
}
.mrep__stream {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.mrep__stream-label {
  margin: 0;
  font-weight: var(--fw-semibold);
}
.mrep__layers {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.mrep__layer {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
}
.mrep__layer-name {
  font-weight: var(--fw-semibold);
}
.mrep__stream p:last-child {
  margin: 0;
}
.mrep__nav {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
  gap: var(--space-2);
}
.mrep__nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.mrep__nav-item:hover {
  border-color: var(--accent);
}
.mrep__nav-item svg {
  flex: none;
  color: var(--accent);
}
.mrep__nav-title {
  flex: 1;
  min-width: 0;
  font-size: var(--fs-sm);
}
.mrep__nav-item strong {
  font-size: var(--fs-h2);
}
.mrep__sec-title {
  scroll-margin-top: calc(var(--header-actual, var(--header-h)) + var(--space-3));
}
.mrep__sec {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.mrep__sec-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text);
}
.mrep__sec-head svg {
  flex: none;
  color: var(--accent);
}
.mrep__sec-title {
  margin: 0;
  font-size: var(--fs-h2);
  font-weight: var(--fw-bold);
}
.mrep__hint {
  margin: 0;
  max-width: 70ch;
}
.mrep__none {
  margin: 0;
}
.mrep__rows {
  display: flex;
  flex-direction: column;
  margin: var(--space-2) 0 0;
  padding: 0;
  list-style: none;
  border-top: var(--border-w) solid var(--border);
}
.mrep__row {
  border-bottom: var(--border-w) solid var(--border);
}
.mrep__row-btn {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto auto;
  grid-template-areas:
    'q freq status chev'
    'ev ev ev ev';
  align-items: center;
  gap: var(--space-1) var(--space-3);
  width: 100%;
  padding: var(--space-3) var(--space-2);
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color var(--dur-fast) var(--ease-standard);
}
.mrep__row-btn:hover {
  background: var(--surface-2);
}
.mrep__row-btn:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: -2px;
}
.mrep__q {
  grid-area: q;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  min-width: 0;
}
.mrep__query {
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.mrep__chips {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
.mrep__chip,
.mrep__kind {
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
  line-height: 1.6;
  white-space: nowrap;
}
.mrep__chip {
  background: var(--surface-3);
  color: var(--text-2);
}
.mrep__kind {
  border: var(--border-w) solid var(--border-control);
  color: var(--text-2);
}
.mrep__freq {
  grid-area: freq;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  white-space: nowrap;
}
.mrep__status {
  grid-area: status;
  justify-self: end;
  white-space: nowrap;
}
.mrep__chev {
  grid-area: chev;
  color: var(--text-3);
  transition: transform var(--dur-fast) var(--ease-standard);
}
.mrep__row.is-open .mrep__chev {
  transform: rotate(180deg);
}
.mrep__evidence {
  grid-area: ev;
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.mrep__more {
  align-self: flex-start;
}
.mrep__foot {
  margin: 0;
}
.mrep__foot .link {
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  cursor: pointer;
}
@media (max-width: 1023px) {
  .mrep__nav {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 767px) {
  .mrep__sec,
  .mrep__stream {
    padding: var(--space-3);
  }
  .mrep__layers {
    flex-direction: column;
    gap: 0;
  }
  .mrep__layer {
    justify-content: space-between;
    padding: var(--space-1) 0;
    border: 0;
    border-bottom: var(--border-w) solid var(--border);
    border-radius: 0;
    background: none;
  }
  .mrep__nav-item {
    padding: var(--space-2);
  }
  .mrep__row-btn {
    grid-template-columns: minmax(0, 1fr) auto;
    /* Статус — своей строкой: длинная подпись («не доразобрано: агент не дошёл до итога») во второй колонке
       расширяла её и сжимала запрос до столбика букв, а частота наезжала на статус. */
    grid-template-areas:
      'q chev'
      'freq freq'
      'status status'
      'ev ev';
    padding: var(--space-3) 0;
  }
  .mrep__freq {
    flex-direction: row;
    align-items: baseline;
    gap: var(--space-1);
    justify-self: start;
  }
  .mrep__status {
    justify-self: start;
    white-space: normal;
    text-align: left;
  }
}
</style>
