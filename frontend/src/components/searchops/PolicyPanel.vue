<script setup lang="ts">
// SearchOps → «Политика ранжирования»: один экран — рычаг маржи, один
// пример «было / стало» (артикул не двигается, два близких товара меняются местами), индикаторы защит, плашка
// «Синтетическая маржа». Кривая компромисса убрана. Движение рычага — песочница (POST /api/margin/simulate);
// «Применить» создаёт версию политики, откат — во «Изменениях» (?tab=changes&view=policy). Адрес: ?tab=policy&w=0.08.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  CircleAlert,
  Equal,
  LoaderCircle,
  Lock,
  RotateCcw,
  Search,
  ShieldCheck,
} from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { applyMarginPolicy, getMarginPolicy, getSimulateStatus, simulateMargin } from '@/api/margin'
import type { MarginSimulateResponse } from '@/api/typesCommerce'
import BlockError from '@/components/common/BlockError.vue'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import MarginLever from '@/components/margin/MarginLever.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { AREA, denyMessage } from '@/lib/auth'
import { buildEvent } from '@/lib/events'
import { formatDateTime, formatInt, plural } from '@/lib/format'
import {
  affectedList,
  estimateNote,
  guardIndicators,
  hasMarginData,
  isLocked,
  pointFor,
  positionIndex,
  qualityLossText,
  queriesLabel,
  rankColumns,
  swapExample,
  isNeighbourSwap,
  weightLabel,
} from '@/lib/margin'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const LAST_SIM = 'plexet.margin.lastSim'

const route = useRoute()
const router = useRouter()
const app = useAppStore()
// Применение политики — зона администратора поиска (и руководителя)
const auth = useAuthStore()
const applyDeny = computed(() => (auth.can(AREA.margin) ? '' : denyMessage(AREA.margin)))
const toast = useToast()

const policy = useResource((opts) => getMarginPolicy(opts))
const sim = useResource((opts) => simulateMargin(policy.data.value?.positions ?? [], opts, 'tune'))

// 503 у симуляции — базовая выдача ещё собирается в фоне: показываем прогресс и повторяем.
const building = ref<{ done: number; total: number } | null>(null)
let buildTimer: ReturnType<typeof setTimeout> | undefined
async function pollBuild() {
  clearTimeout(buildTimer)
  try {
    const st = await getSimulateStatus()
    if (st.running) {
      building.value = { done: st.done, total: st.total }
      buildTimer = setTimeout(pollBuild, 2000)
      return
    }
  } catch {
    /* статус недоступен — просто повторяем симуляцию */
  }
  building.value = null
  await loadSim()
}
// Набора с разметкой нет (статус no_set) — симуляцию не зовём, показываем причину. 503 без идущего сбора — тоже
// не крутимся (раньше опрос шёл по кругу и экран навсегда оставался на «Собираем базовую выдачу»).
const noSet = ref(false)
async function loadSim() {
  noSet.value = false
  try {
    if ((await getSimulateStatus()).no_set) {
      building.value = null
      noSet.value = true
      return
    }
  } catch {
    /* статус недоступен — пробуем симуляцию */
  }
  await sim.load()
  if (sim.status.value === 'error' && sim.error.value?.status === 503) {
    let running = true
    try {
      running = (await getSimulateStatus()).running
    } catch {
      /* статус недоступен — считаем, что сбор идёт */
    }
    if (!running) {
      building.value = null
      noSet.value = /разметк/i.test(sim.error.value?.message ?? '')
      return
    }
    building.value = building.value ?? { done: 0, total: 0 }
    buildTimer = setTimeout(pollBuild, 2000)
  }
}
onBeforeUnmount(() => clearTimeout(buildTimer))

// Последняя удачная симуляция с датой — при ошибке нового прогона.
function readLast(): { at: string; data: MarginSimulateResponse } | null {
  try {
    const raw = window.localStorage.getItem(LAST_SIM)
    return raw ? (JSON.parse(raw) as { at: string; data: MarginSimulateResponse }) : null
  } catch {
    return null
  }
}
const lastGood = ref(readLast())
watch(
  () => sim.data.value,
  (d) => {
    if (!d) return
    const entry = { at: d.ran_at ?? new Date().toISOString(), data: d }
    lastGood.value = entry
    try {
      window.localStorage.setItem(LAST_SIM, JSON.stringify(entry))
    } catch {
      /* хранилище недоступно — живём без него */
    }
  },
)
const simData = computed(
  () => sim.data.value ?? (sim.status.value === 'error' ? (lastGood.value?.data ?? null) : null),
)
const staleSim = computed(() => !sim.data.value && !!simData.value)

async function loadAll() {
  await policy.load()
  if (policy.status.value === 'ready') await loadSim()
}
void loadAll()

const positions = computed(() => policy.data.value?.positions ?? [])
const active = computed(() => policy.data.value?.weight ?? 0)
const baseWeight = computed(
  () => policy.data.value?.base_weight ?? simData.value?.base_weight ?? 0.05,
)
const bounds = computed(
  () =>
    simData.value ?? (policy.data.value ? { stop_weight: policy.data.value.stop_weight } : null),
)
const locked = computed(() => positions.value.filter((w) => isLocked(w, bounds.value)))
const noMargin = computed(() => !!simData.value && !hasMarginData(simData.value))
const synthetic = computed(() => !!(policy.data.value?.synthetic || simData.value?.synthetic))
const guards = computed(() => guardIndicators(policy.data.value))

const selected = computed<number>({
  get: () => {
    const raw = Number(route.query.w)
    const ps = positions.value
    if (Number.isFinite(raw) && ps.length && route.query.w !== undefined) {
      const w = ps[positionIndex(ps, raw)]
      if (!isLocked(w, bounds.value)) return w
    }
    return active.value
  },
  set: (w) => {
    void router.replace({
      query: { ...route.query, w: w === active.value ? undefined : String(w) },
    })
    void postEvent(
      buildEvent(
        'margin_simulate',
        { requestId: null, sessionId: app.sessionId, clientId: null },
        { payload: { weight: w } },
      ),
    ).catch(() => undefined)
  },
})
const point = computed(() => pointFor(simData.value, selected.value))
const changed = computed(() => selected.value !== active.value)
const queries = computed(() => simData.value?.judged ?? simData.value?.queries ?? null)
const example = computed(() => swapExample(affectedList(point.value)))
const exampleCols = computed(() => {
  if (!example.value) return null
  const c = rankColumns(example.value)
  return { before: c.before.slice(0, 3), after: c.after.slice(0, 3) }
})

// ── Применить ──
const applyOpen = ref(false)
const applying = ref(false)
const applyErr = ref('')
const comment = ref('')
function askApply() {
  applyErr.value = ''
  comment.value = ''
  applyOpen.value = true
}
async function confirmApply() {
  const before = policy.data.value?.version ?? null
  applying.value = true
  applyErr.value = ''
  try {
    const r = await applyMarginPolicy(selected.value, comment.value.trim())
    applying.value = false
    applyOpen.value = false
    void postEvent(
      buildEvent(
        'margin_apply',
        { requestId: null, sessionId: app.sessionId, clientId: null },
        { payload: { version_before: before, version_after: r.version } },
      ),
    ).catch(() => undefined)
    await router.replace({ query: { ...route.query, w: undefined } })
    await policy.load({ quiet: true })
    toast.add({
      tone: 'ok',
      title: `Политика ранжирования v${r.version}`,
      detail: `Вес ${weightLabel(selected.value)} действует в поиске. Откат — во «Изменениях».`,
      action: {
        label: 'Открыть «Изменения»',
        run: () =>
          void router.push({ path: '/searchops', query: { tab: 'changes', view: 'policy' } }),
      },
    })
  } catch (e) {
    applying.value = false
    applyErr.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}
</script>

<template>
  <section class="policy" aria-labelledby="policy-title">
    <header class="policy__head">
      <div>
        <h2 id="policy-title" class="t-h2">Рычаг маржи</h2>
        <p class="t-sm t-muted">
          Насколько маржа влияет на порядок внутри группы равных по соответствию товаров. Движение
          рычага проверяется на тестовой копии правил; поиск меняется только после «Применить».
        </p>
      </div>
      <SyntheticBadge v-if="synthetic" />
    </header>

    <div
      v-if="policy.status.value === 'loading' && !policy.data.value"
      class="panel policy__skel"
      aria-busy="true"
    >
      <span class="skeleton" style="width: 30%" />
      <span class="skeleton" style="width: 90%; height: 2.5rem" />
    </div>
    <BlockError
      v-else-if="policy.status.value === 'error' && !policy.data.value"
      :error="policy.error.value!"
      title="Политика ранжирования не загрузилась"
      @retry="loadAll()"
    />

    <template v-else-if="policy.data.value">
      <div class="panel policy__lever">
        <MarginLever
          v-model="selected"
          :positions="positions"
          :locked="locked"
          :active="active"
          :epsilon="policy.data.value.epsilon"
          :disabled="!simData || noMargin"
        />
        <p v-if="policy.data.value.enabled === false" class="t-xs policy__off" role="note">
          Политика выключена флагом FEATURE_MARGIN_POLICY: в поиске она сейчас не действует.
        </p>
        <ul class="policy__guards" aria-label="Защиты, которые рычаг не обходит">
          <li v-for="g in guards" :key="g" class="badge">
            <ShieldCheck :size="12" aria-hidden="true" /> {{ g }}
          </li>
        </ul>
        <p v-if="point && !noMargin" class="t-sm t-muted">
          {{ qualityLossText(point.ndcg_delta_mean, baseWeight) }}; сменился первый результат:
          {{ queriesLabel(point.top1_changed) }}; защиты сработали
          {{ formatInt(point.guards_fired) }}
          {{ plural(point.guards_fired, ['раз', 'раза', 'раз']) }}.
        </p>
        <div class="policy__apply">
          <button
            type="button"
            class="btn btn--primary"
            :disabled="!changed || !point || noMargin || !!applyDeny"
            :title="applyDeny || undefined"
            @click="askApply"
          >
            Применить вес {{ weightLabel(selected) }}
          </button>
          <span v-if="applyDeny" class="t-xs t-muted policy__deny">
            <Lock :size="12" aria-hidden="true" /> {{ applyDeny }}
          </span>
          <span class="t-xs t-muted">
            Действует v{{ policy.data.value.version }}, вес {{ weightLabel(active) }} ·
            <RouterLink
              :to="{ path: '/searchops', query: { tab: 'changes', view: 'policy' } }"
              class="link"
              >версии и откат</RouterLink
            >
          </span>
        </div>
      </div>

      <div
        v-if="(sim.status.value === 'loading' || building) && !sim.data.value"
        class="panel policy__running"
        role="status"
      >
        <LoaderCircle :size="18" aria-hidden="true" class="spin" />
        <span v-if="building">
          Собираем базовую выдачу для сравнения<template v-if="building.total">
            : {{ formatInt(building.done) }} из {{ queriesLabel(building.total) }}</template
          >. Это один раз на каталог, обычно до полуминуты.
        </span>
        <span v-else>Идёт прогон положений рычага на размеченном наборе…</span>
      </div>
      <div v-else-if="noSet && !simData" class="panel policy__empty" data-testid="policy-no-set">
        <p class="t-h2">Тестовой копии правил не на чем сравнить выдачу</p>
        <p class="t-muted">
          Рычаг проверяется прогоном на наборе запросов с разметкой: выдача до и после, какие товары
          сдвинулись. Для этих данных такого набора нет, поэтому прогон не выполняется и вес не
          применяется. Действующий вес и защиты остаются прежними.
        </p>
      </div>
      <BlockError
        v-else-if="sim.status.value === 'error' && !simData && !building"
        :error="sim.error.value!"
        title="Симуляция не выполнена"
        @retry="loadSim()"
      />
      <div v-else-if="noMargin" class="panel policy__empty">
        <p class="t-h2">Нет данных о марже</p>
        <p class="t-muted">
          В каталоге нет маржи позиций — рычагу нечего двигать. Он станет доступен после загрузки
          данных с маржой.
        </p>
      </div>

      <p v-if="staleSim && lastGood && !building" class="panel policy__stale t-sm" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        <span>
          Новый прогон не выполнен: {{ sim.error.value?.message }}. Показана последняя удачная
          симуляция от {{ formatDateTime(lastGood.at) }}.
          <button type="button" class="link" @click="loadSim()">
            <RotateCcw :size="12" aria-hidden="true" /> Повторить
          </button>
        </span>
      </p>

      <section
        v-if="simData && !noMargin"
        class="panel policy__example"
        aria-labelledby="policy-example-title"
      >
        <header class="pex__head">
          <h3 id="policy-example-title" class="t-h2">Пример: было / стало</h3>
          <p class="pex__guard t-xs">
            <ShieldCheck :size="14" aria-hidden="true" />
            Запрос по артикулу не двигается: точные запросы рычаг не трогает.
          </p>
        </header>
        <template v-if="example && exampleCols">
          <div class="pex__query">
            <p class="pex__search">
              <Search :size="16" aria-hidden="true" />
              <strong>{{ example.query }}</strong>
            </p>
            <p class="t-sm t-muted">
              При весе {{ weightLabel(selected) }}
              {{
                isNeighbourSwap(example)
                  ? 'близкие по соответствию товары меняются местами.'
                  : 'меняется первая тройка выдачи.'
              }}
            </p>
          </div>
          <div class="pex__cols">
            <div class="pex__col">
              <p class="pex__label">Было (вес {{ weightLabel(baseWeight) }})</p>
              <ol class="pex__list">
                <li v-for="r in exampleCols.before" :key="r.id" class="pex__row">
                  <span class="pex__pos t-num">{{ r.pos }}</span>
                  <span class="pex__title">{{ r.title }}</span>
                </li>
              </ol>
            </div>
            <span class="pex__arrow" aria-hidden="true"><ArrowRight :size="18" /></span>
            <div class="pex__col pex__col--after">
              <p class="pex__label">Стало (вес {{ weightLabel(selected) }})</p>
              <ol class="pex__list">
                <li
                  v-for="r in exampleCols.after"
                  :key="r.id"
                  :class="['pex__row', { 'pex__row--moved': r.move !== 'same' }]"
                >
                  <span class="pex__pos t-num">{{ r.pos }}</span>
                  <span class="pex__title">{{ r.title }}</span>
                  <span v-if="r.move !== 'same' && r.otherPos" class="pex__move t-xs">
                    <component
                      :is="r.move === 'up' ? ArrowUp : ArrowDown"
                      :size="12"
                      aria-hidden="true"
                    />
                    <span class="t-num">с {{ r.otherPos }}</span>
                  </span>
                  <span v-else-if="r.move === 'new'" class="pex__move t-xs">новая</span>
                </li>
              </ol>
            </div>
          </div>
        </template>
        <p v-else class="pex__none t-sm">
          <Equal :size="18" aria-hidden="true" />
          <span>
            На этом положении ни один запрос набора не меняет первую тройку — выдача как при весе
            {{ weightLabel(baseWeight) }}.
          </span>
        </p>
        <p class="t-xs t-muted pex__note">
          {{ estimateNote(queries, simData.set) }}
          <template v-if="simData.ran_at"> Прогон {{ formatDateTime(simData.ran_at) }}.</template>
        </p>
      </section>
    </template>

    <UiDialog v-model:open="applyOpen" :title="`Применить вес ${weightLabel(selected)}?`">
      <div class="policy__dialog">
        <p>
          Будет создана политика v{{ (policy.data.value?.version ?? 0) + 1 }} с весом
          {{ weightLabel(selected) }} вместо {{ weightLabel(active) }}. Она начнёт действовать в
          поиске сразу; прежнюю можно вернуть во «Изменениях».
        </p>
        <label class="policy__field">
          <span class="t-xs t-muted">Комментарий, если нужно</span>
          <textarea v-model="comment" rows="2" maxlength="500" class="policy__text" />
        </label>
        <p v-if="applyErr" class="policy__err" role="alert">
          <CircleAlert :size="16" aria-hidden="true" />
          Не применено: {{ applyErr }}. Действует политика v{{ policy.data.value?.version }}.
        </p>
      </div>
      <template #footer>
        <button
          type="button"
          class="btn btn--secondary"
          :disabled="applying"
          @click="applyOpen = false"
        >
          Отмена
        </button>
        <button type="button" class="btn btn--primary" :disabled="applying" @click="confirmApply">
          {{ applying ? 'Применяем…' : 'Применить' }}
        </button>
      </template>
    </UiDialog>
  </section>
</template>

<style scoped>
.policy {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.policy__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.policy__head > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 44rem;
}
.policy__skel,
.policy__lever,
.policy__example,
.policy__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.policy__guards {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.policy__guards .badge {
  gap: var(--space-1);
}
.policy__off {
  color: var(--status-pending);
}
.policy__apply {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.policy__deny {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.policy__running,
.policy__stale {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.policy__inline-icon {
  display: inline;
  vertical-align: -2px;
}
/* Пример «было / стало»: две колонки со стрелкой, переставленные позиции выделены и подписаны. */
.policy__example {
  container-type: inline-size;
}
.pex__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
}
.pex__head h3,
.policy__example p {
  margin: 0;
}
.pex__guard {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
  font-weight: var(--fw-semibold);
}
.pex__guard svg {
  flex: none;
}
.pex__query {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.pex__search {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h);
  padding: 0 var(--space-4);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  overflow-wrap: anywhere;
}
.pex__search svg {
  flex: none;
  color: var(--text-3);
}
.pex__cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: stretch;
  gap: var(--space-2);
}
.pex__arrow {
  display: grid;
  place-items: center;
  align-self: center;
  justify-self: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  transform: rotate(90deg);
}
@container (min-width: 48rem) {
  .pex__cols {
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    gap: var(--space-3);
  }
  .pex__arrow {
    transform: none;
  }
}
.pex__col {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface-2);
}
.pex__col--after {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.pex__label {
  padding: 0 var(--space-1);
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-bold);
}
.pex__col--after .pex__label {
  color: var(--accent);
}
.pex__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pex__row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-3);
  min-height: 3.5rem;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
}
.pex__row--moved {
  border: var(--focus-w) solid var(--accent);
}
.pex__pos {
  display: grid;
  place-items: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  font-weight: var(--fw-bold);
}
.pex__row--moved .pex__pos {
  background: var(--accent);
  color: var(--on-accent);
}
.pex__title {
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.pex__move {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: var(--fw-bold);
  white-space: nowrap;
}
.pex__none {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-card);
  background: var(--surface-2);
}
.pex__none svg {
  flex: none;
  color: var(--text-3);
}
.pex__note {
  padding-top: var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.policy__dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.policy__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.policy__text {
  width: 100%;
  box-sizing: border-box;
  padding: var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
.policy__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
}
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
