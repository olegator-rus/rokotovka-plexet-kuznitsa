<script setup lang="ts">
// Лестница версий поиска B0–B4: каждая версия добавляет к предыдущей один слой.
// Строки — слои, столбцы — версии; закрашенная клетка — слой включён, обведённая — добавлен в этой версии.
import TheoryDiagram from '../diagram/TheoryDiagram.vue'

const VERSIONS = ['B0', 'B1', 'B2', 'B3', 'B4'] as const

/** Слой, который добавляет версия с тем же номером. */
const LAYERS = [
  { wide: ['точная подстрока в названии'], narrow: ['точная подстрока в названии'] },
  { wide: ['BM25 по полям, нормализация, словари'], narrow: ['+ BM25, нормализация, словари'] },
  { wide: ['векторы BGE-M3 и слияние RRF'], narrow: ['+ векторы BGE-M3 и RRF'] },
  {
    wide: ['условия, разбор намерения, порог,', 'бизнес-слой, реранкер'],
    narrow: ['+ условия, разбор намерения,', 'порог, бизнес-слой, реранкер'],
  },
  { wide: ['принятые правки агента'], narrow: ['+ принятые правки агента'] },
] as const

/** Широкая раскладка: строки снизу вверх (слой B0 внизу), высота строки по числу строк подписи. */
const WIDE_ROWS = (() => {
  const rows: { y: number; h: number }[] = []
  let y = 0
  for (let r = LAYERS.length - 1; r >= 0; r--) {
    const h = LAYERS[r]!.wide.length > 1 ? 44 : 30
    rows[r] = { y, h }
    y += h + 8
  }
  return { rows, bottom: y - 8 }
})()
const WIDE_H = WIDE_ROWS.bottom + 44
const CX = (c: number) => 324 + c * 80

/** Узкая раскладка: строка на версию, клетки слоёв — маленькие квадраты. */
const NARROW_ROWS = (() => {
  const rows: { y: number; h: number }[] = []
  let y = 0
  for (let v = 0; v < VERSIONS.length; v++) {
    const lines = LAYERS[v]!.narrow.length + (v === VERSIONS.length - 1 ? 1 : 0)
    const h = lines > 1 ? 50 : 36
    rows.push({ y, h })
    y += h + 8
  }
  return { rows, bottom: y - 8 }
})()

const cellClass = (layer: number, version: number) =>
  layer > version ? 'ld-cell ld-cell--off' : layer === version ? 'ld-cell ld-cell--new' : 'ld-cell'
</script>

<template>
  <TheoryDiagram
    title="Лестница версий поиска B0–B4"
    desc="B0 — точная подстрока в названии. B1 добавляет BM25 по полям, нормализацию и словари. B2 — векторы BGE-M3 и слияние RRF. B3 — условия, разбор намерения, порог, бизнес-слой и реранкер. B4 — принятые правки агента, эта версия работает на витрине. Каждая версия включает все слои предыдущей."
    :wide="[720, WIDE_H]"
    :narrow="[328, NARROW_ROWS.bottom]"
  >
    <template #wide>
      <g v-for="(layer, r) in LAYERS" :key="r">
        <text
          v-for="(line, li) in layer.wide"
          :key="li"
          class="ld-label"
          x="308"
          :y="
            WIDE_ROWS.rows[r]!.y +
            WIDE_ROWS.rows[r]!.h / 2 +
            (li - (layer.wide.length - 1) / 2) * 17
          "
          dy="0.35em"
          text-anchor="end"
        >
          {{ line }}
        </text>
        <template v-for="(v, c) in VERSIONS" :key="v">
          <rect
            :class="cellClass(r, c)"
            :x="CX(c)"
            :y="WIDE_ROWS.rows[r]!.y"
            width="72"
            :height="WIDE_ROWS.rows[r]!.h"
            rx="6"
          />
          <text
            v-if="r === c"
            class="ld-plus"
            :x="CX(c) + 36"
            :y="WIDE_ROWS.rows[r]!.y + WIDE_ROWS.rows[r]!.h / 2"
            dy="0.35em"
            text-anchor="middle"
          >
            {{ c === 0 ? 'база' : '+ слой' }}
          </text>
        </template>
      </g>
      <text
        v-for="(v, c) in VERSIONS"
        :key="v"
        class="ld-ver"
        :x="CX(c) + 36"
        :y="WIDE_ROWS.bottom + 16"
        dy="0.35em"
        text-anchor="middle"
      >
        {{ v }}
      </text>
      <text
        class="ld-live"
        :x="CX(VERSIONS.length - 1) + 36"
        :y="WIDE_ROWS.bottom + 33"
        dy="0.35em"
        text-anchor="middle"
      >
        на витрине
      </text>
    </template>

    <template #narrow>
      <g v-for="(v, vi) in VERSIONS" :key="v">
        <text class="ld-ver" x="0" :y="NARROW_ROWS.rows[vi]!.y + 18" dy="0.35em">
          {{ v }}
        </text>
        <rect
          v-for="(_layer, li) in LAYERS"
          :key="li"
          :class="cellClass(li, vi)"
          :x="34 + li * 17"
          :y="NARROW_ROWS.rows[vi]!.y + 11"
          width="14"
          height="14"
          rx="3"
        />
        <text
          v-for="(line, li) in LAYERS[vi]!.narrow"
          :key="li"
          class="ld-label"
          x="130"
          :y="NARROW_ROWS.rows[vi]!.y + 18 + li * 15"
          dy="0.35em"
        >
          {{ line }}
        </text>
        <text
          v-if="vi === VERSIONS.length - 1"
          class="ld-live"
          x="130"
          :y="NARROW_ROWS.rows[vi]!.y + 33"
          dy="0.35em"
        >
          на витрине
        </text>
      </g>
    </template>

    <template #caption>
      Каждая версия включает все слои предыдущей и добавляет один. Закрашенные клетки складываются в
      лестницу; обведённая — слой, который добавила эта версия.
    </template>
  </TheoryDiagram>
</template>

<style scoped>
.ld-cell {
  fill: var(--border);
  stroke: var(--border);
  stroke-width: 1;
}
.ld-cell--new {
  fill: var(--border);
  stroke: var(--text);
  stroke-width: 1.5;
}
.ld-cell--off {
  fill: none;
  stroke: var(--border);
  stroke-dasharray: 2 3;
}
.ld-label {
  fill: var(--text);
  font-size: var(--dg-fs-sub);
}
.ld-plus {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
  font-weight: var(--fw-semibold);
}
.ld-ver {
  fill: var(--text);
  font-size: var(--dg-fs);
  font-weight: var(--fw-bold);
}
.ld-live {
  fill: var(--text-2);
  font-size: var(--dg-fs-sub);
  font-weight: var(--fw-semibold);
}
</style>
