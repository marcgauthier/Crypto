// Package findings persists optimization history in SQLite, independently from
// MetricsDB's historical market candles. A completed scan writes both legs
// atomically and same-day repeats with identical settings update existing rows.
package findings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

// Finding is one strategy leg from a fully evaluated scan. Values are
// percentages (e.g., 3 means 3%, not 0.03).
type Finding struct {
	ID                   int64     `json:"id"`
	Pair                 string    `json:"pair"`
	Symbol               string    `json:"symbol"`
	Side                 string    `json:"side"`
	AnalysisDay          string    `json:"analysis_day"`
	ScannedAt            time.Time `json:"scanned_at"`
	WindowDays           int       `json:"window_days"`
	WindowStart          time.Time `json:"window_start"`
	WindowEnd            time.Time `json:"window_end"`
	LookbackBars         int       `json:"lookback_bars"`
	SettingsKey          string    `json:"settings_key"`
	BuyDropPct           float64   `json:"buy_drop_pct"`
	SellGainPct          float64   `json:"sell_gain_pct"`
	TrainReturnPct       float64   `json:"train_return_pct"`
	TestReturnPct        float64   `json:"test_return_pct"`
	TestBuyHoldPct       float64   `json:"test_buy_hold_pct"`
	TestMaxDrawdownPct   float64   `json:"test_max_drawdown_pct"`
	TestTrades           int       `json:"test_trades"`
	TestWinRatePct       float64   `json:"test_win_rate_pct"`
	TestOpenPosition     bool      `json:"test_open_position"`
	PortfolioReturnPct   float64   `json:"portfolio_return_pct"`
	PortfolioDrawdownPct float64   `json:"portfolio_drawdown_pct"`
}

type Group struct {
	Pair         string `json:"pair"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	WindowDays   int    `json:"window_days"`
	LookbackBars int    `json:"lookback_bars"`
	SettingsKey  string `json:"settings_key"`
	Runs         int    `json:"runs"`
	LatestDay    string `json:"latest_day"`
}

type Stability struct {
	Runs              int     `json:"runs"`
	MeanBuyDropPct    float64 `json:"mean_buy_drop_pct"`
	StdDevBuyDropPct  float64 `json:"stddev_buy_drop_pct"`
	MinBuyDropPct     float64 `json:"min_buy_drop_pct"`
	MaxBuyDropPct     float64 `json:"max_buy_drop_pct"`
	MeanSellGainPct   float64 `json:"mean_sell_gain_pct"`
	StdDevSellGainPct float64 `json:"stddev_sell_gain_pct"`
	MinSellGainPct    float64 `json:"min_sell_gain_pct"`
	MaxSellGainPct    float64 `json:"max_sell_gain_pct"`
	// Repeated scans of heavily overlapping windows are not independent tests.
	Note string `json:"note"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("SQLite filename must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		"PRAGMA busy_timeout=5000",
		"PRAGMA journal_mode=WAL",
		`CREATE TABLE IF NOT EXISTS findings (
            id INTEGER PRIMARY KEY,
            pair TEXT NOT NULL,
            symbol TEXT NOT NULL,
            side TEXT NOT NULL,
            analysis_day TEXT NOT NULL,
            scanned_at TEXT NOT NULL,
            window_days INTEGER NOT NULL CHECK(window_days > 0),
            window_start TEXT NOT NULL,
            window_end TEXT NOT NULL,
            lookback_bars INTEGER NOT NULL CHECK(lookback_bars > 1),
            settings_key TEXT NOT NULL,
            buy_drop_pct REAL NOT NULL,
            sell_gain_pct REAL NOT NULL,
            train_return_pct REAL NOT NULL,
            test_return_pct REAL NOT NULL,
            test_buy_hold_pct REAL NOT NULL,
            test_max_drawdown_pct REAL NOT NULL,
            test_trades INTEGER NOT NULL,
            test_win_rate_pct REAL NOT NULL,
            test_open_position INTEGER NOT NULL,
            portfolio_return_pct REAL NOT NULL,
            portfolio_drawdown_pct REAL NOT NULL,
            UNIQUE (pair, symbol, analysis_day, window_days, lookback_bars, settings_key)
        )`,
		"CREATE INDEX IF NOT EXISTS findings_history ON findings(pair, symbol, window_days, lookback_bars, settings_key, analysis_day)",
	} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize SQLite: %w", err)
		}
	}
	store := &Store{db: db}
	if err := store.initLive(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func validate(f Finding) error {
	if f.Pair == "" || f.Symbol == "" || f.Side == "" || f.SettingsKey == "" || f.WindowDays < 1 || f.LookbackBars < 2 || f.WindowStart.IsZero() || !f.WindowStart.Before(f.WindowEnd) || f.ScannedAt.IsZero() {
		return errors.New("invalid finding metadata")
	}
	if f.BuyDropPct <= 0 || f.SellGainPct <= 0 || f.TestTrades < 0 {
		return errors.New("invalid strategy finding")
	}
	for _, v := range []float64{f.BuyDropPct, f.SellGainPct, f.TrainReturnPct, f.TestReturnPct, f.TestBuyHoldPct, f.TestMaxDrawdownPct, f.TestWinRatePct, f.PortfolioReturnPct, f.PortfolioDrawdownPct} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("non-finite finding metric")
		}
	}
	return nil
}

// Save writes every leg in a single transaction; a rerun for the same UTC
// analysis day/window/settings replaces its previous result, not another point.
func (s *Store) Save(ctx context.Context, legs []Finding) error {
	if len(legs) == 0 {
		return errors.New("no findings to save")
	}
	for _, f := range legs {
		if err := validate(f); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const query = `INSERT INTO findings (
        pair, symbol, side, analysis_day, scanned_at, window_days,
        window_start, window_end, lookback_bars, settings_key, buy_drop_pct,
        sell_gain_pct, train_return_pct, test_return_pct, test_buy_hold_pct,
        test_max_drawdown_pct, test_trades, test_win_rate_pct, test_open_position,
        portfolio_return_pct, portfolio_drawdown_pct
    ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
    ON CONFLICT(pair,symbol,analysis_day,window_days,lookback_bars,settings_key)
    DO UPDATE SET side=excluded.side, scanned_at=excluded.scanned_at,
        window_start=excluded.window_start, window_end=excluded.window_end,
        buy_drop_pct=excluded.buy_drop_pct, sell_gain_pct=excluded.sell_gain_pct,
        train_return_pct=excluded.train_return_pct, test_return_pct=excluded.test_return_pct,
        test_buy_hold_pct=excluded.test_buy_hold_pct,
        test_max_drawdown_pct=excluded.test_max_drawdown_pct,
        test_trades=excluded.test_trades, test_win_rate_pct=excluded.test_win_rate_pct,
        test_open_position=excluded.test_open_position,
        portfolio_return_pct=excluded.portfolio_return_pct,
        portfolio_drawdown_pct=excluded.portfolio_drawdown_pct`
	for _, f := range legs {
		day := f.WindowEnd.UTC().Format("2006-01-02")
		open := 0
		if f.TestOpenPosition {
			open = 1
		}
		if _, err := tx.ExecContext(ctx, query,
			f.Pair, f.Symbol, f.Side, day, f.ScannedAt.UTC().Format(time.RFC3339Nano),
			f.WindowDays, f.WindowStart.UTC().Format(time.RFC3339Nano), f.WindowEnd.UTC().Format(time.RFC3339Nano),
			f.LookbackBars, f.SettingsKey, f.BuyDropPct, f.SellGainPct,
			f.TrainReturnPct, f.TestReturnPct, f.TestBuyHoldPct, f.TestMaxDrawdownPct,
			f.TestTrades, f.TestWinRatePct, open, f.PortfolioReturnPct, f.PortfolioDrawdownPct,
		); err != nil {
			return fmt.Errorf("save %s: %w", f.Symbol, err)
		}
	}
	return tx.Commit()
}

func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pair, symbol, MAX(side), window_days, lookback_bars, settings_key,
       COUNT(*), MAX(analysis_day) FROM findings
       GROUP BY pair, symbol, window_days, lookback_bars, settings_key
       ORDER BY MAX(analysis_day) DESC, pair, symbol, window_days DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]Group, 0)
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.Pair, &g.Symbol, &g.Side, &g.WindowDays, &g.LookbackBars, &g.SettingsKey, &g.Runs, &g.LatestDay); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) History(ctx context.Context, g Group, limit int) ([]Finding, error) {
	if g.Pair == "" || g.Symbol == "" || g.SettingsKey == "" {
		return nil, errors.New("missing history filter")
	}
	if limit < 1 || limit > 2000 {
		return nil, errors.New("history limit must be between 1 and 2000")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, pair, symbol, side, analysis_day, scanned_at, window_days,
       window_start, window_end, lookback_bars, settings_key, buy_drop_pct, sell_gain_pct,
       train_return_pct, test_return_pct, test_buy_hold_pct, test_max_drawdown_pct,
       test_trades, test_win_rate_pct, test_open_position, portfolio_return_pct, portfolio_drawdown_pct
       FROM findings WHERE pair=? AND symbol=? AND window_days=? AND lookback_bars=? AND settings_key=?
       ORDER BY analysis_day DESC, id DESC LIMIT ?`,
		g.Pair, g.Symbol, g.WindowDays, g.LookbackBars, g.SettingsKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := make([]Finding, 0)
	for rows.Next() {
		var f Finding
		var scanned, start, end string
		var open int
		if err := rows.Scan(&f.ID, &f.Pair, &f.Symbol, &f.Side, &f.AnalysisDay, &scanned, &f.WindowDays,
			&start, &end, &f.LookbackBars, &f.SettingsKey, &f.BuyDropPct, &f.SellGainPct,
			&f.TrainReturnPct, &f.TestReturnPct, &f.TestBuyHoldPct, &f.TestMaxDrawdownPct,
			&f.TestTrades, &f.TestWinRatePct, &open, &f.PortfolioReturnPct, &f.PortfolioDrawdownPct); err != nil {
			return nil, err
		}
		if f.ScannedAt, err = time.Parse(time.RFC3339Nano, scanned); err != nil {
			return nil, err
		}
		if f.WindowStart, err = time.Parse(time.RFC3339Nano, start); err != nil {
			return nil, err
		}
		if f.WindowEnd, err = time.Parse(time.RFC3339Nano, end); err != nil {
			return nil, err
		}
		f.TestOpenPosition = open != 0
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

// Summarize uses sample standard deviation in percentage points, based on
// distinct UTC analysis days in a consistent symbol/window/settings group.
func Summarize(rows []Finding) Stability {
	s := Stability{Runs: len(rows), Note: "Rolling windows overlap; consistent thresholds alone do not prove predictive performance."}
	if len(rows) == 0 {
		return s
	}
	s.MinBuyDropPct, s.MaxBuyDropPct = rows[0].BuyDropPct, rows[0].BuyDropPct
	s.MinSellGainPct, s.MaxSellGainPct = rows[0].SellGainPct, rows[0].SellGainPct
	for _, r := range rows {
		s.MeanBuyDropPct += r.BuyDropPct
		s.MeanSellGainPct += r.SellGainPct
		s.MinBuyDropPct = math.Min(s.MinBuyDropPct, r.BuyDropPct)
		s.MaxBuyDropPct = math.Max(s.MaxBuyDropPct, r.BuyDropPct)
		s.MinSellGainPct = math.Min(s.MinSellGainPct, r.SellGainPct)
		s.MaxSellGainPct = math.Max(s.MaxSellGainPct, r.SellGainPct)
	}
	n := float64(len(rows))
	s.MeanBuyDropPct /= n
	s.MeanSellGainPct /= n
	if len(rows) > 1 {
		for _, r := range rows {
			s.StdDevBuyDropPct += math.Pow(r.BuyDropPct-s.MeanBuyDropPct, 2)
			s.StdDevSellGainPct += math.Pow(r.SellGainPct-s.MeanSellGainPct, 2)
		}
		s.StdDevBuyDropPct = math.Sqrt(s.StdDevBuyDropPct / (n - 1))
		s.StdDevSellGainPct = math.Sqrt(s.StdDevSellGainPct / (n - 1))
	}
	if len(rows) < 5 {
		s.Note = "Fewer than five distinct scan days: insufficient history to evaluate stability. Rolling windows overlap, so even a long consistent history is not proof of future performance."
	}
	return s
}
