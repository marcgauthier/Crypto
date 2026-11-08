package backtest

import (
  "math"
  "testing"
  "time"

  "github.com/marcgauthier/Crypto/internal/market"
)

func bracketBars(quotes ...[4]float64) []market.Candle {
  out:=make([]market.Candle,len(quotes))
  start:=time.Date(2026,1,5,14,30,0,0,time.UTC)
  for i,q:=range quotes {
    out[i]=market.Candle{Time:start.Add(time.Duration(i)*time.Hour),Open:q[0],High:q[1],Low:q[2],Close:q[3],Volume:100}
  }
  return out
}
func bracketConfig() Config {return Config{Lookback:2,CostBps:0}}

func TestBracketStopFirstWhenBothLevelsHitSameBar(t *testing.T) {
  b:=bracketBars(
    [4]float64{100,100,100,100},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,100,80,95},
  )
  r,err:=RunBracket(b,Params{DropPct:5,GainPct:5,StopPct:5},bracketConfig())
  if err!=nil { t.Fatal(err) }
  if r.Trades!=1 || r.TradeLog[0].ExitReason!="stop" || math.Abs(r.TradeLog[0].SellPrice-85)>1e-8 {
    t.Fatalf("must choose conservative stop in ambiguous bar: %+v",r.TradeLog)
  }
  if !(r.ReturnPct < 0) || r.OpenPosition {
    t.Fatalf("stop failed: %+v",r)
  }
}

func TestBracketTakeProfitHitIntrabar(t *testing.T) {
  b:=bracketBars(
    [4]float64{100,100,100,100},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,95,89,94},
  )
  r,err:=RunBracket(b,Params{DropPct:5,GainPct:5,StopPct:5},bracketConfig())
  if err!=nil {t.Fatal(err)}
  if r.Trades!=1 || r.TradeLog[0].ExitReason!="take_profit" || math.Abs(r.TradeLog[0].SellPrice-94.5)>1e-8 {
    t.Fatalf("take profit should fill at target: %+v",r.TradeLog)
  }
}

func TestBracketStopGapUsesActualOpen(t *testing.T) {
  b:=bracketBars(
    [4]float64{100,100,100,100},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,92,88,89},
    [4]float64{80,81,77,78},
  )
  r,err:=RunBracket(b,Params{DropPct:5,GainPct:5,StopPct:5},bracketConfig())
  if err!=nil {t.Fatal(err)}
  if r.Trades!=1 || r.TradeLog[0].ExitReason!="stop_gap" || r.TradeLog[0].SellPrice!=80 {
    t.Fatalf("gap must not claim stop threshold execution: %+v",r.TradeLog)
  }
}

func TestBracketRearmsOnlyAfterRecovery(t *testing.T) {
  b:=bracketBars(
    [4]float64{100,100,100,100},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,92,80,82},
    [4]float64{82,83,70,72},
    [4]float64{72,74,69,70},
    [4]float64{100,100,99,100},
    [4]float64{90,90,90,90},
    [4]float64{90,91,80,83},
  )
  r,err:=RunBracket(b,Params{DropPct:5,GainPct:5,StopPct:5},bracketConfig())
  if err!=nil {t.Fatal(err)}
  if r.Trades!=2 {
    t.Fatalf("must not rebuy on every further drawdown, but rearm after recovery: %+v",r.TradeLog)
  }
}

func TestBracketFeesAndOptimizer(t *testing.T) {
  b:=bracketBars(
    [4]float64{100,100,100,100},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,96,89,95},
    [4]float64{100,100,100,100},
    [4]float64{90,90,90,90},
    [4]float64{90,96,89,95},
  )
  p:=Params{DropPct:5,GainPct:5,StopPct:5}
  r1,e:=RunBracket(b,p,bracketConfig())
  r2,e2:=RunBracket(b,p,Config{Lookback:2,CostBps:20})
  if e!=nil || e2!=nil || !(r2.ReturnPct<r1.ReturnPct) {
    t.Fatalf("fees must reduce return: %v %v %f %f",e,e2,r1.ReturnPct,r2.ReturnPct)
  }
  grid:=BracketGrid{Grid:Grid{DropMin:5,DropMax:5,DropStep:1,GainMin:5,GainMax:5,GainStep:1,MinTrades:2},StopMin:5,StopMax:5,StopStep:1}
  best,err:=OptimizeBracket(b,bracketConfig(),grid)
  if err!=nil || best.Trades!=2 || best.Params.StopPct!=5 {
    t.Fatalf("optimizer failed: %v %+v",err,best)
  }
}
