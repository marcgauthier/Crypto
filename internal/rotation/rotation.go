package rotation

import (
 "fmt"
 "math"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)

type Params struct { Lookback int `json:"lookback"`; StableBars int `json:"stable_bars"`; MaxRangePct float64 `json:"max_range_pct"`; ReboundPct float64 `json:"rebound_pct"`; StopPct float64 `json:"stop_pct"` }
type Trade struct { Symbol string `json:"symbol"`; Entry time.Time `json:"entry"`; Exit time.Time `json:"exit"`; ReturnPct float64 `json:"return_pct"`; Reason string `json:"reason"` }
type Result struct { Params Params `json:"params"`; ReturnPct float64 `json:"return_pct"`; MaxDrawdownPct float64 `json:"max_drawdown_pct"`; Trades []Trade `json:"trades"`; Entries map[string]int `json:"entries"`; OpenSymbol string `json:"open_symbol"` }
type signal struct { active bool; strength float64 }
func detect(b []market.Candle,i int,p Params) signal {
 if i<p.Lookback+p.StableBars || p.StableBars<2{return signal{}}
 // Require a meaningful decline relative to the historical peak, scaled
 // by recent average absolute hourly returns instead of a fixed 10%.
 peak:=b[i-p.StableBars-p.Lookback].Close
 vol:=0.0
 for j:=i-p.StableBars-p.Lookback+1;j<=i-p.StableBars;j++{
  if b[j].Close>peak {peak=b[j].Close}
  vol+=math.Abs(b[j].Close/b[j-1].Close-1)*100
 }
 vol/=float64(p.Lookback)
 low,high:=b[i-p.StableBars+1].Close,b[i-p.StableBars+1].Close
 for j:=i-p.StableBars+1;j<=i;j++{
  if b[j].Close<low{low=b[j].Close};if b[j].Close>high{high=b[j].Close}
 }
 drop:=(peak/low-1)*100
 if drop<math.Max(2,vol*float64(p.StableBars)*1.5){return signal{}}
 if (high/low-1)*100>p.MaxRangePct{return signal{}}
 if (b[i].Close/low-1)*100<p.ReboundPct{return signal{}}
 if b[i].Close<b[i-1].Close{return signal{}}
 return signal{true,drop/(vol+0.1)}
}
func Run(a,b []market.Candle,p Params,cutoff time.Time,costBps float64)(Result,error){
 out:=Result{Params:p,Entries:map[string]int{"MSTR":0,"MSTZ":0}}
 if p.Lookback<5||p.StableBars<2||p.MaxRangePct<=0||p.ReboundPct<=0||p.StopPct<=0||costBps<0||costBps>=10000{return out,fmt.Errorf("invalid settings")}
 // Only synchronized completed bars are eligible; never pair a stale ETF bar
 // with a newer bar from the other instrument.
 aa,bb:=[]market.Candle{},[]market.Candle{}
 for i,j:=0,0;i<len(a)&&j<len(b);{
  if a[i].Time.Equal(b[j].Time){aa=append(aa,a[i]);bb=append(bb,b[j]);i++;j++}else if a[i].Time.Before(b[j].Time){i++}else{j++}
 }
 if len(aa)<p.Lookback+p.StableBars+20{return out,fmt.Errorf("insufficient synchronized candles")}
 rate:=costBps/10000;cash,shares,entry:=1.0,0.0,0.0
 holding:=-1;entryTime:=time.Time{};peakEq:=1.0
 // A setup is an event: don't enter repeatedly on an extended plateau.
 armed:=[2]bool{true,true}
 var prev [2]bool
 for i:=p.Lookback+p.StableBars;i<len(aa);i++{
  bars:=[2][]market.Candle{aa,bb}
  now:=aa[i].Time
  sig:=[2]signal{detect(aa,i-1,p),detect(bb,i-1,p)}
  // State changes are based on the last fully completed candle.
  var rising [2]bool
  for k:=0;k<2;k++{rising[k]=sig[k].active&&!prev[k];prev[k]=sig[k].active}
  if now.Before(cutoff){continue}
  exit:=func(raw float64,reason string){
   symbol:="MSTR";if holding==1{symbol="MSTZ"}
   proceeds:=shares*raw*(1-rate)
   out.Trades=append(out.Trades,Trade{Symbol:symbol,Entry:entryTime,Exit:now,ReturnPct:(raw*(1-rate)/entry-1)*100,Reason:reason})
   cash=proceeds;shares=0;holding=-1
  }
  if holding>=0{
   bar:=bars[holding][i]
   stop:=entry/(1+rate)*(1-p.StopPct/100)
   if bar.Open<=stop{exit(bar.Open,"stop_gap")}else if bar.Low<=stop{exit(stop,"stop")}
  }
  choice:=-1
  for k:=0;k<2;k++{if rising[k]&&armed[k]&&(choice<0||sig[k].strength>sig[choice].strength){choice=k}}
  // Opposite confirmed reversal closes the existing direction. Switch only
  // after the old position is fully closed; one shared cash account.
  if choice>=0&&holding>=0&&choice!=holding{
   exit(bars[holding][i].Open,"opposite_reversal")
  }
  if choice>=0&&holding<0{
   price:=bars[choice][i].Open
   entry=price*(1+rate);shares=cash/entry;cash=0;holding=choice;entryTime=now
   if choice==0{out.Entries["MSTR"]++}else{out.Entries["MSTZ"]++}
   armed[choice]=false
  }
  for k:=0;k<2;k++{if !sig[k].active{armed[k]=true}}
  equity:=cash
  if holding>=0{equity=shares*bars[holding][i].Close*(1-rate)}
  if equity>peakEq{peakEq=equity}
  if dd:=(peakEq-equity)/peakEq*100;dd>out.MaxDrawdownPct{out.MaxDrawdownPct=dd}
 }
 equity:=cash
 if holding>=0{equity=shares*[2][]market.Candle{aa,bb}[holding][len(aa)-1].Close*(1-rate);if holding==0{out.OpenSymbol="MSTR"}else{out.OpenSymbol="MSTZ"}}
 out.ReturnPct=(equity-1)*100
 return out,nil
}
