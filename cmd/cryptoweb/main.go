// cryptoweb runs the localhost dashboard and continuously monitors independent
// bullish/inverse ETF legs. It never submits real broker orders.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/marcgauthier/Crypto/internal/cache"
	"github.com/marcgauthier/Crypto/internal/daemon"
	"github.com/marcgauthier/Crypto/internal/dashboard"
	"github.com/marcgauthier/Crypto/internal/findings"
	"github.com/marcgauthier/Crypto/internal/market"
	"github.com/marcgauthier/Crypto/internal/monitor"
	"github.com/marcgauthier/Crypto/internal/scanner"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address (localhost by default)")
	dataDir := flag.String("data-dir", "./data", "MetricsDB cache directory")
	dbPath := flag.String("db", "", "SQLite findings database (default: <data-dir>/findings.sqlite)")
	pairsFlag := flag.String("pairs", "SOXL:SOXS,MSTR:MSTZ", "independently monitored bullish:inverse pairs, comma separated")
	days := flag.Int("days", 180, "rolling calendar days used to optimize the strategy")
	lookback := flag.Int("lookback", 24, "number of completed hourly candles in buy-drop reference")
	cost := flag.Float64("cost-bps", 10, "assumed signal transaction cost per side in basis points")
	historyOnly := flag.Bool("history-only", false, "serve historical SQLite dashboard without scheduled evaluation or quotes")
	flag.Parse()
	if *days < 30 || *days > 3650 || *lookback < 2 || *cost < 0 || *cost >= 10000 {
		log.Fatal("invalid days, lookback or cost")
	}
	pairs, err := daemon.ParsePairs(*pairsFlag)
	if err != nil {
		log.Fatal(err)
	}
	path := *dbPath
	if path == "" {
		path = filepath.Join(*dataDir, "findings.sqlite")
	}
	store, err := findings.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !*historyOnly {
		yahoo := market.Yahoo{}
		// Scanner and checker open MetricsDB sequentially, respecting the
		// storage engine's exclusive directory lock.
		s := &daemon.Service{Store: store, Pairs: pairs, Logger: log.Default()}
		s.Scan = func(ctx context.Context, pair string, at time.Time) ([]findings.Rule, error) {
			args := []string{"--pair", pair, "--days", strconv.Itoa(*days), "--lookback", strconv.Itoa(*lookback), "--cost-bps", strconv.FormatFloat(*cost, 'f', -1, 64), "--data-dir", *dataDir, "--findings-db", path, "--json"}
			var output bytes.Buffer
			if err := scanner.Run(ctx, &output, args, at); err != nil {
				return nil, err
			}
			var report scanner.Report
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				return nil, fmt.Errorf("decode optimizer report: %w", err)
			}
			day := at.UTC().Format("2006-01-02")
			rules := make([]findings.Rule, 0, len(report.Legs))
			for _, leg := range report.Legs {
				rules = append(rules, findings.Rule{Pair: pair, Symbol: leg.Symbol, Side: leg.Side, EvaluationDay: day, LookbackBars: report.Lookback, DropPct: leg.Best.DropPct, GainPct: leg.Best.GainPct})
			}
			return rules, nil
		}
		// Open MetricsDB once per check cycle; refresh today's hourly bars
		// once per completed hour, not on every 5-minute quote request.
		lastHourlySync := map[string]time.Time{}
		s.Checks = func(ctx context.Context, at time.Time) (daemon.CheckFunc, func() error, error) {
			var store *cache.MetricsStore
			var checker monitor.Checker
			check := func(ctx context.Context, r findings.Rule, now time.Time) (string, error) {
				if !monitor.ExchangeWindow(now) {
					return storeStatus(s.Store, r, now)
				}
				if store == nil {
					var err error
					store, err = cache.OpenMetrics(*dataDir, 3650)
					if err != nil {
						return "", err
					}
					checker = monitor.Checker{Store: s.Store, Hourly: store, Quotes: yahoo, CostBps: *cost}
				}
				hour := now.UTC().Truncate(time.Hour)
				if !lastHourlySync[r.Symbol].Equal(hour) {
					_, _, err := cache.Load(ctx, store, yahoo, r.Symbol, now.Add(-30*24*time.Hour), now, now)
					if err != nil {
						_ = s.Store.NoteStatus(ctx, r, now, "hourly_refresh_error", err)
						return "", err
					}
					lastHourlySync[r.Symbol] = hour
				}
				return checker.Check(ctx, r, now)
			}
			closeFn := func() error {
				if store == nil {
					return nil
				}
				return store.Close()
			}
			return check, closeFn, nil
		}
		go s.Run(ctx)
		log.Printf("Monitoring %v: evaluation at 00:00 UTC; completed 5-minute candle checks on UTC 5-minute boundaries", pairs)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: dashboard.New(store), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutCtx)
	}()
	log.Printf("Crypto dashboard: http://%s", listener.Addr())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func storeStatus(store *findings.Store, r findings.Rule, now time.Time) (string, error) {
	return "", store.NoteStatus(context.Background(), r, now, "market_closed", nil)
}
