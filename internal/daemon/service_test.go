package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/marcgauthier/Crypto/internal/findings"
)

func TestDailyEvaluationOnceAndFourIndependentChecks(t *testing.T) {
	ctx := context.Background()
	db, err := findings.Open(filepath.Join(t.TempDir(), "findings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scans, checks := 0, 0
	makeService := func() *Service {
		s := &Service{Store: db, Pairs: []string{"SOXL:SOXS", "MSTR:MSTZ"}}
		s.Scan = func(_ context.Context, pair string, at time.Time) ([]findings.Rule, error) {
			scans++
			sym := "SOXL"
			inv := "SOXS"
			if pair == "MSTR:MSTZ" {
				sym = "MSTR"
				inv = "MSTZ"
			}
			day := at.Format("2006-01-02")
			return []findings.Rule{{Pair: pair, Symbol: sym, Side: "bullish", EvaluationDay: day, LookbackBars: 24, DropPct: 2, GainPct: 4},
				{Pair: pair, Symbol: inv, Side: "inverse", EvaluationDay: day, LookbackBars: 24, DropPct: 3, GainPct: 5}}, nil
		}
		s.Checks = func(_ context.Context, _ time.Time) (CheckFunc, func() error, error) {
			return func(_ context.Context, _ findings.Rule, _ time.Time) (string, error) { checks++; return "", nil }, nil, nil
		}
		return s
	}
	today := time.Date(2026, 11, 9, 17, 0, 0, 0, time.UTC)
	s := makeService()
	if err := s.Tick(ctx, today); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, today.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if scans != 2 || checks != 8 {
		t.Fatalf("same-day double evaluation: scans=%d checks=%d", scans, checks)
	}
	s = makeService() // application restart
	if err := s.Tick(ctx, today.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if scans != 2 || checks != 12 {
		t.Fatalf("restart double evaluated: scans=%d checks=%d", scans, checks)
	}
	tomorrow := NextMidnightUTC(today)
	if err := s.Tick(ctx, tomorrow); err != nil {
		t.Fatal(err)
	}
	if scans != 4 || checks != 16 {
		t.Fatalf("midnight evaluation: scans=%d checks=%d", scans, checks)
	}
}

func TestUTCBoundariesAndPairValidation(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 23, 59, 59, 0, time.UTC)
	if got := NextFiveMinute(t0); got != time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("five-minute next=%s", got)
	}
	if got := NextMidnightUTC(t0); got != time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("next midnight=%s", got)
	}
	if pairs, err := ParsePairs("soxl:soxs, mstr:mstz"); err != nil || len(pairs) != 2 || pairs[0] != "SOXL:SOXS" {
		t.Fatalf("pairs: %+v %v", pairs, err)
	}
	if _, err := ParsePairs("SOXL:SOXL"); err == nil {
		t.Fatal("accepted same ticker")
	}
}
