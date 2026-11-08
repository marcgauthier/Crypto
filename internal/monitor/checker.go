// Package monitor compares completed 5-minute prices against the independently
// optimized daily rules, persisting indicative paper BUY/SELL signals in SQLite.
package monitor

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/marcgauthier/Crypto/internal/findings"
	"github.com/marcgauthier/Crypto/internal/market"
)

type Bars interface {
	ReadBars(context.Context, string, time.Time, time.Time) ([]market.Candle, error)
}

type QuoteProvider interface {
	FetchInterval(context.Context, string, time.Time, time.Time, string) ([]market.Candle, error)
}

type Checker struct {
	Store   *findings.Store
	Hourly  Bars
	Quotes  QuoteProvider
	CostBps float64
}

// ExchangeWindow includes the close of the first 09:30 ET candle and allows
// a brief delay following 16:00 ET. Holidays are handled by stale/no quotes.
func ExchangeWindow(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	n := now.In(loc)
	if n.Weekday() == time.Saturday || n.Weekday() == time.Sunday {
		return false
	}
	minute := n.Hour()*60 + n.Minute()
	return minute >= 9*60+35 && minute <= 16*60+10
}

// Check skips partial/stale candles. A repeated closed candle is deliberately
// idempotent, even across restarts, to prevent repeated BUY/SELL alerts.
func (c Checker) Check(ctx context.Context, r findings.Rule, now time.Time) (string, error) {
	if !ExchangeWindow(now) {
		return "", c.Store.NoteStatus(ctx, r, now, "market_closed", nil)
	}
	quotes, err := c.Quotes.FetchInterval(ctx, r.Symbol, now.Add(-90*time.Minute), now, "5m")
	if err != nil {
		_ = c.Store.NoteStatus(ctx, r, now, "provider_error", err)
		return "", fmt.Errorf("%s 5m quote: %w", r.Symbol, err)
	}
	var quote market.Candle
	for _, q := range quotes {
		if !market.Valid(q) {
			continue
		}
		closeAt := q.Time.Add(5 * time.Minute)
		if closeAt.After(now) {
			continue
		} // provider may include forming bar
		if quote.Time.IsZero() || q.Time.After(quote.Time) {
			quote = q
		}
	}
	if quote.Time.IsZero() || now.Sub(quote.Time.Add(5*time.Minute)) > 15*time.Minute || now.Before(quote.Time.Add(5*time.Minute)) {
		return "", c.Store.NoteStatus(ctx, r, now, "stale_quote", nil)
	}
	bars, err := c.Hourly.ReadBars(ctx, r.Symbol, now.Add(-30*24*time.Hour), now)
	if err != nil {
		_ = c.Store.NoteStatus(ctx, r, now, "hourly_cache_error", err)
		return "", fmt.Errorf("%s hourly cache: %w", r.Symbol, err)
	}
	complete := make([]market.Candle, 0, len(bars))
	for _, b := range bars {
		if b.Time.Add(time.Hour).After(quote.Time.Add(5 * time.Minute)) {
			continue
		}
		complete = append(complete, b)
	}
	if len(complete) < r.LookbackBars {
		return "", c.Store.NoteStatus(ctx, r, now, "insufficient_hourly_data", nil)
	}
	recent := complete[len(complete)-r.LookbackBars:]
	peak := recent[0].Close
	for _, b := range recent[1:] {
		peak = math.Max(peak, b.Close)
	}
	return c.Store.ApplyQuote(ctx, r, quote.Time.Add(5*time.Minute), now, quote.Close, peak, c.CostBps)
}
