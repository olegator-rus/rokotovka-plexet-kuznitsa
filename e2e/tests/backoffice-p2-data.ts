// Ответы API контура коммерции бэк-офиса — для e2e, пока бэкенд-треки не готовы
// (E2E_P2_LIVE=1 — ходить в живой API). Числа здесь — тестовые входы экрана, не результаты замеров.
import type { Page, Route } from '@playwright/test'

export const POSITIONS = [0, 0.03, 0.05, 0.08, 0.12, 0.15]

export const policy = {
  version: 1,
  weight: 0.05,
  positions: POSITIONS,
  guards: { exact_top1: true, max_shift: 2, compatibility: true },
  epsilon: 0.01,
  max_shift: 2,
  base_weight: 0.05,
  stop_weight: 0.08,
  synthetic: true,
  enabled: true,
  versions: [
    { version: 0, weight: 0.05, source: 'default', author: 'system', comment: '', created_at: null },
    { version: 1, weight: 0.05, source: 'human', author: 'human', comment: '', created_at: '2026-05-27T08:00:00Z' },
  ],
}

// Положения 0..5; i = 2 — вес контура поиска (0,05), относительно него изменения качества.
const point = (w: number, i: number) => ({
  weight: w,
  available: i <= 3,
  ndcg10: 0.85 - 0.002 * (i - 2),
  ndcg_delta_mean: -0.002 * (i - 2),
  ndcg_delta_p10: -0.006 * i,
  share_over_eps: i >= 4 ? 0.05 : 0,
  margin_index: 1 + 0.1 * i,
  margin_index_relevant: 1 + 0.08 * i,
  top1_changed: i,
  top3_changed: 2 * i,
  guards_fired: 3 * i,
  affected:
    i === 0
      ? []
      : [
          {
            query: 'антивирус для бизнеса на 50 пк',
            top1_changed: true,
            top3_changed: true,
            ndcg_delta: -0.004,
            before: [
              { product_id: 'kes-50', title: 'Kaspersky Endpoint Security, 50 узлов' },
              { product_id: 'drweb-50', title: 'Dr.Web Desktop Security Suite, 50 ПК' },
              { product_id: 'eset-50', title: 'ESET PROTECT Entry, 50 устройств' },
            ],
            after: [
              { product_id: 'drweb-50', title: 'Dr.Web Desktop Security Suite, 50 ПК' },
              { product_id: 'kes-50', title: 'Kaspersky Endpoint Security, 50 узлов' },
              { product_id: 'eset-50', title: 'ESET PROTECT Entry, 50 устройств' },
            ],
            guards: ['max_shift'],
          },
          {
            query: 'офисный пакет для дома',
            top1_changed: false,
            top3_changed: false,
            ndcg_delta: -0.009,
            before: [
              { product_id: 'p1', title: 'Офисный контур поиска' },
              { product_id: 'p2', title: 'Офисный контур коммерции' },
            ],
            after: [
              { product_id: 'p2', title: 'Офисный контур коммерции', moved: 1 },
              { product_id: 'p1', title: 'Офисный контур поиска', moved: -1 },
            ],
          },
        ],
})

export const simulation = {
  set: 'tune',
  queries: 140,
  judged: 120,
  base_weight: 0.05,
  synthetic: true,
  stop_weight: 0.08,
  min_weight: 0,
  items: POSITIONS.map((w, i) => point(w, i)),
}

const relBroken = {
  id: 'rel-0001',
  from_family: 'windows-server-2025',
  to_family: 'windows-server-cal-2022',
  from_name: 'Windows Server 2025 Standard',
  to_name: 'Windows Server CAL 2022',
  type: 'complement',
  note: 'Для подключения пользователей нужны клиентские лицензии',
  requirement: 'conditional_required',
  compatibility: 'unknown',
  conditions: ['access_license_required'],
  quantity_strategy: 'per_user',
  source: 'catalog_rule',
  evidence_level: 'E3',
  review_status: 'approved',
  reviewed_at: '2024-08-03',
}
const relFixed = {
  ...relBroken,
  to_family: 'windows-server-cal-2025',
  to_name: 'Windows Server CAL 2025',
  compatibility: 'verified',
  source: 'vendor_documentation',
  evidence_level: 'E2',
  reviewed_at: '2026-05-27',
}
const relMem = {
  id: 'rel-0002',
  from_family: 'dell-poweredge-r740',
  to_family: 'ddr4-rdimm-32',
  from_name: 'Dell PowerEdge R740',
  to_name: 'Память DDR4 RDIMM 32 ГБ',
  type: 'compatibility',
  requirement: 'optional',
  compatibility: 'verified',
  source: 'vendor_documentation',
  evidence_level: 'E2',
  review_status: 'approved',
  reviewed_at: '2026-05-22',
}

export const relations = {
  version: 2,
  materialized: true,
  stale_after_days: 365,
  total: 2,
  items: [{ ...relFixed, needs_review: false }, { ...relMem, needs_review: false }],
}
export const relationVersions = {
  active: 2,
  items: [
    { version: 1, created_at: '2026-05-27T08:00:00Z', author: 'system', source: 'import', diff: null },
    {
      version: 2,
      created_at: '2026-05-27T10:00:00Z',
      author: 'agent+human',
      source: 'agent',
      proposal_id: 'p2-rel-1',
      description: 'BROKEN_RELATION: CAL 2022 → CAL 2025',
      diff: { add: [], remove: [], update: [{ before: relBroken, after: relFixed }] },
    },
  ],
}

const cfg = (config: string, metrics: Record<string, number | null>, extra = {}) => ({
  config,
  rules_version: 3,
  catalog_version: 'mock-2',
  metrics,
  block: null,
  regressions: [],
  ...extra,
})

export const commerce = {
  run_id: 'r1',
  set: 'p2',
  set_hash: 'a1b2c3d4e5f6a7b8',
  queries: 120,
  pairs: 40,
  commit: '63200a4f00',
  started_at: '2026-05-27T09:30:00Z',
  repro: 'set=p2 hash=a1b2c3d4 n=120 pairs=40 commit=63200a4',
  synthetic: true,
  ready: { hardware: true, companions: true, margin: false },
  ladder: [
    cfg('C0', { mpn_top1: 0.4, units_parse_acc: 0.1, ndcg10: 0.81, compat_violations: 4 }),
    cfg('C1', { mpn_top1: 0.95, units_parse_acc: 0.9, ndcg10: 0.86, compat_violations: 1 }),
    cfg(
      'C2',
      {
        mpn_top1: 0.95,
        units_parse_acc: 0.9,
        ndcg10: 0.86,
        compat_violations: 0,
        required_recall: 0.9,
        coverage: 0.65,
        block_precision_judge: null,
      },
      {
        block: { violations: [] },
        regressions: [
          { text: 'сервер dell r740', against: 'C1', ndcg_from: 0.9, ndcg_to: 0.85, ndcg_delta: -0.05 },
        ],
      },
    ),
    cfg('C3', { ndcg10: null }),
  ],
  single_changes: [
    cfg(
      'C1+companions',
      { ndcg10: 0.858, compat_violations: 0, coverage: 0.65 },
      {
        base: 'C1',
        changed: '+companions',
        against: 'C1',
        regressions: [{ text: 'сервер dell r740', ndcg_from: 0.9, ndcg_to: 0.85 }],
      },
    ),
  ],
  notes: ['C3: политика маржи ещё не дала шага в трассе — строка без чисел'],
}

export const relationProposal = {
  id: 'p2-rel-1',
  run_id: 'run-p2',
  cluster: { id: 'c-rel', label: 'Windows Server 2025 → CAL', queries: [], lost_demand: 0 },
  diagnosis: 'BROKEN_RELATION',
  diagnosis_text: 'К Windows Server 2025 привязан CAL 2022: правило вендора требует CAL той же версии или новее.',
  role: 'product',
  action: 'relation_patch',
  patch: { add: [], remove: [], update: [relFixed] },
  evaluation: {
    resolved: true,
    findings_before: 1,
    findings_after: 0,
    new_findings: [],
    block: {
      pairs: 40,
      enabled: true,
      before: { required_recall: 0.5, compat_violations: 1 },
      after: { required_recall: 1, compat_violations: 0 },
    },
    regression: { set: 'regression', total: 120, worse: 0, better: 0, block_changed: 3 },
    passed: true,
    reasons: [],
  },
  evidence: {
    relation_before: relBroken,
    finding_text: 'К Windows Server 2025 привязан CAL 2022',
    license_rules: [{ id: 'lr-ws-cal', reason: 'версия CAL не ниже версии сервера' }],
    technical_note: 'техническая корректность проверена кодом',
  },
  evidence_level: 'E2',
  technical_status: 'verified',
  economic_status: 'unknown',
  hypotheses_tried: 1,
  status: 'pending',
  created_at: '2026-05-27T09:40:00Z',
  decided_at: null,
  decided_reason: null,
  config_version: null,
}

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })

/** Подменить API контура коммерции бэк-офиса контрактными ответами. Возвращает журнал мутирующих запросов. */
export async function mockBackofficeP2(page: Page) {
  const posts: { path: string; body: unknown }[] = []
  if (process.env.E2E_P2_LIVE === '1') return posts
  await page.route(/\/api\/margin\/policy$/, (r) => {
    if (r.request().method() === 'POST') {
      posts.push({ path: '/api/margin/policy', body: r.request().postDataJSON() })
      return json(r, { version: 2, weight: 0.08, version_before: 1, weight_before: 0.05 }, 201)
    }
    return json(r, policy)
  })
  await page.route(/\/api\/margin\/simulate$/, (r) => json(r, simulation))
  // Статус сбора: набор с разметкой есть (на фиде без queries.json живой API отвечает no_set: true).
  await page.route(/\/api\/margin\/simulate\/status$/, (r) =>
    json(r, { running: false, done: 0, total: 0, no_set: false }),
  )
  await page.route(/\/api\/relations(\?.*)?$/, (r) => json(r, relations))
  await page.route(/\/api\/relations\/versions$/, (r) => json(r, relationVersions))
  await page.route(/\/api\/relations\/rollback$/, (r) => {
    posts.push({ path: '/api/relations/rollback', body: r.request().postDataJSON() })
    return json(r, { version: 3, rolled_back_to: 1 })
  })
  await page.route(/\/api\/evaluations\/commerce(\?.*)?$/, (r) => json(r, commerce))
  await page.route(/\/api\/proposals(\?.*)?$/, (r) => json(r, { items: [relationProposal] }))
  return posts
}
