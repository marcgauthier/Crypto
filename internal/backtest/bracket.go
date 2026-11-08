package backtest

import (
    "errors"
    "fmt"
    "math"
    "time"

    "github.com/marcgauthier/Crypto/internal/market"
)

// RunBracket buys after a completed hourly close crosses the rolling drop
// threshold (filled at next bar open), then exits via a take-profit or
// stop-loss bracket based on the raw entry open. Both orders are assumed
// active immediately after entry. If both touch in one hourly bar, the stop
// takes precedence; if a bar opens beyond a threshold, its actual open price
// is used rather than an impossible threshold fill.
func RunBracket(bars []market.Candle, p Params, c Config) (Result, error) {
    out := Result{Params: p}
    if c.Lookback < 2 || c.StartIndex < 0 || c.StartIndex >= len(bars) ||
        p.DropPct <= 0 || p.GainPct <= 0 || p.StopPct <= 0 ||
        p.DropPct >= 100 || p.StopPct >= 100 || c.CostBps < 0 || c.CostBps >= 10000 {
        return out, errors.New("invalid bracket backtest settings")
    }
    for _, b := range bars {
        if !market.Valid(b) {
            return out, errors.New("invalid OHLCV candle")
        }
    }
    rate := c.CostBps / 10000
    cash, shares := 1.0, 0.0
    var entryRaw, entryPrice float64
    var entryAt time.Time
    peakEquity := 1.0
    armed, pendingBuy := true, false
    sumTrade, totalHold := 0.0, 0.0

    for i := c.StartIndex; i < len(bars); i++ {
        b := bars[i]
        entered := false
        if pendingBuy {
            pendingBuy = false
            entryRaw = b.Open
            entryPrice = entryRaw * (1 + rate)
            shares = cash / entryPrice
            cash = 0
            entryAt = b.Time
            entered = true
        }

        if shares > 0 {
            stop := entryRaw * (1 - p.StopPct/100)
            take := entryRaw * (1 + p.GainPct/100)
            exitRaw, reason := 0.0, ""
            // Opening gaps are not filled at the old limit/stop threshold.
            // The opening bar of a new entry cannot gap past its own entry.
            if !entered && b.Open <= stop {
                exitRaw, reason = b.Open, "stop_gap"
            } else if !entered && b.Open >= take {
                exitRaw, reason = b.Open, "take_gap"
            } else if b.Low <= stop {
                // Conservative on ambiguous hourly bars crossing both levels.
                exitRaw, reason = stop, "stop"
            } else if b.High >= take {
                exitRaw, reason = take, "take_profit"
            }
            if reason != "" {
                exitPrice := exitRaw * (1 - rate)
                cash = shares * exitPrice
                tradeReturn := (exitPrice/entryPrice - 1) * 100
                hold := b.Time.Sub(entryAt).Hours()
                out.TradeLog = append(out.TradeLog, Trade{
                    BuyAt: entryAt, SellAt: b.Time,
                    BuyPrice: entryPrice, SellPrice: exitPrice,
                    ReturnPct: tradeReturn, HoldHours: hold,
                    ExitReason: reason,
                })
                out.Trades++
                if tradeReturn > 0 {
                    out.Wins++
                }
                sumTrade += tradeReturn
                totalHold += hold
                shares, entryRaw, entryPrice = 0, 0, 0
            }
        }

        equity := cash
        if shares > 0 {
            equity += shares * b.Close * (1 - rate)
        }
        out.Equity = append(out.Equity, EquityPoint{Time: b.Time, Value: equity})
        if equity > peakEquity {
            peakEquity = equity
        }
        if dd := (peakEquity-equity)/peakEquity*100; dd > out.MaxDrawdownPct {
            out.MaxDrawdownPct = dd
        }
        if i < c.Lookback-1 {
            continue
        }
        start := i-c.Lookback+1
        peak := bars[start].Close
        for j:=start+1;j<=i;j++ {
            if bars[j].Close > peak {
                peak=bars[j].Close
            }
        }
        threshold := peak * (1-p.DropPct/100)
        // Require re-arming above the threshold before a subsequent entry.
        // A stop must not continuously rebuy every bar during a large crash.
        if b.Close > threshold {
            armed = true
        } else if armed {
            armed = false
            if shares == 0 {
                pendingBuy = true
            }
        }
    }

    out.ReturnPct = (out.Equity[len(out.Equity)-1].Value-1)*100
    first := bars[c.StartIndex].Open*(1+rate)
    last := bars[len(bars)-1].Close*(1-rate)
    out.BuyHoldPct = (last/first-1)*100
    out.OpenPosition = shares>0
    if pendingBuy {
        out.PendingSignal = "buy"
    }
    if out.Trades > 0 {
        out.WinRatePct = float64(out.Wins)/float64(out.Trades)*100
        out.AvgTradePct = sumTrade/float64(out.Trades)
        out.AvgHoldHours = totalHold/float64(out.Trades)
    }
    return out, nil
}

type BracketGrid struct {
    Grid
    StopMin, StopMax, StopStep float64
}

func OptimizeBracket(bars []market.Candle, cfg Config, g BracketGrid) (Result, error) {
    if len(bars) < cfg.Lookback+3 {
        return Result{}, fmt.Errorf("insufficient candles: %d",len(bars))
    }
    if g.DropMin<=0 || g.DropMax<g.DropMin || g.DropStep<=0 ||
       g.GainMin<=0 || g.GainMax<g.GainMin || g.GainStep<=0 ||
       g.StopMin<=0 || g.StopMax<g.StopMin || g.StopStep<=0 ||
       g.StopMax>=100 || g.DropMax>=100 || g.MinTrades<0 {
        return Result{}, errors.New("invalid bracket optimization grid")
    }
    var best Result
    found:=false
    for d:=g.DropMin;d<=g.DropMax+1e-8;d+=g.DropStep {
        for gain:=g.GainMin;gain<=g.GainMax+1e-8;gain+=g.GainStep {
            for stop:=g.StopMin;stop<=g.StopMax+1e-8;stop+=g.StopStep {
                p:=Params{
                    DropPct:math.Round(d*1000)/1000,
                    GainPct:math.Round(gain*1000)/1000,
                    StopPct:math.Round(stop*1000)/1000,
                }
                r,err:=RunBracket(bars,p,cfg)
                if err!=nil {
                    return Result{},err
                }
                if r.Trades<g.MinTrades {
                    continue
                }
                if !found || r.ReturnPct>best.ReturnPct+1e-9 ||
                   (math.Abs(r.ReturnPct-best.ReturnPct)<1e-9 && r.MaxDrawdownPct<best.MaxDrawdownPct) {
                    best,found=r,true
                }
            }
        }
    }
    if !found {
        return Result{},fmt.Errorf("no bracket parameters produced at least %d completed training trades",g.MinTrades)
    }
    return best,nil
}
