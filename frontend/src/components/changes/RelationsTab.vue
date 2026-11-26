<script setup lang="ts">
// «Изменения» → «Связи»: справочник связей с фильтрами, карточка связи с историей версий,
// версии справочника с разницей «было / стало» и откат через диалог (новая версия, история не стирается).
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { CircleAlert, ExternalLink, Link2, Minus, Pencil, Plus, Undo2 } from 'lucide-vue-next'
import { postEvent } from '@/api/events'
import { getRelations, getRelationVersions, rollbackRelations } from '@/api/relations'
import type { Relation, RelationVersion } from '@/api/typesCommerce'
import BlockError from '@/components/common/BlockError.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { buildEvent } from '@/lib/events'
import { formatDateTime } from '@/lib/format'
import { authorLabel, versionDescription } from '@/lib/proposals'
import {
  compatibilityLabel,
  evidenceLabel,
  filterOptions,
  filterRelations,
  needsReview,
  patchSummary,
  relationDiff,
  relationEnds,
  relationHistory,
  relationsCount,
  relationTypeLabel,
  requirementLabel,
  REVIEW_MAX_DAYS,
  reviewLabel,
  sourceLabel,
} from '@/lib/relations'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const toast = useToast()

const rel = useResource((opts) => getRelations({}, opts))
const vers = useResource((opts) => getRelationVersions(opts))
void rel.load()
void vers.load()

const items = computed(() => rel.data.value?.items ?? [])
const staleDays = computed(() => rel.data.value?.stale_after_days ?? REVIEW_MAX_DAYS)
const versions = computed(() =>
  [...(vers.data.value?.items ?? [])].sort((a, b) => b.version - a.version),
)
const active = computed(
  () => vers.data.value?.active ?? rel.data.value?.version ?? versions.value[0]?.version ?? null,
)

// ── Фильтры ──
const ALL = 'all'
const q = ref('')
const fType = ref(ALL)
const fSource = ref(ALL)
const fStatus = ref(ALL)
const opts = (
  key: 'type' | 'source' | 'review_status',
  label: (v: string) => string,
  all: string,
) => [
  { value: ALL, label: all },
  ...filterOptions(items.value, key).map((v) => ({ value: v, label: label(v) })),
]
const typeOptions = computed(() => opts('type', relationTypeLabel, 'Все типы'))
const sourceOptions = computed(() => opts('source', sourceLabel, 'Все источники'))
const statusOptions = computed(() => opts('review_status', reviewLabel, 'Любой статус'))
const shown = computed(() =>
  filterRelations(items.value, {
    q: q.value,
    type: fType.value === ALL ? '' : fType.value,
    source: fSource.value === ALL ? '' : fSource.value,
    status: fStatus.value === ALL ? '' : fStatus.value,
  }),
)
const filtered = computed(
  () => !!q.value.trim() || [fType, fSource, fStatus].some((r) => r.value !== ALL),
)
// Большой справочник — страницами по PAGE строк; разница версии — первые DIFF_SHOWN строк.
const PAGE = 50
const DIFF_SHOWN = 5
const limit = ref(PAGE)
watch([q, fType, fSource, fStatus], () => (limit.value = PAGE))
const page = computed(() => shown.value.slice(0, limit.value))
const expanded = ref(new Set<number>())
function toggleDiff(v: number) {
  const next = new Set(expanded.value)
  if (next.has(v)) next.delete(v)
  else next.add(v)
  expanded.value = next
}
const diffRows = (v: RelationVersion) => relationDiff(v.diff, items.value)

function resetFilters() {
  q.value = ''
  fType.value = fSource.value = fStatus.value = ALL
}

// ── Карточка связи ──
const openRel = ref<Relation | null>(null)
const relOpen = computed({
  get: () => openRel.value !== null,
  set: (v) => {
    if (!v) openRel.value = null
  },
})
const openHistory = computed(() =>
  openRel.value ? relationHistory(versions.value, openRel.value.id) : [],
)

// ── Откат ──
const target = ref<{ to: number; from: number; relationId?: string } | null>(null)
const busy = ref(false)
const err = ref('')
const rbOpen = computed({
  get: () => target.value !== null,
  set: (v) => {
    if (!v && !busy.value) target.value = null
  },
})
function askRollback(v: RelationVersion, relationId?: string) {
  if (v.version <= 1) return
  err.value = ''
  openRel.value = null
  target.value = { from: v.version, to: v.version - 1, relationId }
}
async function confirmRollback() {
  if (!target.value) return
  busy.value = true
  err.value = ''
  const { from, to, relationId } = target.value
  try {
    const r = await rollbackRelations(to)
    busy.value = false
    target.value = null
    void postEvent(
      buildEvent(
        'relation_decision',
        { requestId: null, sessionId: app.sessionId, clientId: null },
        { payload: { relation_id: relationId ?? `version:${from}`, decision: 'rolled_back' } },
      ),
    ).catch(() => undefined)
    await Promise.all([rel.load({ quiet: true }), vers.load({ quiet: true })])
    toast.add({
      tone: 'ok',
      title: `Изменения версии ${from} откачены`,
      detail: `Действует версия ${r?.version ?? '—'} — копия справочника версии ${r?.rolled_back_to ?? to}.`,
    })
  } catch (e) {
    busy.value = false
    err.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}

const diffIcon = (op: string) => (op === 'add' ? Plus : op === 'remove' ? Minus : Pencil)
const diffWord = (op: string) =>
  op === 'add' ? 'добавить' : op === 'remove' ? 'удалить' : 'изменить'
</script>

<template>
  <div class="rels">
    <p class="t-sm t-muted rels__lead">
      Какие связи действуют в блоке «Дополните решение», откуда они и когда проверялись. Принятая
      правка агента или откат — новая версия справочника.
      <template v-if="active">Действует версия {{ active }}.</template>
    </p>

    <!-- Справочник -->
    <div
      v-if="rel.status.value === 'loading' && !rel.data.value"
      class="panel rels__skel"
      aria-busy="true"
    >
      <span v-for="i in 6" :key="i" class="skeleton" :style="{ width: `${95 - i * 6}%` }" />
    </div>
    <BlockError
      v-else-if="rel.status.value === 'error' && !rel.data.value"
      :error="rel.error.value!"
      title="Справочник связей не загрузился"
      @retry="rel.load()"
    />
    <div v-else-if="rel.data.value && !items.length" class="panel rels__empty">
      <Link2 :size="20" aria-hidden="true" class="t-muted" />
      <div>
        <p class="t-h2">Связей нет</p>
        <p class="t-muted">
          Справочник пуст: связи приходят с каталогом или из принятых правок агента.
        </p>
        <RouterLink to="/searchops?tab=queue" class="link t-sm"
          >Открыть очередь предложений</RouterLink
        >
      </div>
    </div>

    <template v-else-if="rel.data.value">
      <div class="rels__filters" role="search" aria-label="Фильтры связей">
        <input
          v-model="q"
          type="search"
          class="rels__q"
          placeholder="Товар или семейство"
          aria-label="Фильтр по товару"
        />
        <UiSelect v-model="fType" :options="typeOptions" label="Тип связи" />
        <UiSelect v-model="fSource" :options="sourceOptions" label="Источник" />
        <UiSelect v-model="fStatus" :options="statusOptions" label="Статус проверки" />
      </div>
      <p class="t-xs t-muted" role="status">
        {{ relationsCount(shown.length)
        }}<template v-if="filtered"> из {{ items.length }}</template> · «проверить» — без проверки
        дольше {{ staleDays }} дней
      </p>
      <p v-if="!shown.length" class="panel rels__empty t-sm">
        Под фильтр не попала ни одна связь.
        <button type="button" class="link" @click="resetFilters">Сбросить фильтры</button>
      </p>
      <div v-else class="data-scroll">
        <table class="data-table rels__table">
          <caption class="sr-only">
            Справочник связей
          </caption>
          <thead>
            <tr>
              <th scope="col">Откуда</th>
              <th scope="col">Куда</th>
              <th scope="col">Тип</th>
              <th scope="col">Обязательность</th>
              <th scope="col">Совместимость</th>
              <th scope="col">Источник</th>
              <th scope="col">Доказательность</th>
              <th scope="col">Проверено</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in page" :key="r.id">
              <th scope="row">
                <button type="button" class="link rels__open" @click="openRel = r">
                  {{ relationEnds(r).from }}
                </button>
              </th>
              <td>{{ relationEnds(r).to }}</td>
              <td>{{ relationTypeLabel(r.type) }}</td>
              <td>{{ requirementLabel(r.requirement) }}</td>
              <td>{{ compatibilityLabel(r.compatibility) }}</td>
              <td>{{ sourceLabel(r.source) }}</td>
              <td class="t-mono">{{ r.evidence_level ?? '—' }}</td>
              <td>
                <span class="t-num">{{ r.reviewed_at ?? '—' }}</span>
                <span v-if="needsReview(r, Date.now(), staleDays)" class="badge rels__review"
                  >проверить</span
                >
                <span v-if="r.review_status && r.review_status !== 'approved'" class="t-xs t-muted">
                  · {{ reviewLabel(r.review_status) }}</span
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <button
        v-if="shown.length > limit"
        type="button"
        class="btn btn--secondary btn--sm rels__more"
        @click="limit += PAGE"
      >
        Показать ещё {{ Math.min(PAGE, shown.length - limit) }} из {{ shown.length - limit }}
      </button>
    </template>

    <!-- Версии -->
    <section class="rels__versions" aria-labelledby="rv-title">
      <h2 id="rv-title" class="t-h2">Версии справочника</h2>
      <div
        v-if="vers.status.value === 'loading' && !vers.data.value"
        class="panel rels__skel"
        aria-busy="true"
      >
        <span v-for="i in 2" :key="i" class="skeleton" :style="{ width: `${70 - i * 10}%` }" />
      </div>
      <BlockError
        v-else-if="vers.status.value === 'error' && !vers.data.value"
        :error="vers.error.value!"
        title="Версии справочника не загрузились"
        @retry="vers.load()"
      />
      <p v-else-if="versions.length <= 1" class="panel rels__empty t-sm t-muted">
        Действует исходный справочник из каталога. Принятые правки связей появятся здесь версиями —
        с разницей и откатом.
      </p>
      <ol v-else class="rels__vlist">
        <li
          v-for="v in versions"
          :key="v.version"
          :class="['panel', 'rv', { 'rv--on': v.version === active }]"
        >
          <header class="rv__head">
            <span class="t-num rv__num">v{{ v.version }}</span>
            <span v-if="v.version === active" class="badge badge--strong">действует</span>
            <span class="t-sm t-muted">{{ formatDateTime(v.created_at) }}</span>
            <span v-if="v.author" class="t-sm t-muted">· {{ authorLabel(v.author) }}</span>
            <span class="t-xs t-muted">· {{ patchSummary(v.diff) }}</span>
            <RouterLink
              v-if="v.proposal_id"
              :to="{ path: '/searchops', query: { tab: 'queue', status: 'all', p: v.proposal_id } }"
              class="link t-sm"
              >открыть предложение</RouterLink
            >
            <button
              v-if="v.version === active && v.version > 1"
              type="button"
              class="btn btn--secondary btn--sm rv__rb"
              @click="askRollback(v)"
            >
              <Undo2 :size="14" aria-hidden="true" /> Откатить
            </button>
          </header>
          <p v-if="v.description" class="t-sm">
            {{ versionDescription({ description: v.description }) }}
          </p>
          <ul v-if="diffRows(v).length" class="rv__diff">
            <li
              v-for="d in expanded.has(v.version) ? diffRows(v) : diffRows(v).slice(0, DIFF_SHOWN)"
              :key="`${d.op}-${d.id}`"
              class="rv__line"
            >
              <component :is="diffIcon(d.op)" :size="14" aria-hidden="true" />
              <span class="sr-only">{{ diffWord(d.op) }}:</span>
              <span class="rv__title">{{ d.title }}</span>
              <span v-if="d.before" class="t-xs t-muted rv__ba">было: {{ d.before }}</span>
              <span v-if="d.after" class="t-xs rv__ba">стало: {{ d.after }}</span>
            </li>
          </ul>
          <button
            v-if="diffRows(v).length > DIFF_SHOWN"
            type="button"
            class="link t-sm rels__more"
            @click="toggleDiff(v.version)"
          >
            {{
              expanded.has(v.version) ? 'Свернуть' : `Показать все ${diffRows(v).length} изменений`
            }}
          </button>
        </li>
      </ol>
    </section>

    <!-- Карточка связи -->
    <UiDialog
      v-model:open="relOpen"
      wide
      :title="openRel ? `${relationEnds(openRel).from} → ${relationEnds(openRel).to}` : ''"
    >
      <div v-if="openRel" class="rd">
        <p v-if="openRel.note">{{ openRel.note }}</p>
        <dl class="rd__fields">
          <dt>Тип</dt>
          <dd>{{ relationTypeLabel(openRel.type) }}</dd>
          <dt>Обязательность</dt>
          <dd>{{ requirementLabel(openRel.requirement) }}</dd>
          <dt>Совместимость</dt>
          <dd>{{ compatibilityLabel(openRel.compatibility) }}</dd>
          <template v-if="openRel.conditions?.length">
            <dt>Условия</dt>
            <dd>{{ openRel.conditions.join(', ') }}</dd>
          </template>
          <template v-if="openRel.quantity_strategy">
            <dt>Количество</dt>
            <dd class="t-mono">{{ openRel.quantity_strategy }}</dd>
          </template>
          <dt>Источник</dt>
          <dd>
            {{ sourceLabel(openRel.source) }}
            <a
              v-if="openRel.source_url"
              :href="openRel.source_url"
              target="_blank"
              rel="noopener noreferrer"
              class="link"
              >основание <ExternalLink :size="12" aria-hidden="true"
            /></a>
          </dd>
          <dt>Доказательность</dt>
          <dd>{{ evidenceLabel(openRel.evidence_level) }}</dd>
          <dt>Проверка</dt>
          <dd>
            {{ reviewLabel(openRel.review_status ?? 'approved') }} ·
            {{ openRel.reviewed_at ?? 'дата не указана' }}
            <span v-if="needsReview(openRel, Date.now(), staleDays)" class="badge rels__review"
              >проверить</span
            >
          </dd>
          <dt>Номер</dt>
          <dd class="t-mono">{{ openRel.id }}</dd>
        </dl>
        <p class="t-xs t-muted">История версий этой связи</p>
        <p v-if="!openHistory.length" class="t-sm t-muted">
          Связь из исходного справочника каталога, правок не было.
        </p>
        <ol v-else class="rd__hist">
          <li v-for="v in openHistory" :key="v.version">
            <span class="t-num">v{{ v.version }}</span>
            <span class="t-sm t-muted">
              {{ formatDateTime(v.created_at)
              }}<template v-if="v.author"> · утвердил: {{ authorLabel(v.author) }}</template>
            </span>
            <button
              v-if="v.version === active && v.version > 1"
              type="button"
              class="btn btn--secondary btn--sm"
              @click="askRollback(v, openRel.id)"
            >
              <Undo2 :size="14" aria-hidden="true" /> Откатить к v{{ v.version - 1 }}
            </button>
          </li>
        </ol>
      </div>
    </UiDialog>

    <!-- Откат -->
    <UiDialog v-model:open="rbOpen" :title="`Откатить версию ${target?.from ?? ''}?`">
      <div class="rels__dialog">
        <p>
          Будет создана новая версия справочника — копия версии {{ target?.to }}. Блок допродажи
          начнёт использовать её сразу; версия {{ target?.from }} останется в истории.
        </p>
        <p v-if="err" class="rels__err" role="alert">
          <CircleAlert :size="16" aria-hidden="true" />
          Откат не выполнен: {{ err }}. Действует версия {{ active }}.
        </p>
      </div>
      <template #footer>
        <button type="button" class="btn btn--secondary" :disabled="busy" @click="target = null">
          Отмена
        </button>
        <button type="button" class="btn btn--primary" :disabled="busy" @click="confirmRollback">
          {{ busy ? 'Откатываем…' : 'Откатить' }}
        </button>
      </template>
    </UiDialog>
  </div>
</template>

<style scoped>
.rels {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.rels__lead {
  max-width: 48rem;
}
.rels__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.rels__empty {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-4);
}
.rels__empty > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
}
.rels__filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.rels__q {
  box-sizing: border-box;
  flex: 1 1 14rem;
  min-width: 0;
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
}
.rels__table th[scope='row'] {
  font-weight: var(--fw-regular);
}
.rels__open {
  text-align: left;
}
.rels__more {
  align-self: flex-start;
}
.rels__review {
  margin-left: var(--space-1);
  color: var(--text);
  border-color: var(--border-control);
}
.rels__versions {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-2);
}
.rels__vlist {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.rv {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.rv--on {
  border-color: var(--border-control);
}
.rv__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.rv__num {
  font-weight: var(--fw-semibold);
}
.rv__rb {
  margin-left: auto;
}
.rv__diff {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.rv__line {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr);
  gap: 0 var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.rv__line > svg {
  align-self: center;
  color: var(--text-2);
}
.rv__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.rv__ba {
  grid-column: 2;
  overflow-wrap: anywhere;
}
.rd {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.rd__fields {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  margin: 0;
  font-size: var(--fs-sm);
}
.rd__fields dt {
  color: var(--text-2);
}
.rd__fields dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.rd__hist {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.rd__hist li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.rels__dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.rels__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
  font-size: var(--fs-sm);
}
</style>
