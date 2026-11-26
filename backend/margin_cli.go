package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"kuznica/api/in"
	"kuznica/api/services"

	"go.uber.org/zap"
)

// runMargin — `go run . margin --set tune [--json]`: симуляция всех положений рычага маржи на размеченном
// наборе (POST /api/margin/simulate без HTTP), таблица в stdout. В базу ничего не пишется.
func runMargin(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("margin", flag.ExitOnError)
	set := fs.String("set", "tune", "набор: tune | regression | demo (test заморожен — только для проверки)")
	asJSON := fs.Bool("json", false, "вывести ответ целиком в JSON")
	_ = fs.Parse(args)

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	res, err := svc.SimulateMargin(ctx, in.MarginSimulateRequest{Set: *set})
	if err != nil {
		log.Fatal("маржа: симуляция", zap.Error(err))
	}
	if *asJSON {
		raw, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(raw))
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Симуляция рычага маржи: набор %s (%d запр., размечено %d) sha256:%s · каталог %s · правила v%d · %d мс\n",
		res.Set, res.Queries, res.Judged, res.SetHash, res.CatalogVersion, res.RulesVersion, res.DurationMs)
	fmt.Fprintf(&b, "NDCG@10 контура поиска (вес %.2f): %.4f · ε = %.2f · упор рычага: %.2f (вниз %.2f) · действует v%d вес %.2f\n\n",
		res.BaseWeight, res.BaseNDCG10, res.Epsilon, res.StopWeight, res.MinWeight, res.PolicyVersion, res.CurrentWeight)
	fmt.Fprintf(&b, "%-6s %-7s %-9s %-9s %-8s %-8s %-8s %-5s %-5s %-6s %-6s %-6s %s\n", "вес", "NDCG10", "ΔNDCGср", "ΔNDCGp10",
		">ε доля", "индекс", "инд.рел", "топ1", "топ3", "точн1", "защит", "сдвиг", "доступно")
	for _, p := range res.Items {
		fmt.Fprintf(&b, "%-6.2f %-7.4f %-9.4f %-9.4f %-8.4f %-8.4f %-8.4f %-5d %-5d %-6d %-6d %-6d %t\n", p.Weight, p.NDCG10,
			p.NDCGDeltaMean, p.NDCGDeltaP10, p.ShareOverEps, p.MarginIndex, p.MarginIndexRelevant, p.Top1Changed,
			p.Top3Changed, p.ExactTop1Changed, p.GuardsFired, p.QueriesWithShifts, p.Available)
	}
	for _, p := range res.Items {
		if len(p.GuardsByKind) > 0 {
			fmt.Fprintf(&b, "  защиты %.2f: %v\n", p.Weight, p.GuardsByKind)
		}
	}
	fmt.Fprintln(&b, res.Note)
	_, _ = os.Stdout.WriteString(b.String())
}
