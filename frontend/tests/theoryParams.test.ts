// Параметры вкладки «Теоретические основы» (src/lib/theoryParams.ts) совпадают с константами Go-кода.
// Текст вкладки берёт числа только из theoryParams.ts: поменяли порог в backend/ — этот тест падает, пока
// параметры (и текст рядом с ними) не приведены к коду.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { THEORY_PARAMS, pct, ru, searchWithTrace } from '../src/lib/theoryParams.ts'

const go = (path: string) => readFileSync(new URL(`../../backend/${path}`, import.meta.url), 'utf8')

/** Параметр → файл и выражение с одной группой — значение в коде. */
const SOURCES: Record<string, [string, RegExp]> = {
  minLayoutWord: ['services/search_normalize.go', /const minLayoutWord = (\d+)/],
  typoMinRunes: [
    'services/search_normalize.go',
    /if len\(rs\) < (\d+) \|\| knownWord\(t, lx, rules\)/,
  ],
  typoShortMax: ['services/search_normalize.go', /maxD := 1\s+if len\(rs\) > (\d+) \{\s+maxD = 2/],
  minPrefixRunes: ['services/search_lexicon_qa.go', /const minPrefixRunes = (\d+)/],
  minPhoneticRunes: ['services/search_lexicon_qa.go', /minPhoneticRunes = (\d+)/],
  minPhoneticKey: ['services/search_lexicon_qa.go', /minPhoneticKey\s+= (\d+)/],
  namedCoverageMin: ['services/search_core.go', /const namedCoverageMin = ([\d.]+)/],
  searchLimitSec: ['services/search.go', /searchLimit\s+= (\d+) \* time\.Second/],
  queryEmbedLimitSec: ['services/search.go', /queryEmbedLimit = (\d+) \* time\.Second/],
  intentCallTimeoutSec: ['services/intent.go', /intentCallTimeout = (\d+) \* time\.Second/],
  candidatesTop: ['services/search_query.go', /const candidatesTop = (\d+)/],
  tieBreaker: [
    'services/search_query.go',
    /"tie_breaker": ([\d.]+),\s+"minimum_should_match": minMatch/,
  ],
  expansionsBoost: ['services/search_query.go', /"boost": ([\d.]+),/],
  edgeMinGram: [
    'services/search_index.go',
    /"edge":\s+map\[string\]any\{"type": "edge_ngram", "min_gram": (\d+)/,
  ],
  edgeMaxGram: ['services/search_index.go', /"edge_ngram", "min_gram": 2, "max_gram": (\d+)\}/],
  embeddingDim: ['services/search_index.go', /embeddingDim = (\d+)/],
  embedAllMax: ['services/catalog_import.go', /embedAllMax = ([\d_]+)/],
  hnswM: ['services/search_index.go', /"m": (\d+), "ef_construction"/],
  hnswEfConstruction: ['services/search_index.go', /"ef_construction": (\d+)/],
  modelCacheSize: ['config/config.go', /defaultModelCacheSize = (\d+)/],
  modelCacheTtlHours: ['config/config.go', /defaultModelCacheTTL\s+= (\d+) \* time\.Hour/],
  rrfK: ['services/rank.go', /b3RRFK = (\d+)/],
  rrfKB2: ['services/search_fusion.go', /const rrfK = (\d+)/],
  relThreshold: ['services/rank.go', /relThreshold = ([\d.]+)/],
  denseOnlyMin: ['services/rank.go', /denseOnlyMin = ([\d.]+)/],
  abbrevDenseMin: ['services/rank.go', /abbrevDenseMin = ([\d.]+)/],
  absentDenseMin: ['services/rank.go', /absentDenseMin = ([\d.]+)/],
  analogBaseRel: ['services/rank.go', /analogBaseRel = ([\d.]+)/],
  textTopN: ['services/rank.go', /const textTopN = (\d+)/],
  namedFillMax: ['services/search_negation.go', /const namedFillMax = (\d+)/],
  maxRejected: ['services/rank.go', /maxRejected = (\d+)/],
  rerankTop: ['services/search_rerank.go', /rerankTop = (\d+)/],
  rerankDescRunes: ['services/search_rerank.go', /rerankDescRunes = (\d+)/],
  rerankWeight: ['config/config.go', /const defaultRerankWeight = ([\d.]+)/],
  fitExactQuantity: ['services/rank.go', /case q == a\.quantity:\s+score \+= (\d+)/],
  fitCoversQuantity: ['services/rank.go', /case quantityOK\(d, a\.quantity\):\s+score \+= (\d+)/],
  fitMetric: ['services/rank.go', /if \*d\.LicenseMetric == a\.metric \{\s+score \+= (\d+)/],
  fitLicenseType: ['services/rank.go', /if d\.LicenseType == a\.licType \{\s+score \+= (\d+)/],
  fitDuration: ['services/rank.go', /\*d\.LicenseDuration == a\.duration \{\s+score \+= (\d+)/],
  // score++ — одно очко: в коде нет числа, поэтому «++» читается как 1.
  fitNewLicense: [
    'services/rank.go',
    /d\.LicenseType != domain\.LicenseUpgrade \{\s+score(\+\+) \/\/ без «продлить»/,
  ],
  wRelevance: ['services/rank.go', /businessWeights = map\[string\]float64\{"relevance": ([\d.]+)/],
  wPopularity: ['services/rank.go', /businessWeights = .*"popularity": ([\d.]+)/],
  wMargin: ['services/rank.go', /businessWeights = .*"margin": ([\d.]+)/],
  wContext: ['services/rank.go', /businessWeights = .*"context": ([\d.]+)/],
  licenseFitWeight: ['services/rank.go', /licenseFitWeight = ([\d.]+)/],
  licenseTypeMissFactor: ['services/rank.go', /licenseTypeMissFactor\s+= ([\d.]+)/],
  licenseDurationMissFactor: ['services/rank.go', /licenseDurationMissFactor = ([\d.]+)/],
  tailRatio: ['services/rank.go', /tailRatio\s+= ([\d.]+)/],
  tailRatioSameRoot: ['services/rank.go', /tailRatioSameRoot = ([\d.]+)/],
  businessQuantity: ['services/rank.go', /const businessQuantity = (\d+)/],
  marginBaseWeight: ['services/margin.go', /marginBaseWeight = ([\d.]+)/],
  marginMaxShift: ['services/margin.go', /marginMaxShift\s+= (\d+)/],
  maxComplements: ['services/search_b3.go', /const maxComplements = (\d+)/],
  mpnMinLen: ['services/hardware.go', /mpnMinLen\s+= (\d+)/],
  mpnPrefixMin: ['services/hardware.go', /mpnPrefixMin = (\d+)/],
  exactListWeight: ['services/hardware_search.go', /exactListWeight = ([\d.]+)/],
  structListWeight: ['services/hardware_search.go', /structListWeight = ([\d.]+)/],
  compatListWeight: ['services/hardware_search.go', /compatListWeight = ([\d.]+)/],
  toleranceScreenInch: [
    'services/hardware_search.go',
    /approxTolerance = map\[string\]float64\{"screen_inch": ([\d.]+)\}/,
  ],
  toleranceStorageShare: [
    'services/hardware_search.go',
    /approxShare = map\[string\]float64\{"storage_gb": ([\d.]+)/,
  ],
  tolerancePowerShare: ['services/hardware_search.go', /approxShare = .*"power_w": ([\d.]+)\}/],
  suggestMinFreq: ['services/suggest.go', /suggestMinFreq\s+= (\d+)/],
  dailyBudgetRub: ['config/ops.go', /defaultDailyBudgetRub\s+= (\d+)/],
  agentReserveRub: ['config/ops.go', /defaultAgentReserveRub = (\d+)/],
  perIPDailyRub: ['config/ops.go', /defaultPerIPDailyRub\s+= (\d+)/],
  agentToolCallsPerCluster: ['services/agent.go', /agentToolCallsPerCluster = (\d+)/],
  agentMaxHypotheses: ['services/agent.go', /agentMaxHypotheses\s+= (\d+)/],
  agentPatchMaxAliases: [
    'services/agent_tools.go',
    /len\(args\.Add\) == 0 \|\| len\(args\.Add\) > (\d+)/,
  ],
  agentRunSteps: ['config/agent.go', /BudgetConfig\{Steps: (\d+)/],
  agentRunTokens: ['config/agent.go', /BudgetConfig\{.*Tokens: ([\d_]+)/],
  agentRunCostRub: ['config/agent.go', /BudgetConfig\{.*CostRub: (\d+)/],
  agentRunMinutes: ['config/agent.go', /BudgetConfig\{.*Duration: (\d+) \* time\.Minute/],
  clusterMinFailures: ['services/cluster.go', /clusterMinFailures\s+= (\d+)/],
  clusterMinFailShare: ['services/cluster.go', /clusterMinFailShare = ([\d.]+)/],
  clusterCosine: ['services/cluster.go', /clusterCosine\s+= ([\d.]+)/],
  clusterCosineAlone: ['services/cluster.go', /clusterCosineAlone = ([\d.]+)/],
  planNewFactor: ['services/agent_plan.go', /const planNewFactor = ([\d.]+)/],
  effectCheckMax: ['services/agent_memory_p5.go', /const effectCheckMax = (\d+)/],
  spikeMinSearches: ['config/agent.go', /MinSearches: (\d+)/],
  spikeEmptyShare: ['config/agent.go', /EmptyShare: ([\d.]+)/],
  spikeReformShare: ['config/agent.go', /ReformShare: ([\d.]+)/],
  spikeQueryFreq: ['config/agent.go', /QueryFreq: (\d+)/],
  rulesTtlSec: ['services/search_core.go', /const rulesTTL = (\d+) \* time\.Second/],
  evalTopK: ['services/eval_metrics.go', /evalTopK\s+= (\d+)/],
  relevantGrade: ['services/eval_metrics.go', /relevantGrade\s+= (\d+)/],
  shareTune: ['services/evalsets_split.go', /\{domain\.EvalSetTune, ([\d.]+)\}/],
  shareTest: ['services/evalsets_split.go', /\{domain\.EvalSetTest, ([\d.]+)\}/],
  shareRegression: ['services/evalsets_split.go', /\{domain\.EvalSetRegression, ([\d.]+)\}/],
}

test('каждый числовой параметр вкладки сверяется с кодом', () => {
  const numeric = Object.entries(THEORY_PARAMS).filter(([, v]) => typeof v === 'number')
  for (const [key] of numeric) assert.ok(SOURCES[key], `у параметра ${key} нет источника в Go`)
})

for (const [key, [file, re]] of Object.entries(SOURCES)) {
  test(`${key} = ${file}`, () => {
    const m = go(file).match(re)
    assert.ok(m?.[1], `в ${file} не найдено ${re}`)
    const want = m[1] === '++' ? 1 : Number(m[1].replace(/_/g, ''))
    assert.equal(THEORY_PARAMS[key as keyof typeof THEORY_PARAMS], want, `${key}: в коде ${want}`)
  })
}

test('minimum_should_match B3 = search_query.go', () => {
  const m = go('services/search_query.go').match(/const b3MinMatch = "([^"]+)"/)
  assert.equal(THEORY_PARAMS.minimumShouldMatch, m?.[1])
})

test('поля и веса BM25 = search_query.go', () => {
  const m = go('services/search_query.go').match(/var bm25Fields = \[\]string\{([^}]+)\}/)
  assert.ok(m?.[1])
  const fields = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1])
  assert.deepEqual([...THEORY_PARAMS.bm25Fields], fields)
})

test('поля и веса BM25 полного пути = search_query.go', () => {
  const m = go('services/search_query.go').match(/var b3Fields = \[\]string\{([^}]+)\}/)
  assert.ok(m?.[1])
  const fields = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1])
  assert.deepEqual([...THEORY_PARAMS.b3Fields], fields)
})

test('веса бизнес-слоя в сумме дают 1', () => {
  const p = THEORY_PARAMS
  assert.equal(Math.round((p.wRelevance + p.wPopularity + p.wMargin + p.wContext) * 100), 100)
})

test('ru и pct: запятая, неразрывные пробелы', () => {
  assert.equal(ru(0.745), '0,745')
  assert.equal(ru(600000), '600 000')
  assert.equal(ru(20), '20')
  assert.equal(pct(0.12), '12 %')
  assert.equal(pct(0.25), '25 %')
})

test('ссылка в поиск с трассой — параметры q и trace=1 экрана «Поиск»', () => {
  assert.equal(
    searchWithTrace('винда 11 про'),
    '/search?q=%D0%B2%D0%B8%D0%BD%D0%B4%D0%B0+11+%D0%BF%D1%80%D0%BE&trace=1',
  )
  const src = readFileSync(new URL('../src/lib/searchState.ts', import.meta.url), 'utf8')
  assert.match(src, /if \(state\.trace\) out\.trace = '1'/)
  assert.match(src, /trace: first\(query\.trace\) === '1'/)
})
