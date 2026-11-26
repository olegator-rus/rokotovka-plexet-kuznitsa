// Лестница конфигураций поиска. Подписи для переключателя и подсказок.
import type { ConfigId } from '@/api/types'

export const CONFIG_HINTS: Record<ConfigId, string> = {
  live: 'Как у покупателя: B3 на действующей версии правил (с принятыми правками агента)',
  B0: 'Точное совпадение подстроки в названии — как было до проекта',
  B1: 'BM25 по нескольким полям, нормализация и словари',
  B2: 'B1 + векторный поиск BGE-M3 и слияние RRF',
  B3: 'B2 + атрибуты и намерение, порог и бизнес-слой',
  B4: 'B3 + принятые правки агента SearchOps',
}

/** Конфигурации для переключателя на экране «Поиск»: из meta, без meta — B0–B3. */
export const SEARCH_CONFIGS_FALLBACK: readonly ConfigId[] = ['B0', 'B1', 'B2', 'B3']

/** Переключатель: конфигурация по умолчанию (live) первой, если её нет в лестнице из meta. */
export function withDefaultConfig(
  configs: readonly ConfigId[],
  def: ConfigId,
): readonly ConfigId[] {
  return configs.includes(def) ? configs : [def, ...configs]
}

export function configHint(id: ConfigId): string {
  return CONFIG_HINTS[id] ?? id
}
