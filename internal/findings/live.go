package findings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

// Rule is the most recently completed UTC day's optimized plan for one instrument.
// The inverse/bearish leg is a BUY of an inverse ETF, not a margin short sale.
type Rule struct {
	Pair          string  `json:"pair"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	EvaluationDay string  `json:"evaluation_day"`
	LookbackBars  int     `json:"lookback_bars"`
	DropPct       float64 `json:"drop_pct"`
	GainPct       float64 `json:"gain_pct"`
}

// LiveState is an indicative paper position and the most recent price-check status.
type LiveState struct {
	Rule
	Position      string  `json:"position"`
	EntryPrice    float64 `json:"entry_price"`
	EntryAt       string  `json:"entry_at"`
	LastBarAt     string  `json:"last_bar_at"`
	LastPrice     float64 `json:"last_price"`
	RecentPeak    float64 `json:"recent_peak"`
	LastDropPct   float64 `json:"last_drop_pct"`
	LastCheckedAt string  `json:"last_checked_at"`
	Status        string  `json:"status"`
	Error         string  `json:"error"`
}

type Signal struct {
	ID          int64   `json:"id"`
	Pair        string  `json:"pair"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	Action      string  `json:"action"`
	BarClosedAt string  `json:"bar_closed_at"`
	DetectedAt  string  `json:"detected_at"`
	Price       float64 `json:"price"`
	EntryPrice  float64 `json:"entry_price"`
	RecentPeak  float64 `json:"recent_peak"`
	DropPct     float64 `json:"drop_pct"`
	GainPct     float64 `json:"gain_pct"`
	ReturnPct   float64 `json:"return_pct"`
}

func (s *Store) initLive() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS evaluation_days (
            pair TEXT NOT NULL, day TEXT NOT NULL, completed_at TEXT NOT NULL,
            PRIMARY KEY(pair, day))`,
		`CREATE TABLE IF NOT EXISTS live_rules (
            pair TEXT NOT NULL, symbol TEXT NOT NULL, side TEXT NOT NULL,
            evaluation_day TEXT NOT NULL, lookback_bars INTEGER NOT NULL,
            drop_pct REAL NOT NULL, gain_pct REAL NOT NULL,
            PRIMARY KEY(pair, symbol))`,
		`CREATE TABLE IF NOT EXISTS live_state (
            pair TEXT NOT NULL, symbol TEXT NOT NULL, side TEXT NOT NULL,
            position TEXT NOT NULL DEFAULT 'FLAT' CHECK(position IN ('FLAT','OPEN')),
            entry_price REAL NOT NULL DEFAULT 0, entry_at TEXT NOT NULL DEFAULT '',
            last_bar_at TEXT NOT NULL DEFAULT '', last_price REAL NOT NULL DEFAULT 0,
            recent_peak REAL NOT NULL DEFAULT 0, last_drop_pct REAL NOT NULL DEFAULT 0,
            last_checked_at TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'waiting',
            error TEXT NOT NULL DEFAULT '', PRIMARY KEY(pair,symbol))`,
		`CREATE TABLE IF NOT EXISTS live_signals (
            id INTEGER PRIMARY KEY, pair TEXT NOT NULL, symbol TEXT NOT NULL,
            side TEXT NOT NULL, action TEXT NOT NULL CHECK(action IN ('BUY','SELL')),
            bar_closed_at TEXT NOT NULL, detected_at TEXT NOT NULL,
            price REAL NOT NULL, entry_price REAL NOT NULL,
            recent_peak REAL NOT NULL, drop_pct REAL NOT NULL, gain_pct REAL NOT NULL,
            return_pct REAL NOT NULL, UNIQUE(pair,symbol,bar_closed_at))`,
		`CREATE INDEX IF NOT EXISTS live_signals_recent ON live_signals(detected_at DESC,id DESC)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("initialize live SQLite: %w", err)
		}
	}
	return nil
}

func (s *Store) Evaluated(ctx context.Context, pair, day string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM evaluation_days WHERE pair=? AND day=?`, pair, day).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return exists == 1, err
}

// PublishRules commits the successful daily evaluation marker and both rules
// together. Reloading the application never resets a paper position.
func (s *Store) PublishRules(ctx context.Context, pair, day string, rules []Rule, now time.Time) error {
	if pair == "" || len(rules) != 2 || day != now.UTC().Format("2006-01-02") {
		return errors.New("invalid daily evaluation")
	}
	if rules[0].Symbol == rules[1].Symbol {
		return errors.New("duplicate symbol")
	}
	for _, r := range rules {
		if r.Pair != pair || r.EvaluationDay != day || r.Symbol == "" || r.Side == "" || r.LookbackBars < 2 || !finitePositive(r.DropPct) || !finitePositive(r.GainPct) {
			return errors.New("invalid live rule")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rules {
		_, err = tx.ExecContext(ctx, `INSERT INTO live_rules(pair,symbol,side,evaluation_day,lookback_bars,drop_pct,gain_pct)
            VALUES(?,?,?,?,?,?,?) ON CONFLICT(pair,symbol) DO UPDATE SET side=excluded.side,
            evaluation_day=excluded.evaluation_day,lookback_bars=excluded.lookback_bars,
            drop_pct=excluded.drop_pct,gain_pct=excluded.gain_pct`, r.Pair, r.Symbol, r.Side, r.EvaluationDay, r.LookbackBars, r.DropPct, r.GainPct)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO live_state(pair,symbol,side) VALUES(?,?,?) ON CONFLICT(pair,symbol) DO UPDATE SET side=excluded.side`, r.Pair, r.Symbol, r.Side)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO evaluation_days(pair,day,completed_at) VALUES(?,?,?) ON CONFLICT(pair,day) DO NOTHING`, pair, day, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Rules(ctx context.Context, pair string) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pair,symbol,side,evaluation_day,lookback_bars,drop_pct,gain_pct FROM live_rules WHERE pair=? ORDER BY symbol`, pair)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		if err = rows.Scan(&r.Pair, &r.Symbol, &r.Side, &r.EvaluationDay, &r.LookbackBars, &r.DropPct, &r.GainPct); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func finitePositive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

// NoteStatus changes only the observation status; it never mutates a position.
func (s *Store) NoteStatus(ctx context.Context, r Rule, now time.Time, status string, problem error) error {
	message := ""
	if problem != nil {
		message = problem.Error()
	}
	if len(message) > 240 {
		message = message[:240]
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO live_state(pair,symbol,side,last_checked_at,status,error) VALUES(?,?,?,?,?,?)
       ON CONFLICT(pair,symbol) DO UPDATE SET last_checked_at=excluded.last_checked_at,status=excluded.status,error=excluded.error`,
		r.Pair, r.Symbol, r.Side, now.UTC().Format(time.RFC3339Nano), status, message)
	return err
}

// ApplyQuote commits at most one signal per symbol per completed five-minute candle.
// The stored position is an indicative, not broker-confirmed, fill at the
// completed candle close plus/minus modeled trading costs.
func (s *Store) ApplyQuote(ctx context.Context, r Rule, closedAt, checkedAt time.Time, price, peak, costBps float64) (string, error) {
	if !finitePositive(price) || !finitePositive(peak) || !finitePositive(r.DropPct) || !finitePositive(r.GainPct) || costBps < 0 || costBps >= 10000 || closedAt.After(checkedAt) || closedAt.IsZero() {
		return "", errors.New("invalid live quote")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO live_state(pair,symbol,side) VALUES(?,?,?) ON CONFLICT(pair,symbol) DO NOTHING`, r.Pair, r.Symbol, r.Side)
	if err != nil {
		return "", err
	}
	var position, entryAt, lastBar string
	var entry float64
	err = tx.QueryRowContext(ctx, `SELECT position,entry_price,entry_at,last_bar_at FROM live_state WHERE pair=? AND symbol=?`, r.Pair, r.Symbol).Scan(&position, &entry, &entryAt, &lastBar)
	if err != nil {
		return "", err
	}
	stamp := closedAt.UTC().Format(time.RFC3339Nano)
	// RFC3339Nano is lexicographically time-ordered for UTC values produced
	// at the fixed five-minute cadence; parse to avoid fractional edge cases.
	if lastBar != "" {
		previous, parseErr := time.Parse(time.RFC3339Nano, lastBar)
		if parseErr != nil {
			return "", parseErr
		}
		if !closedAt.After(previous) {
			return "", tx.Commit()
		}
	}
	drop := (1 - price/peak) * 100
	action := ""
	cost := costBps / 10000
	returnPct := 0.0
	signalEntry := entry
	if position == "FLAT" && drop >= r.DropPct {
		action = "BUY"
		position = "OPEN"
		entry = price * (1 + cost)
		signalEntry = entry
		entryAt = stamp
	} else if position == "OPEN" && price >= entry*(1+r.GainPct/100) {
		action = "SELL"
		position = "FLAT"
		returnPct = (price*(1-cost)/entry - 1) * 100
		signalEntry = entry
		entry = 0
		entryAt = ""
	}
	if action != "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO live_signals(pair,symbol,side,action,bar_closed_at,detected_at,price,entry_price,recent_peak,drop_pct,gain_pct,return_pct) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.Pair, r.Symbol, r.Side, action, stamp, checkedAt.UTC().Format(time.RFC3339Nano), price, signalEntry, peak, r.DropPct, r.GainPct, returnPct)
		if err != nil {
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE live_state SET side=?,position=?,entry_price=?,entry_at=?,last_bar_at=?,last_price=?,recent_peak=?,last_drop_pct=?,last_checked_at=?,status='watching',error='' WHERE pair=? AND symbol=?`,
		r.Side, position, entry, entryAt, stamp, price, peak, drop, checkedAt.UTC().Format(time.RFC3339Nano), r.Pair, r.Symbol)
	if err != nil {
		return "", err
	}
	return action, tx.Commit()
}

func (s *Store) LiveStates(ctx context.Context) ([]LiveState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.pair,s.symbol,s.side,COALESCE(r.evaluation_day,''),COALESCE(r.lookback_bars,0),COALESCE(r.drop_pct,0),COALESCE(r.gain_pct,0),s.position,s.entry_price,s.entry_at,s.last_bar_at,s.last_price,s.recent_peak,s.last_drop_pct,s.last_checked_at,s.status,s.error FROM live_state s LEFT JOIN live_rules r ON r.pair=s.pair AND r.symbol=s.symbol ORDER BY s.pair,s.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LiveState{}
	for rows.Next() {
		var x LiveState
		if err := rows.Scan(&x.Pair, &x.Symbol, &x.Side, &x.EvaluationDay, &x.LookbackBars, &x.DropPct, &x.GainPct, &x.Position, &x.EntryPrice, &x.EntryAt, &x.LastBarAt, &x.LastPrice, &x.RecentPeak, &x.LastDropPct, &x.LastCheckedAt, &x.Status, &x.Error); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) RecentSignals(ctx context.Context, limit int) ([]Signal, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("invalid signal limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,pair,symbol,side,action,bar_closed_at,detected_at,price,entry_price,recent_peak,drop_pct,gain_pct,return_pct FROM live_signals ORDER BY detected_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Signal{}
	for rows.Next() {
		var x Signal
		if err := rows.Scan(&x.ID, &x.Pair, &x.Symbol, &x.Side, &x.Action, &x.BarClosedAt, &x.DetectedAt, &x.Price, &x.EntryPrice, &x.RecentPeak, &x.DropPct, &x.GainPct, &x.ReturnPct); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
