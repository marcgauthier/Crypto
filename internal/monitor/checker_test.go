package monitor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/marcgauthier/Crypto/internal/findings"
	"github.com/marcgauthier/Crypto/internal/market"
)

type fakeHistory struct{ bars []market.Candle }

func (f fakeHistory) ReadBars(_ context.Context, _ string, _, _ time.Time) ([]market.Candle, error) {
	return f.bars, nil
}

type fakeQuotes struct{ bars []market.Candle }

func (f *fakeQuotes) FetchInterval(_ context.Context, _ string, _, _ time.Time, interval string) ([]market.Candle, error) {
	if interval != "5m" {
		panic("wrong interval")
	}
	return f.bars, nil
}
func quote(t time.Time, close float64) market.Candle {
	return market.Candle{Time: t, Open: close, High: close, Low: close, Close: close, Volume: 100}
}

func TestCompletedFiveMinuteSignalsAndStaleness(t *testing.T) {
	ctx := context.Background()
	db, err := findings.Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 11, 9, 15, 35, 0, 0, time.UTC) // 10:35 EST
	r := findings.Rule{Pair: "SOXL:SOXS", Symbol: "SOXL", Side: "bullish", EvaluationDay: "2026-11-09", LookbackBars: 24, DropPct: 2, GainPct: 4}
	bars := make([]market.Candle, 24)
	for i := range bars {
		bars[i] = quote(at.Add(time.Duration(i-25)*time.Hour), 100)
	}
	source := &fakeQuotes{bars: []market.Candle{quote(at.Add(-5*time.Minute), 97), quote(at, 10)}} // latest is still forming and must be ignored
	c := Checker{Store: db, Hourly: fakeHistory{bars}, Quotes: source, CostBps: 10}
	if action, err := c.Check(ctx, r, at); err != nil || action != "BUY" {
		t.Fatalf("buy = %q %v", action, err)
	}
	if action, err := c.Check(ctx, r, at); err != nil || action != "" {
		t.Fatalf("duplicate = %q %v", action, err)
	}
	at = at.Add(5 * time.Minute)
	source.bars = []market.Candle{quote(at.Add(-5*time.Minute), 102), quote(at, 10)}
	if action, err := c.Check(ctx, r, at); err != nil || action != "SELL" {
		t.Fatalf("sell = %q %v", action, err)
	}
	states, err := db.LiveStates(ctx)
	if err != nil || states[0].Position != "FLAT" {
		t.Fatalf("bad state: %+v %v", states, err)
	}
	at = at.Add(60 * time.Minute)
	source.bars = []market.Candle{quote(at.Add(-30*time.Minute), 88)}
	if action, err := c.Check(ctx, r, at); err != nil || action != "" {
		t.Fatalf("stale signal: %s %v", action, err)
	}
	states, err = db.LiveStates(ctx)
	if err != nil || states[0].Status != "stale_quote" {
		t.Fatalf("stale status: %+v %v", states, err)
	}
}

func TestMarketHoursDSTAndWeekends(t *testing.T) {
	for _, tc := range []struct {
		at   string
		want bool
	}{
		{"2026-11-09T14:30:00Z", false}, // 9:30 AM EST, first candle is not closed
		{"2026-11-09T14:35:00Z", true},
		{"2026-07-09T13:35:00Z", true}, // summer EDT
		{"2026-07-09T20:30:00Z", false},
		{"2026-11-08T15:35:00Z", false}, // Sunday
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if got := ExchangeWindow(at); got != tc.want {
			t.Errorf("%s: want %t got %t", tc.at, tc.want, got)
		}
	}
}
