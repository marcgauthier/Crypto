package stockrotation

import (
 "errors"
 "sort"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)

// RunTrailing uses completed daily closes or completed hourly closes.
// A signal at a completed bar executes at the next bar's open.
// Profit arming is based on close-to-close gain of the held ETF;
// trailing high tracks closes after arming. Stops use entry execution price.
func RunTrailing(a,b []market.Candle,start,end time.Time,activation,trail,stop,costBps float64,initial,mode string)(Result,error){
 out:=Result{}
 if activation<=0||trail<=0||stop<=0||costBps<0||costBps>=10000||!start.Before(end)||(initial!="MSTR"&&initial!="MSTZ")||(mode!="daily"&&mode!="hourly"){return out,errors.New("invalid configuration")}
 bm:=map[int64]market.Candle{}
 for _,v:=range b{if market.Valid(v){bm[v.Time.Unix()]=v}}
 ps:=[]pair{}
 for _,v:=range a{if w,ok:=bm[v.Time.Unix()];ok&&market.Valid(v)&&!v.Time.Before(start)&&v.Time.Before(end){ps=append(ps,pair{v,w})}}
 sort.Slice(ps,func(i,j int)bool{return ps[i].a.Time.Before(ps[j].a.Time)})
 if len(ps)<10{return out,errors.New("insufficient aligned candles")}
 ny,err:=time.LoadLocation("America/New_York");if err!=nil{return out,err}
 px:=func(p pair,h string,open bool)float64{if h=="MSTR"{if open{return p.a.Open};return p.a.Close};if open{return p.b.Open};return p.b.Close}
 rate:=costBps/10000
 held:=initial;entry:=px(ps[0],held,true);shares:=1/(entry*(1+rate));peakEquity:=1.0
 armed:=false;high:=0.0;pending:=false
 lastSession:="";var priorClose,olderClose pair;havePrior,haveOlder:=false,false
 var lastPair pair
 for i,p:=range ps{
  session:=p.a.Time.In(ny).Format("2006-01-02")
  newSession:=session!=lastSession
  if newSession&&lastSession!=""{olderClose=priorClose;haveOlder=havePrior;priorClose=lastPair;havePrior=true}
  if newSession{lastSession=session}
  // Evaluate completed bars before executing at the current bar open.
  // The evaluation block below sets pending for this opening.
  // Completed-bar exit evaluation; pending execution is at the next bar open.
  eval:=false;var current,previous pair
  if mode=="hourly"&&i>=1{current=ps[i-1];if i>=2{previous=ps[i-2];eval=true}}
  if mode=="daily"&&newSession&&haveOlder{current=priorClose;previous=olderClose;eval=true}
  if eval{
   now:=px(current,held,false);old:=px(previous,held,false)
   if old>0&&!armed&&(now/old-1)*100>=activation{armed=true;high=now}
   if armed&&now>high{high=now}
   if now<=entry*(1-stop/100)||(armed&&now<=high*(1-trail/100)){pending=true}
  }
  if pending{
   proceeds:=shares*px(p,held,true)*(1-rate)
   if held=="MSTR"{held="MSTZ"}else{held="MSTR"}
   entry=px(p,held,true)
   shares=proceeds/(entry*(1+rate))
   armed=false;high=0;pending=false;out.Switches++
  }
  if newSession{if held=="MSTR"{out.MSTRSessions++}else{out.MSTZSessions++}}
  equity:=shares*px(p,held,false)*(1-rate)
  if equity>peakEquity{peakEquity=equity}
  if dd:=(peakEquity-equity)/peakEquity*100;dd>out.MaxDrawdownPct{out.MaxDrawdownPct=dd}
  out.ReturnPct=(equity-1)*100
  lastPair=p
 }
 out.Active=held
 return out,nil
}
