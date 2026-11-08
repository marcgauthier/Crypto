package stockrotation

import(
 "errors"
 "sort"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)
type Result struct{
 ReturnPct float64 `json:"return_pct"`
 MaxDrawdownPct float64 `json:"max_drawdown_pct"`
 Switches int `json:"switches"`
 MSTRSessions int `json:"mstr_sessions"`
 MSTZSessions int `json:"mstz_sessions"`
 Active string `json:"active"`
}
type pair struct{a,b market.Candle}
func Run(mstr,mstz []market.Candle,start,end time.Time,threshold,costBps float64,initial,mode string)(Result,error){
 out:=Result{}
 if threshold<=0||costBps<0||costBps>=10000||!start.Before(end)||(initial!="MSTR"&&initial!="MSTZ")||(mode!="daily"&&mode!="hourly"){return out,errors.New("invalid configuration")}
 bm:=map[int64]market.Candle{}
 for _,b:=range mstz{if market.Valid(b){bm[b.Time.Unix()]=b}}
 pairs:=[]pair{}
 for _,a:=range mstr{if b,ok:=bm[a.Time.Unix()];ok&&market.Valid(a)&&!a.Time.Before(start)&&a.Time.Before(end){pairs=append(pairs,pair{a,b})}}
 sort.Slice(pairs,func(i,j int)bool{return pairs[i].a.Time.Before(pairs[j].a.Time)})
 if len(pairs)<10{return out,errors.New("insufficient aligned candles")}
 ny,err:=time.LoadLocation("America/New_York");if err!=nil{return out,err}
 rate:=costBps/10000;cash,shares:=1.0,0.0;held:="";peak:=1.0
 lastSession:="";var prevSessionClose,prevPrevSessionClose pair
 var lastPair pair;havePrev:=false;havePrevPrev:=false
 for i,p:=range pairs{
  session:=p.a.Time.In(ny).Format("2006-01-02")
  newSession:=session!=lastSession
  if newSession&&lastSession!=""{
   prevPrevSessionClose=prevSessionClose;havePrevPrev=havePrev
   prevSessionClose=lastPair;havePrev=true
  }
  if newSession{lastSession=session}
  switchNow:=false
  if held!=""{
   var older,newer pair
   valid:=false
   if mode=="daily"&&newSession&&havePrevPrev{
    older,newer=prevPrevSessionClose,prevSessionClose;valid=true
   }else if mode=="hourly"&&i>=2{
    older,newer=pairs[i-2],pairs[i-1];valid=true
   }
   if valid{
    oldPx,newPx:=older.a.Close,newer.a.Close
    if held=="MSTZ"{oldPx,newPx=older.b.Close,newer.b.Close}
    // Exit a held position after a 1% decline in its own completed-bar close.
    if oldPx>0&&(newPx/oldPx-1)*100<=-threshold{switchNow=true}
   }
  }
  if held==""||switchNow{
   if held=="MSTR"{cash=shares*p.a.Open*(1-rate)}
   if held=="MSTZ"{cash=shares*p.b.Open*(1-rate)}
   if held!=""{out.Switches++}
   if held==""{held=initial}else if held=="MSTR"{held="MSTZ"}else{held="MSTR"}
   px:=p.a.Open;if held=="MSTZ"{px=p.b.Open}
   shares=cash/(px*(1+rate));cash=0
  }
  if newSession{
   if held=="MSTR"{out.MSTRSessions++}else{out.MSTZSessions++}
  }
  equity:=shares*p.a.Close*(1-rate);if held=="MSTZ"{equity=shares*p.b.Close*(1-rate)}
  if equity>peak{peak=equity}
  if dd:=(peak-equity)/peak*100;dd>out.MaxDrawdownPct{out.MaxDrawdownPct=dd}
  out.ReturnPct=(equity-1)*100
  lastPair=p
 }
 out.Active=held
 return out,nil
}
