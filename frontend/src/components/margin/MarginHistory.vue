<script setup lang="ts">
// История версий политики маржи и откат: откат через диалог создаёт новую версию-копию,
// история не стирается. Версии приходят в GET /api/margin/policy (versions); v0 — исходная, вес контура поиска.
import { computed, ref } from 'vue'
import { CircleAlert, Undo2 } from 'lucide-vue-next'
import { rollbackMarginPolicy } from '@/api/margin'
import type { MarginPolicyVersion } from '@/api/typesCommerce'
import UiDialog from '@/components/ui/UiDialog.vue'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { weightLabel } from '@/lib/margin'
import { authorLabel } from '@/lib/proposals'
import { AREA, denyMessage } from '@/lib/auth'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ versions: MarginPolicyVersion[]; active: number }>()
const emit = defineEmits<{ changed: [version: number] }>()

const list = computed(() => [...props.versions].sort((a, b) => b.version - a.version))
const SOURCE_LABELS: Record<string, string> = {
  default: 'исходная',
  human: 'применена человеком',
  rollback: 'откат',
  agent: 'по предложению агента',
}

const target = ref<MarginPolicyVersion | null>(null)
const busy = ref(false)
const err = ref('')
const open = computed({
  get: () => target.value !== null,
  set: (v) => {
    if (!v && !busy.value) target.value = null
  },
})

function ask(v: MarginPolicyVersion) {
  err.value = ''
  target.value = v
}

async function confirm() {
  if (!target.value) return
  busy.value = true
  err.value = ''
  try {
    const r = await rollbackMarginPolicy(target.value.version)
    busy.value = false
    target.value = null
    emit('changed', r.version)
  } catch (e) {
    busy.value = false
    err.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}
// Возврат версии политики маржи — зона менеджера e-com (и руководителя)
const auth = useAuthStore()
const deny = computed(() => (auth.can(AREA.margin) ? '' : denyMessage(AREA.margin)))
</script>

<template>
  <section class="hist" aria-labelledby="hist-title">
    <h2 id="hist-title" class="t-h2">История политики</h2>
    <p v-if="list.length <= 1" class="panel hist__empty t-sm t-muted">
      Действует исходная политика. Применённые положения рычага появятся здесь версиями.
    </p>
    <ol v-else class="hist__list">
      <li
        v-for="v in list"
        :key="v.version"
        :class="['panel', 'hist__row', { 'hist__row--on': v.version === active }]"
      >
        <span class="t-num hist__v">v{{ v.version }}</span>
        <span v-if="v.version === active" class="badge badge--strong">действует</span>
        <span class="t-sm"
          >вес <span class="t-num">{{ weightLabel(v.weight) }}</span></span
        >
        <span class="t-xs t-muted"
          >{{ formatDateTime(v.created_at)
          }}<template v-if="v.author"> · {{ authorLabel(v.author) }}</template
          ><template v-if="v.source && SOURCE_LABELS[v.source]">
            · {{ SOURCE_LABELS[v.source] }}</template
          ></span
        >
        <span v-if="v.comment" class="t-xs t-muted hist__c">«{{ v.comment }}»</span>
        <button
          v-if="v.version !== active"
          type="button"
          class="btn btn--secondary btn--sm hist__btn"
          :disabled="!!deny"
          :title="deny || undefined"
          @click="ask(v)"
        >
          <Undo2 :size="14" aria-hidden="true" /> Вернуть
        </button>
      </li>
    </ol>

    <UiDialog v-model:open="open" :title="`Вернуть политику v${target?.version ?? ''}?`">
      <p>
        Будет создана новая версия с весом {{ target ? weightLabel(target.weight) : '' }} — копия
        версии {{ target?.version }}. Она начнёт действовать в поиске сразу; история останется.
      </p>
      <p v-if="err" class="hist__err" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        Откат не выполнен: {{ err }}. Действует версия {{ active }}.
      </p>
      <template #footer>
        <button type="button" class="btn btn--secondary" :disabled="busy" @click="target = null">
          Отмена
        </button>
        <button type="button" class="btn btn--primary" :disabled="busy" @click="confirm">
          {{ busy ? 'Возвращаем…' : 'Вернуть' }}
        </button>
      </template>
    </UiDialog>
  </section>
</template>

<style scoped>
.hist {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.hist__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
}
.hist__empty {
  padding: var(--space-3) var(--space-4);
}
.hist__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.hist__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  padding: var(--space-2) var(--space-3);
}
.hist__row--on {
  border-color: var(--border-control);
}
.hist__v {
  font-weight: var(--fw-semibold);
}
.hist__c {
  overflow-wrap: anywhere;
}
.hist__btn {
  margin-left: auto;
}
.hist__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
  font-size: var(--fs-sm);
}
</style>
