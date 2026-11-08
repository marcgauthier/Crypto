package cache

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/marcgauthier/Crypto/internal/market"
	"github.com/marcgauthier/metrics"
)

const source = "yahoo_1h"
const barMetric = "stock_hourly_ohlcv"
const coverageMetric = "stock_hourly_covered_day"

// Store isolates the time-series database from the fetch planner, making the
// no-redownload behavior easy to test without contacting an external API.
type Store interface {
	ReadBars(context.Context, string, time.Time, time.Time) ([]market.Candle, error)
	ReadCovered(context.Context, string, time.Time, time.Time) (map[int64]bool, error)
	WriteBars(string, []market.Candle) error
	WriteCovered(string, []time.Time) error
	Sync() error
}

type MetricsStore struct{ DB *metrics.DB }

func OpenMetrics(dir string, retentionDays int) (*MetricsStore, error) {
	db, err := metrics.Open(dir, metrics.WithRetentionDays(retentionDays))
	if err != nil {
		return nil, err
	}
	return &MetricsStore{DB: db}, nil
}
func (s *MetricsStore) Close() error { return s.DB.Close() }

func labels(symbol string) map[string]string {
	return map[string]string{"symbol": symbol, "source": source}
}

func (s *MetricsStore) WriteBars(symbol string, bars []market.Candle) error {
	samples := make([]metrics.Sample, 0, len(bars)*5)
	for _, c := range bars {
		for _, field := range []struct {
			name  string
			value float64
		}{{"open", c.Open}, {"high", c.High}, {"low", c.Low}, {"close", c.Close}, {"volume", c.Volume}} {
			samples = append(samples, metrics.Sample{Metric: barMetric, Labels: map[string]string{"symbol": symbol, "source": source, "field": field.name}, Time: c.Time, Value: field.value})
		}
	}
	return s.DB.WriteBatch(samples)
}

func (s *MetricsStore) ReadBars(ctx context.Context, symbol string, start, end time.Time) ([]market.Candle, error) {
	ss, err := s.DB.Select(ctx, metrics.Selector{Metric: barMetric, Labels: labels(symbol)}, metrics.TimeRange{Start: start, End: end}, 10)
	if err != nil {
		return nil, err
	}
	type record struct {
		candle market.Candle
		mask   uint8
	}
	records := make(map[int64]*record)
	for _, ser := range ss {
		field := ser.Labels["field"]
		for _, p := range ser.Points {
			if p.Time.Before(start) || !p.Time.Before(end) {
				continue
			}
			ts := p.Time.UnixMilli()
			rec := records[ts]
			if rec == nil {
				rec = &record{candle: market.Candle{Time: p.Time}}
				records[ts] = rec
			}
			switch field {
			case "open":
				rec.candle.Open = p.Value
				rec.mask |= 1
			case "high":
				rec.candle.High = p.Value
				rec.mask |= 2
			case "low":
				rec.candle.Low = p.Value
				rec.mask |= 4
			case "close":
				rec.candle.Close = p.Value
				rec.mask |= 8
			case "volume":
				rec.candle.Volume = p.Value
				rec.mask |= 16
			}
		}
	}
	bars := make([]market.Candle, 0, len(records))
	for _, r := range records {
		if r.mask == 31 && market.Valid(r.candle) {
			bars = append(bars, r.candle)
		}
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Time.Before(bars[j].Time) })
	return bars, nil
}
func (s *MetricsStore) WriteCovered(symbol string, days []time.Time) error {
	rows := make([]metrics.Sample, 0, len(days))
	for _, d := range days {
		rows = append(rows, metrics.Sample{Metric: coverageMetric, Labels: labels(symbol), Time: d, Value: 1})
	}
	return s.DB.WriteBatch(rows)
}
func (s *MetricsStore) ReadCovered(ctx context.Context, symbol string, start, end time.Time) (map[int64]bool, error) {
	ss, err := s.DB.Select(ctx, metrics.Selector{Metric: coverageMetric, Labels: labels(symbol)}, metrics.TimeRange{Start: start, End: end}, 2)
	if err != nil {
		return nil, err
	}
	done := make(map[int64]bool)
	for _, ser := range ss {
		for _, p := range ser.Points {
			if !p.Time.Before(start) && p.Time.Before(end) && p.Value == 1 {
				done[p.Time.Unix()] = true
			}
		}
	}
	return done, nil
}
func (s *MetricsStore) Sync() error { return s.DB.Flush() }

// Stats counts only requests to the remote market source, not local reads.
type Stats struct {
	DownloadRequests int
	DownloadedBars   int
	CachedBars       int
	CoveredDays      int
}

func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Load uses covered-day markers to avoid repeat downloads, including market
// holidays and weekends. Today's data is refreshed, never marked as complete.
// A failed fetch never records a coverage marker.
func Load(ctx context.Context, store Store, provider market.Provider, symbol string, start, end, now time.Time) ([]market.Candle, Stats, error) {
	var stats Stats
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || !start.Before(end) || end.After(now.Add(time.Minute)) {
		return nil, stats, fmt.Errorf("invalid symbol or time range")
	}
	first, last := day(start), day(end.Add(-time.Nanosecond)).AddDate(0, 0, 1)
	today := day(now)
	historicalEnd := last
	if historicalEnd.After(today) {
		historicalEnd = today
	}
	if historicalEnd.After(first) {
		done, err := store.ReadCovered(ctx, symbol, first, historicalEnd)
		if err != nil {
			return nil, stats, err
		}
		for cursor := first; cursor.Before(historicalEnd); {
			if done[cursor.Unix()] {
				stats.CoveredDays++
				cursor = cursor.AddDate(0, 0, 1)
				continue
			}
			chunkStart := cursor
			var chunkDays []time.Time
			for cursor.Before(historicalEnd) && len(chunkDays) < 28 && !done[cursor.Unix()] {
				chunkDays = append(chunkDays, cursor)
				cursor = cursor.AddDate(0, 0, 1)
			}
			fresh, err := provider.Fetch(ctx, symbol, chunkStart, cursor)
			stats.DownloadRequests++
			if err != nil {
				return nil, stats, fmt.Errorf("download %s %s..%s: %w", symbol, chunkStart.Format("2006-01-02"), cursor.Format("2006-01-02"), err)
			}
			// A whole multi-week window without bars usually indicates a bad
			// symbol or provider outage. Do not cache it as a successful miss.
			if len(fresh) == 0 && len(chunkDays) >= 5 {
				return nil, stats, fmt.Errorf("%s: provider returned no candles for %d days; refusing to cache empty range", symbol, len(chunkDays))
			}
			if err = store.WriteBars(symbol, fresh); err != nil {
				return nil, stats, err
			}
			if err = store.Sync(); err != nil {
				return nil, stats, err
			}
			if err = store.WriteCovered(symbol, chunkDays); err != nil {
				return nil, stats, err
			}
			if err = store.Sync(); err != nil {
				return nil, stats, err
			}
			stats.DownloadedBars += len(fresh)
			stats.CoveredDays += len(chunkDays)
		}
	}
	// Current UTC day is always a moving target, so only re-fetch its portion.
	if last.After(today) && end.After(today) && marketCouldHaveCompletedBar(now) {
		from := today
		if first.After(from) {
			from = first
		}
		if from.Before(end) {
			fresh, err := provider.Fetch(ctx, symbol, from, end)
			stats.DownloadRequests++
			if err != nil {
				return nil, stats, err
			}
			// Hourly bars still in progress are deliberately excluded.
			complete := fresh[:0]
			for _, c := range fresh {
				if !c.Time.Add(time.Hour).After(now) {
					complete = append(complete, c)
				}
			}
			if err = store.WriteBars(symbol, complete); err != nil {
				return nil, stats, err
			}
			if len(complete) > 0 {
				if err = store.Sync(); err != nil {
					return nil, stats, err
				}
			}
			stats.DownloadedBars += len(complete)
		}
	}
	bars, err := store.ReadBars(ctx, symbol, start, end)
	if err != nil {
		return nil, stats, err
	}
	stats.CachedBars = len(bars) - stats.DownloadedBars
	if stats.CachedBars < 0 {
		stats.CachedBars = 0
	}
	return bars, stats, nil
}

// Avoid empty intraday requests on weekends and before the first NYSE hourly
// candle can have completed. Individual exchange holidays remain provider-defined.
func marketCouldHaveCompletedBar(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	local := now.In(loc)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	return local.Hour() >= 11
}
