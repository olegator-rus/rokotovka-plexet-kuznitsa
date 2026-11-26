<script setup lang="ts">
// «Изменения»: версии правил, новые сверху; автор, описание, ссылка на предложение,
// разница списком. «Откатить» — диалог; откат создаёт новую версию-копию, история не стирается.
// Вкладка «Связи» (контур коммерции) — справочник связей и его версии: /changes?tab=relations.
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { CircleAlert, CircleCheck, CircleX, History, Undo2 } from 'lucide-vue-next'
import { getConfigs, rollbackConfig } from '@/api/agent'
import type { ConfigVersion } from '@/api/types'
import RelationsTab from '@/components/changes/RelationsTab.vue'
import BlockError from '@/components/common/BlockError.vue'
import PatchDiff from '@/components/searchops/PatchDiff.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { AREA, denyMessage } from '@/lib/auth'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/lib/format'
import { authorLabel, effectLine, sortVersions, versionDescription } from '@/lib/proposals'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
// Откат правил поиска — зона менеджера e-com (и руководителя)
const auth = useAuthStore()
const rollbackDeny = computed(() => (auth.can(AREA.rules) ? '' : denyMessage(AREA.rules)))
const router = useRouter()
const route = useRoute()
const toast = useToast()

type Tab = 'rules' | 'relations'
const tab = computed<Tab>({
  get: () => (route.query.tab === 'relations' ? 'relations' : 'rules'),
  set: (t) =>
    void router.replace({ query: { ...route.query, tab: t === 'rules' ? undefined : t } }),
})

const res = useResource((opts) => getConfigs(opts))
void res.load()

const versions = computed(() => sortVersions(res.data.value?.items ?? []))
const active = computed(() => res.data.value?.active ?? versions.value[0]?.version ?? null)
const onlyInitial = computed(() => versions.value.length <= 1)

const target = ref<ConfigVersion | null>(null)
const open = computed({
  get: () => target.value !== null,
  set: (v) => {
    if (!v && !busy.value) target.value = null
  },
})
const busy = ref(false)
const err = ref('')
const comment = ref('')

/** Эффект версии одной строкой: до правки, обещание прогона, факт на действующих правилах. */
const effectOf = (v: ConfigVersion) => effectLine(v.effect)

/** Откатить версию N = вернуть правила, действовавшие до неё (N−1). */
const rollbackTo = computed(() => (target.value ? target.value.version - 1 : null))

function askRollback(v: ConfigVersion) {
  err.value = ''
  comment.value = ''
  target.value = v
}

async function confirmRollback() {
  if (!target.value || rollbackTo.value === null || rollbackTo.value < 1) return
  busy.value = true
  err.value = ''
  const from = target.value.version
  const to = rollbackTo.value
  try {
    const r = await rollbackConfig(to, comment.value.trim())
    busy.value = false
    target.value = null
    await res.load({ quiet: true })
    void app.loadMeta(true)
    void app.loadPending()
    const back = r?.rolled_back_proposals?.length ?? 0
    toast.add({
      tone: 'ok',
      title: `Версия ${from} откачена`,
      detail: `Действует версия ${r?.version ?? res.data.value?.active ?? '—'} — копия правил версии ${r?.rolled_back_to ?? to}.${back ? ` Откачено предложений: ${back}.` : ''}`,
      action: { label: 'Проверить в поиске', run: () => void router.push('/search') },
    })
  } catch (e) {
    busy.value = false
    err.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}
</script>

<template>
  <section class="page changes" aria-labelledby="changes-title">
    <header class="changes__head">
      <h1 id="changes-title" class="t-h1">Изменения</h1>
      <p class="t-sm t-muted">
        Версии правил поиска и справочника связей. Каждая принятая правка агента — новая версия;
        откат тоже создаёт версию, история остаётся полной.
      </p>
    </header>

    <TabsRoot v-model="tab" class="changes">
      <TabsList class="tabs" aria-label="Что менялось">
        <TabsTrigger value="rules" class="tabs__trigger">Правила поиска</TabsTrigger>
        <TabsTrigger value="relations" class="tabs__trigger">Связи</TabsTrigger>
      </TabsList>

      <TabsContent value="relations" class="changes">
        <RelationsTab />
      </TabsContent>

      <TabsContent value="rules" class="changes">
        <p v-if="active" class="t-sm t-muted">Действует версия правил {{ active }}.</p>

        <div
          v-if="res.status.value === 'loading' && !res.data.value"
          class="changes__list"
          aria-busy="true"
        >
          <div v-for="i in 3" :key="i" class="panel changes__skel">
            <span class="skeleton" style="width: 35%" />
            <span class="skeleton" style="width: 70%" />
          </div>
        </div>

        <BlockError
          v-else-if="res.status.value === 'error'"
          :error="res.error.value!"
          title="Версии правил не загрузились"
          @retry="res.load()"
        />

        <template v-else-if="res.data.value">
          <div v-if="onlyInitial" class="panel changes__empty">
            <History :size="20" aria-hidden="true" class="changes__empty-icon" />
            <div>
              <p class="t-h2">Действует исходная версия правил</p>
              <p class="t-muted">
                Принятые правки агента появятся здесь версиями — с разницей и кнопкой отката.
              </p>
              <RouterLink to="/searchops?tab=queue" class="link t-sm"
                >Открыть очередь предложений</RouterLink
              >
            </div>
          </div>

          <ol class="changes__list">
            <li
              v-for="v in versions"
              :key="v.version"
              :class="['panel', 'ver', { 'ver--active': v.version === active }]"
            >
              <header class="ver__head">
                <span class="ver__num t-num">v{{ v.version }}</span>
                <span v-if="v.version === active" class="badge badge--strong">действует</span>
                <span class="t-sm t-muted">{{ formatDateTime(v.created_at) }}</span>
                <span class="t-sm t-muted">· {{ authorLabel(v.author) }}</span>
                <RouterLink
                  v-if="v.proposal_id"
                  :to="{
                    path: '/searchops',
                    query: { tab: 'queue', status: 'all', p: v.proposal_id },
                  }"
                  class="link t-sm"
                >
                  открыть предложение
                </RouterLink>
                <button
                  v-if="v.version === active && v.version > 1"
                  type="button"
                  class="btn btn--secondary btn--sm ver__rollback"
                  :disabled="!!rollbackDeny"
                  :title="rollbackDeny || undefined"
                  @click="askRollback(v)"
                >
                  <Undo2 :size="14" aria-hidden="true" /> Откатить
                </button>
                <span
                  v-if="v.version === active && v.version > 1 && rollbackDeny"
                  class="t-xs t-muted"
                  >{{ rollbackDeny }}</span
                >
              </header>
              <p v-if="v.description && v.version > 1">{{ versionDescription(v) }}</p>
              <PatchDiff
                v-if="v.version > 1"
                :patch="v.diff"
                empty="Разница с предыдущей версией пуста"
              />
              <p v-else class="t-sm t-muted">{{ versionDescription(v) }}</p>
              <div v-if="effectOf(v)" class="ver__effect">
                <p class="t-xs t-muted">
                  Эффект после применения<template v-if="v.effect?.checked_at">
                    · проверил агент {{ formatDateTime(v.effect.checked_at) }}</template
                  >
                </p>
                <p
                  :class="[
                    'ver__effect-line',
                    effectOf(v)!.holds ? 'status-accepted' : 'status-rejected',
                  ]"
                >
                  <component
                    :is="effectOf(v)!.holds ? CircleCheck : CircleX"
                    :size="14"
                    aria-hidden="true"
                  />
                  <span>
                    {{ effectOf(v)!.text }} — {{ effectOf(v)!.holds ? 'держится' : 'не держится' }}
                  </span>
                </p>
                <ul v-if="v.effect?.queries?.length" class="ver__effect-queries t-sm">
                  <li v-for="q in v.effect.queries" :key="q.text">
                    <span>{{ q.text }}</span>
                    <span :class="q.found ? '' : 't-muted'">{{
                      q.found ? 'находится' : 'нет'
                    }}</span>
                  </li>
                </ul>
              </div>
              <p v-else-if="v.version > 1" class="t-xs t-muted">
                Эффект появится после следующего запуска агента: он сравнит метрики целевого
                кластера.
              </p>
            </li>
          </ol>
        </template>
      </TabsContent>
    </TabsRoot>

    <UiDialog v-model:open="open" :title="`Откатить версию ${target?.version ?? ''}?`">
      <div class="changes__dialog">
        <p>
          Будет создана новая версия — копия правил версии {{ rollbackTo }}. Она начнёт действовать
          в поиске сразу; версия {{ target?.version }} останется в истории.
        </p>
        <label class="changes__field">
          <span class="t-xs t-muted">Комментарий, если нужно</span>
          <textarea v-model="comment" rows="2" maxlength="500" class="changes__text" />
        </label>
        <p v-if="err" class="changes__err" role="alert">
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
  </section>
</template>

<style scoped>
.changes {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.changes__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 48rem;
}
.changes__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.changes__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.changes__empty {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.changes__empty > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
}
.changes__empty-icon {
  flex: none;
  color: var(--text-2);
}
.ver {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.ver--active {
  border-color: var(--border-control);
}
.ver__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.ver__num {
  font-weight: var(--fw-semibold);
}
.ver__rollback {
  margin-left: auto;
}
.ver__effect {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.ver__effect-line {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
}
.ver__effect-line svg {
  flex: none;
  margin-top: 3px;
  color: var(--tone);
}
.ver__effect-queries {
  display: grid;
  grid-template-columns: max-content max-content;
  gap: 2px var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ver__effect-queries li {
  display: contents;
}
.changes__dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.changes__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.changes__text {
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
.changes__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
</style>
