// Package btcstrategy backtests BTC-led entry signals into MSTR or MSTZ.
// BTC signals use only completed UTC-day closes. Entry can only occur at the
// first common US regular-session bar after midnight; exits use bracket OHLC.
package btcstrategy

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/marcgauthier/Crypto/internal/market"
)

const LongSymbol = "MSTR"
const BearSymbol = "MSTZ"

type Params struct {
	WeekThresholdPct float64 `json:"btc_7d_threshold_pct"`
	DayThresholdPct  float64 `json:"btc_1d_threshold_pct"`
	TakeProfitPct    float64 `json:"take_profit_pct"`
	StopLossPct      float64 `json:"stop_loss_pct"`
}

type Config struct {
	CostBps float64
	Start   time.Time // inclusive UTC; accounts start flat at the beginning of this period
	End     time.Time // exclusive UTC
}

type BTCMove struct {
	AsOf        time.Time `json:"as_of_utc"`
	Change1dPct float64   `json:"change_1d_pct"`
	Change7dPct float64   `json:"change_7d_pct"`
}

type Trade struct {
	Symbol     string    `json:"symbol"`
	EntryAt    time.Time `json:"entry_at"`
	ExitAt     time.Time `json:"exit_at"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	ReturnPct  float64   `json:"return_pct"`
	ExitReason string    `json:"exit_reason"`
	BTCMove    BTCMove   `json:"btc_entry_signal"`
}

type Point struct {
	Time   time.Time `json:"time"`
	Equity float64   `json:"equity"`
}

type Result struct {
	Parameters       Params  `json:"parameters"`
	ReturnPct        float64 `json:"return_pct"`
	MaxDrawdownPct   float64 `json:"max_drawdown_pct"`
	Trades           int     `json:"trades"`
	LongTrades       int     `json:"mstr_trades"`
	BearTrades       int     `json:"mstz_trades"`
	Wins             int     `json:"wins"`
	WinRatePct       float64 `json:"win_rate_pct"`
	ActiveSymbol     string  `json:"active_symbol,omitempty"`
	EligibleSessions int     `json:"eligible_sessions"`
	Equity           []Point `json:"equity,omitempty"`
	TradeLog         []Trade `json:"trade_log,omitempty"`
}

// UTCMoves requires the closing 23:00-00:00 UTC hourly BTC candle for each
// referenced UTC day. It never uses partial current-day prices, intermittent
// prior closes, or next-day price information.
func UTCMoves(btc []market.Candle) map[int64]BTCMove {
	closes := make(map[int64]float64)
	for _, c := range btc {
		t := c.Time.UTC()
		if !market.Valid(c) || t.Hour() != 23 || t.Minute() != 0 || t.Second() != 0 {
			continue
		}
		end := t.Add(time.Hour)
		closes[end.Unix()] = c.Close
	}
	out := make(map[int64]BTCMove)
	for unix, price := range closes {
		t := time.Unix(unix, 0).UTC()
		priorDay, okDay := closes[t.AddDate(0, 0, -1).Unix()]
		priorWeek, okWeek := closes[t.AddDate(0, 0, -7).Unix()]
		if !okDay || !okWeek || priorDay <= 0 || priorWeek <= 0 {
			continue
		}
		out[unix] = BTCMove{AsOf: t, Change1dPct: (price/priorDay - 1) * 100, Change7dPct: (price/priorWeek - 1) * 100}
	}
	return out
}

func direction(b BTCMove, p Params) string {
	if b.Change1dPct >= p.DayThresholdPct && b.Change7dPct >= p.WeekThresholdPct {
		return LongSymbol
	}
	if b.Change1dPct <= -p.DayThresholdPct && b.Change7dPct <= -p.WeekThresholdPct {
		return BearSymbol
	}
	return ""
}

type barPair struct {
	t    time.Time
	a, b market.Candle
}

// commonBars conservatively ignores unmatched hourly bars; it will never use
// a later candle for either symbol when deciding an earlier action.
func commonBars(a, b []market.Candle, start, end time.Time) ([]barPair, error) {
	if !start.Before(end) {
		return nil, errors.New("invalid evaluation range")
	}
	seen := make(map[int64]market.Candle, len(b))
	for _, q := range b {
		if market.Valid(q) && !q.Time.Before(start) && q.Time.Before(end) {
			seen[q.Time.Unix()] = q
		}
	}
	out := make([]barPair, 0, len(a))
	for _, q := range a {
		if !market.Valid(q) || q.Time.Before(start) || !q.Time.Before(end) {
			continue
		}
		if other, ok := seen[q.Time.Unix()]; ok {
			out = append(out, barPair{t: q.Time, a: q, b: other})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].t.Before(out[j].t) })
	if len(out) < 10 {
		return nil, fmt.Errorf("insufficient overlapping US market bars: %d", len(out))
	}
	return out, nil
}

// Run invests one shared cash account in either MSTR or MSTZ, never both.
// Every new trading day's first common market candle is eligible for entry.
// BTC signals come only from the previous *completed* UTC day (00:00 close).
// A position is exited solely via TP/SL, even if BTC changes direction.
// Stop-first is used on hourly candles touching both levels. A gap through a
// bracket is filled at the actual stock opening price, not the threshold.
func Run(mstr, mstz, btc []market.Candle, p Params, c Config) (Result, error) {
	out := Result{Parameters: p}
	if p.WeekThresholdPct <= 0 || p.DayThresholdPct < 0 || p.TakeProfitPct <= 0 || p.StopLossPct <= 0 || p.StopLossPct >= 100 || c.CostBps < 0 || c.CostBps >= 10000 {
		return out, errors.New("invalid BTC signal or bracket configuration")
	}
	pairs, err := commonBars(mstr, mstz, c.Start, c.End)
	if err != nil {
		return out, err
	}
	moves := UTCMoves(btc)
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		return out, err
	}
	cash, shares := 1.0, 0.0
	entryRaw, entryNet := 0.0, 0.0
	current := ""
	var entryAt time.Time
	var entryMove BTCMove
	peakEq := 1.0
	rate := c.CostBps / 10000
	priorSession := ""
	for _, pair := range pairs {
		session := pair.t.In(ny).Format("2006-01-02")
		firstBar := session != priorSession
		priorSession = session
		justEntered := false
		// An opening signal uses BTC's 00:00 UTC daily close, and earlier
		// BTC returns, all already available before the US stock-market open.
		if firstBar && current == "" {
			midnight := time.Date(pair.t.Year(), pair.t.Month(), pair.t.Day(), 0, 0, 0, 0, time.UTC)
			if move, ok := moves[midnight.Unix()]; ok {
				out.EligibleSessions++
				which := direction(move, p)
				if which != "" {
					stock := pair.a
					if which == BearSymbol {
						stock = pair.b
					}
					current = which
					shares = cash / (stock.Open * (1 + rate))
					entryRaw, entryNet = stock.Open, stock.Open*(1+rate)
					cash = 0
					entryAt = pair.t
					entryMove = move
					justEntered = true
				}
			}
		}
		if current != "" {
			stock := pair.a
			if current == BearSymbol {
				stock = pair.b
			}
			stop := entryRaw * (1 - p.StopLossPct/100)
			target := entryRaw * (1 + p.TakeProfitPct/100)
			exitRaw, reason := 0.0, ""
			if !justEntered && stock.Open <= stop {
				exitRaw, reason = stock.Open, "stop_gap"
			} else if !justEntered && stock.Open >= target {
				exitRaw, reason = stock.Open, "take_gap"
			} else if stock.Low <= stop {
				exitRaw, reason = stop, "stop"
			} else if stock.High >= target {
				exitRaw, reason = target, "take_profit"
			}
			if reason != "" {
				fill := exitRaw * (1 - rate)
				cash = shares * fill
				ret := (fill/entryNet - 1) * 100
				out.TradeLog = append(out.TradeLog, Trade{Symbol: current, EntryAt: entryAt, ExitAt: pair.t, EntryPrice: entryNet, ExitPrice: fill, ReturnPct: ret, ExitReason: reason, BTCMove: entryMove})
				out.Trades++
				if current == LongSymbol {
					out.LongTrades++
				} else {
					out.BearTrades++
				}
				if ret > 0 {
					out.Wins++
				}
				current = ""
				shares = 0
			}
		}
		equity := cash
		if current == LongSymbol {
			equity = shares * pair.a.Close * (1 - rate)
		}
		if current == BearSymbol {
			equity = shares * pair.b.Close * (1 - rate)
		}
		out.Equity = append(out.Equity, Point{Time: pair.t, Equity: equity})
		if equity > peakEq {
			peakEq = equity
		}
		if drawdown := (peakEq - equity) / peakEq * 100; drawdown > out.MaxDrawdownPct {
			out.MaxDrawdownPct = drawdown
		}
	}
	out.ReturnPct = (out.Equity[len(out.Equity)-1].Equity - 1) * 100
	out.ActiveSymbol = current
	if out.Trades > 0 {
		out.WinRatePct = float64(out.Wins) / float64(out.Trades) * 100
	}
	return out, nil
}

type Grid struct {
	WeekThresholds, DayThresholds, TakeProfits, StopLosses []float64
	MinTrades                                              int
}

func Optimize(mstr, mstz, btc []market.Candle, c Config, g Grid) (Result, error) {
	if len(g.WeekThresholds) == 0 || len(g.DayThresholds) == 0 || len(g.TakeProfits) == 0 || len(g.StopLosses) == 0 || g.MinTrades < 1 {
		return Result{}, errors.New("invalid BTC optimization grid")
	}
	var best Result
	found := false
	for _, w := range g.WeekThresholds {
		for _, d := range g.DayThresholds {
			for _, tp := range g.TakeProfits {
				for _, sl := range g.StopLosses {
					p := Params{WeekThresholdPct: w, DayThresholdPct: d, TakeProfitPct: tp, StopLossPct: sl}
					r, err := Run(mstr, mstz, btc, p, c)
					if err != nil {
						return Result{}, err
					}
					if r.Trades < g.MinTrades || r.LongTrades == 0 || r.BearTrades == 0 {
						continue
					}
					// Maximize net returns, use lower drawdown as tie breaker.
					if !found || r.ReturnPct > best.ReturnPct+1e-8 || (math.Abs(r.ReturnPct-best.ReturnPct) < 1e-8 && r.MaxDrawdownPct < best.MaxDrawdownPct) {
						best, found = r, true
					}
				}
			}
		}
	}
	if !found {
		return Result{}, fmt.Errorf("no parameters completed at least %d trades with both MSTR and MSTZ; widen the thresholds", g.MinTrades)
	}
	return best, nil
}
