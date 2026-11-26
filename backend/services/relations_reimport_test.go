package services

import (
	"encoding/json"
	"testing"

	"kuznica/api/in"
)

// QA r2 №3: каталог той же версии переимпортирован с другими связями (mock-2 с новыми связями). Раньше цепочка
// считалась устаревшей: ни одна версия не действовала, отката не было. Теперь связи каталога записываются версией
// import поверх цепочки, она действует, откат к прежним версиям доступен.
func TestRelationsReimportKeepsChain(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	st := &memRelStore{memStore: ms}
	s.agentStoreOverride = st
	cv, base, err := s.relationsBaseline(t.Context())
	if err != nil || len(base) < 2 {
		t.Fatalf("связи каталога: %v", err)
	}
	// Прежний импорт того же каталога — без последней связи; поверх — правка человека.
	old := base[:len(base)-1]
	ch, _ := s.relationChain(t.Context())
	ch.Base = old
	if _, _, err := s.commitRelations(t.Context(), ch, old[1:], "human", "human", "правка", nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	// Связи каталога теперь base (переимпорт): цепочка не устарела, но версия import ещё не записана.
	ch, err = s.relationChain(t.Context())
	if err != nil || ch.Stale || !ch.Reimported || ch.Materialized || len(ch.Versions) != 2 {
		t.Fatalf("переимпорт: %+v %v", ch, err)
	}
	s.ReloadRelations(t.Context())
	vs, err := s.RelationVersions(t.Context())
	if err != nil || vs.Active != 3 || len(vs.Items) != 3 {
		t.Fatalf("после перезагрузки действует версия импорта 3: %+v %v", vs, err)
	}
	for _, it := range vs.Items {
		if it.Archived || it.CatalogVersion != cv {
			t.Fatalf("история той же версии каталога не архивная: %+v", it)
		}
	}
	top := vs.Items[0]
	var diff struct {
		Add []json.RawMessage `json:"add"`
	}
	_ = json.Unmarshal(top.Diff, &diff)
	if top.Source != "import" || top.Relations != len(base) || len(diff.Add) != 2 {
		t.Fatalf("версия импорта с разницей к прежней действующей: %+v", top)
	}
	// Повторная загрузка не плодит версий.
	s.ReloadRelations(t.Context())
	if vs, _ := s.RelationVersions(t.Context()); len(vs.Items) != 3 {
		t.Fatalf("лишняя версия: %d", len(vs.Items))
	}
	// Откат к версии 2 доступен.
	rb, err := s.RollbackRelations(t.Context(), in.RollbackRequest{ToVersion: 2})
	if err != nil || rb.Version != 4 {
		t.Fatalf("откат: %+v %v", rb, err)
	}
	if ch, _ := s.relationChain(t.Context()); !ch.Materialized || ch.Version != 4 || len(ch.Active) != len(old)-1 {
		t.Fatalf("после отката: v%d, связей %d", ch.Version, len(ch.Active))
	}
}
