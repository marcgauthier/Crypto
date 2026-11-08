// cryptoscan optimizes simple buy-the-dip/take-profit rules on independently
// traded bullish and inverse instruments, using completed hourly candles.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcgauthier/Crypto/internal/backtest"
	"github.com/marcgauthier/Crypto/internal/cache"
	"github.com/marcgauthier/Crypto/internal/findings"
	"github.com/marcgauthier/Crypto/internal/market"
)

type LegReport struct {
	Symbol  string          `json:"symbol"`
	Side    string          `json:"side"`
	Candles int             `json:"candles"`
	Cache   cache.Stats     `json:"cache"`
	Best    backtest.Params `json:"best_parameters"`
	Train   backtest.Result `json:"training"`
	Test    backtest.Result `json:"out_of_sample"`
}
type Report struct {
	Pair                       string      `json:"pair"`
	From                       time.Time   `json:"from"`
	Through                    time.Time   `json:"through"`
	TrainUntil                 time.Time   `json:"train_until"`
	Lookback                   int         `json:"lookback_hours_of_candles"`
	CostsBpsPerSide            float64     `json:"cost_bps_per_side"`
	Legs                       []LegReport `json:"legs"`
	CombinedTestReturnPct      float64     `json:"combined_test_return_pct"`
	CombinedTestMaxDrawdownPct float64     `json:"combined_test_max_drawdown_pct"`
}

func main() {
	if err := run(context.Background(), os.Stdout, os.Args[1:], time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, w io.Writer, args []string, now time.Time) error {
	fs := flag.NewFlagSet("cryptoscan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pair := fs.String("pair", "SOXL:SOXS", "bullish:bearish symbols, e.g. SOXL:SOXS or MSTR:MSTZ")
	months := fs.Int("months", 6, "calendar months of hourly data (unless --days is set)")
	days := fs.Int("days", 0, "rolling number of calendar days of hourly data; overrides --months")
	dataDir := fs.String("data-dir", "./data", "persistent MetricsDB storage directory")
	findingsPath := fs.String("findings-db", "", "SQLite history file (default: <data-dir>/findings.sqlite)")
	noSave := fs.Bool("no-save", false, "skip recording this backtest in SQLite")
	lookback := fs.Int("lookback", 24, "number of past hourly closes for rolling peak")
	split := fs.Float64("train-fraction", 0.70, "initial chronological training fraction")
	minTrades := fs.Int("min-trades", 2, "minimum closed trades on training period")
	cost := fs.Float64("cost-bps", 10, "estimated transaction fee+slippage per side, basis points")
	dropMin := fs.Float64("drop-min", 1, "minimum drop threshold (percent)")
	dropMax := fs.Float64("drop-max", 12, "maximum drop threshold (percent)")
	dropStep := fs.Float64("drop-step", 1, "drop threshold step (percentage points)")
	gainMin := fs.Float64("gain-min", 1, "minimum gain threshold (percent)")
	gainMax := fs.Float64("gain-max", 16, "maximum gain threshold (percent)")
	gainStep := fs.Float64("gain-step", 1, "gain threshold step (percentage points)")
	endDate := fs.String("end", "", "optional inclusive UTC end date, YYYY-MM-DD (for repeatable tests)")
	jsonOutput := fs.Bool("json", false, "machine-readable summary (includes last five trades)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(*pair)), ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || parts[0] == parts[1] {
		return errors.New("--pair must contain two different symbols, e.g. SOXL:SOXS")
	}
	for _, sym := range parts {
		for _, c := range sym {
			if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '.' && c != '-' {
				return fmt.Errorf("invalid ticker: %q", sym)
			}
		}
	}
	if *months < 1 || *months > 36 || *days < 0 || *days > 3650 || *lookback < 2 || *split <= 0.1 || *split >= 0.9 || *cost < 0 || *cost >= 10000 {
		return errors.New("invalid months, lookback, training fraction or cost")
	}
	grid := backtest.Grid{DropMin: *dropMin, DropMax: *dropMax, DropStep: *dropStep, GainMin: *gainMin, GainMax: *gainMax, GainStep: *gainStep, MinTrades: *minTrades}
	end := now
	if *endDate != "" {
		d, err := time.Parse("2006-01-02", *endDate)
		if err != nil {
			return fmt.Errorf("--end: %w", err)
		}
		end = d.AddDate(0, 0, 1)
		if end.After(now) {
			return errors.New("end date must not be in the future")
		}
	}
	start := end.AddDate(0, -*months, 0)
	if *days > 0 {
		start = end.Add(-time.Duration(*days) * 24 * time.Hour)
	}
	db, err := cache.OpenMetrics(*dataDir, 3650)
	if err != nil {
		return fmt.Errorf("open MetricsDB: %w", err)
	}
	defer db.Close()
	provider := market.Yahoo{}
	var candles [2][]market.Candle
	var cs [2]cache.Stats
	for i, sym := range parts {
		candles[i], cs[i], err = cache.Load(ctx, db, provider, sym, start, end, now)
		if err != nil {
			return err
		}
		if len(candles[i]) < 2*(*lookback)+10 {
			return fmt.Errorf("%s: need more than %d valid hourly candles, got %d", sym, 2*(*lookback)+10, len(candles[i]))
		}
	}
	// Use a common calendar training boundary, not separate timestamps for each leg.
	commonStart := candles[0][0].Time
	if candles[1][0].Time.After(commonStart) {
		commonStart = candles[1][0].Time
	}
	commonEnd := candles[0][len(candles[0])-1].Time
	if candles[1][len(candles[1])-1].Time.Before(commonEnd) {
		commonEnd = candles[1][len(candles[1])-1].Time
	}
	if !commonStart.Before(commonEnd) {
		return errors.New("symbols have no overlapping time range")
	}
	cutoff := commonStart.Add(time.Duration(float64(commonEnd.Sub(commonStart)) * (*split)))
	report := Report{Pair: strings.Join(parts, ":"), From: start, Through: end, TrainUntil: cutoff, Lookback: *lookback, CostsBpsPerSide: *cost}
	results := make([]backtest.Result, 2)
	for i, sym := range parts {
		bars := candles[i]
		from := 0
		for from < len(bars) && bars[from].Time.Before(commonStart) {
			from++
		}
		until := len(bars)
		for until > from && bars[until-1].Time.After(commonEnd) {
			until--
		}
		bars = bars[from:until]
		index := 0
		for index < len(bars) && bars[index].Time.Before(cutoff) {
			index++
		}
		if index < *lookback+5 || len(bars)-index < *lookback+5 {
			return fmt.Errorf("%s: too few bars in training or testing split", sym)
		}
		train, err := backtest.Optimize(bars[:index], backtest.Config{Lookback: *lookback, CostBps: *cost}, grid)
		if err != nil {
			return fmt.Errorf("%s optimization: %w", sym, err)
		}
		test, err := backtest.Run(bars, train.Params, backtest.Config{Lookback: *lookback, CostBps: *cost, StartIndex: index})
		if err != nil {
			return err
		}
		results[i] = test
		label := "bullish"
		if i == 1 {
			label = "inverse/bearish"
		}
		// Limit output size; all closed trades are included in aggregate statistics.
		if len(train.TradeLog) > 5 {
			train.TradeLog = train.TradeLog[len(train.TradeLog)-5:]
		}
		train.Equity = nil
		report.Legs = append(report.Legs, LegReport{Symbol: sym, Side: label, Candles: len(bars), Cache: cs[i], Best: train.Params, Train: train, Test: test})
	}
	report.CombinedTestReturnPct, report.CombinedTestMaxDrawdownPct = backtest.Combine(results[0].Equity, results[1].Equity)
	for i := range report.Legs {
		if len(report.Legs[i].Test.TradeLog) > 5 {
			r := report.Legs[i].Test.TradeLog
			report.Legs[i].Test.TradeLog = r[len(r)-5:]
		}
		report.Legs[i].Test.Equity = nil
	}
	if math.IsNaN(report.CombinedTestReturnPct) {
		return errors.New("invalid portfolio return")
	}
	if !*noSave {
		path := *findingsPath
		if path == "" {
			path = filepath.Join(*dataDir, "findings.sqlite")
		}
		saved, err := findings.Open(path)
		if err != nil {
			return fmt.Errorf("open findings SQLite: %w", err)
		}
		windowDays := int(math.Round(end.Sub(start).Hours() / 24))
		settings := fmt.Sprintf("v1;train=%.4f;cost=%.3f;min=%d;drop=%.3f..%.3f/%.3f;gain=%.3f..%.3f/%.3f",
			*split, *cost, *minTrades, *dropMin, *dropMax, *dropStep, *gainMin, *gainMax, *gainStep)
		legs := make([]findings.Finding, 0, len(report.Legs))
		for _, leg := range report.Legs {
			legs = append(legs, findings.Finding{
				Pair: report.Pair, Symbol: leg.Symbol, Side: leg.Side, ScannedAt: now,
				WindowDays: windowDays, WindowStart: start, WindowEnd: end,
				LookbackBars: *lookback, SettingsKey: settings,
				BuyDropPct: leg.Best.DropPct, SellGainPct: leg.Best.GainPct,
				TrainReturnPct: leg.Train.ReturnPct, TestReturnPct: leg.Test.ReturnPct,
				TestBuyHoldPct: leg.Test.BuyHoldPct, TestMaxDrawdownPct: leg.Test.MaxDrawdownPct,
				TestTrades: leg.Test.Trades, TestWinRatePct: leg.Test.WinRatePct,
				TestOpenPosition:     leg.Test.OpenPosition,
				PortfolioReturnPct:   report.CombinedTestReturnPct,
				PortfolioDrawdownPct: report.CombinedTestMaxDrawdownPct,
			})
		}
		saveErr := saved.Save(ctx, legs)
		closeErr := saved.Close()
		if saveErr != nil {
			return fmt.Errorf("save findings: %w", saveErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close findings SQLite: %w", closeErr)
		}
		if !*jsonOutput {
			fmt.Fprintf(w, "Findings saved in SQLite: %s\n", path)
		}
	}
	if *jsonOutput {
		return json.NewEncoder(w).Encode(report)
	}
	fmt.Fprintf(w, "PAIR %s | %s to %s UTC | train/test %.0f/%.0f | cost %.0f bps/side\n", report.Pair, start.Format("2006-01-02"), end.Format("2006-01-02"), *split*100, (1-*split)*100, *cost)
	fmt.Fprintf(w, "Rule: last %d completed hourly closes; signal at bar close, order at NEXT bar open.\n", *lookback)
	for _, leg := range report.Legs {
		fmt.Fprintf(w, "\n%s (%s) | %d hourly candles | cache: %d fetched / %d already stored / %d HTTP requests\n", leg.Symbol, leg.Side, leg.Candles, leg.Cache.DownloadedBars, leg.Cache.CachedBars, leg.Cache.DownloadRequests)
		fmt.Fprintf(w, "  BUY %s after a %.1f%% drop from its rolling %d-bar closing high.\n", leg.Symbol, leg.Best.DropPct, *lookback)
		fmt.Fprintf(w, "  SELL %s after a %.1f%% gain from its actual purchase price (after estimated entry cost).\n", leg.Symbol, leg.Best.GainPct)
		fmt.Fprintf(w, "  Training: %+.2f%% return | %d closed trades | %.1f%% winners | %.1f%% max drawdown\n", leg.Train.ReturnPct, leg.Train.Trades, leg.Train.WinRatePct, leg.Train.MaxDrawdownPct)
		fmt.Fprintf(w, "  OUT-OF-SAMPLE: %+.2f%% return | %d closed trades | %.1f%% winners | %.1f%% max drawdown | buy-and-hold %+.2f%%\n", leg.Test.ReturnPct, leg.Test.Trades, leg.Test.WinRatePct, leg.Test.MaxDrawdownPct, leg.Test.BuyHoldPct)
		if leg.Test.OpenPosition {
			fmt.Fprintln(w, "  Note: final trade still open; marked to market, not counted as a win.")
		}
		if leg.Test.PendingSignal != "" {
			fmt.Fprintf(w, "  Final-bar signal (not executed): %s at next available bar open.\n", strings.ToUpper(leg.Test.PendingSignal))
		}
	}
	fmt.Fprintf(w, "\nCombined 50/50 independently managed portfolio (out-of-sample): %+.2f%% return | %.2f%% max drawdown\n", report.CombinedTestReturnPct, report.CombinedTestMaxDrawdownPct)
	fmt.Fprintln(w, "Research backtest only; no orders are placed. Inverse leveraged ETFs reset daily, with path-dependent returns.")
	return nil
}
