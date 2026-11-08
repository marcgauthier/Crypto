package findings

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func sample(day int, symbol string, drop, gain float64) Finding {
	end := time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC)
	return Finding{
		Pair: "SOXL:SOXS", Symbol: symbol, Side: "bullish", ScannedAt: end.Add(12 * time.Hour),
		WindowDays: 180, WindowStart: end.AddDate(0, 0, -180), WindowEnd: end, LookbackBars: 24,
		SettingsKey: "train=.7;cost=10;grid=v1", BuyDropPct: drop, SellGainPct: gain,
		TrainReturnPct: 14, TestReturnPct: 5, TestBuyHoldPct: 2, TestMaxDrawdownPct: 9,
		TestTrades: 3, TestWinRatePct: 66, PortfolioReturnPct: 6, PortfolioDrawdownPct: 7,
	}
}

func TestSaveHistoryStabilityAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.sqlite")
	ctx := context.Background()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := sample(5, "SOXL", 2, 4)
	b := sample(5, "SOXS", 6, 5)
	if err := db.Save(ctx, []Finding{a, b}); err != nil {
		t.Fatal(err)
	}
	a.BuyDropPct = 3
	if err := db.Save(ctx, []Finding{a, b}); err != nil {
		t.Fatal(err)
	} // one point per day
	if err := db.Save(ctx, []Finding{sample(6, "SOXL", 4, 8), sample(6, "SOXS", 7, 6)}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	groups, err := db.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %+v", groups)
	}
	var soxl Group
	for _, g := range groups {
		if g.Symbol == "SOXL" {
			soxl = g
		}
	}
	if soxl.Runs != 2 {
		t.Fatalf("duplicate scan: %+v", soxl)
	}
	rows, err := db.History(ctx, soxl, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].BuyDropPct != 4 || rows[1].BuyDropPct != 3 {
		t.Fatalf("incorrect history: %+v", rows)
	}
	summary := Summarize(rows)
	if math.Abs(summary.StdDevBuyDropPct-math.Sqrt(.5)) > 1e-10 || summary.MeanSellGainPct != 6 {
		t.Fatalf("incorrect statistics: %+v", summary)
	}
	if _, err := db.History(ctx, soxl, 0); err == nil {
		t.Fatal("expected limit validation")
	}
}

func TestRejectInvalidWithoutPartialSave(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	invalid := sample(6, "SOXS", math.NaN(), 3)
	if err := db.Save(ctx, []Finding{sample(6, "SOXL", 2, 3), invalid}); err == nil {
		t.Fatal("accepted NaN")
	}
	groups, err := db.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("partial insert: %+v", groups)
	}
}
