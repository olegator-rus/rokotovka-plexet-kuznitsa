<script setup lang="ts">
// «Экономика» → «Эффект»: модель чувствительности контура коммерции.
// Три сценария, разложение член за членом (спасённые заказы, допродажа, сдвиг микса, вычеты), источник каждого входа,
// редактируемые допущения (сервер пересчитывает — GET /api/effect/commerce с изменёнными входами), проверочное
// тождество ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG и строка о пилоте. Фронт ничего не считает.
import { computed, reactive, ref } from 'vue'
import { CircleAlert, CircleCheck, ExternalLink, LoaderCircle, RotateCcw } from 'lucide-vue-next'
import { getEffectCommerce } from '@/api/analytics'
import type { EffectCommerceInput } from '@/api/typesKiller'
import BlockError from '@/components/common/BlockError.vue'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import { useResource } from '@/composables/useResource'
import { formatDecimal, formatPercent } from '@/lib/format'
import {
  EFFECT_EDITABLE,
  effectSourceLabel,
  fieldToInput,
  identityText,
  inputText,
  inputToField,
  isShareInput,
  orderEffectScenarios,
  signedRub,
  termRows,
} from '@/lib/killer'

const props = withDefaults(defineProps<{ days?: number }>(), { days: 30 })

const overrides = reactive<Record<string, number>>({})
const fields = reactive<Record<string, string>>({})
const fieldErr = reactive<Record<string, string>>({})
const res = useResource((opts) => getEffectCommerce(props.days, { ...overrides }, opts))
void res.load()

const data = computed(() => res.data.value)
const scenarios = computed(() => orderEffectScenarios(data.value?.scenarios ?? []))
const rows = computed(() => termRows(scenarios.value))
const mechanisms = computed(() => rows.value.filter((r) => r.kind !== 'deduction'))
const deductions = computed(() => rows.value.filter((r) => r.kind === 'deduction'))
const changed = computed(() => Object.keys(overrides).length > 0)
const editable = (i: EffectCommerceInput) => EFFECT_EDITABLE.has(i.key)
const recalculating = computed(() => res.status.value === 'loading' && !!data.value)

function fieldValue(i: EffectCommerceInput): string {
  return fields[i.key] ?? inputToField(i, i.value)
}
function onInput(i: EffectCommerceInput, raw: string) {
  fields[i.key] = raw
}
function commit(i: EffectCommerceInput) {
  const raw = fields[i.key]
  if (raw === undefined) return
  const v = fieldToInput(i, raw)
  if (v === null) {
    fieldErr[i.key] = isShareInput(i) ? 'Доля — от 0 до 100 %' : 'Число не меньше 0'
    return
  }
  delete fieldErr[i.key]
  delete fields[i.key]
  if (i.default !== null && Math.abs(v - i.default) < 1e-9) delete overrides[i.key]
  else overrides[i.key] = v
  void res.load({ quiet: true })
}
function resetAll() {
  for (const k of Object.keys(overrides)) delete overrides[k]
  for (const k of Object.keys(fields)) delete fields[k]
  for (const k of Object.keys(fieldErr)) delete fieldErr[k]
  void res.load()
}
const unitSuffix = (i: EffectCommerceInput) => (isShareInput(i) ? '%' : i.unit)
const multiplier = (i: EffectCommerceInput) => {
  const s = i.scenarios
  if (!s) return null
  const order = ['conservative', 'base', 'optimistic']
  const names: Record<string, string> = {
    conservative: 'конс.',
    base: 'баз.',
    optimistic: 'опт.',
  }
  return order
    .filter((k) => typeof s[k] === 'number')
    .map(
      (k) =>
        `${names[k]} ${i.key === 'rescued_orders_month' ? formatDecimal(s[k], 1) : `× ${formatDecimal(s[k], 1)}`}`,
    )
    .join(' · ')
}
const openInputs = ref(true)
</script>

<template>
  <section class="ef" aria-labelledby="ef-title">
    <!-- Загрузка -->
    <div v-if="res.status.value === 'loading' && !data" class="panel ef__skel" aria-busy="true">
      <span class="skeleton" style="width: 40%" />
      <span class="skeleton" style="width: 90%; height: 4rem" />
      <span class="skeleton" style="width: 70%" />
    </div>

    <BlockError
      v-else-if="res.status.value === 'error' && !data"
      :error="res.error.value!"
      title="Модель эффекта не посчитана"
      @retry="res.load()"
    />

    <template v-else-if="data">
      <header class="ef__head">
        <div>
          <h2 id="ef-title" class="t-h2">{{ data.title }}</h2>
          <p class="t-sm t-muted">{{ data.note }}</p>
        </div>
        <SyntheticBadge v-if="data.synthetic" />
      </header>

      <p v-if="res.status.value === 'error'" class="panel ef__warn t-sm" role="alert">
        <CircleAlert :size="16" aria-hidden="true" />
        <span
          >Пересчёт не выполнен: {{ res.error.value?.message }}. Показан прошлый расчёт.
          <button type="button" class="link" @click="res.load()">Повторить</button></span
        >
      </p>
      <p v-if="recalculating" class="t-sm t-muted ef__busy" role="status">
        <LoaderCircle :size="14" aria-hidden="true" class="spin" /> Пересчитываем…
      </p>

      <div v-if="!scenarios.length" class="panel ef__empty">
        <p class="t-h2">Модель не посчитана</p>
        <p class="t-muted">
          Не хватает данных: {{ data.missing.join('; ') || 'нет заказов поиска за период' }}.
        </p>
        <button v-if="changed" type="button" class="btn btn--secondary btn--sm" @click="resetAll">
          Вернуть входы по умолчанию
        </button>
      </div>

      <template v-else>
        <ul class="ef__scen" aria-label="Сценарии">
          <li
            v-for="s in scenarios"
            :key="s.id"
            class="panel ef__card"
            :data-testid="`scenario-${s.id}`"
          >
            <span class="t-sm t-muted">{{ s.label }}</span>
            <span class="ef__value t-num">{{ signedRub(s.delta_gp_rub_month) }}</span>
            <span class="t-xs t-muted"
              >валовой прибыли в месяц<template v-if="s.uplift !== null">
                · {{ formatPercent(s.uplift, 1) }} к прибыли поиска сейчас</template
              ><template v-if="!s.complete"> · не все члены посчитаны</template></span
            >
            <span
              v-if="s.check"
              :class="['t-xs ef__check', s.check.ok ? 'status-accepted' : 'status-rejected']"
            >
              <component
                :is="s.check.ok ? CircleCheck : CircleAlert"
                :size="12"
                aria-hidden="true"
              />
              {{ s.check.ok ? 'тождество сходится' : 'тождество не сходится' }}
            </span>
          </li>
        </ul>

        <div class="data-scroll">
          <table class="data-table ef__table">
            <caption class="sr-only">
              Разложение эффекта по механизмам и вычетам, рубли валовой прибыли в месяц
            </caption>
            <thead>
              <tr>
                <th scope="col">Член разложения</th>
                <th v-for="s in scenarios" :key="s.id" scope="col" class="num">{{ s.label }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in mechanisms" :key="r.key">
                <th scope="row">
                  {{ r.label }}
                  <span class="ef__formula t-xs t-muted">{{ r.formula }}</span>
                </th>
                <td v-for="(c, i) in r.cells" :key="i" class="num">{{ c }}</td>
              </tr>
              <tr v-if="deductions.length" class="ef__sub">
                <th :colspan="scenarios.length + 1" scope="rowgroup" class="t-xs t-muted">
                  Вычеты
                </th>
              </tr>
              <tr v-for="r in deductions" :key="r.key">
                <th scope="row">
                  {{ r.label }}
                  <span class="ef__formula t-xs t-muted">{{ r.formula }}</span>
                </th>
                <td v-for="(c, i) in r.cells" :key="i" class="num">{{ c }}</td>
              </tr>
              <tr class="ef__total">
                <th scope="row">Итого ΔGP в месяц</th>
                <td v-for="s in scenarios" :key="s.id" class="num">
                  {{ signedRub(s.delta_gp_rub_month) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <section class="panel ef__identity" aria-labelledby="ef-id-title">
          <h3 id="ef-id-title" class="t-sm">Проверочное тождество: {{ data.identity }}</h3>
          <ul class="ef__list">
            <li v-for="s in scenarios" :key="s.id" class="t-sm">
              <span class="t-muted">{{ s.label }}:</span> {{ identityText(s) ?? 'не посчитано' }}
            </li>
          </ul>
          <p class="t-xs t-muted">
            Один товар учитывается либо в сдвиге микса (якорь), либо в допродаже (компаньон).
          </p>
        </section>
      </template>

      <section class="ef__inputs" aria-labelledby="ef-in-title">
        <div class="ef__inputs-head">
          <h3 id="ef-in-title" class="t-h2">Входы модели</h3>
          <button v-if="changed" type="button" class="btn btn--secondary btn--sm" @click="resetAll">
            <RotateCcw :size="14" aria-hidden="true" /> Сбросить изменения
          </button>
          <button type="button" class="link t-sm" @click="openInputs = !openInputs">
            {{ openInputs ? 'Свернуть' : 'Показать' }}
          </button>
        </div>
        <p class="t-xs t-muted">
          Изменённый вход пересчитывается сервером. У каждого входа — источник: замер стенда,
          синтетические данные, допущение или литература со ссылкой.
        </p>
        <ul v-if="openInputs" class="ef__inlist">
          <li v-for="i in data.inputs" :key="i.key" class="panel ef__input">
            <div class="ef__input-top">
              <label :for="`ef-${i.key}`" class="ef__input-label">{{ i.label }}</label>
              <span :class="['badge', i.source === 'request' ? 'status-pending badge--tone' : '']">
                {{ effectSourceLabel(i.source) }}
              </span>
            </div>
            <div v-if="editable(i)" class="ef__field">
              <input
                :id="`ef-${i.key}`"
                class="ef__control t-num"
                inputmode="decimal"
                :value="fieldValue(i)"
                :aria-invalid="!!fieldErr[i.key]"
                :aria-describedby="`ef-${i.key}-ref`"
                @input="onInput(i, ($event.target as HTMLInputElement).value)"
                @change="commit(i)"
                @keydown.enter="commit(i)"
              />
              <span class="t-sm t-muted">{{ unitSuffix(i) }}</span>
            </div>
            <p v-else :id="`ef-${i.key}`" class="t-num">{{ inputText(i) }}</p>
            <p v-if="fieldErr[i.key]" class="ef__err t-xs" role="alert">{{ fieldErr[i.key] }}</p>
            <p :id="`ef-${i.key}-ref`" class="t-xs t-muted">
              {{ i.source_ref }}
              <a
                v-if="i.source_url"
                :href="i.source_url"
                target="_blank"
                rel="noopener noreferrer"
                class="link"
                >источник <ExternalLink :size="11" aria-hidden="true"
              /></a>
            </p>
            <p v-if="multiplier(i)" class="t-xs t-muted">Сценарии: {{ multiplier(i) }}</p>
          </li>
        </ul>
      </section>

      <ul v-if="data.warnings.length" class="ef__list">
        <li v-for="w in data.warnings" :key="w" class="t-xs t-muted">{{ w }}</li>
      </ul>
      <p class="panel ef__pilot t-sm"><span class="t-muted">Пилот:</span> {{ data.pilot }}</p>
    </template>
  </section>
</template>

<style scoped>
.ef {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.ef__skel,
.ef__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.ef__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2);
}
.ef__head > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 48rem;
}
.ef__warn {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.ef__warn svg {
  flex: none;
  color: var(--danger);
}
.ef__busy {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}
.ef__scen {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 14rem), 1fr));
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ef__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
}
.ef__value {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.ef__check {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--tone);
}
.ef__table th[scope='row'] {
  font-weight: var(--fw-regular);
}
.ef__formula {
  display: block;
  overflow-wrap: anywhere;
}
.ef__total th,
.ef__total td {
  font-weight: var(--fw-semibold);
}
.ef__identity {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.ef__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  overflow-wrap: anywhere;
}
.ef__inputs {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ef__inputs-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.ef__inlist {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ef__input {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3);
  min-width: 0;
}
.ef__input-top {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-1) var(--space-2);
}
.ef__input-label {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.ef__input p {
  overflow-wrap: anywhere;
}
.ef__field {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.ef__control {
  box-sizing: border-box;
  width: 9rem;
  max-width: 100%;
  height: var(--control-h);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
}
.ef__control[aria-invalid='true'] {
  border-color: var(--danger);
}
.ef__err {
  color: var(--danger);
}
.ef__pilot {
  padding: var(--space-3) var(--space-4);
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
