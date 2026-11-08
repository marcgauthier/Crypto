package backtest

import (
	"github.com/marcgauthier/Crypto/internal/market"
	"math"
	"testing"
	"time"
)

func bars(closes, opens []float64) []market.Candle {
	out := make([]market.Candle, len(closes))
	base := time.Date(2026, 1, 5, 14, 30, 0, 0, time.UTC)
	for i := range closes {
		op := closes[i]
		if opens != nil {
			op = opens[i]
		}
		out[i] = market.Candle{Time: base.Add(time.Duration(i) * time.Hour), Open: op, High: math.Max(op, closes[i]) * 2, Low: math.Min(op, closes[i]) / 2, Close: closes[i], Volume: 100}
	}
	return out
}
func TestSignalsFillNextOpenAndNotIntrabar(t *testing.T) {
	// At i=2, close 8 is -20% from recent close 10, prompting purchase on i=3.
	// At i=4, close 9.2 crosses +5% vs the actual next-open fill (8.5).
	b := bars([]float64{10, 10, 8, 8.5, 9.2, 11}, []float64{10, 10, 8, 8.5, 9.2, 10})
	r, err := Run(b, Params{DropPct: 10, GainPct: 5}, Config{Lookback: 2, CostBps: 0})
	if err != nil {
		t.Fatal(err)
	}
	if r.Trades != 1 || len(r.TradeLog) != 1 || r.TradeLog[0].BuyPrice != 8.5 || r.TradeLog[0].SellPrice != 10 {
		t.Fatalf("unexpected trade %+v", r.TradeLog)
	}
	if math.Abs(r.ReturnPct-(10/8.5-1)*100) > 1e-6 {
		t.Fatalf("unexpected return %+v", r)
	}
	// Extreme low/high in the candle must NOT trigger orders by itself.
	flat := bars([]float64{10, 10, 10, 10, 10}, nil)
	r, err = Run(flat, Params{DropPct: 2, GainPct: 2}, Config{Lookback: 2})
	if err != nil || r.Trades != 0 || r.OpenPosition {
		t.Fatalf("intrabar lookahead detected %+v %v", r, err)
	}
}
func TestTrailingUnrealizedLossCounted(t *testing.T) {
	b := bars([]float64{10, 10, 8, 7, 5}, []float64{10, 10, 8, 8, 5})
	r, err := Run(b, Params{DropPct: 10, GainPct: 30}, Config{Lookback: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.Trades != 0 || !r.OpenPosition || !(r.ReturnPct < 0) || !(r.MaxDrawdownPct > 0) {
		t.Fatalf("open losing trade must hurt total return: %+v", r)
	}
}
func TestOptimizeAndCombine(t *testing.T) {
	b := bars([]float64{10, 10, 8, 8, 9, 10, 10, 8, 8, 9, 10, 10, 8, 8, 9, 10, 10, 8, 8, 9, 10}, nil)
	r, err := Optimize(b, Config{Lookback: 3}, Grid{DropMin: 5, DropMax: 20, DropStep: 5, GainMin: 2, GainMax: 10, GainStep: 2, MinTrades: 2})
	if err != nil || r.Trades < 2 {
		t.Fatalf("optimization %v %+v", err, r)
	}
	a := []EquityPoint{{Time: b[0].Time, Value: 1.1}, {Time: b[1].Time, Value: .9}}
	c := []EquityPoint{{Time: b[0].Time, Value: 1.0}, {Time: b[1].Time, Value: 1.1}}
	total, dd := Combine(a, c)
	if math.Abs(total) > 1e-6 || dd <= 0 {
		t.Fatalf("portfolio math failed %.3f %.3f", total, dd)
	}
}
func TestTradingCostsReduceReturn(t *testing.T) {
	b := bars([]float64{10, 10, 8, 8, 9, 10}, nil)
	low, _ := Run(b, Params{DropPct: 10, GainPct: 5}, Config{Lookback: 2})
	high, _ := Run(b, Params{DropPct: 10, GainPct: 5}, Config{Lookback: 2, CostBps: 20})
	if !(high.ReturnPct < low.ReturnPct) {
		t.Fatalf("trading cost not applied: %.3f >= %.3f", high.ReturnPct, low.ReturnPct)
	}
}
