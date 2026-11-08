package main

import(
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
type pos struct{symbol string;shares,cost,target float64;opened time.Time}
type stats struct{Symbol string `json:"symbol"`;Buys int `json:"buys"`;Sold int `json:"sold"`;Bags int `json:"bags"`;Realized float64 `json:"realized"`;Unrealized float64 `json:"unrealized"`;BagCost float64 `json:"bag_cost"`;BagValue float64 `json:"bag_value"`}
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive end");flag.Parse()
 day,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-3,0)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 y:=market.Yahoo{}
 a,err:=y.Fetch(ctx,"MSTR",start,end);if err!=nil{return err}
 b,err:=y.Fetch(ctx,"MSTZ",start,end);if err!=nil{return err}
 bm:=map[int64]market.Candle{};for _,c:=range b{bm[c.Time.Unix()]=c}
 type pair struct{a,b market.Candle}
 bars:=[]pair{}
 for _,c:=range a{if d,ok:=bm[c.Time.Unix()];ok{bars=append(bars,pair{c,d})}}
 sort.Slice(bars,func(i,j int)bool{return bars[i].a.Time.Before(bars[j].a.Time)})
 if len(bars)<100{return fmt.Errorf("insufficient aligned bars %d",len(bars))}
 loc,err:=time.LoadLocation("America/New_York");if err!=nil{return err}
 fee:=0.001;initial:=10000.0;cash:=initial;peak:=initial;dd:=0.0;open:=[]pos{}
 st:=map[string]*stats{"MSTR":{Symbol:"MSTR"},"MSTZ":{Symbol:"MSTZ"}}
 entries:=0;skipped:=0;signalDays:=0
 for i,p:=range bars{
  now:=p.a.Time
  keep:=[]pos{}
  for _,v:=range open{
   c:=p.a;if v.symbol=="MSTZ"{c=p.b}
   if now.After(v.opened)&&c.High>=v.target{
    sell:=math.Max(c.Open,v.target);proceeds:=v.shares*sell*(1-fee)
    cash+=proceeds;st[v.symbol].Sold++;st[v.symbol].Realized+=proceeds-v.cost
   }else{keep=append(keep,v)}
  }
  open=keep
  // First regular-session 60-minute candle is 09:30 to 10:30 ET.
  // Use its completed close versus open; trade at next bar open.
  if i>0{
   prev:=bars[i-1].a
   pt:=prev.Time.In(loc);nt:=now.In(loc)
   if pt.Hour()==9&&pt.Minute()==30&&nt.YearDay()==pt.YearDay()&&nt.Year()==pt.Year()&&nt.Hour()==10&&nt.Minute()==30{
    signalDays++
    symbol:="";c:=p.a
    if prev.Close>prev.Open{symbol="MSTR"}else if prev.Close<prev.Open{symbol="MSTZ";c=p.b}
    if symbol!=""{
     if cash>=100{
      open=append(open,pos{symbol,100/(c.Open*(1+fee)),100,c.Open*1.02,now})
      cash-=100;entries++;st[symbol].Buys++
     }else{skipped++}
    }
   }
  }
  eq:=cash
  for _,v:=range open{price:=p.a.Close;if v.symbol=="MSTZ"{price=p.b.Close};eq+=v.shares*price*(1-fee)}
  if eq>peak{peak=eq};dd=math.Max(dd,(peak-eq)/peak*100)
 }
 last:=bars[len(bars)-1]
 equity:=cash
 for _,v:=range open{
  price:=last.a.Close;if v.symbol=="MSTZ"{price=last.b.Close}
  value:=v.shares*price*(1-fee);x:=st[v.symbol];x.Bags++;x.BagCost+=v.cost;x.BagValue+=value;x.Unrealized+=value-v.cost;equity+=value
 }
 hold:=(last.a.Close/bars[0].a.Open*(1-fee)/(1+fee)-1)*100
 output:=struct{Start,End string;AlignedBars,SignalDays,Entries,SkippedNoCash int;Cash,Equity,ReturnPct,MaxDrawdownPct,BuyHoldMSTRPct float64;Assets []stats}{bars[0].a.Time.Format("2006-01-02"),last.a.Time.Format("2006-01-02"),len(bars),signalDays,entries,skipped,cash,equity,(equity/initial-1)*100,dd,hold,[]stats{*st["MSTR"],*st["MSTZ"]}}
 return json.NewEncoder(os.Stdout).Encode(output)
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
