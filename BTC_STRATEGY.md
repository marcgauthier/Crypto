# BTC-led entry signal for MSTR / MSTZ (research experiment)

BTC's preceding 7 full UTC days and most recently completed 1 UTC day decide the **entry direction**. This is distinct from MSTR/MSTZ's own price-dip strategy:

- On each US stock-market session's first shared MSTR/MSTZ hourly bar, compute BTC's last completed 00:00 UTC close, compared to the closes 1 day and 7 days earlier.
- **Bullish:** BTC 7d >= +X% and 1d >= +Y% => **buy MSTR** at that session's first bar open if flat.
- **Bearish:** BTC 7d <= -X% and 1d <= -Y% => **buy MSTZ** at that session's first bar open if flat.
- Otherwise, no new position. Missing/incomplete BTC daily closes => no new position.
- One *shared* paper capital pool; at most one ETF open. Existing position is **not** closed just because BTC reverses. Exit solely from per-position take-profit (+TP%) or stop-loss (-SL%).
- On each subsequent hourly high/low: check both bracket levels. If both hit within one candle, conservatively fill **stop first**; gaps use the actual bar's open price. Re-entry only at a later stock session's first bar, not repeatedly every hour.
- Dynamic threshold search is **training-only**: BTC 7d [0.5,1,2,3,4,6]%, 1d [0,0.5,1,1.5,2,3]%, TP [3,5,8,12]%, SL [2,4,6,8]%; require at least two closed trades and at least one MSTR and MSTZ trade in the training period.
- Holdout: **May 8–August 7, 2026** optimization; **August 8–November 7** frozen-rule test. Includes 0.1% estimated fee/slippage per side and valuation of open paper positions.

## Run

```sh
go mod tidy
go run ./cmd/btcbacktest --end 2026-11-07 --train-until 2026-08-08 --months 6
# machine-readable JSON
go run ./cmd/btcbacktest --end 2026-11-07 --train-until 2026-08-08 --json
```

Results are also produced by `.github/workflows/btc_holdout.yml` and saved in the `btc-led-holdout` artifact.

The historical data uses `BTC-USD` 1-hour OHLCV plus `MSTR` and `MSTZ` via Yahoo Finance, stored with the existing MetricsDB caching mechanism. BTC's 23:00–00:00 UTC candle is treated as that day's completed close. This is not a predictor by itself: BTC is 24/7 while MSTR/MSTZ trade regular US stock-market hours and respond to premium/discount, volatility, leverage, and company-specific moves. Unlike a true BTC short, buying MSTZ is buying an inverse MSTR ETF.

**Not live yet:** this CLI is a standalone research backtest. It does not update SQLite findings or change the five-minute monitoring app. Avoid automatically trading these rules until independently validated with walk-forward holdouts, data-quality checks, and executable broker-native brackets.
