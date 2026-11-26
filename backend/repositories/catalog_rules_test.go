package repositories

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Повторный импорт и цепочки версий правил (ревью M1). Нужна живая база: KUZNICA_TEST_DATABASE_URL=postgres://…
// go test ./repositories. Тест работает в своей схеме и удаляет её; без переменной — пропуск.

func testSchemaRepos(t *testing.T) *Repos {
	t.Helper()
	dsn := os.Getenv("KUZNICA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KUZNICA_TEST_DATABASE_URL не задан — тест на живой базе пропущен")
	}
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("itest_rules_%d", time.Now().UnixNano())
	if err := base.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = base.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		if db, err := base.DB(); err == nil {
			_ = db.Close()
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	r, err := InitRepos(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err := r.MigrateDB(); err != nil {
		t.Fatal(err)
	}
	return r
}

func rulesJSON(t *testing.T, catalog, alias string) domain.JSONB {
	t.Helper()
	j, err := domain.ToJSONB(map[string]any{"catalog_version": catalog, "aliases": []map[string]string{{"alias": alias}}})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestImportRulesChains(t *testing.T) {
	r := testSchemaRepos(t)
	ctx := t.Context()
	if ir, err := r.SaveImportRules(ctx, "c1", rulesJSON(t, "c1", "a"), "исходные"); err != nil || ir.Version != 1 || ir.NewChain {
		t.Fatalf("первый импорт: %+v %v", ir, err)
	}
	// Правка агента поверх версии 1 — каталог берётся у базы.
	v2 := &domain.ConfigVersion{Version: 2, Source: "agent", Rules: rulesJSON(t, "c1", "зуп")}
	if err := r.ApplyRulesChange(ctx, &RulesChange{Base: 1, Version: v2}); err != nil {
		t.Fatal(err)
	}
	pending := &domain.Proposal{Status: domain.ProposalPending, BaseVersion: 2}
	accepted := &domain.Proposal{Status: domain.ProposalAccepted, BaseVersion: 1}
	for _, p := range []*domain.Proposal{pending, accepted} {
		if err := r.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	// Тот же каталог: база цепочки перезаписана, правка агента осталась действующей.
	if ir, err := r.SaveImportRules(ctx, "c1", rulesJSON(t, "c1", "b"), "повтор"); err != nil || ir.Version != 2 || ir.NewChain {
		t.Fatalf("повторный импорт того же каталога: %+v %v", ir, err)
	}
	if v, _ := r.GetConfigVersion(ctx, 1); v == nil || v.Comment != "повтор" || v.Archived {
		t.Fatalf("база цепочки: %+v", v)
	}
	if v, _ := r.GetConfigVersion(ctx, 2); v == nil || v.CatalogVersion != "c1" || v.Archived {
		t.Fatalf("правка агента: %+v", v)
	}
	// Другой каталог: новая цепочка, прежние версии в архиве, ждущее предложение — stale, принятое не тронуто.
	ir, err := r.SaveImportRules(ctx, "c2", rulesJSON(t, "c2", "c"), "новый каталог")
	if err != nil || ir.Version != 3 || !ir.NewChain || ir.Stale != 1 {
		t.Fatalf("импорт нового каталога: %+v %v", ir, err)
	}
	vs, err := r.ListConfigVersions(ctx)
	if err != nil || len(vs) != 3 {
		t.Fatalf("версии: %+v %v", vs, err)
	}
	for _, v := range vs {
		if (v.Version < 3) != v.Archived {
			t.Fatalf("архив — только прежний каталог: %+v", v)
		}
	}
	if latest, err := r.LatestRules(ctx); err != nil || latest == nil || latest.Version != 3 {
		t.Fatalf("действующая версия — нового каталога: %+v %v", latest, err)
	}
	if p, _ := r.GetProposal(ctx, pending.ID); p == nil || p.Status != domain.ProposalStale || p.DecidedReason != domain.ProposalStaleReason {
		t.Fatalf("ждущее предложение: %+v", p)
	}
	if p, _ := r.GetProposal(ctx, accepted.ID); p == nil || p.Status != domain.ProposalAccepted {
		t.Fatalf("принятое предложение: %+v", p)
	}
	// Принять устаревшее нельзя: оно уже не pending.
	pending.Status = domain.ProposalAccepted
	v4 := &domain.ConfigVersion{Version: 4, Source: "agent", Rules: rulesJSON(t, "c1", "зуп")}
	if err := r.ApplyRulesChange(ctx, &RulesChange{Base: 3, Version: v4, Proposal: pending}); !errors.Is(err, domain.ErrProposalDecided) {
		t.Fatalf("принятие stale: %v", err)
	}
	// Откат к версии прежнего каталога — 409, новой версии нет.
	rb := &domain.ConfigVersion{Version: 4, Source: "rollback", Rules: rulesJSON(t, "c1", "a")}
	err = r.ApplyRulesChange(ctx, &RulesChange{Base: 3, Version: rb, RollBackAbove: 1})
	var e *errs.Error
	if !errors.As(err, &e) || e.HTTPStatus() != 409 {
		t.Fatalf("откат к архиву: %v", err)
	}
	if v, _ := r.RulesVersion(ctx); v != 3 {
		t.Fatalf("после отказа версия %d", v)
	}
	// Правка поверх новой цепочки получает её каталог.
	v4 = &domain.ConfigVersion{Version: 4, Source: "agent", Rules: rulesJSON(t, "c2", "d")}
	if err := r.ApplyRulesChange(ctx, &RulesChange{Base: 3, Version: v4}); err != nil || v4.CatalogVersion != "c2" {
		t.Fatalf("правка нового каталога: %+v %v", v4, err)
	}
}
