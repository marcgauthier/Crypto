package btcstrategy

import (
	"github.com/marcgauthier/Crypto/internal/market"
	"math"
	"testing"
	"time"
)

func candle(t time.Time, o, h, l, c float64) market.Candle {
	return market.Candle{Time: t.UTC(), Open: o, High: h, Low: l, Close: c, Volume: 100}
}

func makeData() (mstr, mstz, btc []market.Candle) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 45; d++ {
		day := start.AddDate(0, 0, d)
		// Daily BTC closes rise +2% daily; daily 7d/1d > 0.
		price := 100 * math.Pow(1.02, float64(d))
		btc = append(btc, candle(day.Add(23*time.Hour), price, price, price, price))
	}
	loc, _ := time.LoadLocation("America/New_York")
	for d := 11; d < 31; d++ {
		day := start.AddDate(0, 0, d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		open := time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, loc).UTC()
		mstr = append(mstr, candle(open, 100, 101, 99, 100))
		mstz = append(mstz, candle(open, 100, 101, 99, 100))
		mstr = append(mstr, candle(open.Add(time.Hour), 100, 107, 99, 105))
		mstz = append(mstz, candle(open.Add(time.Hour), 100, 101, 97, 99))
	}
	return
}

func TestUTCMovesRequiresCompleteLastHourAndLookback(t *testing.T) {
	_, _, btc := makeData()
	all := UTCMoves(btc)
	day := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	move, ok := all[day.Unix()]
	if !ok || move.Change1dPct < 1.99 || move.Change7dPct < 14 {
		t.Fatalf("bad lookback move: %+v %t", move, ok)
	}
	if _, ok := all[time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).Unix()]; ok {
		t.Fatal("insufficient history should not produce signal")
	}
	// No partial candle or any other day's late value can backfill a gap.
	btc[10].Time = btc[10].Time.Add(-time.Hour)
	all = UTCMoves(btc)
	if _, ok := all[day.Unix()]; ok {
		t.Fatal("must not use an incomplete 00:00 BTC close")
	}
}

func TestBTCEntryAndBracketNoFutureLeak(t *testing.T) {
	a, b, btc := makeData()
	p := Params{WeekThresholdPct: 3, DayThresholdPct: 1, TakeProfitPct: 5, StopLossPct: 6}
	cfg := Config{Start: a[0].Time, End: a[len(a)-1].Time.Add(time.Hour), CostBps: 10}
	r, err := Run(a, b, btc, p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.LongTrades == 0 || r.BearTrades != 0 || r.ActiveSymbol != "" {
		t.Fatalf("unexpected direction: %+v", r)
	}
	for _, x := range r.TradeLog {
		if x.Symbol != "MSTR" || x.BTCMove.AsOf.After(x.EntryAt) || x.BTCMove.AsOf.Hour() != 0 || x.ExitReason != "take_profit" {
			t.Fatalf("bad entry: %+v", x)
		}
		if x.EntryAt.In(time.FixedZone("ET", -5*3600)).Hour() != 9 {
			t.Fatalf("not session open: %+v", x)
		}
	}
	btc[len(btc)-1].Close = 1_000_000 // Future prices cannot alter trades within earlier test range.
	next, err := Run(a, b, btc, p, cfg)
	if err != nil || next.LongTrades != r.LongTrades || next.ReturnPct != r.ReturnPct {
		t.Fatal("future leak", err, next.ReturnPct, r.ReturnPct)
	}
}

func TestBTCOnlyTradesInverseForBearishWeekAndDay(t *testing.T) {
	a, b, btc := makeData()
	for i := range btc {
		p := 100 * math.Pow(0.98, float64(i))
		btc[i].Open, btc[i].High, btc[i].Low, btc[i].Close = p, p, p, p
	}
	cfg := Config{Start: a[0].Time, End: a[len(a)-1].Time.Add(time.Hour)}
	p := Params{WeekThresholdPct: 3, DayThresholdPct: 1, TakeProfitPct: 5, StopLossPct: 3}
	r, err := Run(a, b, btc, p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.BearTrades == 0 || r.LongTrades != 0 {
		t.Fatalf("should only buy MSTZ: %+v", r)
	}
}

func TestBTCStopFirstInAmbiguousHourlyBarAndGap(t *testing.T) {
	a, b, btc := makeData()
	for i := range a {
		if i%2 == 0 {
			a[i].High, a[i].Low = 101, 99
		} else {
			a[i].High, a[i].Low = 115, 80
		}
	}
	p := Params{WeekThresholdPct: 3, DayThresholdPct: 1, TakeProfitPct: 5, StopLossPct: 4}
	cfg := Config{Start: a[0].Time, End: a[len(a)-1].Time.Add(time.Hour)}
	r, err := Run(a, b, btc, p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Trades == 0 || r.TradeLog[0].ExitReason != "stop" {
		t.Fatalf("stop must win ambiguous candle: %+v", r.TradeLog)
	}
	if math.Abs(r.TradeLog[0].ExitPrice-96) > 1e-8 {
		t.Fatalf("incorrect stop fill %+v", r.TradeLog[0])
	}
	// New session gap: carry entry from previous session until a later bar opens through stop.
	for i := range a {
		a[i].High, a[i].Low = 101, 99
	}
	// First trade continues overnight; second next-session open should gap below stop.
	if len(a) < 3 {
		t.Fatal("bad fixture")
	}
	a[2].Open, a[2].High, a[2].Low, a[2].Close = 80, 82, 79, 81
	r, err = Run(a, b, btc, p, cfg)
	if err != nil || r.Trades == 0 || r.TradeLog[0].ExitReason != "stop_gap" || r.TradeLog[0].ExitPrice != 80 {
		t.Fatalf("gap must fill at open: %+v %v", r.TradeLog, err)
	}
}

func TestBTCOptimizerOnlyTrainingAndNeedsBothDirections(t *testing.T) {
	a, b, btc := makeData()
	p := Grid{WeekThresholds: []float64{3}, DayThresholds: []float64{1}, TakeProfits: []float64{5}, StopLosses: []float64{4}, MinTrades: 1}
	cfg := Config{Start: a[0].Time, End: a[len(a)-1].Time.Add(time.Hour)}
	if _, err := Optimize(a, b, btc, cfg, p); err == nil {
		t.Fatal("require at least one historical trade on each leg")
	}
}
