// Геометрия схем «Теоретических основ»: ломаная со скруглёнными углами и наконечник стрелки.
// Координаты — в единицах viewBox; размер на экране задаёт CSS (TheoryDiagram).
import { inject, type InjectionKey } from 'vue'

export type Pt = readonly [number, number]

/** Раскладка схемы: широкая (от 670 px контейнера) или узкая (телефон). Кегль задаёт CSS, здесь — шаг строк. */
export interface DgMetrics {
  mode: 'wide' | 'narrow'
  /** Межстрочный шаг текста в узле. */
  lh: number
}

export const DG_METRICS: Record<DgMetrics['mode'], DgMetrics> = {
  wide: { mode: 'wide', lh: 17 },
  narrow: { mode: 'narrow', lh: 15 },
}

export const DG_KEY: InjectionKey<DgMetrics> = Symbol('theory-diagram')

export function useDg(): DgMetrics {
  return inject(DG_KEY, DG_METRICS.wide)
}

/** Точки из строки «x,y x,y …» (как атрибут points у polyline) или готового массива. */
export function parsePts(pts: string | readonly Pt[]): Pt[] {
  if (typeof pts !== 'string') return [...pts]
  return pts
    .trim()
    .split(/\s+/)
    .map((pair) => {
      const [x, y] = pair.split(',').map(Number)
      return [x ?? 0, y ?? 0] as const
    })
}

/** Центры строк текста, выровненные по вертикали внутри [y, y + h]. */
export function lineCenters(y: number, h: number, count: number, lh: number): number[] {
  const top = y + (h - count * lh) / 2 + lh / 2
  return Array.from({ length: count }, (_, i) => top + i * lh)
}

/** Путь по точкам; на изломах — дуга радиуса r (не больше половины соседних отрезков). */
export function roundedPath(pts: readonly Pt[], r = 6): string {
  const first = pts[0]
  if (!first || pts.length < 2) return ''
  let d = `M${first[0]} ${first[1]}`
  for (let i = 1; i < pts.length - 1; i++) {
    const [px, py] = pts[i - 1]!
    const [cx, cy] = pts[i]!
    const [nx, ny] = pts[i + 1]!
    const l1 = Math.hypot(cx - px, cy - py)
    const l2 = Math.hypot(nx - cx, ny - cy)
    if (l1 === 0 || l2 === 0) continue
    const rr = Math.min(r, l1 / 2, l2 / 2)
    const ax = cx - ((cx - px) / l1) * rr
    const ay = cy - ((cy - py) / l1) * rr
    const bx = cx + ((nx - cx) / l2) * rr
    const by = cy + ((ny - cy) / l2) * rr
    d += ` L${ax} ${ay} Q${cx} ${cy} ${bx} ${by}`
  }
  const last = pts[pts.length - 1]!
  return `${d} L${last[0]} ${last[1]}`
}

/** Линия укорачивается на длину наконечника, наконечник — треугольник с остриём в последней точке. */
export function withHead(pts: readonly Pt[], len = 7, half = 3.6): { line: Pt[]; head: string } {
  const end = pts[pts.length - 1]
  const prev = pts[pts.length - 2]
  if (!end || !prev) return { line: [...pts], head: '' }
  const dx = end[0] - prev[0]
  const dy = end[1] - prev[1]
  const l = Math.hypot(dx, dy) || 1
  const ux = dx / l
  const uy = dy / l
  const bx = end[0] - ux * len
  const by = end[1] - uy * len
  const head = [
    [end[0], end[1]],
    [bx - uy * half, by + ux * half],
    [bx + uy * half, by - ux * half],
  ]
    .map(([x, y]) => `${x},${y}`)
    .join(' ')
  return { line: [...pts.slice(0, -1), [bx, by]], head }
}
