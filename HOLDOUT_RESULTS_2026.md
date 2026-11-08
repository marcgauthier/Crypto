# Fixed 3-month out-of-sample validation — November 8, 2026

**Question:** Can buy-after-drop/sell-after-gain strategies, selected exclusively on the previous three months, turn a profit on the next three months for SOXL, SOXS, MSTR and MSTZ?

**Answer: No — two of the four strategies lost money.**

- **Training window:** 2026-05-08 00:00 UTC through 2026-08-08 00:00 UTC (exclusive). No test-window candles used for threshold optimization.
- **Testing window:** 2026-08-08 00:00 UTC through 2026-11-08 00:00 UTC (exclusive). Last trading session in this range was November 6.
- **Data:** Yahoo Finance regular-session 1-hour OHLCV, 890 valid hourly candles per ticker over the six months.
- **Rules:** Rolling 24 completed hourly closes, buy when price closes at least X% below the rolling closing high; sell when price closes at least Y% above actual entry, fill on next bar open. Independently optimize each ticker on training data, then freeze X and Y for testing.
- **Parameters searched:** buy-drop 1–12% and sell-gain 1–16%, 1-percentage-point steps; at least two completed training trades.
- **Costs:** 10 basis points per buy and sell, approximate slippage/fees, no taxes or dividends; open positions marked to market with estimated exit costs.

| Symbol | Buy drop | Sell gain | Training return | Test return | Test buy-and-hold | Completed test trades | Test max drawdown | Open at end |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| SOXL | 3% | 9% | +15.20% | **-63.72%** | -67.87% | 2 | 75.61% | Yes |
| SOXS | 6% | 4% | +716.15% | **+90.72%** | +71.24% | 11 | 37.19% | Yes |
| MSTR | 9% | 1% | -14.09% | **+57.50%** | +29.38% | 18 | 3.93% | Yes |
| MSTZ | 1% | 2% | +155.09% | **-66.10%** | -68.74% | 7 | 80.95% | Yes |

## Fixed 50/50 pair returns (test period only)

| Pair | Strategy return | Max drawdown |
| --- | ---: | ---: |
| SOXL / SOXS | **+13.50%** | 10.19% |
| MSTR / MSTZ | **-4.30%** | 36.04% |

These are *independently invested* equal-weight portfolios, not an automatically switched long-versus-inverse strategy. Buying SOXS or MSTZ means buying an inverse ETF, not directly short-selling SOXL or MSTR. Daily leverage resets and reverse splits make results path-dependent. The strategy has **no stop loss**. A profitable training period can still produce a severe test loss. A large return in an inverse fund must not be extrapolated.

**Scope limitation:** This validates the existing **hourly historical backtester**. It does **not** establish what the live five-minute-monitoring system would have earned; that system can signal mid-hour, uses five-minute closed candle prices for signals, and requires a full three months of five-minute historical candles for a like-for-like replay. The free Yahoo 5-minute history source is not sufficient for that replay. There is no broker execution or live deployment.

## Reproduction

The [GitHub Actions run](https://github.com/marcgauthier/Crypto/actions/runs/42486860612) executed the following commands on Go 1.26.10. Download its `2026-fixed-holdout` artifact for full JSON report files.

```bash
go run ./cmd/cryptoscan --pair SOXL:SOXS --months 6 --end 2026-11-07 --train-until 2026-08-08 --lookback 24 --cost-bps 10 --no-save --json
go run ./cmd/cryptoscan --pair MSTR:MSTZ --months 6 --end 2026-11-07 --train-until 2026-08-08 --lookback 24 --cost-bps 10 --no-save --json
```

Results are simulations, not recommendations or expected future returns. Historical data corrections, splitting adjustments, fills, and fees may change the outcome.