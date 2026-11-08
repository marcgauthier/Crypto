// Package simplebtc evaluates a 1% BTC daily-change switch with a single
// shared account. Only completed UTC daily BTC closes can trigger orders.
package simplebtc
import(
 "errors"
 "time"
 "github.com/marcgauthier/Crypto/internal/btcstrategy"
 "github.com/marcgauthier/Crypto/internal/market"
)
type Result struct{
 ReturnPct float64 `json:"return_pct"`
 MaxDrawdownPct float64 `json:"max_drawdown_pct"`
 Switches int `json:"switches"`
 MSTRSessions int `json:"mstr_sessions"`
 MSTZSessions int `json:"mstz_sessions"`
 CashSessions int `json:"cash_sessions"`
 Active string `json:"active"`
}
func Run(mstr,mstz,btc []market.Candle,start,end time.Time,threshold,costBps float64)(Result,error){
 out:=Result{}
 if threshold<=0||costBps<0||costBps>=10000||!start.Before(end){return out,errors.New("invalid parameters")}
 moves:=btcstrategy.UTCMoves(btc)
 ny,err:=time.LoadLocation("America/New_York");if err!=nil{return out,err}
 byTime:=make(map[int64]market.Candle,len(mstz))
 for _,b:=range mstz{byTime[b.Time.Unix()]=b}
 cash,shares:=1.0,0.0
 held:="";lastSession:="";peak:=1.0;rate:=costBps/10000
 for _,a:=range mstr{
  if a.Time.Before(start)||!a.Time.Before(end){continue}
  b,ok:=byTime[a.Time.Unix()];if !ok{continue}
  session:=a.Time.In(ny).Format("2006-01-02")
  if session!=lastSession{
   lastSession=session
   midnight:=time.Date(a.Time.Year(),a.Time.Month(),a.Time.Day(),0,0,0,0,time.UTC)
   if move,ok:=moves[midnight.Unix()];ok{
    desired:=held
    if move.Change1dPct>=threshold{desired="MSTR"}
    if move.Change1dPct<=-threshold{desired="MSTZ"}
    // On the first eligible session, choose the daily BTC direction,
    // even when its magnitude is below 1%, to meet always-invested intent.
    if desired==""{if move.Change1dPct>=0{desired="MSTR"}else{desired="MSTZ"}}
    if desired!=held{
     if held=="MSTR"{cash=shares*a.Open*(1-rate)}
     if held=="MSTZ"{cash=shares*b.Open*(1-rate)}
     if held!=""{out.Switches++}
     px:=a.Open;if desired=="MSTZ"{px=b.Open}
     shares=cash/(px*(1+rate));cash=0;held=desired
    }
   }
   if held=="MSTR"{out.MSTRSessions++}else if held=="MSTZ"{out.MSTZSessions++}else{out.CashSessions++}
  }
  equity:=cash
  if held=="MSTR"{equity=shares*a.Close*(1-rate)}
  if held=="MSTZ"{equity=shares*b.Close*(1-rate)}
  if equity>peak{peak=equity}
  if dd:=(peak-equity)/peak*100;dd>out.MaxDrawdownPct{out.MaxDrawdownPct=dd}
  out.ReturnPct=(equity-1)*100
 }
 if lastSession==""{return out,errors.New("no matching market bars")}
 out.Active=held
 return out,nil
}
