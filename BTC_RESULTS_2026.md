# BTC-led MSTR / MSTZ three-month validation — November 8, 2026

**Question:** Can the previous seven completed UTC days of BTC action and the last completed UTC day decide when to buy MSTR (bullish) or MSTZ (inverse), with take-profit / stop-loss controlling exits?

**Answer in the first independent holdout: No demonstrated edge.** The training-optimized thresholds had a **-52.61%** out-of-sample return, despite +44.36% during optimization. A simpler fixed BTC momentum threshold also lost money (between -13.79% and -13.81% on two independent downloads).

Training: May 8, 2026 00:00 UTC through August 8 00:00 UTC (exclusive). Holdout: August 8 through November 8 00:00 UTC (exclusive); last US market session Nov 6. Parameters selected using training period only. Data: Yahoo Finance BTC-USD 1h (4607 candles including BTC lookback warmup), MSTR/MSTZ between 873 and 890 1h candles across two successful CI runs, reflecting Yahoo historical response variability. Fees/slippage 10 bps/side; open paper position valued using final price including expected exit fee.

## Signal and execution

- BTC 7d return >= +X% AND BTC previous day >= +Y%: buy MSTR at the first common MSTR/MSTZ regular-session hourly open, **if flat**.
- BTC 7d <= -X% AND BTC previous day <= -Y%: buy MSTZ (inverse MSTR ETF) at the first common regular-session hourly open, **if flat**.
- Conflicting/neutral BTC signals: stay flat. Never buy both simultaneously. No BTC-based position closures.
- After entry, close only when the stock high/low reaches TP or SL; conservative stop-first assumption if both hit in an hourly bar. Overnight gap beyond SL/TP fills at the true opening price.

## Executed results

| Strategy | BTC 7-day threshold | BTC previous-day threshold | Take profit | Stop loss | Training return | Holdout return | Holdout trades MSTR / MSTZ | Holdout max drawdown | Final open position |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| Train-optimized | 0.5% | 0.0% | 12% | 8% | +44.36% | **-52.61%** | 8 / 7 | 58.69% | MSTZ |
| Preset rule, not optimized | 2.0% | 1.0% | 5% | 4% | — | **-13.79% to -13.81%** | 7 / 9 | 18.05% to 18.08% | Cash |

**Data quality:** Repeated successful CI runs used the same requested date range, but Yahoo returned different counts of stock hourly candles (890/890 versus 874/873). The optimized result stayed -52.61%, and the preset changed by 0.02 percentage point. Figures should be considered approximate until stock time-series completeness is independently verified. Do not rely on incomplete coverage for production signals.

**Interpretation:** Even with both 7-day and prior-day BTC trends confirming the direction, buying MSTR or inverse MSTZ the following US session did not generalize to the holdout period. MSTR is not BTC and MSTZ is not a direct Bitcoin short. Company-specific risk, changing BTC correlation, ETF leverage/reset behavior, and overnight gaps matter. These two backtests are insufficient to establish an edge; search across a large parameter grid on just three months is especially prone to overfitting. The comparison also holds one shared account and thus is **not** directly comparable to earlier independent 50/50 MSTR/MSTZ portfolios.

**Reproduce:** [successful GitHub Actions run](https://github.com/marcgauthier/Crypto/actions/runs/42489360852); download the `btc-led-holdout` artifact for the full JSON and trade log.

```bash
go run ./cmd/btcbacktest --end 2026-11-07 --train-until 2026-08-08 --months 6 --cost-bps 10 --json
```

The existing continuously running five-minute system remains unchanged. This is an hourly OHLC research simulation, **not** an order-execution engine. Slippage and bid-ask costs are modeled as 0.1%/side, but actual spreads, fill priority, split adjustments, intra-hour stop ordering and data errors could materially change outcomes.