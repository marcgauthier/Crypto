# Entry-only signal with take-profit / stop-loss bracket — 2026 holdout

## Scope
A research backtest in `cryptoscan --exit-mode bracket`. **This has NOT been activated in the 5-minute live monitoring dashboard.** No broker orders are submitted.

- Train: 2026-05-08 00:00 UTC to 2026-08-08 00:00 UTC (exclusive).
- Holdout: 2026-08-08 00:00 UTC to 2026-11-08 00:00 UTC (exclusive). Last market session Nov 6.
- Four tickers: SOXL, SOXS, MSTR, MSTZ (two independent bullish/inverse pairs).
- Historical source: Yahoo Finance, regular-session 1-hour OHLCV.
- Lookback: trailing 24 completed hourly closes; entry signal when price closes at or below X% under the rolling closing high; buy at **next** hourly open.
- After entry: take profit at +Y% above raw purchase open, or stop loss at -Z% below raw purchase open. Use intrabar OHLC high/low. Both touched in one bar -> **assume stop first**. Opening gaps fill at their actual opening prices, not idealized stop values. Bracket may exit in entry bar.
- Re-arm only after a subsequent close above the drop threshold. The start of holdout derives the prior market arming state from training bars, but starts with flat cash and no inherited position.
- Optimize three parameters on training bars only: buy drop 1–12% by 1; TP 1–16% by 1; SL 2–12% by 2. Require >=2 closed training trades. Freeze before the test period.
- Costs: 10 bps per entry and exit. Unrealized positions marked to market after cost.
- No broker spreads, borrow fees, dividends, taxes, reverse-split adjustments beyond source data, or guaranteed stop fills. Simulation only.

## Results (holdout, percentages)

| Symbol | Buy after drop | Take profit | Stop loss | Baseline return | Bracket return | Bracket trades | Bracket max drawdown |
|---|---:|---:|---:|---:|---:|---:|---:|
| SOXL | 6% | 16% | 4% | -63.72% | **-28.98%** | 14 | 41.23% |
| SOXS | 6% | 7% | 12% | +90.72% | **+72.01%** | 14 | 19.64% |
| MSTR | 9% | 10% | 4% | +57.50% | **-0.03%** | 9 | 13.30% |
| MSTZ | 7% | 13% | 10% | -66.10% | **-12.06%** | 14 | 45.65% |

Pair results (each instrument independent with 50% initial allocation, no regular rebalancing):
- **SOXL/SOXS** baseline **+13.50%**, bracket **+21.52%**, bracket maximum drawdown **17.08%**.
- **MSTR/MSTZ** baseline **-4.30%**, bracket **-6.04%**, bracket maximum drawdown **24.47%**.

**Conclusion:** TP/SL reduced large open losses on SOXL and MSTZ but also sacrificed gains on SOXS and MSTR. It did not make all four symbols profitable. Bracket mode additionally requires a fresh re-armed entry crossing, so this is a distinct strategy rather than a perfectly isolated exit-rule comparison. Optimizing 3 dimensions on just 3 months is prone to overfitting; verify other rolling holdouts before deployment.

## Reproduce

```bash
go run ./cmd/cryptoscan --pair SOXL:SOXS --months 6 --end 2026-11-07 --train-until 2026-08-08 --exit-mode bracket --stop-min 2 --stop-max 12 --stop-step 2 --lookback 24 --cost-bps 10 --no-save --json
go run ./cmd/cryptoscan --pair MSTR:MSTZ --months 6 --end 2026-11-07 --train-until 2026-08-08 --exit-mode bracket --stop-min 2 --stop-max 12 --stop-step 2 --lookback 24 --cost-bps 10 --no-save --json
```

Bracket mode requires `--no-save` until the SQLite history schema and live rule publisher are extended to persist the stop threshold; it would be unsafe to publish a TP/SL rule with the stop omitted.

CI: [successful bracket holdout](https://github.com/marcgauthier/Crypto/actions/runs/42487838181). Download the `entry-bracket-holdout` artifact for side-by-side baseline/bracket JSON outputs and the generated comparison report.

**Five-minute limitation:** hourly high/low data does not reconstruct actual intra-hour paths and cannot validate five-minute monitoring signals. A broker-native OCO bracket offers better protection than a five-minute polling loop, though gaps and stop execution risk remain.
