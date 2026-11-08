# Live six-month test — November 8, 2026

Source: [GitHub Actions successful workflow](https://github.com/marcgauthier/Crypto/actions/runs/42481847680). The market-data test fetched Yahoo Finance's hourly OHLCV for 2026-05-08 through 2026-11-08 UTC (890 hourly candles per ticker) and optimized BUY after a drop, SELL after a gain using a 24-bar rolling high and 10 bps per-side transaction-cost estimate. Thresholds were selected on the earlier 70% of the period and tested on the later 30%, with open positions marked to market.

| Pair | Leg | Buy after a drop of | Sell after a gain of | Training return | Out-of-sample return | Completed test trades | Test max drawdown | Test buy-and-hold |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| SOXL/SOXS | SOXL (bullish) | 3% | 9% | +0.07% | -59.45% | 3 | 75.6% | -54.81% |
| SOXL/SOXS | SOXS (bearish ETF) | 6% | 4% | +756.79% | +46.42% | 7 | 37.2% | +37.20% |
| MSTR/MSTZ | MSTR (bullish stock) | 9% | 1% | +36.21% | +38.39% | 15 | 2.6% | -2.69% |
| MSTR/MSTZ | MSTZ (bearish ETF) | 1% | 2% | +8.38% | -12.45% | 4 | 49.3% | -25.95% |

**50/50 independently traded out-of-sample portfolio:** SOXL/SOXS -6.52% return / 14.72% max drawdown; MSTR/MSTZ +12.97% return / 26.25% max drawdown. These are fixed equal allocations, not a dynamically switched long-vs-inverse strategy.

**Caching verified:** initial SOXL/SOXS request fetched 890 bars per ticker in 7 HTTP calls each. Immediately rerunning against the same MetricsDB fetched **0 bars and made 0 HTTP calls**.

**Quality checks:** the same workflow passed Go race-enabled unit tests, MetricsDB persistence (close and reopen), and `go vet`.

**Critical qualification:** all three closed SOXL test trades were winners, yet the final SOXL portfolio lost 59.45%, because an open losing position was included in the final equity. The strategy has **no stop-loss**; win rate on completed trades can be misleading. Results are sensitive to Yahoo data and splits, threshold fitting, leverage decay, fill assumptions and implementation. Not investment advice; these thresholds are historical backtest findings, not forecasts or recommended live orders.