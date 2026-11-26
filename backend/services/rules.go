package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"go.uber.org/zap"
)

// Версии словарей и правил поиска (config_versions): действующая версия — последняя строка.
// Принятая правка агента и откат создают новую версию поверх последней; гонку двух писателей ловит
// проверка версии в транзакции (repositories.ApplyRulesChange). B4 видит новую версию сразу: кеш правил
// поиска сбрасывается.

// Тексты отказов.
const (
	MsgNoAgentStore     = "Сервис запущен без базы данных: очередь и версии правил недоступны"
	MsgRulesConflict    = "Правила изменились, пока вы решали: обновите страницу и проверьте снова"
	MsgProposalDecided  = "По предложению уже принято решение"
	MsgRollbackNotFound = "Такой версии правил нет"
	MsgRollbackLatest   = "Эта версия уже действует"
)

// ruleAlias — алиас в снимке правил (формат config_versions.rules.aliases).
type ruleAlias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	TargetID  string `json:"target_id"`
}

// ruleSet — разобранная версия правил.
type ruleSet struct {
	Version        int
	CatalogVersion string
	Aliases        []ruleAlias
}

// RulePatch — правка правил: добавить и удалить алиасы.
type RulePatch struct {
	Add    []PatchAlias `json:"add"`
	Remove []PatchAlias `json:"remove"`
}

// PatchAlias — односторонний алиас «как пишут → как в каталоге».
type PatchAlias struct {
	Kind      string `json:"kind"` // alias
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Target    string `json:"target"`
	TargetID  string `json:"target_id"`
	// AliasKind — slang | abbreviation | transliteration | alias.
	AliasKind string `json:"alias_kind,omitempty"`
}

// aliasKey — ключ алиаса: слова в нижнем регистре через пробел (как в поиске).
func aliasKey(a string) string { return strings.Join(strings.Fields(foldText(a)), " ") }

// patchKey — «алиас→цель» для памяти: одна и та же правка узнаётся между запусками.
func patchKey(a PatchAlias) string {
	t := a.TargetID
	if t == "" {
		t = aliasKey(a.Canonical)
	}
	return aliasKey(a.Alias) + "→" + a.Target + ":" + t
}

// patchKeys — ключи всех добавлений правки.
func patchKeys(p RulePatch) []string {
	var ks []string
	for _, a := range p.Add {
		ks = append(ks, patchKey(a))
	}
	for _, a := range p.Remove {
		ks = append(ks, "-"+aliasKey(a.Alias))
	}
	sort.Strings(ks)
	return ks
}

// parseRules — версия правил из строки config_versions.
func parseRules(v *domain.ConfigVersion) (*ruleSet, error) {
	var body struct {
		CatalogVersion string      `json:"catalog_version"`
		Aliases        []ruleAlias `json:"aliases"`
	}
	if len(v.Rules) > 0 {
		if err := json.Unmarshal(v.Rules, &body); err != nil {
			return nil, err
		}
	}
	return &ruleSet{Version: v.Version, CatalogVersion: body.CatalogVersion, Aliases: body.Aliases}, nil
}

// rulesJSON — снимок для config_versions.rules и песочницы.
func (rs *ruleSet) rulesJSON() domain.JSONB {
	b, _ := json.Marshal(map[string]any{"catalog_version": rs.CatalogVersion, "aliases": rs.Aliases})
	return b
}

// snapshot — снимок правил для SearchOptions.Rules.
func (rs *ruleSet) snapshot() *domain.RulesSnapshot {
	return &domain.RulesSnapshot{Version: rs.Version, Rules: rs.rulesJSON()}
}

// find — алиас по ключу.
func (rs *ruleSet) find(alias string) *ruleAlias {
	k := aliasKey(alias)
	for i := range rs.Aliases {
		if aliasKey(rs.Aliases[i].Alias) == k {
			return &rs.Aliases[i]
		}
	}
	return nil
}

// applyPatch — новая версия правил: удаления, затем добавления (существующий ключ заменяется).
func (rs *ruleSet) applyPatch(p RulePatch) *ruleSet {
	drop := map[string]bool{}
	for _, a := range p.Remove {
		drop[aliasKey(a.Alias)] = true
	}
	for _, a := range p.Add {
		drop[aliasKey(a.Alias)] = true
	}
	next := &ruleSet{Version: rs.Version, CatalogVersion: rs.CatalogVersion}
	for _, a := range rs.Aliases {
		if !drop[aliasKey(a.Alias)] {
			next.Aliases = append(next.Aliases, a)
		}
	}
	for _, a := range p.Add {
		kind := a.AliasKind
		if kind == "" {
			kind = "alias"
		}
		next.Aliases = append(next.Aliases, ruleAlias{Alias: strings.TrimSpace(strings.ToLower(a.Alias)),
			Canonical: strings.TrimSpace(a.Canonical), Kind: kind, Target: a.Target, TargetID: a.TargetID})
	}
	return next
}

// rulesDiff — разница двух версий списком (не сырым JSON).
func rulesDiff(prev, next *ruleSet) RulePatch {
	d := RulePatch{Add: []PatchAlias{}, Remove: []PatchAlias{}}
	pm := map[string]ruleAlias{}
	for _, a := range prev.Aliases {
		pm[aliasKey(a.Alias)] = a
	}
	nm := map[string]ruleAlias{}
	for _, a := range next.Aliases {
		nm[aliasKey(a.Alias)] = a
	}
	for _, a := range next.Aliases {
		if p, ok := pm[aliasKey(a.Alias)]; !ok || p != a {
			d.Add = append(d.Add, PatchAlias{Kind: "alias", Alias: a.Alias, Canonical: a.Canonical, Target: a.Target,
				TargetID: a.TargetID, AliasKind: a.Kind})
		}
	}
	for _, a := range prev.Aliases {
		if n, ok := nm[aliasKey(a.Alias)]; !ok || n != a {
			d.Remove = append(d.Remove, PatchAlias{Kind: "alias", Alias: a.Alias, Canonical: a.Canonical, Target: a.Target,
				TargetID: a.TargetID, AliasKind: a.Kind})
		}
	}
	return d
}

// activeRules — действующая (последняя) версия правил. Без версий в базе — исходные словари каталога.
func (s *Services) activeRules(ctx context.Context) (*ruleSet, error) {
	if st := s.store(); st != nil {
		vs, err := st.ListConfigVersions(ctx)
		if err != nil {
			return nil, err
		}
		if len(vs) > 0 {
			return parseRules(&vs[0])
		}
	}
	rs := &ruleSet{Version: 1}
	if cs := s.catalog.Load(); cs != nil {
		rs.CatalogVersion = cs.version
		for _, a := range cs.lex.baseAliases {
			rs.Aliases = append(rs.Aliases, ruleAlias{Alias: a.Alias, Canonical: a.Canonical, Kind: a.Kind,
				Target: a.Target, TargetID: a.TargetID})
		}
	}
	return rs, nil
}

// rulesChanged — новая версия действует сразу: кеш правил B4 сброшен, /api/meta видит номер.
func (s *Services) rulesChanged(version int) {
	s.core.latest.Store(nil)
	if st := s.catalog.Load(); st != nil {
		next := *st
		next.rulesVersion = version
		s.catalog.Store(&next)
	}
}

// rulesError — гонки в понятные HTTP-ошибки.
func rulesError(err error) error {
	switch {
	case errors.Is(err, domain.ErrRulesVersionConflict):
		return errs.NewConflictError(MsgRulesConflict).WithCause(err)
	case errors.Is(err, domain.ErrProposalDecided):
		return errs.NewConflictError(MsgProposalDecided).WithCause(err)
	}
	return err
}

// Configs — GET /api/configs: версии правил, новые сверху, с разницей.
func (s *Services) Configs(ctx context.Context) (*out.ConfigList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	vs, err := st.ListConfigVersions(ctx)
	if err != nil {
		return nil, err
	}
	res := &out.ConfigList{Items: []out.ConfigVersion{}}
	for i := range vs {
		v := vs[i]
		if res.Active == 0 && !v.Archived {
			res.Active = v.Version
		}
		rs, _ := parseRules(&v)
		author := v.Author
		if author == "" {
			author = "system"
		}
		diff := json.RawMessage(v.Diff)
		if len(diff) == 0 {
			diff = json.RawMessage(`{"add":[],"remove":[]}`)
		}
		item := out.ConfigVersion{Version: v.Version, CreatedAt: v.CreatedAt, Author: author, Source: v.Source,
			Description: v.Comment, ProposalID: v.ProposalID, Diff: diff, Effect: rawOrNull(v.Effect),
			CatalogVersion: v.CatalogVersion, Archived: v.Archived}
		if rs != nil {
			item.Aliases = len(rs.Aliases)
		}
		res.Items = append(res.Items, item)
	}
	return res, nil
}

// RollbackConfig — POST /api/configs/rollback: новая версия = копия выбранной; принятые после неё
// предложения становятся «откачено». История не стирается.
func (s *Services) RollbackConfig(ctx context.Context, req in.RollbackRequest) (*out.ConfigRollback, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	if req.ToVersion < 1 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "to_version", Code: errs.CodeInvalidFormat, Message: "Укажите номер версии"})
	}
	target, err := st.GetConfigVersion(ctx, req.ToVersion)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errs.NewNotFoundError(MsgRollbackNotFound)
	}
	cur, err := s.activeRules(ctx)
	if err != nil {
		return nil, err
	}
	if cur.Version == req.ToVersion {
		return nil, errs.NewConflictError(MsgRollbackLatest)
	}
	to, err := parseRules(target)
	if err != nil {
		return nil, err
	}
	diff, _ := json.Marshal(rulesDiff(cur, to))
	comment := fmt.Sprintf("Откат к версии %d", req.ToVersion)
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	cv := &domain.ConfigVersion{Version: cur.Version + 1, Source: "rollback", Author: "human", Comment: comment,
		Rules: to.rulesJSON(), Diff: diff}
	ch := &repositories.RulesChange{Base: cur.Version, Version: cv, RollBackAbove: req.ToVersion,
		Decision: &domain.Decision{Comment: truncateRunes(strings.TrimSpace(req.Comment), 300)}}
	if err := st.ApplyRulesChange(ctx, ch); err != nil {
		return nil, rulesError(err)
	}
	s.rulesChanged(cv.Version)
	res := &out.ConfigRollback{Version: cv.Version, RolledBackTo: req.ToVersion, Proposals: []string{}}
	for _, d := range ch.RolledBack {
		res.Proposals = append(res.Proposals, d.ProposalID.String())
	}
	zap.L().Info("правила: откат", zap.Int("version", cv.Version), zap.Int("to", req.ToVersion),
		zap.Int("proposals", len(res.Proposals)))
	return res, nil
}

// rawOrNull — пустой JSONB → null.
func rawOrNull(j domain.JSONB) json.RawMessage {
	if len(j) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(j)
}

// truncateRunes — строка не длиннее n символов.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
