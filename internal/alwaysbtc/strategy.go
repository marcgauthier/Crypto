package alwaysbtc

import (
 "errors"
 "math"
 "time"
 "github.com/marcgauthier/Crypto/internal/btcstrategy"
 "github.com/marcgauthier/Crypto/internal/market"
)
type Params struct {
 DayWeight float64 `json:"day_weight"`
 SwitchThreshold float64 `json:"switch_threshold"`
 ConfirmDays int `json:"confirm_days"`
}
type Config struct { Start,End time.Time; CostBps float64 }
type Result struct {
 Params Params `json:"params"`
 ReturnPct float64 `json:"return_pct"`
 MaxDrawdownPct float64 `json:"max_drawdown_pct"`
 Switches int `json:"switches"`
 MSTRDays int `json:"mstr_sessions"`
 MSTZDays int `json:"mstz_sessions"`
 Active string `json:"active"`
 Equity []float64 `json:"equity,omitempty"`
}
func Run(mstr,mstz,btc []market.Candle,p Params,c Config)(Result,error){
 out:=Result{Params:p}
 if p.DayWeight<0||p.SwitchThreshold<0||p.ConfirmDays<1||p.ConfirmDays>7||c.CostBps<0||c.CostBps>=10000||!c.Start.Before(c.End){return out,errors.New("invalid configuration")}
 moves:=btcstrategy.UTCMoves(btc)
 byB:=make(map[int64]market.Candle,len(mstz))
 for _,b:=range mstz{byB[b.Time.Unix()]=b}
 ny,err:=time.LoadLocation("America/New_York");if err!=nil{return out,err}
 cash,shares:=1.0,0.0;position:="";rate:=c.CostBps/10000
 lastSession:="";lastScoreDate:=int64(0);lastSign:=0;streak:=0
 peak:=1.0;count:=0
 for _,a:=range mstr{
  if a.Time.Before(c.Start)||!a.Time.Before(c.End){continue}
  b,ok:=byB[a.Time.Unix()];if !ok{continue}
  session:=a.Time.In(ny).Format("2006-01-02")
  if session!=lastSession{
   lastSession=session
   midnight:=time.Date(a.Time.Year(),a.Time.Month(),a.Time.Day(),0,0,0,0,time.UTC)
   if move,valid:=moves[midnight.Unix()];valid{
    score:=move.Change7dPct+p.DayWeight*move.Change1dPct
    sign:=0
    if score>p.SwitchThreshold{sign=1}else if score< -p.SwitchThreshold{sign=-1}
    if midnight.Unix()!=lastScoreDate{
     if sign!=0&&sign==lastSign{streak++}else if sign!=0{streak=1}else{streak=0}
     lastSign=sign;lastScoreDate=midnight.Unix()
    }
    desired:=position
    // Initial entry never waits for confirmation: the portfolio must be
    // invested whenever eligible prices and BTC history are available.
    if position==""{
     if score>=0{desired="MSTR"}else{desired="MSTZ"}
    }else if streak>=p.ConfirmDays{
     if sign==1{desired="MSTR"}else if sign==-1{desired="MSTZ"}
    }
    if desired!=position{
     if position=="MSTR"{cash=shares*a.Open*(1-rate)}
     if position=="MSTZ"{cash=shares*b.Open*(1-rate)}
     if position!=""{out.Switches++}
     price:=a.Open;if desired=="MSTZ"{price=b.Open}
     shares=cash/(price*(1+rate));cash=0;position=desired
    }
   }
   if position=="MSTR"{out.MSTRDays++}else if position=="MSTZ"{out.MSTZDays++}
  }
  if position==""{continue}
  equity:=shares*a.Close*(1-rate);if position=="MSTZ"{equity=shares*b.Close*(1-rate)}
  out.Equity=append(out.Equity,equity);count++
  if equity>peak{peak=equity}
  if dd:=(peak-equity)/peak*100;dd>out.MaxDrawdownPct{out.MaxDrawdownPct=dd}
 }
 if count==0{return out,errors.New("no eligible sessions with BTC history")}
 out.ReturnPct=(out.Equity[len(out.Equity)-1]-1)*100;out.Active=position
 return out,nil
}
func Optimize(mstr,mstz,btc []market.Candle,c Config)(Result,error){
 best:=Result{};found:=false
 for _,w:=range []float64{0,0.5,1,2,3}{
 for _,threshold:=range []float64{0,1,2,4,6}{
 for _,confirm:=range []int{1,2,3}{
  r,err:=Run(mstr,mstz,btc,Params{DayWeight:w,SwitchThreshold:threshold,ConfirmDays:confirm},c)
  if err!=nil{return r,err}
  if r.Switches<2{continue}
  if !found||r.ReturnPct>best.ReturnPct+1e-9||(math.Abs(r.ReturnPct-best.ReturnPct)<1e-9&&r.MaxDrawdownPct<best.MaxDrawdownPct){best=r;found=true}
 }}}
 if !found{return best,errors.New("no training candidate switched at least twice")}
 return best,nil
}
