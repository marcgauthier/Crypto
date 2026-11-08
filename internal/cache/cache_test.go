package cache

import (
	"context"
	"errors"
	"github.com/marcgauthier/Crypto/internal/market"
	"testing"
	"time"
)

type mem struct {
	bars map[string]map[int64]market.Candle
	done map[string]map[int64]bool
}

func newMem() *mem {
	return &mem{bars: map[string]map[int64]market.Candle{}, done: map[string]map[int64]bool{}}
}
func (s *mem) ReadBars(_ context.Context, sym string, start, end time.Time) ([]market.Candle, error) {
	var b []market.Candle
	for _, c := range s.bars[sym] {
		if !c.Time.Before(start) && c.Time.Before(end) {
			b = append(b, c)
		}
	}
	return b, nil
}
func (s *mem) ReadCovered(_ context.Context, sym string, start, end time.Time) (map[int64]bool, error) {
	r := map[int64]bool{}
	for ts := range s.done[sym] {
		t := time.Unix(ts, 0)
		if !t.Before(start) && t.Before(end) {
			r[ts] = true
		}
	}
	return r, nil
}
func (s *mem) WriteBars(sym string, bars []market.Candle) error {
	if s.bars[sym] == nil {
		s.bars[sym] = map[int64]market.Candle{}
	}
	for _, c := range bars {
		s.bars[sym][c.Time.Unix()] = c
	}
	return nil
}
func (s *mem) WriteCovered(sym string, days []time.Time) error {
	if s.done[sym] == nil {
		s.done[sym] = map[int64]bool{}
	}
	for _, d := range days {
		s.done[sym][d.Unix()] = true
	}
	return nil
}
func (s *mem) Sync() error { return nil }

type provider struct {
	calls int
	fail  bool
}

func (p *provider) Fetch(_ context.Context, _ string, start, end time.Time) ([]market.Candle, error) {
	p.calls++
	if p.fail {
		return nil, errors.New("offline")
	}
	var b []market.Candle
	for d := day(start); d.Before(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		b = append(b, market.Candle{Time: d.Add(14 * time.Hour), Open: 10, High: 11, Low: 9, Close: 10, Volume: 100})
	}
	return b, nil
}
func TestHistoricalDownloadOnce(t *testing.T) {
	ctx := context.Background()
	store := newMem()
	p := &provider{}
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	a, stats, err := Load(ctx, store, p, "soxl", start, end, now)
	if err != nil || len(a) != 6 || stats.DownloadRequests != 1 {
		t.Fatalf("first %+v %+v %v", a, stats, err)
	}
	p.fail = true
	b, stats, err := Load(ctx, store, p, "soxl", start, end, now)
	if err != nil || len(b) != len(a) || stats.DownloadRequests != 0 {
		t.Fatalf("re-fetch %+v %+v %v", b, stats, err)
	}
	// Another symbol is an independent data set, and should attempt a fetch.
	_, _, err = Load(ctx, store, p, "soxs", start, end, now)
	if err == nil {
		t.Fatal("second ticker should request new data")
	}
}
func TestMetricsStorePersistsAcrossReopen(t *testing.T) {
	path := t.TempDir()
	start := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	s, err := OpenMetrics(path, 3650)
	if err != nil {
		t.Fatal(err)
	}
	c := market.Candle{Time: start.Add(14 * time.Hour), Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 500}
	if err = s.WriteBars("SOXL", []market.Candle{c}); err != nil {
		t.Fatal(err)
	}
	if err = s.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = s.WriteCovered("SOXL", []time.Time{start}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenMetrics(path, 3650)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bars, err := s.ReadBars(context.Background(), "SOXL", start, start.Add(24*time.Hour))
	if err != nil || len(bars) != 1 || bars[0].Close != 10.5 {
		t.Fatalf("persistent bars %+v %v", bars, err)
	}
	covered, err := s.ReadCovered(context.Background(), "SOXL", start, start.Add(24*time.Hour))
	if err != nil || !covered[start.Unix()] {
		t.Fatalf("persistent coverage %+v %v", covered, err)
	}
}
