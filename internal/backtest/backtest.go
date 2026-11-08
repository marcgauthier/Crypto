package backtest

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/marcgauthier/Crypto/internal/market"
)

type Params struct {
	DropPct float64 `json:"drop_pct"`
	GainPct float64 `json:"gain_pct"`
}
type Config struct {
	Lookback   int
	CostBps    float64
	StartIndex int
}
type EquityPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}
type Trade struct {
	BuyAt     time.Time `json:"buy_at"`
	SellAt    time.Time `json:"sell_at"`
	BuyPrice  float64   `json:"buy_price"`
	SellPrice float64   `json:"sell_price"`
	ReturnPct float64   `json:"return_pct"`
	HoldHours float64   `json:"hold_hours"`
}
type Result struct {
	Params         Params        `json:"params"`
	ReturnPct      float64       `json:"return_pct"`
	BuyHoldPct     float64       `json:"buy_hold_pct"`
	MaxDrawdownPct float64       `json:"max_drawdown_pct"`
	Trades         int           `json:"trades"`
	Wins           int           `json:"wins"`
	WinRatePct     float64       `json:"win_rate_pct"`
	AvgTradePct    float64       `json:"avg_trade_pct"`
	AvgHoldHours   float64       `json:"avg_hold_hours"`
	OpenPosition   bool          `json:"open_position"`
	PendingSignal  string        `json:"pending_signal,omitempty"`
	Equity         []EquityPoint `json:"equity,omitempty"`
	TradeLog       []Trade       `json:"trade_log,omitempty"`
}

// Run generates signals exclusively from the close of an already-completed bar
// and fills them at the next bar's open. It never inspects future candles.
// One leg starts with $1, spends it all on each buy, and holds cash while flat.
func Run(bars []market.Candle, p Params, c Config) (Result, error) {
	out := Result{Params: p}
	if c.Lookback < 2 || c.StartIndex < 0 || c.StartIndex >= len(bars) || p.DropPct <= 0 || p.GainPct <= 0 || c.CostBps < 0 || c.CostBps >= 10000 {
		return out, errors.New("invalid backtest settings")
	}
	rate := c.CostBps / 10000
	cash, shares, entry := 1.0, 0.0, 0.0
	var entryTime time.Time
	pending := ""
	var peakEq = 1.0
	sumTrade, holdTotal := 0.0, 0.0
	// All observations before StartIndex are warm-up only, never tradable.
	for i := c.StartIndex; i < len(bars); i++ {
		b := bars[i]
		switch pending {
		case "buy":
			entry = b.Open * (1 + rate)
			shares = cash / entry
			cash = 0
			entryTime = b.Time
		case "sell":
			exit := b.Open * (1 - rate)
			cash = shares * exit
			gross := (exit/entry - 1) * 100
			hold := b.Time.Sub(entryTime).Hours()
			out.TradeLog = append(out.TradeLog, Trade{BuyAt: entryTime, SellAt: b.Time, BuyPrice: entry, SellPrice: exit, ReturnPct: gross, HoldHours: hold})
			out.Trades++
			if gross > 0 {
				out.Wins++
			}
			sumTrade += gross
			holdTotal += hold
			shares = 0
			entry = 0
		}
		pending = ""
		equity := cash
		if shares > 0 {
			equity += shares * b.Close * (1 - rate)
		}
		out.Equity = append(out.Equity, EquityPoint{Time: b.Time, Value: equity})
		if equity > peakEq {
			peakEq = equity
		}
		if dd := (peakEq - equity) / peakEq * 100; dd > out.MaxDrawdownPct {
			out.MaxDrawdownPct = dd
		}
		if i < c.Lookback-1 {
			continue
		}
		if shares > 0 {
			if b.Close >= entry*(1+p.GainPct/100) {
				pending = "sell"
			}
		} else {
			start := i - c.Lookback + 1
			recentPeak := bars[start].Close
			for j := start + 1; j <= i; j++ {
				if bars[j].Close > recentPeak {
					recentPeak = bars[j].Close
				}
			}
			if b.Close <= recentPeak*(1-p.DropPct/100) {
				pending = "buy"
			}
		}
	}
	out.ReturnPct = (out.Equity[len(out.Equity)-1].Value - 1) * 100
	first := bars[c.StartIndex].Open * (1 + rate)
	last := bars[len(bars)-1].Close * (1 - rate)
	out.BuyHoldPct = (last/first - 1) * 100
	out.OpenPosition = shares > 0
	out.PendingSignal = pending
	if out.Trades > 0 {
		out.WinRatePct = float64(out.Wins) / float64(out.Trades) * 100
		out.AvgTradePct = sumTrade / float64(out.Trades)
		out.AvgHoldHours = holdTotal / float64(out.Trades)
	}
	return out, nil
}

type Grid struct {
	DropMin, DropMax, DropStep float64
	GainMin, GainMax, GainStep float64
	MinTrades                  int
}

func Optimize(bars []market.Candle, cfg Config, g Grid) (Result, error) {
	if len(bars) < cfg.Lookback+3 {
		return Result{}, fmt.Errorf("insufficient candles: %d", len(bars))
	}
	if g.DropMin <= 0 || g.DropMax < g.DropMin || g.DropStep <= 0 || g.GainMin <= 0 || g.GainMax < g.GainMin || g.GainStep <= 0 || g.MinTrades < 0 {
		return Result{}, errors.New("invalid optimization grid")
	}
	best := Result{}
	found := false
	for d := g.DropMin; d <= g.DropMax+1e-8; d += g.DropStep {
		for ga := g.GainMin; ga <= g.GainMax+1e-8; ga += g.GainStep {
			r, err := Run(bars, Params{DropPct: math.Round(d*1000) / 1000, GainPct: math.Round(ga*1000) / 1000}, cfg)
			if err != nil {
				return Result{}, err
			}
			if r.Trades < g.MinTrades {
				continue
			}
			if !found || r.ReturnPct > best.ReturnPct+1e-9 || (math.Abs(r.ReturnPct-best.ReturnPct) < 1e-9 && r.MaxDrawdownPct < best.MaxDrawdownPct) {
				best = r
				found = true
			}
		}
	}
	if !found {
		return Result{}, fmt.Errorf("no parameter combination completed at least %d trades; lower --min-trades or widen ranges", g.MinTrades)
	}
	return best, nil
}

// Combine calculates a 50/50 portfolio of independent, non-rebalanced cash
// accounts. Each leg may be invested, uninvested, or invested simultaneously.
func Combine(a, b []EquityPoint) (totalPct, maxDrawdownPct float64) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0
	}
	type event struct {
		t   time.Time
		leg int
		val float64
	}
	events := make([]event, 0, len(a)+len(b))
	for _, p := range a {
		events = append(events, event{p.Time, 0, p.Value})
	}
	for _, p := range b {
		events = append(events, event{p.Time, 1, p.Value})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].t.Before(events[j].t) })
	vals := [2]float64{1, 1}
	peak := 1.0
	for i := 0; i < len(events); {
		current := events[i].t
		for i < len(events) && events[i].t.Equal(current) {
			e := events[i]
			vals[e.leg] = e.val
			i++
		}
		eq := (vals[0] + vals[1]) / 2
		if eq > peak {
			peak = eq
		}
		if dd := (peak - eq) / peak * 100; dd > maxDrawdownPct {
			maxDrawdownPct = dd
		}
	}
	return ((vals[0]+vals[1])/2 - 1) * 100, maxDrawdownPct
}
