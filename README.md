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
