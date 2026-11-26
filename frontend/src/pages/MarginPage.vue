<script setup lang="ts">
// «Маржа»: один рычаг «Приоритет маржи», кривая качество ↔ индекс показанной
// маржи, затронутые запросы «было / стало», защиты. Все положения прогоняются одной симуляцией в песочнице;
// движение рычага выбирает посчитанную точку и ничего не меняет в боевом поиске до «Применить».
// Адрес: /margin?w=0.08 — подставленное положение; /margin?w=0.08&cat=<категория>&p=<id предложения> — песочница
// предложения агента MARGIN_OPPORTUNITY: рычаг только для позиций категории, рядом числа предложения и
// сверка с ними; «Применить» пишет вес категории с proposal_id — предложение становится принятым.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { RouterLink } from 'vue-router'
import {
  Bot,
  CircleAlert,
  CircleCheck,
  LoaderCircle,
  RotateCcw,
  ShieldCheck,
} from 'lucide-vue-next'
import { getProposal } from '@/api/agent'
import { postEvent } from '@/api/events'
import {
  applyMarginCategory,
  applyMarginPolicy,
  getMarginPolicy,
  getSimulateStatus,
  simulateMargin,
} from '@/api/margin'
import type { MarginProposalEval } from '@/api/typesKiller'
import type { MarginSimulateResponse } from '@/api/typesCommerce'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import AffectedQueries from '@/components/margin/AffectedQueries.vue'
import MarginCurve from '@/components/margin/MarginCurve.vue'
import MarginHistory from '@/components/margin/MarginHistory.vue'
import MarginLever from '@/components/margin/MarginLever.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { buildEvent } from '@/lib/events'
import { formatDateTime, formatDecimal, formatInt, formatPercent, plural } from '@/lib/format'
import {
  affectedList,
  estimateNote,
  guardIndicators,
  hasMarginData,
  isLocked,
  marginIndexLabel,
  ndcgDeltaLabel,
  pointFor,
  positionIndex,
  qualityLossText,
  queriesLabel,
  weightLabel,
} from '@/lib/margin'
import { indexLabel4, marginProposalView, proposalMatch } from '@/lib/killer'
import { useAppStore } from '@/stores/app'

const LAST_SIM_BASE = 'plexet.margin.lastSim'

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const toast = useToast()

const qs = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
// Категория песочницы (?cat=) и предложение агента (?p=), из карточки F28.
const catId = computed(() => qs('cat'))
const proposalId = computed(() => qs('p'))

const policy = useResource((opts) => getMarginPolicy(opts))
const sim = useResource((opts) =>
  simulateMargin(policy.data.value?.positions ?? [], opts, 'tune', catId.value || undefined),
)
const proposal = useResource((opts) => getProposal(proposalId.value, opts))
watch(
  proposalId,
  (id) => {
    if (id) void proposal.load()
  },
  { immediate: true },
)
const proposalView = computed(() =>
  proposal.data.value?.action === 'margin_policy' ? marginProposalView(proposal.data.value) : null,
)
const proposalPending = computed(
  () => proposal.data.value?.status === 'pending' && !!proposalView.value,
)

// 503 у симуляции — базовая выдача ещё собирается в фоне: показываем прогресс и повторяем, когда готова.
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
async function loadSim() {
  await sim.load()
  if (sim.status.value === 'error' && sim.error.value?.status === 503) {
    building.value = building.value ?? { done: 0, total: 0 }
    buildTimer = setTimeout(pollBuild, 2000)
  }
}
onBeforeUnmount(() => clearTimeout(buildTimer))

// Последняя удачная симуляция с датой — показывается при ошибке нового прогона.
// Своя последняя удачная симуляция у общего рычага и у каждой категории.
const lastKey = () => (catId.value ? `${LAST_SIM_BASE}.${catId.value}` : LAST_SIM_BASE)
function readLast(): { at: string; data: MarginSimulateResponse } | null {
  try {
    const raw = window.localStorage.getItem(lastKey())
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
      window.localStorage.setItem(lastKey(), JSON.stringify(entry))
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
// Смена категории — новая симуляция (общий рычаг ↔ рычаг категории).
watch(catId, () => {
  lastGood.value = readLast()
  if (policy.status.value === 'ready') void loadSim()
})

const positions = computed(() => policy.data.value?.positions ?? [])
const globalWeight = computed(() => policy.data.value?.weight ?? 0)
// У категории действует свой вес, иначе — общий.
const active = computed(() => {
  const own = catId.value ? policy.data.value?.categories?.[catId.value] : undefined
  return typeof own === 'number' ? own : globalWeight.value
})
const scope = computed(() => (catId.value ? (simData.value?.scope ?? null) : null))
const categoryName = computed(
  () =>
    scope.value?.category ??
    policy.data.value?.category_names?.[catId.value] ??
    proposalView.value?.category ??
    catId.value,
)
const indexLabel = (v: number | null | undefined) =>
  catId.value ? indexLabel4(v) : marginIndexLabel(v)
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

// ── Выбранное положение: ?w= в адресе, иначе действующее ──
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
const base = computed(() => pointFor(simData.value, active.value))
const affected = computed(() => affectedList(point.value))
const queries = computed(() => simData.value?.judged ?? simData.value?.queries ?? null)
const changed = computed(() => selected.value !== active.value)
const guards = computed(() => guardIndicators(policy.data.value))
// Сверка с предложением агента: те же ли числа даёт симуляция сейчас (до → после).
const match = computed(() => {
  const pv = proposalView.value
  if (!pv || !proposal.data.value || !simData.value) return null
  const ev = proposal.data.value.evaluation as unknown as MarginProposalEval | null
  const from = pv.from ?? active.value
  return proposalMatch(ev, pointFor(simData.value, from), pointFor(simData.value, pv.to))
})
// Применяется предложение: категория и вес совпадают с ним, предложение ещё ждёт решения.
const applyProposal = computed(
  () =>
    proposalPending.value &&
    !!proposalView.value &&
    proposalView.value.categoryId === catId.value &&
    Math.abs(proposalView.value.to - selected.value) < 1e-9,
)
function leaveCategory() {
  void router.replace({ query: { ...route.query, cat: undefined, p: undefined, w: undefined } })
}

// ── «Как посчитано» ──
const explain = ref<Explanation | null>(null)
const explainOpen = computed({
  get: () => explain.value !== null,
  set: (v) => {
    if (!v) explain.value = null
  },
})
const src = computed(
  () =>
    `Симуляция POST /api/margin/simulate на наборе ${simData.value?.set ?? 'tune'}${queries.value ? `, ${queriesLabel(queries.value)}` : ''}${synthetic.value ? '. Маржа в данных синтетическая' : ''}.`,
)
function explainQuality() {
  const p = point.value
  if (!p) return
  explain.value = {
    title: 'Соответствие в первой десятке',
    value: qualityLossText(p.ndcg_delta_mean, baseWeight.value),
    what: 'Насколько выдача с этим весом маржи хуже выдачи только по соответствию запросу.',
    how: `Среднее по размеченным запросам изменение NDCG@10 к выдаче с весом ${weightLabel(baseWeight.value)} (поиск контура поиска). Худший дециль и доля запросов, которые без бюджета ε потеряли бы больше ε, — ниже.`,
    rows: [
      { label: 'Среднее ΔNDCG@10', value: formatDecimal(p.ndcg_delta_mean, 4) },
      { label: 'Худшие 10 % запросов, ΔNDCG@10', value: formatDecimal(p.ndcg_delta_p10, 4) },
      { label: 'Доля запросов за бюджетом ε', value: formatPercent(p.share_over_eps, 1) },
      ...(typeof p.ndcg10 === 'number'
        ? [{ label: 'NDCG@10 положения', value: formatDecimal(p.ndcg10, 3) }]
        : []),
      { label: 'Бюджет ε', value: formatDecimal(policy.data.value?.epsilon ?? 0.01, 3) },
    ],
    source: src.value,
  }
}
function explainIndex() {
  const p = point.value
  if (!p) return
  explain.value = {
    title: 'Индекс показанной маржи',
    value: indexLabel(p.margin_index) ?? '—',
    what: 'Сколько маржи покупатель видит в первой десятке. Это индекс, не рубли и не прогноз продаж.',
    how: 'Маржа позиции — перцентиль внутри категории; сумма по первой десятке с весом по позиции. Отдельно — только по релевантным товарам.',
    rows: [
      { label: 'Индекс, вся десятка', value: indexLabel(p.margin_index) ?? '—' },
      {
        label: 'Индекс, только релевантные',
        value: indexLabel(p.margin_index_relevant) ?? '—',
      },
      ...(base.value
        ? [
            {
              label: `Действующее положение (${weightLabel(active.value)})`,
              value: indexLabel(base.value.margin_index) ?? '—',
            },
          ]
        : []),
    ],
    source: src.value,
  }
}
function explainCount(kind: 'top1' | 'guards') {
  const p = point.value
  if (!p) return
  explain.value =
    kind === 'top1'
      ? {
          title: 'Изменился первый результат',
          value: queriesLabel(p.top1_changed),
          what: 'Запросы, у которых с этим весом маржи первым стал другой товар, чем при выдаче по соответствию.',
          rows: [
            { label: 'Сменился топ-1', value: formatInt(p.top1_changed) },
            { label: 'Изменилась первая тройка', value: formatInt(p.top3_changed) },
          ],
          source: src.value,
        }
      : {
          title: 'Сработало защит',
          value: formatInt(p.guards_fired),
          what: 'Сколько раз ограничения не дали марже переставить позицию: точный запрос, потолок сдвига, группы, совместимость, бюджет качества.',
          source: src.value,
        }
}

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
    const viaProposal = applyProposal.value
    const r = catId.value
      ? await applyMarginCategory(
          { [catId.value]: selected.value },
          comment.value.trim(),
          viaProposal ? proposalId.value : undefined,
        )
      : await applyMarginPolicy(selected.value, comment.value.trim())
    applying.value = false
    applyOpen.value = false
    void postEvent(
      buildEvent(
        'margin_apply',
        { requestId: null, sessionId: app.sessionId, clientId: null },
        {
          payload: { version_before: before, version_after: r.version },
        },
      ),
    ).catch(() => undefined)
    await onChanged(
      r.version,
      `Политика маржи v${r.version}`,
      catId.value
        ? `Вес ${weightLabel(selected.value)} для категории «${categoryName.value}» действует в поиске.${viaProposal ? ' Предложение агента принято.' : ''} Откат — в истории ниже.`
        : `Вес ${weightLabel(selected.value)} действует в поиске. Откат — в истории ниже.`,
    )
    if (viaProposal) {
      await proposal.load({ quiet: true })
      void app.loadPending()
    }
  } catch (e) {
    applying.value = false
    applyErr.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}

async function onChanged(
  version: number,
  title = `Политика маржи v${version}`,
  detail = 'Действует новая версия — копия выбранной.',
) {
  await router.replace({ query: { ...route.query, w: undefined } })
  await policy.load({ quiet: true })
  toast.add({ tone: 'ok', title, detail })
}
</script>

<template>
  <section class="page margin" aria-labelledby="margin-title">
    <header class="margin__head">
      <div>
        <h1 id="margin-title" class="t-h1">Маржа</h1>
        <p class="t-sm t-muted">
          Один рычаг: насколько маржа влияет на порядок внутри группы равных по соответствию
          товаров. Движение рычага — песочница; боевой поиск меняется только после «Применить».
        </p>
      </div>
      <SyntheticBadge v-if="synthetic" />
    </header>

    <!-- Загрузка политики -->
    <div
      v-if="policy.status.value === 'loading' && !policy.data.value"
      class="panel margin__skel"
      aria-busy="true"
    >
      <span class="skeleton" style="width: 30%" />
      <span class="skeleton" style="width: 90%; height: 2.5rem" />
      <span class="skeleton" style="width: 60%" />
    </div>
    <BlockError
      v-else-if="policy.status.value === 'error' && !policy.data.value"
      :error="policy.error.value!"
      title="Политика маржи не загрузилась"
      @retry="loadAll()"
    />

    <template v-else-if="policy.data.value">
      <section v-if="catId" class="panel margin__scope" aria-label="Песочница категории">
        <div class="margin__scope-head">
          <p>
            Рычаг категории <strong>«{{ categoryName }}»</strong>: двигает только её позиции,
            остальные — по действующей политике v{{ policy.data.value.version }} (общий вес
            {{ weightLabel(globalWeight) }}).
            <template v-if="scope">
              Позиции категории — в {{ queriesLabel(scope.queries_in_scope) }} набора, размеченных
              {{ formatInt(scope.judged_in_scope) }}.</template
            >
          </p>
          <button type="button" class="btn btn--secondary btn--sm" @click="leaveCategory">
            Общий рычаг
          </button>
        </div>
        <div v-if="proposalId" class="margin__prop" data-testid="margin-proposal">
          <p class="t-sm">
            <Bot :size="14" aria-hidden="true" />
            <template v-if="proposal.status.value === 'loading' && !proposal.data.value">
              Загружаем предложение агента…
            </template>
            <template v-else-if="proposal.status.value === 'error'">
              Предложение агента не загрузилось: {{ proposal.error.value?.message }}.
              <button type="button" class="link" @click="proposal.load()">Повторить</button>
            </template>
            <template v-else-if="proposalView">
              Предложение агента: {{ weightLabel(proposalView.from ?? active) }} →
              {{ weightLabel(proposalView.to)
              }}<template v-if="proposalView.index">
                · индекс показанной маржи {{ proposalView.index.before }} →
                {{ proposalView.index.after }}</template
              ><template v-if="proposalView.ndcg">
                · NDCG@10 {{ proposalView.ndcg.before }} → {{ proposalView.ndcg.after }}</template
              ><template v-if="proposalView.checked"> · {{ proposalView.checked }}</template
              >.
              <template v-if="!proposalPending">
                Статус:
                {{
                  proposal.data.value?.status === 'accepted' ? 'принято' : 'решение уже принято'
                }}.
              </template>
            </template>
          </p>
          <p
            v-if="match"
            :class="['t-sm margin__match', match.same ? 'status-accepted' : 'status-pending']"
            role="status"
          >
            <component :is="match.same ? CircleCheck : CircleAlert" :size="14" aria-hidden="true" />
            <span v-if="match.same">
              Симуляция даёт те же числа, что в предложении: индекс {{ match.now }}.
            </span>
            <span v-else>
              Числа разошлись с предложением: в предложении индекс {{ match.proposal }}, симуляция
              сейчас {{ match.now }} — базовая выдача изменилась после запуска агента. Решайте по
              текущей симуляции.
            </span>
          </p>
          <RouterLink
            :to="{ path: '/searchops', query: { tab: 'queue', status: 'all', p: proposalId } }"
            class="link t-xs"
            >Карточка в очереди SearchOps</RouterLink
          >
        </div>
      </section>
      <div class="margin__grid">
        <div class="panel margin__lever">
          <MarginLever
            v-model="selected"
            :positions="positions"
            :locked="locked"
            :active="active"
            :epsilon="policy.data.value.epsilon"
            :disabled="!simData || noMargin"
          />
          <p v-if="policy.data.value.enabled === false" class="t-xs margin__off" role="note">
            Политика маржи выключена флагом FEATURE_MARGIN_POLICY: в поиске она сейчас не действует.
          </p>
          <ul class="margin__guards" aria-label="Защиты, которые рычаг не обходит">
            <li v-for="g in guards" :key="g" class="badge">
              <ShieldCheck :size="12" aria-hidden="true" /> {{ g }}
            </li>
          </ul>
          <div class="margin__apply">
            <button
              type="button"
              class="btn btn--primary"
              :disabled="!changed || !point || noMargin"
              @click="askApply"
            >
              Применить вес {{ weightLabel(selected) }}
            </button>
            <span class="t-xs t-muted">
              Действует политика v{{ policy.data.value.version }}, вес {{ weightLabel(active) }}
            </span>
          </div>
        </div>

        <!-- Прогон -->
        <div
          v-if="(sim.status.value === 'loading' || building) && !sim.data.value"
          class="panel margin__running"
          role="status"
        >
          <LoaderCircle :size="18" aria-hidden="true" class="spin" />
          <span v-if="building">
            Собираем базовую выдачу для сравнения<template v-if="building.total">
              : {{ formatInt(building.done) }} из {{ queriesLabel(building.total) }}</template
            >. Это один раз на каталог, обычно до полуминуты.
          </span>
          <span v-else>
            Идёт прогон {{ formatInt(positions.length) }}
            {{ plural(positions.length, ['положения', 'положений', 'положений']) }} рычага на
            размеченном наборе<template v-if="lastGood?.data.judged ?? lastGood?.data.queries">
              ({{
                queriesLabel((lastGood?.data.judged ?? lastGood?.data.queries) as number)
              }})</template
            >…
          </span>
        </div>
        <BlockError
          v-else-if="sim.status.value === 'error' && !simData && !building"
          :error="sim.error.value!"
          title="Симуляция не выполнена"
          @retry="loadSim()"
        />
        <div v-else-if="noMargin" class="panel margin__empty">
          <p class="t-h2">Нет данных о марже</p>
          <p class="t-muted">
            В каталоге нет маржи позиций — рычагу нечего двигать. Он станет доступен после загрузки
            данных с маржой.
          </p>
        </div>
        <div v-else-if="simData" class="panel margin__curve">
          <MarginCurve
            :items="simData.items"
            :bounds="bounds"
            :selected="selected"
            :active="active"
            @select="(w) => (selected = w)"
          />
        </div>
      </div>

      <p v-if="staleSim && lastGood && !building" class="panel margin__stale t-sm" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        <span>
          Новый прогон не выполнен: {{ sim.error.value?.message }}. Показана последняя удачная
          симуляция от {{ formatDateTime(lastGood.at) }}.
          <button type="button" class="link" @click="loadSim()">
            <RotateCcw :size="12" aria-hidden="true" /> Повторить
          </button>
        </span>
      </p>

      <template v-if="point && !noMargin">
        <div class="margin__tiles">
          <StatTile
            label="Соответствие в первой десятке"
            :value="qualityLossText(point.ndcg_delta_mean, baseWeight)"
            :sub="ndcgDeltaLabel(point.ndcg_delta_mean)"
            class="margin__tile--wide"
            @explain="explainQuality"
          />
          <StatTile
            label="Индекс показанной маржи"
            :value="indexLabel(point.margin_index)"
            :sub="
              base && changed
                ? `сейчас ${indexLabel(base.margin_index) ?? '—'}`
                : 'по показам в первой десятке'
            "
            missing="маржа не посчитана"
            @explain="explainIndex"
          />
          <StatTile
            label="Изменился первый результат"
            :value="queriesLabel(point.top1_changed)"
            :sub="`первая тройка: ${formatInt(point.top3_changed)}`"
            @explain="explainCount('top1')"
          />
          <StatTile
            label="Сработало защит"
            :value="formatInt(point.guards_fired)"
            @explain="explainCount('guards')"
          />
        </div>
        <p class="t-xs t-muted margin__note">
          {{ estimateNote(queries, simData?.set) }}
          <template v-if="simData?.ran_at"> Прогон {{ formatDateTime(simData.ran_at) }}.</template>
        </p>
        <AffectedQueries :items="affected" :base="baseWeight" />
      </template>

      <MarginHistory
        :versions="policy.data.value.versions ?? []"
        :active="policy.data.value.version"
        @changed="(v) => onChanged(v)"
      />
    </template>

    <ExplainDialog v-model:open="explainOpen" :explanation="explain" />

    <UiDialog v-model:open="applyOpen" :title="`Применить вес ${weightLabel(selected)}?`">
      <div class="margin__dialog">
        <p>
          Будет создана политика v{{ (policy.data.value?.version ?? 0) + 1 }} с весом
          {{ weightLabel(selected) }} вместо {{ weightLabel(active)
          }}<template v-if="catId">
            для категории «{{ categoryName }}» (общий вес не меняется)</template
          >. Она начнёт действовать в поиске сразу; прежнюю можно вернуть из истории.
          <template v-if="applyProposal">Предложение агента станет принятым.</template>
        </p>
        <p v-if="point" class="t-sm t-muted">
          {{ qualityLossText(point.ndcg_delta_mean, baseWeight) }}; изменится первый результат у
          {{ queriesLabel(point.top1_changed) }}.
        </p>
        <label class="margin__field">
          <span class="t-xs t-muted">Комментарий, если нужно</span>
          <textarea v-model="comment" rows="2" maxlength="500" class="margin__text" />
        </label>
        <p v-if="applyErr" class="margin__err" role="alert">
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
.margin {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.margin__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.margin__head > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 48rem;
}
.margin__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.margin__scope {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.margin__scope-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
}
.margin__scope-head p {
  max-width: 48rem;
  overflow-wrap: anywhere;
}
.margin__prop {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding-top: var(--space-2);
  border-top: var(--border-w) solid var(--border);
  overflow-wrap: anywhere;
}
.margin__prop svg {
  display: inline-block;
  vertical-align: -2px;
}
.margin__match {
  display: flex;
  gap: var(--space-1);
}
.margin__match svg {
  flex: none;
  margin-top: 3px;
  color: var(--tone);
}
.margin__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
  gap: var(--space-3);
  align-items: start;
}
.margin__lever,
.margin__curve,
.margin__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.margin__guards {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.margin__guards .badge {
  white-space: normal;
}
.margin__off {
  color: var(--text-2);
}
.margin__apply {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.margin__running {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-4);
  color: var(--text-2);
}
.margin__stale {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  color: var(--text-2);
}
.margin__stale svg {
  flex: none;
  color: var(--danger);
}
.margin__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 12rem), 1fr));
  gap: var(--space-3);
}
.margin__tile--wide :deep(.tile__value) {
  white-space: normal;
  overflow-wrap: anywhere;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
}
.margin__note {
  max-width: 48rem;
}
.margin__dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.margin__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.margin__text {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
.margin__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
  font-size: var(--fs-sm);
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
