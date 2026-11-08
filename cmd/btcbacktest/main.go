// btcbacktest compares a BTC-led MSTR/MSTZ rotation rule on a frozen
// historical training window and subsequent holdout. No real orders.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/marcgauthier/Crypto/internal/btcstrategy"
	"github.com/marcgauthier/Crypto/internal/cache"
	"github.com/marcgauthier/Crypto/internal/market"
)

type report struct {
	TrainingFrom  time.Time          `json:"training_from"`
	TrainingUntil time.Time          `json:"training_until"`
	TestingUntil  time.Time          `json:"testing_until"`
	Source        string             `json:"source"`
	BTCBars       int                `json:"btc_hourly_bars"`
	MSTRBars      int                `json:"mstr_hourly_bars"`
	MSTZBars      int                `json:"mstz_hourly_bars"`
	Training      btcstrategy.Result `json:"training"`
	Holdout       btcstrategy.Result `json:"out_of_sample"`
	FixedRule     btcstrategy.Result `json:"fixed_rule_holdout"`
}

func execute() error {
	endText := flag.String("end", "2026-11-07", "inclusive final UTC test day YYYY-MM-DD")
	cutoffText := flag.String("train-until", "2026-08-08", "UTC train/test boundary YYYY-MM-DD (exclusive for training)")
	months := flag.Int("months", 6, "calendar months of training + testing OHLCV")
	dataDir := flag.String("data-dir", "./data", "MetricsDB cache root; share with Crypto dashboard if stopped")
	minTrades := flag.Int("min-trades", 2, "minimum completed training trades; at least one per direction")
	cost := flag.Float64("cost-bps", 10, "estimated fees and slippage per transaction side")
	jsonOut := flag.Bool("json", false, "JSON summary suitable for CI artifacts")
	flag.Parse()
	day, err := time.Parse("2006-01-02", *endText)
	if err != nil {
		return err
	}
	end := day.AddDate(0, 0, 1)
	if end.After(time.Now().UTC()) {
		return fmt.Errorf("end date must be historical")
	}
	cutoff, err := time.Parse("2006-01-02", *cutoffText)
	if err != nil {
		return err
	}
	if *months < 2 || *months > 18 {
		return fmt.Errorf("months must be 2..18")
	}
	start := end.AddDate(0, -*months, 0)
	if !start.Before(cutoff) || !cutoff.Before(end) {
		return fmt.Errorf("training cutoff must fall within downloaded range")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	db, err := cache.OpenMetrics(*dataDir, 3650)
	if err != nil {
		return err
	}
	defer db.Close()
	provider := market.Yahoo{}
	fetch := func(symbol string, from time.Time) ([]market.Candle, error) {
		bars, stats, err := cache.Load(ctx, db, provider, symbol, from, end, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		if !*jsonOut {
			fmt.Fprintf(os.Stderr, "%s: %d bars (%d newly downloaded; %d requests)\n", symbol, len(bars), stats.DownloadedBars, stats.DownloadRequests)
		}
		return bars, nil
	}
	mstr, err := fetch("MSTR", start)
	if err != nil {
		return err
	}
	mstz, err := fetch("MSTZ", start)
	if err != nil {
		return err
	}
	// Request at least 8 prior calendar days so 7d momentum is available
	// for the first day of training. BTC-USD is a 24/7 instrument.
	btc, err := fetch("BTC-USD", start.AddDate(0, 0, -8))
	if err != nil {
		return err
	}
	if len(mstr) < 150 || len(mstz) < 150 || len(btc) < 400 {
		return fmt.Errorf("insufficient historical candles")
	}
	grid := btcstrategy.Grid{
		WeekThresholds: []float64{0.5, 1, 2, 3, 4, 6},
		DayThresholds:  []float64{0, 0.5, 1, 1.5, 2, 3},
		TakeProfits:    []float64{3, 5, 8, 12},
		StopLosses:     []float64{2, 4, 6, 8},
		MinTrades:      *minTrades,
	}
	train, err := btcstrategy.Optimize(mstr, mstz, btc, btcstrategy.Config{Start: start, End: cutoff, CostBps: *cost}, grid)
	if err != nil {
		return fmt.Errorf("BTC optimization: %w", err)
	}
	test, err := btcstrategy.Run(mstr, mstz, btc, train.Parameters, btcstrategy.Config{Start: cutoff, End: end, CostBps: *cost})
	if err != nil {
		return err
	}
	// A priori sensitivity check: unoptimized 2% weekly +1% daily, +5/-4 exit.
	control, err := btcstrategy.Run(mstr, mstz, btc, btcstrategy.Params{WeekThresholdPct: 2, DayThresholdPct: 1, TakeProfitPct: 5, StopLossPct: 4}, btcstrategy.Config{Start: cutoff, End: end, CostBps: *cost})
	if err != nil {
		return err
	}
	// Equity series are used internally to calculate drawdowns; omit verbose
	// per-bar arrays from the permanent JSON result.
	train.Equity = nil
	test.Equity = nil
	control.Equity = nil
	result := report{TrainingFrom: start, TrainingUntil: cutoff, TestingUntil: end, Source: "Yahoo Finance hourly, UTC BTC daily close -> next US market session open", BTCBars: len(btc), MSTRBars: len(mstr), MSTZBars: len(mstz), Training: train, Holdout: test, FixedRule: control}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	fmt.Printf("BTC signal: 7-day >= +%.1f%% and 1-day >= +%.1f%% -> open MSTR; 7-day <= -%.1f%% and 1-day <= -%.1f%% -> open MSTZ; otherwise CASH.\n", train.Parameters.WeekThresholdPct, train.Parameters.DayThresholdPct, train.Parameters.WeekThresholdPct, train.Parameters.DayThresholdPct)
	fmt.Printf("Exit: TP +%.1f%% / SL -%.1f%% on whichever ETF is held. Stop first if both touched.\n", train.Parameters.TakeProfitPct, train.Parameters.StopLossPct)
	fmt.Printf("Training %s..%s: %+.2f%% (%d closed trades, max DD %.2f%%)\n", start.Format("2006-01-02"), cutoff.Format("2006-01-02"), train.ReturnPct, train.Trades, train.MaxDrawdownPct)
	fmt.Printf("HOLDOUT %s..%s: %+.2f%% (%d trades: MSTR %d/MSTZ %d; max DD %.2f%%; open %s)\n", cutoff.Format("2006-01-02"), end.Format("2006-01-02"), test.ReturnPct, test.Trades, test.LongTrades, test.BearTrades, test.MaxDrawdownPct, test.ActiveSymbol)
	fmt.Printf("Fixed preset holdout +2%%/+1%% BTC, +5%%/-4%% bracket: %+.2f%% (%d trades)\n", control.ReturnPct, control.Trades)
	return nil
}

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, "btcbacktest:", err)
		os.Exit(1)
	}
}
