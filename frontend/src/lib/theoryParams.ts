// Параметры алгоритмов для вкладки «Доказательства → Теоретические основы». Это настройки кода, а не замеры:
// источник истины — константы Go в backend/, тест tests/theoryParams.test.ts сверяет каждое значение с ними.
// Поменяли порог в коде — тест упадёт, пока здесь и в тексте вкладки не станет так же.
// Качество, скорость и деньги сюда не входят: они только на соседних вкладках, из прогонов и журналов.

export const THEORY_PARAMS = {
  // Нормализация (search_normalize.go, search_lexicon_qa.go)
  minLayoutWord: 3, // короче — слово само по себе раскладкой не переводится
  typoMinRunes: 4, // короче — опечатка не исправляется
  typoShortMax: 5, // до стольких букв — одна правка, длиннее — две
  minPrefixRunes: 3, // достройка последнего слова — с трёх букв
  minPhoneticRunes: 5,
  minPhoneticKey: 4,

  // Сущности (search_core.go)
  namedCoverageMin: 0.6, // семейство «названо целиком»

  // Сроки (search.go, intent.go)
  searchLimitSec: 10,
  queryEmbedLimitSec: 3,
  intentCallTimeoutSec: 8,

  // Текстовый поиск (search_query.go, search_index.go)
  candidatesTop: 50,
  minimumShouldMatch: '2<-34%',
  tieBreaker: 0.3,
  expansionsBoost: 0.5,
  edgeMinGram: 2,
  edgeMaxGram: 15,
  bm25Fields: [
    'title.exact^5',
    'title.ru^3',
    'title.ngram',
    'family_name^3',
    'aliases^2',
    'vendor^2',
    'sku^5',
    'description',
    'attributes',
  ],
  // Поля текстовой ветки полного пути (B3) — на фиде заказчика название семейства весит больше названия позиции.
  b3Fields: [
    'title.exact^5',
    'title.ru^3',
    'title.ngram',
    'family_name^4',
    'family_name.exact^5',
    'aliases^2',
    'vendor^2',
    'sku^5',
    'category_path',
    'description',
    'attributes',
  ],

  // Векторы (search_index.go, catalog_import.go, config/config.go)
  embeddingDim: 1024,
  embedAllMax: 20_000, // больше позиций в каталоге — вектор по одному на семейство
  hnswM: 16,
  hnswEfConstruction: 128,
  modelCacheSize: 4096,
  modelCacheTtlHours: 6,

  // Слияние и пороги (search_fusion.go, rank.go)
  rrfK: 20,
  rrfKB2: 60,
  relThreshold: 0.3,
  denseOnlyMin: 0.745,
  abbrevDenseMin: 0.7,
  absentDenseMin: 0.76,
  analogBaseRel: 0.5,
  textTopN: 3,
  namedFillMax: 6,
  maxRejected: 60,

  // Реранкер (search_rerank.go, config/config.go)
  rerankTop: 10,
  rerankDescRunes: 300,
  rerankWeight: 0.45,

  // Голова семейства: очки соответствия варианта условиям запроса (rank.go, fit)
  fitExactQuantity: 8, // ровно запрошенное количество
  fitCoversQuantity: 6, // закрывает количество ступенью, поштучно или пакетами
  fitMetric: 4, // та же метрика (устройство, пользователь, ядро)
  fitLicenseType: 3, // названный тип лицензии
  fitDuration: 2, // названный срок
  fitNewLicense: 1, // в запросе нет «продлить» — новая лицензия, а не продление или переход

  // Бизнес-слой (rank.go)
  wRelevance: 0.8,
  wPopularity: 0.1,
  wMargin: 0.05,
  wContext: 0.05,
  licenseFitWeight: 0.5,
  licenseTypeMissFactor: 0.5,
  licenseDurationMissFactor: 0.75,
  tailRatio: 0.7,
  tailRatioSameRoot: 0.5,
  businessQuantity: 10,
  marginBaseWeight: 0.05,
  marginMaxShift: 2,
  maxComplements: 3,

  // Цель ответа и лестница «минимум 4 товара» (search_target.go, search_ladder.go, search_family_pool.go)
  lineMin: 3, // столько равных семейств одного вендора — линейка: цель — вендор или вендор + категория
  ladderMin: 4, // столько разных семейств лестница добирает, если они есть
  maxAlternatives: 3, // карточек «Можно купить сейчас» под продуктом только по запросу
  skuVariantsMax: 3, // других вариантов того же семейства под точным артикулом
  familyPoolMinShare: 0.5, // разных семейств меньше этой доли мест BM25 — текстовый список по семействам

  // Техника (hardware.go, hardware_search.go)
  mpnMinLen: 4,
  mpnPrefixMin: 5,
  exactListWeight: 3,
  structListWeight: 1,
  compatListWeight: 2,
  toleranceScreenInch: 0.7,
  toleranceStorageShare: 0.1,
  tolerancePowerShare: 0.12,

  // Подсказки (suggest.go)
  suggestMinFreq: 2,

  // LLM (config/ops.go)
  dailyBudgetRub: 1000,
  agentReserveRub: 100,
  perIPDailyRub: 20,

  // Агент (agent.go, config/agent.go, cluster.go, agent_plan.go, agent_memory_p5.go, agent_tools.go)
  agentToolCallsPerCluster: 15,
  agentMaxHypotheses: 3,
  agentPatchMaxAliases: 3,
  agentRunSteps: 80,
  agentRunTokens: 600_000,
  agentRunCostRub: 40,
  agentRunMinutes: 25,
  clusterMinFailures: 2,
  clusterMinFailShare: 0.5,
  clusterCosine: 0.8,
  clusterCosineAlone: 0.88,
  planNewFactor: 2,
  effectCheckMax: 5,
  spikeMinSearches: 60,
  spikeEmptyShare: 0.25,
  spikeReformShare: 0.12,
  spikeQueryFreq: 8,
  rulesTtlSec: 10,

  // Оценка (eval_metrics.go, evalsets_split.go)
  evalTopK: 10, // NDCG@10 и MRR@10 — по первым десяти позициям
  relevantGrade: 2,
  shareTune: 0.4,
  shareTest: 0.4,
  shareRegression: 0.2,
} as const

/** Число по-русски: десятичная запятая, тысячи через неразрывный пробел («0,745», «600 000»). */
export function ru(n: number): string {
  const [int, frac] = String(n).split('.')
  const grouped = (int ?? '').replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
  return frac === undefined ? grouped : `${grouped},${frac}`
}

/** Доля в процентах без хвостовых нулей: 0.12 → «12 %». */
export function pct(share: number): string {
  return `${ru(Math.round(share * 1000) / 10)} %`
}

/**
 * Ссылка «открыть в поиске с трассой»: параметры адреса экрана «Поиск» (src/lib/searchState.ts) — q и trace=1.
 * Запросы — только из витрины и демо-набора, которые не зовут модель или уже лежат в кэше разборов.
 */
export function searchWithTrace(query: string): string {
  return `/search?${new URLSearchParams({ q: query, trace: '1' }).toString()}`
}
