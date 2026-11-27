#!/usr/bin/env python3
"""Прогон отложенного набора с ручной разметкой (data/softline/eval/holdout.csv) через API поиска.

Разметка — по семействам: target_family_id — цель (оценка 3), allowed_families — допустимые альтернативы
(оценка 2, через «;»), остальное — 0. Считает по каждой конфигурации:
  - «верное семейство первым» — у первой карточки семейство цели (строки с целью, кроме группы absent);
  - NDCG@10 по семействам — семейства в выдаче по первому вхождению, идеал — цель и альтернативы;
  - долю пустых выдач по всем размеченным строкам;
  - для группы absent («товара нет в каталоге»): честный ответ — пустая выдача, ответ «нет в каталоге» или первая
    карточка из allowed_families.
Строки без разметки (пустые target_family_id и allowed_families вне группы absent) пропускаются и считаются.

Поиски идут с заголовком X-Test-Run: e2e — в журнал и аналитику не пишутся, набор остаётся отложенным.

    python3 scripts/eval/holdout_eval.py [--csv data/softline/eval/holdout.csv] [--api http://127.0.0.1:8088]
        [--configs B3,B4] [--json out.json]
"""
import argparse
import csv
import json
import math
import sys
import urllib.request
from collections import defaultdict

ORIGIN = 'http://localhost:5177'


def search(api, query, config):
    body = json.dumps({'query': query, 'config': config, 'limit': 50}).encode('utf-8')
    req = urllib.request.Request(api.rstrip('/') + '/api/search', data=body, method='POST', headers={
        'Content-Type': 'application/json', 'Origin': ORIGIN, 'X-Test-Run': 'e2e'})
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def families(resp):
    seen, seq = set(), []
    for it in resp.get('items') or []:
        f = str(it.get('family_id') or it.get('product_id') or '')
        if f and f not in seen:
            seen.add(f)
            seq.append(f)
    return seq


def dcg(grades):
    return sum((2 ** g - 1) / math.log2(i + 2) for i, g in enumerate(grades[:10]))


def ndcg(seq, target, allowed):
    ideal = ([3] if target else []) + [2] * len(allowed)
    best = dcg(ideal)
    if best == 0:
        return None
    grades = [3 if f == target else 2 if f in allowed else 0 for f in seq]
    return dcg(grades) / best


def split_families(raw):
    return {x.strip() for x in (raw or '').replace(',', ';').split(';') if x.strip()}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--csv', default='data/softline/eval/holdout.csv')
    ap.add_argument('--api', default='http://127.0.0.1:8088')
    ap.add_argument('--configs', default='B3,B4')
    ap.add_argument('--json', help='куда записать результаты по запросам (JSON)')
    a = ap.parse_args()

    rows = list(csv.DictReader(open(a.csv, encoding='utf-8')))
    labelled, skipped = [], []
    for r in rows:
        r['target'] = (r.get('target_family_id') or '').strip()
        r['allowed'] = split_families(r.get('allowed_families'))
        if r['target'] or r['allowed'] or r['group'] == 'absent':
            labelled.append(r)
        else:
            skipped.append(r['query'])
    if not labelled:
        sys.exit(f'В {a.csv} нет размеченных строк: заполните target_family_id (см. README рядом с набором)')

    configs = [c.strip().upper() for c in a.configs.split(',') if c.strip()]
    per_query, summary = [], {}
    for cfg in configs:
        acc = defaultdict(list)
        for r in labelled:
            resp = search(a.api, r['query'], cfg)
            seq = families(resp)
            empty = not seq
            res = {'config': cfg, 'query': r['query'], 'group': r['group'], 'target': r['target'],
                   'first_family': seq[0] if seq else None, 'empty': empty}
            acc['empty'].append(empty)
            if r['group'] == 'absent':
                ok = empty or resp.get('empty') is not None or (bool(seq) and seq[0] in r['allowed'])
                res['honest'] = ok
                acc['absent_ok'].append(ok)
            else:
                if r['target']:
                    hit = bool(seq) and seq[0] == r['target']
                    res['family_first'] = hit
                    acc['family_first'].append(hit)
                    acc['family_first:' + r['group']].append(hit)
                n = ndcg(seq, r['target'], r['allowed'])
                if n is not None:
                    res['ndcg10_family'] = round(n, 4)
                    acc['ndcg'].append(n)
            per_query.append(res)
        mean = lambda v: sum(v) / len(v) if v else None
        summary[cfg] = {k: (mean(v), len(v)) for k, v in acc.items()}

    def cell(cfg, key, pct=True):
        v, n = summary[cfg].get(key, (None, 0))
        if v is None:
            return '—'
        return (f'{v * 100:.1f} %' if pct else f'{v:.3f}') + f' (n={n})'

    print(f'Отложенный набор: {len(rows)} строк, размечено {len(labelled)}, без разметки {len(skipped)}')
    print('| Метрика | ' + ' | '.join(configs) + ' |')
    print('| --- |' + ' --- |' * len(configs))
    print('| Верное семейство первым | ' + ' | '.join(cell(c, 'family_first') for c in configs) + ' |')
    print('| NDCG@10 по семействам | ' + ' | '.join(cell(c, 'ndcg', False) for c in configs) + ' |')
    print('| Пустые выдачи | ' + ' | '.join(cell(c, 'empty') for c in configs) + ' |')
    print('| Товара нет: честный ответ | ' + ' | '.join(cell(c, 'absent_ok') for c in configs) + ' |')
    groups = sorted({k.split(':', 1)[1] for c in configs for k in summary[c] if k.startswith('family_first:')})
    for g in groups:
        print(f'| — верное семейство первым, {g} | ' + ' | '.join(cell(c, 'family_first:' + g) for c in configs) + ' |')
    if a.json:
        with open(a.json, 'w', encoding='utf-8') as f:
            json.dump({'summary': summary, 'per_query': per_query, 'skipped': skipped}, f, ensure_ascii=False, indent=2)


if __name__ == '__main__':
    main()
