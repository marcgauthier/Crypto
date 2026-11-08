# Crypto — hourly dip/gain strategy research

Go CLI that downloads up to six months (configurable) of **1-hour candles** for
any bullish/inverse pair, persists every requested historical day in
[`marcgauthier/metrics`](https://github.com/marcgauthier/metrics) (embedded
VictoriaMetrics storage), and searches independently for simple **buy after a
X% drop; sell after a Y% gain** rules.

## Run

Requires **Go 1.26.10+** and network access for the initial data download.

```bash
go mod tidy
go run ./cmd/cryptoscan --pair SOXL:SOXS
go run ./cmd/cryptoscan --pair MSTR:MSTZ
# later runs reuse ./data, including completed market holidays/weekends
go run ./cmd/cryptoscan --pair SOXL:SOXS --months 6 --lookback 24 --min-trades 2
# repeatable UTC window; inspect full JSON summary
go run ./cmd/cryptoscan --pair SOXL:SOXS --end 2026-11-07 --json
```

`--data-dir` changes the MetricsDB path. The database retains ten years of
history by default (via `WithRetentionDays(3650)`). No database server is
required. The local database directory is excluded from Git. Different symbols
share one persistent database but separate series and coverage markers. Closed
historical UTC days are never re-requested; the current UTC day is refreshed
for newly completed bars. The provider is Yahoo Finance's **unofficial** chart
API; it does not need an API key but may rate-limit, reject queries or revise
historical bars. Historical downloads are split into at most 28-day requests.
Missing/bad responses are not marked cached.

## What the rule means

For **each** of the two symbols independently:

1. Inspect the last `--lookback` completed hourly **closing prices** (24 by
   default, meaning 24 trading bars rather than 24 wall-clock hours).
2. If the latest close is at least **X% below the highest closing price** in
   those bars, generate a BUY signal. Execute at the **next hourly bar's open**.
3. Hold until an hourly close rises at least **Y% above the effective purchase
   price**, then execute a SELL at the **next bar's open**. No automatic
   stop-loss is implied; a position can remain open and lose money.
4. Model `--cost-bps` (default 10 basis points per side) by increasing the
   entry execution price and decreasing the exit execution price to approximate
   fees/slippage; mark open positions to market **including estimated exit cost**.
5. Stay in cash while flat. No actual orders are transmitted.

**Buying SOXS or MSTZ is not short-selling shares on margin**: it buys a
separate inverse ETF, which seeks a daily inverse return, not a guaranteed
inverse return over multiple hours/days. SOXL and SOXS may be held at the
same time because each leg receives independent entry/exit signals. MSTR is a
stock exposed to Bitcoin-related business activity, not BTC itself; MSTZ
seeks -2x of **MSTR's daily** returns, not BTC's daily returns. Fund splits,
tracking differences, leverage decay and fees matter.

## Optimization and evaluation

- Scan drop thresholds 1–12% and gain thresholds 1–16% (both 1-percentage-point
  steps by default); override with `--drop-min/max/step` and
  `--gain-min/max/step`.
- Use the **earlier 70%** of observed dates (same calendar cutoff for both
  legs) to select the pair maximizing final training account equity among
  rules with at least `--min-trades` completed trades (default 2). An open
  underwater position **reduces** the objective rather than disappearing.
- Freeze the selected parameters, then run on the **later 30%** as independent
  out-of-sample evaluation. Prior bars are available for indicator warm-up but
  no positions are carried forward into the test window.
- Print explicit BUY/SELL thresholds, total return, completed trade count, win
  rate, max drawdown, and buy-and-hold for each leg, plus a combined 50/50
  **non-rebalanced** portfolio return/drawdown. Both legs can be simultaneously
  open; cash in one leg is not automatically transferred to the other.
- Backtesting uses close-to-next-open signals to avoid same-candle look-ahead.
  Gaps, spreads, taxes, liquidity, dividend/split adjustments, limitations of
  hourly bars and survivorship/parameter-fitting bias can materially change
  realized outcomes. Results are **not** a live recommendation.

## Tests

```bash
go test -race ./...
go vet ./...
```

Tests cover next-open fills and avoidance of intrabar look-ahead; portfolio
mark-to-market of open losing positions; trading costs; optimization and
portfolio math; Yahoo JSON parsing with a fake HTTP server; cache misses/hits
and avoidance of duplicate downloads; and **MetricsDB persistence after
closing and reopening**. GitHub Actions runs this suite. No real-market API
credentials or network requests are required by tests.

## SQLite optimization history and local website

The scan now **records its findings after every successful run** in
`./data/findings.sqlite`. The historical OHLCV candles are **still stored
separately in MetricsDB**, so no previously downloaded market data needs to be
fetched again just to view the dashboard. SQLite is used only for scan results.

```bash
# Run a rolling, fixed-length 180-day scan for both independent instruments.
go run ./cmd/cryptoscan --pair SOXL:SOXS --days 180
# Add another pair using the same local data and SQLite history.
go run ./cmd/cryptoscan --pair MSTR:MSTZ --days 180
# Open read-only history website; by default, bound only to localhost.
go run ./cmd/cryptoweb
# visit http://localhost:8080
```

The homepage has a list view (one row per scan date per symbol), an interactive
filter for pair/symbol/window/lookback/configuration, and two charts:

- **Buy after a drop of X%** and **sell after a gain of Y%**, plotted by UTC
  analysis date, with mean, range and **sample standard deviation in percentage
  points**. A smaller standard deviation indicates more consistent *selected
  parameters* over the recorded scan days.
- Out-of-sample total return compared with buy-and-hold in the same period,
  to help identify regimes where the thresholds stop being profitable.

Each SQLite row also includes training return, test return, completed trades,
win rate, open-position flag, test max drawdown and combined two-leg return.
A fixed `--days N` window makes comparisons consistent as time advances;
`--months 6` remains supported for calendar-month-based requests. Use
`--findings-db /some/path/findings.sqlite` on the scanner and `--db` on
`cryptoweb` to point both at the same custom database. Use `--no-save` to run a
scan without adding findings. For security, the website has read-only endpoints
and **defaults to `127.0.0.1:8080`** rather than exposing the service to the
network. It uses native SVG charts, not external scripts.

SQLite deduplicates the *same symbol, pair, UTC end date, day window, lookback,
and optimization settings*: rerunning the same configuration on the same day
updates that point rather than manufacturing stability. Different optimization
settings are charted separately. A new day adds a new point. **Multiple scan
days are needed to see any stability trend; the initial run has only one
point.** Rolling historical windows strongly overlap, so consistent parameters
can still be overfitted; separately monitor out-of-sample performance and
true forward performance before interpreting this as a trading signal.

The code uses the pure-Go `modernc.org/sqlite` SQLite driver and `database/sql`.
The web server uses only Go standard-library HTTP primitives and embedded SVG.
No broker integration or live order submission is included; the daemon now schedules
analysis and paper signals automatically.

## Continuously running signal website (new)

`cryptoweb` is now the **continuous signal service** as well as the dashboard.
It does **not** send orders to a broker; BUY and SELL are indicative paper
signals, logged to SQLite, based on completed 5-minute candles.

```bash
# Run from the repository root; defaults to both pairs and localhost:8080.
go run ./cmd/cryptoweb --pairs SOXL:SOXS,MSTR:MSTZ --days 180 --lookback 24
# Visit http://localhost:8080
# To retain the old read-only dashboard without background market access:
go run ./cmd/cryptoweb --history-only
```

**Schedules (UTC):**

- The service evaluates each configured pair **once at 00:00 UTC** each day.
  When started after midnight, it performs a catch-up evaluation if no
  successful evaluation is saved for that UTC date. The successful daily
  marker and two active rules are atomically committed to SQLite; restarting
  the app does not re-optimize an already-completed date. Failed evaluations
  retry no more often than every 30 minutes, and live signals are paused until
  that day's rules are available.
- On **every UTC 5-minute boundary**, the service checks each symbol during
  US regular trading hours (09:35–16:10 America/New_York; holidays and stale
  feeds are automatically skipped). It consumes only **completed 5m candles**
  and rejects quotes more than 15 minutes old. The rolling reference peak is
  calculated from the last `--lookback` completed **hourly closes** in
  MetricsDB. Current-day hourly candles are refreshed once per hour and reused
  between checks. Yahoo's unofficial data can be delayed or unavailable;
  alerts are **not** real-time execution promises.
- For a **flat** symbol, signal **BUY** when the latest 5m close is at least
  the optimized drop % below the highest recent hourly close. For an **open
  paper position**, signal **SELL** when the latest 5m close is at least the
  optimized gain % above the stored cost-adjusted paper entry price. The
  scanner's historical fills are modeled at the **next hourly open**, whereas
  the live signals use the *completed 5m close* as an indicative reference;
  live results may diverge from historical backtests.
- SOXL and SOXS (or MSTR and MSTZ) remain **independent**: an SOXL SELL does
  not force a SOXS BUY, and both paper positions may be OPEN simultaneously.
  Buying an inverse ETF is not borrowing/shorting stock. Daily optimization
  changes the rules without resetting any open paper position; the gain target
  continues to reference that position's recorded entry.

**Persistence:** `./data/findings.sqlite` stores `evaluation_days`,
`live_rules`, `live_state`, and `live_signals` in addition to historical
`findings`. One symbol/closed-candle timestamp can create at most one signal,
including across crashes/restarts. MetricsDB continues to own hourly OHLCV
cache data. Website endpoints `/api/live` and `/api/signals?limit=100` expose
read-only, regularly refreshed positions and alerts; original `/api/groups`
and `/api/history` stability charts remain available. Run the service from a
persistent directory; GitHub Actions **is not** a daemon host.

To keep it running after a Linux reboot, build a binary and install the
example systemd service at `deploy/cryptoweb.service`. Create an unprivileged
`crypto` user, a writable `/var/lib/crypto` directory owned by it, and install
the binary at `/usr/local/bin/cryptoweb`. Then enable the service using
`systemctl enable --now cryptoweb`. The HTTP listener is intentionally
`127.0.0.1:8080`; use an authenticated TLS reverse proxy if remote access is
needed. Only one service instance should use a MetricsDB directory.
