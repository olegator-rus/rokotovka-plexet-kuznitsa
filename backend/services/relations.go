package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"go.uber.org/zap"
)

// Справочник связей и его версии (контур коммерции): config_versions.kind =
// relations. Первая версия цепочки — связи каталога как есть; дальше — принятые правки агента (relation_patch)
// и откаты. Действующая версия подменяет связи в справочниках контура коммерции в памяти (catalogState.commerce):
// поиск и блок допродажи видят новую связь сразу после принятия, без переимпорта. Импорт с другими связями
// начинает цепочку заново (прежняя — archived при следующей записи).

// Тексты отказов справочника связей.
const (
	MsgRelationsNoStore       = "Сервис запущен без базы данных: версии справочника связей недоступны"
	MsgRelationsNoCatalog     = "Каталог не загружен — справочник связей пуст (make import)"
	MsgRelationsVersionAbsent = "Такой версии справочника связей нет"
	MsgRelationsVersionLatest = "Эта версия справочника связей уже действует"
	MsgRelationsArchived      = "Эта версия справочника связей собрана для прежнего каталога — откатиться к ней нельзя"
	MsgRelationsConflict      = "Справочник связей изменился, пока вы решали: обновите страницу и проверьте снова"
)

// relationStaleDays — связь без проверки дольше этого срока помечается «проверить».
const relationStaleDays = 365

// relationDoc — связь в форме файла relations.json: так она хранится в версиях, уходит в API и в
// правку агента. ID пустой у связей контура поиска — заменяется устойчивым "p1:<from>><to>:<type>".
type relationDoc struct {
	ID               string   `json:"id"`
	FromFamily       string   `json:"from_family"`
	ToFamily         string   `json:"to_family"`
	FromProduct      *string  `json:"from_product"`
	ToProduct        *string  `json:"to_product"`
	Type             string   `json:"type"`
	Note             string   `json:"note"`
	Requirement      string   `json:"requirement"`
	Compatibility    string   `json:"compatibility"`
	Conditions       []string `json:"conditions"`
	QuantityStrategy string   `json:"quantity_strategy"`
	Source           string   `json:"source"`
	SourceURL        string   `json:"source_url"`
	EvidenceLevel    string   `json:"evidence_level"`
	ReviewStatus     string   `json:"review_status"`
	ReviewedAt       string   `json:"reviewed_at"`
	UpgradeAdds      []string `json:"upgrade_adds"`
}

// RelationPatch — правка справочника связей: новые связи, id удаляемых, изменённые связи целиком.
type RelationPatch struct {
	Add    []relationDoc `json:"add"`
	Remove []string      `json:"remove"`
	Update []relationDoc `json:"update"`
}

func (p RelationPatch) empty() bool {
	return len(p.Add) == 0 && len(p.Remove) == 0 && len(p.Update) == 0
}

// relationsBody — содержимое версии (config_versions.rules).
type relationsBody struct {
	Relations []relationDoc `json:"relations"`
}

func docOfRelation(r domain.Relation) relationDoc {
	d := relationDoc{ID: r.RelID, FromFamily: r.FromFamily, ToFamily: r.ToFamily, FromProduct: r.FromProduct,
		ToProduct: r.ToProduct, Type: r.Type, Note: r.Note, Requirement: r.Requirement, Compatibility: r.Compatibility,
		Conditions: r.Conditions, QuantityStrategy: r.QuantityStrategy, Source: r.Source, SourceURL: r.SourceURL,
		EvidenceLevel: r.EvidenceLevel, ReviewStatus: r.ReviewStatus, ReviewedAt: r.ReviewedAt, UpgradeAdds: r.UpgradeAdds}
	if d.ID == "" {
		d.ID = "p1:" + r.FromFamily + ">" + r.ToFamily + ":" + r.Type
	}
	return d.normalized()
}

// normalized — значения по умолчанию, как у импорта (catalogsource.validate): пустые списки — [], а не null.
func (d relationDoc) normalized() relationDoc {
	if d.Requirement == "" {
		d.Requirement = domain.RequirementOptional
	}
	if d.Compatibility == "" {
		d.Compatibility = domain.CompatUnknown
	}
	if d.Conditions == nil {
		d.Conditions = []string{}
	}
	if d.UpgradeAdds == nil {
		d.UpgradeAdds = []string{}
	}
	return d
}

func (d relationDoc) relation() domain.Relation {
	return domain.Relation{RelID: d.ID, FromFamily: d.FromFamily, ToFamily: d.ToFamily, FromProduct: d.FromProduct,
		ToProduct: d.ToProduct, Type: d.Type, Note: d.Note, Requirement: d.Requirement, Compatibility: d.Compatibility,
		Conditions: d.Conditions, QuantityStrategy: d.QuantityStrategy, Source: d.Source, SourceURL: d.SourceURL,
		EvidenceLevel: d.EvidenceLevel, ReviewStatus: d.ReviewStatus, ReviewedAt: d.ReviewedAt, UpgradeAdds: d.UpgradeAdds}
}

func relationsOfDocs(ds []relationDoc) []domain.Relation {
	out := make([]domain.Relation, len(ds))
	for i, d := range ds {
		out[i] = d.relation()
	}
	return out
}

func docsOfRelations(rs []domain.Relation) []relationDoc {
	out := make([]relationDoc, len(rs))
	for i, r := range rs {
		out[i] = docOfRelation(r)
	}
	return out
}

// sameEnds — две связи об одном и том же (концы, позиции, тип): дубль в справочнике.
func (d relationDoc) sameEnds(o relationDoc) bool {
	return d.FromFamily == o.FromFamily && d.ToFamily == o.ToFamily && d.Type == o.Type &&
		derefStr(d.FromProduct) == derefStr(o.FromProduct) && derefStr(d.ToProduct) == derefStr(o.ToProduct)
}

// relationsFingerprint — отпечаток набора связей без учёта порядка: по нему цепочка версий узнаёт свой каталог.
func relationsFingerprint(ds []relationDoc) string {
	ids := make([]string, 0, len(ds))
	for _, d := range ds {
		b, _ := json.Marshal(d.normalized())
		ids = append(ids, string(b))
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:8])
}

// withRelations — справочники контура коммерции с другим набором связей (остальное — то же): индексы пересобираются
// тем же конструктором, что при импорте.
func (c *commerceCatalog) withRelations(rels []domain.Relation) *commerceCatalog {
	snap := &domain.CatalogSnapshot{Relations: rels, LicenseRules: c.licenseRules, CompatibilityRules: c.compatRules,
		Synthetic: c.synthetic}
	ids := make([]string, 0, len(c.products))
	for id := range c.products {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		snap.Products = append(snap.Products, *c.products[id])
	}
	cats := make([]string, 0, len(c.attrDefs))
	for k := range c.attrDefs {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	for _, k := range cats {
		snap.AttributeDefinitions = append(snap.AttributeDefinitions, domain.AttributeDefinition{CategoryID: k, Attrs: c.attrDefs[k]})
	}
	return newCommerceCatalog(snap)
}

type commerceOverrideKey struct{}

// withCommerceOverride — поиски в ctx видят справочники контура коммерции с правкой песочницы (evaluate_companions),
// боевые не меняются.
func withCommerceOverride(ctx context.Context, c *commerceCatalog) context.Context {
	return context.WithValue(ctx, commerceOverrideKey{}, c)
}

// commerceFor — справочники контура коммерции для запроса: песочница агента из ctx, иначе действующие. Блоки контура коммерции
// читают связи через неё — тогда прогон правки связей в песочнице меняет и их ответ.
func (s *Services) commerceFor(ctx context.Context) *commerceCatalog {
	if ctx != nil {
		if c, ok := ctx.Value(commerceOverrideKey{}).(*commerceCatalog); ok && c != nil {
			return c
		}
	}
	if cs := s.catalog.Load(); cs != nil {
		return cs.commerce
	}
	return nil
}

// relationStore — хранилище версий справочника связей (*repositories.Repos; в тестах — память).
type relationStore interface {
	ListRelationVersions(ctx context.Context) ([]domain.ConfigVersion, error)
	ApplyRelationsChange(ctx context.Context, ch *repositories.RelationsChange) error
}

func (s *Services) relStore() relationStore {
	st := s.store()
	if st == nil {
		return nil
	}
	rs, _ := st.(relationStore)
	return rs
}

// relationChain — действующий справочник связей и его цепочка версий.
type relationChain struct {
	CatalogVersion string
	// Base — связи каталога (импорт); Active — действующие (последняя версия цепочки или Base).
	Base   []relationDoc
	Active []relationDoc
	// Version — номер действующей версии; Materialized=false — версий цепочки ещё нет (Version — номер, который
	// получит исходная версия).
	Version      int
	Materialized bool
	// Max — наибольший номер версии relations в базе (архивные тоже); Stale — есть версии прежнего каталога,
	// не архивные (при записи — в архив).
	Max      int
	Stale    bool
	Versions []domain.ConfigVersion // не архивные версии этой цепочки, новые сверху
	// Reimported — каталог той же версии переимпортирован с другими связями (QA r2 №3): цепочка остаётся живой,
	// новые связи каталога ждут записи версией import поверх неё (ReloadRelations); Prev — связи последней версии.
	Reimported bool
	Prev       []relationDoc
}

// relationsBaseline — связи каталога как импортированы: из базы, иначе из источника.
func (s *Services) relationsBaseline(ctx context.Context) (string, []relationDoc, error) {
	var snap *domain.CatalogSnapshot
	if s.repos.Enabled() {
		var err error
		if snap, err = s.repos.LoadCatalog(ctx); err != nil {
			return "", nil, err
		}
	}
	if snap == nil && s.source != nil {
		var err error
		if snap, err = s.source.Load(ctx); err != nil {
			return "", nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
		}
	}
	if snap == nil {
		return "", nil, errs.NewUnavailableError(MsgRelationsNoCatalog)
	}
	return snap.Version, docsOfRelations(snap.Relations), nil
}

// relationChain — цепочка версий текущего каталога. Цепочка «своя», если её первая версия совпадает со связями
// каталога по отпечатку; иначе (переимпорт с другими связями) она считается устаревшей.
func (s *Services) relationChain(ctx context.Context) (*relationChain, error) {
	cv, base, err := s.relationsBaseline(ctx)
	if err != nil {
		return nil, err
	}
	ch := &relationChain{CatalogVersion: cv, Base: base, Active: base, Version: 1}
	st := s.relStore()
	if st == nil {
		return ch, nil
	}
	vs, err := st.ListRelationVersions(ctx)
	if err != nil {
		return nil, err
	}
	if len(vs) > 0 {
		ch.Max = vs[0].Version
	}
	var live []domain.ConfigVersion
	for _, v := range vs {
		if !v.Archived {
			live = append(live, v)
		}
	}
	ch.Version = ch.Max + 1
	if len(live) == 0 {
		return ch, nil
	}
	first := live[len(live)-1]
	if first.CatalogVersion != cv {
		ch.Stale = true
		return ch, nil
	}
	var ab relationsBody
	if err := json.Unmarshal(live[0].Rules, &ab); err != nil {
		return nil, fmt.Errorf("версия связей %d не разбирается: %w", live[0].Version, err)
	}
	// Связи каталога сверяются с последним импортом цепочки, а не с первой версией: переимпорт того же каталога
	// с другими связями (mock-2 с новыми связями) — новая версия import поверх цепочки, а не потеря истории.
	imp := first
	for _, v := range live {
		if v.Source == "import" && v.Author == "system" {
			imp = v
			break
		}
	}
	var ib relationsBody
	_ = json.Unmarshal(imp.Rules, &ib)
	if relationsFingerprint(ib.Relations) != relationsFingerprint(base) {
		ch.Reimported, ch.Prev, ch.Versions = true, ab.Relations, live
		return ch, nil
	}
	ch.Active, ch.Version, ch.Materialized, ch.Versions = ab.Relations, live[0].Version, true, live
	return ch, nil
}

// materializeImport — переимпорт с другими связями: записать связи каталога версией import поверх живой цепочки
// (разница — с прежней действующей версией). После записи версия действует, откат к прежним доступен.
func (s *Services) materializeImport(ctx context.Context, ch *relationChain) error {
	st := s.relStore()
	if st == nil || !ch.Reimported {
		return nil
	}
	v := relationVersion(ch.Max+1, ch.CatalogVersion, "import", "system",
		"Справочник связей каталога "+ch.CatalogVersion+" (переимпорт)", nil, ch.Prev, ch.Base)
	if err := st.ApplyRelationsChange(ctx, &repositories.RelationsChange{Base: ch.Max, Versions: []*domain.ConfigVersion{v}}); err != nil {
		return err
	}
	ch.Max, ch.Version, ch.Materialized, ch.Reimported, ch.Active = v.Version, v.Version, true, false, ch.Base
	ch.Versions = append([]domain.ConfigVersion{*v}, ch.Versions...)
	return nil
}

// applyRelationsOverlay — действующие связи в справочники в памяти (поиск видит их сразу).
func (s *Services) applyRelationsOverlay(docs []relationDoc) {
	cs := s.catalog.Load()
	if cs == nil || cs.commerce == nil {
		return
	}
	rels := relationsOfDocs(docs)
	next := *cs
	next.commerce = cs.commerce.withRelations(rels)
	if cs.lex != nil {
		// Аналоги, «С этим покупают» и продления контура поиска тоже читают связи — из словаря поиска.
		next.lex = cs.lex.withRelations(rels)
	}
	s.catalog.Store(&next)
}

// ReloadRelations — действующая версия справочника связей поверх каталога в памяти (старт, импорт). Цепочки нет
// или она прежнего каталога — связи каталога как есть.
func (s *Services) ReloadRelations(ctx context.Context) {
	if s.relStore() == nil {
		return
	}
	ch, err := s.relationChain(ctx)
	if err != nil {
		zap.L().Warn("справочник связей: версии не прочитаны", zap.Error(err))
		return
	}
	if ch.Reimported {
		if err := s.materializeImport(ctx, ch); err != nil {
			zap.L().Warn("справочник связей: версия переимпорта не записана", zap.Error(err))
			return
		}
		zap.L().Info("справочник связей: переимпорт каталога — новая версия", zap.Int("version", ch.Version),
			zap.Int("relations", len(ch.Active)))
	}
	if ch.Materialized && len(ch.Versions) > 1 {
		s.applyRelationsOverlay(ch.Active)
		zap.L().Info("справочник связей: действует версия", zap.Int("version", ch.Version), zap.Int("relations", len(ch.Active)))
	}
}

// relationNewID — id новой связи агента: rel-a<NNN>, не занятый в справочнике.
func relationNewID(docs []relationDoc, taken map[string]bool) string {
	used := map[string]bool{}
	for _, d := range docs {
		used[d.ID] = true
	}
	for k := range taken {
		used[k] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("rel-a%03d", i)
		if !used[id] {
			taken[id] = true
			return id
		}
	}
}

// applyRelationPatch — правка поверх набора связей. Удаляемые и изменяемые связи должны быть в наборе,
// новые не должны дублировать существующие (те же концы и тип). Принятие ставит review_status approved и дату.
func applyRelationPatch(docs []relationDoc, p RelationPatch, reviewedAt string) ([]relationDoc, error) {
	byID := map[string]int{}
	for i, d := range docs {
		byID[d.ID] = i
	}
	next := slices.Clone(docs)
	drop := map[string]bool{}
	for _, id := range p.Remove {
		if _, ok := byID[id]; !ok {
			return nil, fmt.Errorf("связи %s нет в справочнике", id)
		}
		drop[id] = true
	}
	for _, u := range p.Update {
		i, ok := byID[u.ID]
		if !ok {
			return nil, fmt.Errorf("связи %s нет в справочнике", u.ID)
		}
		if drop[u.ID] {
			return nil, fmt.Errorf("связь %s и удаляется, и меняется", u.ID)
		}
		u = u.normalized()
		if reviewedAt != "" {
			u.ReviewStatus, u.ReviewedAt = domain.ReviewApproved, reviewedAt
		}
		next[i] = u
	}
	var res []relationDoc
	for _, d := range next {
		if !drop[d.ID] {
			res = append(res, d)
		}
	}
	taken := map[string]bool{}
	for _, a := range p.Add {
		a = a.normalized()
		if _, dup := byID[a.ID]; a.ID == "" || dup || taken[a.ID] {
			a.ID = relationNewID(res, taken)
		}
		for _, d := range res {
			if d.sameEnds(a) {
				return nil, fmt.Errorf("такая связь уже есть: %s (%s → %s, %s)", d.ID, d.FromFamily, d.ToFamily, d.Type)
			}
		}
		if reviewedAt != "" {
			a.ReviewStatus, a.ReviewedAt = domain.ReviewApproved, reviewedAt
		}
		res = append(res, a)
	}
	for i := range res {
		for j := i + 1; j < len(res); j++ {
			if res[i].sameEnds(res[j]) && (slices.ContainsFunc(p.Update, func(u relationDoc) bool { return u.ID == res[i].ID || u.ID == res[j].ID })) {
				return nil, fmt.Errorf("после правки связи %s и %s совпадают", res[i].ID, res[j].ID)
			}
		}
	}
	return res, nil
}

// relationsDiff — разница двух наборов связей по id: {"add":[…],"remove":[…],"update":[{"before","after"}]}.
func relationsDiff(prev, next []relationDoc) map[string]any {
	pm, nm := map[string]relationDoc{}, map[string]relationDoc{}
	for _, d := range prev {
		pm[d.ID] = d
	}
	for _, d := range next {
		nm[d.ID] = d
	}
	add, remove, update := []relationDoc{}, []relationDoc{}, []map[string]relationDoc{}
	for _, d := range next {
		p, ok := pm[d.ID]
		switch {
		case !ok:
			add = append(add, d)
		default:
			a, _ := json.Marshal(p.normalized())
			b, _ := json.Marshal(d.normalized())
			if string(a) != string(b) {
				update = append(update, map[string]relationDoc{"before": p, "after": d})
			}
		}
	}
	for _, d := range prev {
		if _, ok := nm[d.ID]; !ok {
			remove = append(remove, d)
		}
	}
	return map[string]any{"add": add, "remove": remove, "update": update}
}

// relationStale — связь давно не проверялась (или дата не указана) — «проверить» в справочнике.
func relationStale(d relationDoc, now time.Time) bool {
	if d.ReviewedAt == "" {
		return true
	}
	t, err := time.Parse("2006-01-02", d.ReviewedAt)
	if err != nil {
		return true
	}
	return now.Sub(t) > relationStaleDays*24*time.Hour
}

// Relations — GET /api/relations?family=&status=&type=&source=: действующий справочник связей с фильтрами.
func (s *Services) Relations(ctx context.Context, q in.RelationsQuery) (*out.RelationList, error) {
	ch, err := s.relationChain(ctx)
	if err != nil {
		return nil, err
	}
	if q.Status != "" && !slices.Contains([]string{domain.ReviewApproved, domain.ReviewPending, domain.ReviewRejected}, q.Status) {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "status",
			Code: errs.CodeInvalidFormat, Message: "Статус проверки — approved, pending или rejected"})
	}
	if q.Type != "" && !slices.Contains(domain.RelationTypes, q.Type) {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "type",
			Code: errs.CodeInvalidFormat, Message: "Тип — substitute, complement, renewal, compatibility или upgrade"})
	}
	names := s.familyNames()
	now := s.now().UTC()
	res := &out.RelationList{Version: ch.Version, Materialized: ch.Materialized, CatalogVersion: ch.CatalogVersion,
		StaleAfterDays: relationStaleDays, Items: []out.Relation{}}
	for _, d := range ch.Active {
		if q.Family != "" && d.FromFamily != q.Family && d.ToFamily != q.Family {
			continue
		}
		if q.Status != "" && d.ReviewStatus != q.Status || q.Type != "" && d.Type != q.Type || q.Source != "" && d.Source != q.Source {
			continue
		}
		raw, _ := json.Marshal(d)
		res.Items = append(res.Items, out.Relation{Relation: raw, FromName: names[d.FromFamily], ToName: names[d.ToFamily],
			NeedsReview: relationStale(d, now)})
	}
	res.Total = len(res.Items)
	return res, nil
}

// familyNames — id семейства → название (для подписей справочника).
func (s *Services) familyNames() map[string]string {
	names := map[string]string{}
	if c := s.commerceFor(context.Background()); c != nil {
		for _, p := range c.products {
			if names[p.FamilyID] == "" {
				names[p.FamilyID] = firstNonEmpty(p.FamilyName, p.Title)
			}
		}
	}
	return names
}

// RelationVersions — GET /api/relations/versions: версии справочника связей, новые сверху, с разницей.
func (s *Services) RelationVersions(ctx context.Context) (*out.RelationVersionList, error) {
	st := s.relStore()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgRelationsNoStore)
	}
	ch, err := s.relationChain(ctx)
	if err != nil {
		return nil, err
	}
	vs, err := st.ListRelationVersions(ctx)
	if err != nil {
		return nil, err
	}
	res := &out.RelationVersionList{Items: []out.RelationVersion{}}
	if ch.Materialized {
		res.Active = ch.Version
	}
	for _, v := range vs {
		var b relationsBody
		_ = json.Unmarshal(v.Rules, &b)
		diff := json.RawMessage(v.Diff)
		if len(diff) == 0 {
			diff = json.RawMessage(`{"add":[],"remove":[],"update":[]}`)
		}
		res.Items = append(res.Items, out.RelationVersion{Version: v.Version, CreatedAt: v.CreatedAt,
			Author: firstNonEmpty(v.Author, "system"), Source: v.Source, Description: v.Comment, ProposalID: v.ProposalID,
			Diff: diff, Relations: len(b.Relations), CatalogVersion: v.CatalogVersion,
			Archived: v.Archived || ch.Stale || v.CatalogVersion != ch.CatalogVersion})
	}
	return res, nil
}

// relationVersion — новая версия справочника связей.
func relationVersion(n int, catalogVersion, source, author, comment string, proposalID *string, prev, next []relationDoc) *domain.ConfigVersion {
	body, _ := json.Marshal(relationsBody{Relations: next})
	diff, _ := json.Marshal(relationsDiff(prev, next))
	return &domain.ConfigVersion{Kind: domain.ConfigKindRelations, Version: n, Source: source, Author: author,
		Comment: comment, ProposalID: proposalID, Rules: body, Diff: diff, CatalogVersion: catalogVersion}
}

// commitRelations — записать новую версию справочника поверх цепочки ch (если цепочки нет — сначала исходная
// версия из каталога) вместе с решением по предложению; затем связи действуют в поиске.
func (s *Services) commitRelations(ctx context.Context, ch *relationChain, next []relationDoc, source, author, comment string,
	p *domain.Proposal, dec *domain.Decision, rollBackAbove int) (*domain.ConfigVersion, []domain.Decision, error) {
	st := s.relStore()
	if st == nil {
		return nil, nil, errs.NewUnavailableError(MsgRelationsNoStore)
	}
	change := &repositories.RelationsChange{Base: ch.Max, ArchiveActive: ch.Stale, Proposal: p, Decision: dec,
		RollBackAbove: rollBackAbove}
	n := ch.Max + 1
	prev := ch.Active
	if !ch.Materialized {
		comment, basePrev := "Справочник связей каталога "+ch.CatalogVersion, []relationDoc(nil)
		if ch.Reimported {
			comment, basePrev = comment+" (переимпорт)", ch.Prev
		}
		change.Versions = append(change.Versions, relationVersion(n, ch.CatalogVersion, "import", "system",
			comment, nil, basePrev, ch.Base))
		n++
		prev = ch.Base
	}
	var pid *string
	if p != nil {
		id := p.ID.String()
		pid = &id
	}
	v := relationVersion(n, ch.CatalogVersion, source, author, comment, pid, prev, next)
	change.Versions = append(change.Versions, v)
	if p != nil {
		p.ConfigVersion = &v.Version
	}
	if err := st.ApplyRelationsChange(ctx, change); err != nil {
		if errors.Is(err, domain.ErrRulesVersionConflict) {
			return nil, nil, errs.NewConflictError(MsgRelationsConflict).WithCause(err)
		}
		return nil, nil, rulesError(err)
	}
	s.applyRelationsOverlay(next)
	return v, change.RolledBack, nil
}

// RollbackRelations — POST /api/relations/rollback {to_version, comment}: новая версия = копия выбранной;
// принятые после неё правки связей — «откачено». История не стирается.
func (s *Services) RollbackRelations(ctx context.Context, req in.RollbackRequest) (*out.ConfigRollback, error) {
	if s.relStore() == nil {
		return nil, errs.NewUnavailableError(MsgRelationsNoStore)
	}
	if req.ToVersion < 1 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "to_version", Code: errs.CodeInvalidFormat, Message: "Укажите номер версии"})
	}
	ch, err := s.relationChain(ctx)
	if err != nil {
		return nil, err
	}
	var target *domain.ConfigVersion
	for i := range ch.Versions {
		if ch.Versions[i].Version == req.ToVersion {
			target = &ch.Versions[i]
		}
	}
	if target == nil {
		vs, _ := s.relStore().ListRelationVersions(ctx)
		if slices.ContainsFunc(vs, func(v domain.ConfigVersion) bool { return v.Version == req.ToVersion }) {
			return nil, errs.NewConflictError(MsgRelationsArchived)
		}
		return nil, errs.NewNotFoundError(MsgRelationsVersionAbsent)
	}
	if ch.Version == req.ToVersion {
		return nil, errs.NewConflictError(MsgRelationsVersionLatest)
	}
	var b relationsBody
	if err := json.Unmarshal(target.Rules, &b); err != nil {
		return nil, errs.NewInternalError("Версия справочника связей не разбирается").WithCause(err)
	}
	comment := fmt.Sprintf("Откат справочника связей к версии %d", req.ToVersion)
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	v, rolled, err := s.commitRelations(ctx, ch, b.Relations, "rollback", "human", comment, nil,
		&domain.Decision{Comment: truncateRunes(strings.TrimSpace(req.Comment), 300)}, req.ToVersion)
	if err != nil {
		return nil, err
	}
	res := &out.ConfigRollback{Version: v.Version, RolledBackTo: req.ToVersion, Proposals: []string{}}
	for _, d := range rolled {
		res.Proposals = append(res.Proposals, d.ProposalID.String())
	}
	zap.L().Info("справочник связей: откат", zap.Int("version", v.Version), zap.Int("to", req.ToVersion),
		zap.Int("proposals", len(res.Proposals)))
	return res, nil
}

// approveRelationPatch — принятие relation_patch: правка перепроверяется на действующем справочнике (если он
// сменился после прогона агента) и становится новой версией kind=relations; поиск видит её сразу.
func (s *Services) approveRelationPatch(ctx context.Context, p *domain.Proposal, comment string) (*out.ProposalDecision, error) {
	var patch RelationPatch
	if err := json.Unmarshal(p.Patch, &patch); err != nil {
		return nil, errs.NewInternalError("Правка связей предложения не разбирается").WithCause(err)
	}
	ch, err := s.relationChain(ctx)
	if err != nil {
		return nil, err
	}
	if ch.Version != p.BaseVersion {
		// Справочник сменился после прогона: правка проверяется заново детерминированно (без поиска).
		var ev map[string]any
		_ = json.Unmarshal(p.Evidence, &ev)
		key, _ := ev["finding_key"].(string)
		re := s.recheckRelationPatch(ctx, ch, patch, key)
		if !re.Passed {
			return nil, errs.NewConflictError(fmt.Sprintf("Справочник связей изменился после проверки агента (v%d → v%d), и "+
				"на действующем справочнике правка не проходит: %s. Предложение осталось в очереди", p.BaseVersion, ch.Version,
				strings.Join(re.Reasons, "; ")))
		}
	}
	next, err := applyRelationPatch(ch.Active, patch, s.now().UTC().Format("2006-01-02"))
	if err != nil {
		return nil, errs.NewConflictError("Правка не накладывается на действующий справочник связей: " + err.Error())
	}
	now := s.now().UTC()
	p.Status, p.DecidedAt, p.DecidedReason, p.DecidedComment = domain.ProposalAccepted, &now, "accepted", comment
	dec := &domain.Decision{ProposalID: p.ID, Decision: domain.ProposalAccepted, Reason: "accepted", Comment: comment,
		ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
	v, _, err := s.commitRelations(ctx, ch, next, "agent", "agent+human", relationPatchSummary(p, patch), p, dec, 0)
	if err != nil {
		return nil, err
	}
	zap.L().Info("предложение принято: новая версия справочника связей", zap.String("proposal_id", p.ID.String()),
		zap.Int("version", v.Version))
	return &out.ProposalDecision{Proposal: proposalView(p), ConfigVersion: p.ConfigVersion}, nil
}

// relationPatchSummary — описание версии справочника одной строкой.
func relationPatchSummary(p *domain.Proposal, patch RelationPatch) string {
	var parts []string
	for _, a := range patch.Add {
		parts = append(parts, fmt.Sprintf("+ %s → %s (%s)", a.FromFamily, a.ToFamily, a.Type))
	}
	for _, id := range patch.Remove {
		parts = append(parts, "− "+id)
	}
	for _, u := range patch.Update {
		parts = append(parts, "≈ "+u.ID)
	}
	return fmt.Sprintf("%s: %s", p.Diagnosis, strings.Join(parts, ", "))
}
