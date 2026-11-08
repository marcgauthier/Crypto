package main

import (
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "math"
 "os"
 "sort"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)

type position struct{symbol string;shares,entry,cost,target float64;opened time.Time}
type result struct{
 IntervalHours int `json:"interval_hours"`
 ReturnPct float64 `json:"return_pct"`
 MaxDrawdownPct float64 `json:"max_drawdown_pct"`
 Entries int `json:"entries"`
 Closed int `json:"closed"`
 Wins int `json:"wins"`
 Open int `json:"open"`
 LongEntries int `json:"long_entries"`
 InverseEntries int `json:"inverse_entries"`
 CashPct float64 `json:"cash_pct"`
 MaxOpen int `json:"max_open"`
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive ending date")
 trendArg:=flag.Int("trend-hours",4,"lookback hours for MSTR direction")
 feeArg:=flag.Float64("fee-bps",10,"fee per side in basis points")
 monthsArg:=flag.Int("months",3,"backtest duration in calendar months")
 flag.Parse()
 day,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-*monthsArg,0)
 // Fetch extra history for trend warmup; only score the six-month window.
 fetchStart:=start.AddDate(0,0,-7)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 y:=market.Yahoo{}
 a,err:=y.Fetch(ctx,"MSTR",fetchStart,end);if err!=nil{return fmt.Errorf("MSTR: %w",err)}
 b,err:=y.Fetch(ctx,"MSTZ",fetchStart,end);if err!=nil{return fmt.Errorf("MSTZ: %w",err)}
 bmap:=map[int64]market.Candle{};for _,c:=range b{if market.Valid(c){bmap[c.Time.Unix()]=c}}
 type pair struct{a,b market.Candle}
 bars:=[]pair{}
 for _,c:=range a{if d,ok:=bmap[c.Time.Unix()];ok&&market.Valid(c)&&market.Valid(d){bars=append(bars,pair{c,d})}}
 sort.Slice(bars,func(i,j int)bool{return bars[i].a.Time.Before(bars[j].a.Time)})
 first:=0;for first<len(bars)&&bars[first].a.Time.Before(start){first++}
 if len(bars)-first<100{return fmt.Errorf("only %d aligned test candles; need >=100",len(bars)-first)}
 fee:=*feeArg/10000
 // Buy-and-hold is fully invested at the first aligned MSTR open.
 firstOpen:=bars[first].a.Open
 buyHold:=(bars[len(bars)-1].a.Close/firstOpen*(1-fee)/(1+fee)-1)*100
 results:=[]result{}
 for _,interval:=range []int{4,8}{
  cash:=10000.0;initial:=cash;peak:=initial;maxDD:=0.0;open:=[]position{};out:=result{IntervalHours:interval}
  nextEntry:=time.Time{}
  for i:=first;i<len(bars);i++{
   p:=bars[i];now:=p.a.Time
   // Existing positions can exit at this candle's open if gapped above the target,
   // otherwise at target if high reaches it. Same-bar entry exits are disallowed.
   remain:=make([]position,0,len(open))
   for _,pos:=range open{
    c:=p.a;if pos.symbol=="MSTZ"{c=p.b}
    if now.After(pos.opened)&&c.High>=pos.target{
     sell:=pos.target;if c.Open>sell{sell=c.Open}
     proceeds:=pos.shares*sell*(1-fee);cash+=proceeds;out.Closed++
     if proceeds>pos.cost{out.Wins++}
    }else{remain=append(remain,pos)}
   }
   open=remain
   if !now.Before(nextEntry)&&i>=*trendArg&&i>0{
    // Signal from COMPLETED previous candles only; compare last close
    // against a close at least trend-hours earlier in elapsed time.
    past:=i-1
    for past>=0&&bars[past].a.Time.After(bars[i-1].a.Time.Add(-time.Duration(*trendArg)*time.Hour)){past--}
    if past>=0{
     delta:=bars[i-1].a.Close/bars[past].a.Close-1
     symbol:="";c:=p.a
     if delta>0{symbol="MSTR"}else if delta<0{symbol="MSTZ";c=p.b}
     allocation:=initial*0.01
     if symbol!=""&&cash>=allocation{
      shares:=allocation/(c.Open*(1+fee))
      cash-=allocation
      open=append(open,position{symbol,shares,c.Open,allocation,c.Open*1.02,now})
      out.Entries++
      if symbol=="MSTR"{out.LongEntries++}else{out.InverseEntries++}
      if len(open)>out.MaxOpen{out.MaxOpen=len(open)}
     }
    }
    nextEntry=now.Add(time.Duration(interval)*time.Hour)
   }
   equity:=cash
   for _,pos:=range open{price:=p.a.Close;if pos.symbol=="MSTZ"{price=p.b.Close};equity+=pos.shares*price*(1-fee)}
   if equity>peak{peak=equity}
   maxDD=math.Max(maxDD,(peak-equity)/peak*100)
   if i==len(bars)-1{out.ReturnPct=(equity/initial-1)*100;out.MaxDrawdownPct=maxDD;out.Open=len(open);out.CashPct=cash/initial*100}
  }
  results=append(results,out)
 }
 output:=struct{Start,End string;AlignedHours int;TrendLookbackHours int;ProfitTargetPct float64;AllocationPct float64;FeeBps float64;BuyHoldMSTRPct float64;Results []result}{bars[first].a.Time.Format("2006-01-02"),bars[len(bars)-1].a.Time.Format("2006-01-02"),len(bars)-first,*trendArg,2,1,*feeArg,buyHold,results}
 return json.NewEncoder(os.Stdout).Encode(output)
}
