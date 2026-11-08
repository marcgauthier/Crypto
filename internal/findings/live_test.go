package findings

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveSignalsPersistAndDeduplicate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "signal.sqlite")
	at := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	pair := "SOXL:SOXS"
	rules := []Rule{{Pair: pair, Symbol: "SOXL", Side: "bullish", EvaluationDay: "2026-11-09", LookbackBars: 24, DropPct: 3, GainPct: 4},
		{Pair: pair, Symbol: "SOXS", Side: "inverse/bearish", EvaluationDay: "2026-11-09", LookbackBars: 24, DropPct: 5, GainPct: 6}}
	if err := db.PublishRules(ctx, pair, "2026-11-09", rules, at); err != nil {
		t.Fatal(err)
	}
	if yes, err := db.Evaluated(ctx, pair, "2026-11-09"); err != nil || !yes {
		t.Fatalf("evaluation = %v, %v", yes, err)
	}
	bar := at.Add(15 * time.Hour)
	if action, err := db.ApplyQuote(ctx, rules[0], bar, bar.Add(time.Second), 96, 100, 10); err != nil || action != "BUY" {
		t.Fatalf("buy = %s %v", action, err)
	}
	if action, err := db.ApplyQuote(ctx, rules[0], bar, bar.Add(2*time.Second), 96, 100, 10); err != nil || action != "" {
		t.Fatalf("duplicate buy = %s %v", action, err)
	}
	bar = bar.Add(5 * time.Minute)
	if action, err := db.ApplyQuote(ctx, rules[0], bar, bar.Add(time.Second), 102, 100, 10); err != nil || action != "SELL" {
		t.Fatalf("sell = %s %v", action, err)
	}
	if action, err := db.ApplyQuote(ctx, rules[1], bar, bar.Add(time.Second), 94, 100, 10); err != nil || action != "BUY" {
		t.Fatalf("inverse buy = %s %v", action, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	states, err := db.LiveStates(ctx)
	if err != nil || len(states) != 2 {
		t.Fatalf("states: %+v %v", states, err)
	}
	for _, s := range states {
		if s.Symbol == "SOXL" && s.Position != "FLAT" {
			t.Fatalf("SOXL should be flat: %+v", s)
		}
		if s.Symbol == "SOXS" && (s.Position != "OPEN" || s.EntryPrice <= 94) {
			t.Fatalf("SOXS should be independently long: %+v", s)
		}
	}
	signals, err := db.RecentSignals(ctx, 100)
	if err != nil || len(signals) != 3 {
		t.Fatalf("signals: %+v %v", signals, err)
	}
	if signals[1].Action != "SELL" || signals[1].ReturnPct <= 0 {
		t.Fatalf("missing profitable paper exit: %+v", signals)
	}
	if err := db.PublishRules(ctx, pair, "2026-11-10", []Rule{
		{Pair: pair, Symbol: "SOXL", Side: "bullish", EvaluationDay: "2026-11-10", LookbackBars: 24, DropPct: 5, GainPct: 7},
		{Pair: pair, Symbol: "SOXS", Side: "inverse/bearish", EvaluationDay: "2026-11-10", LookbackBars: 24, DropPct: 6, GainPct: 8}}, at.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	states, err = db.LiveStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if states[1].Position != "OPEN" {
		t.Fatal("daily optimization discarded paper position")
	}
}

func TestLiveRejectInvalidAndNoPartialPublish(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "f.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	wrong := []Rule{{Pair: "A:B", Symbol: "A", Side: "bullish", EvaluationDay: "2026-11-09", LookbackBars: 24, DropPct: 1, GainPct: 3},
		{Pair: "A:B", Symbol: "A", Side: "inverse", EvaluationDay: "2026-11-09", LookbackBars: 24, DropPct: 1, GainPct: 3}}
	if err := db.PublishRules(ctx, "A:B", "2026-11-09", wrong, at); err == nil {
		t.Fatal("accepted duplicate leg")
	}
	if done, _ := db.Evaluated(ctx, "A:B", "2026-11-09"); done {
		t.Fatal("partially marked evaluation")
	}
}
