package rotation
import (
 "testing"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)
func TestExclusivePositionAndNoFutureSignals(t *testing.T){
 a,b:=[]market.Candle{},[]market.Candle{}
 start:=time.Date(2026,5,1,14,30,0,0,time.UTC)
 for i:=0;i<120;i++{
  x:=100.0
  if i>=25&&i<35{x=100-float64(i-24)*2}
  if i>=35&&i<42{x=80}
  if i>=42{x=80+float64(i-41)*0.4}
  y:=100.0
  if i>=70&&i<80{y=100-float64(i-69)*2}
  if i>=80&&i<87{y=80}
  if i>=87{y=80+float64(i-86)*0.4}
  a=append(a,market.Candle{Time:start.Add(time.Duration(i)*time.Hour),Open:x,High:x*1.01,Low:x*.99,Close:x,Volume:100})
  b=append(b,market.Candle{Time:start.Add(time.Duration(i)*time.Hour),Open:y,High:y*1.01,Low:y*.99,Close:y,Volume:100})
 }
 p:=Params{Lookback:12,StableBars:3,MaxRangePct:3,ReboundPct:0.2,StopPct:8}
 r,err:=Run(a,b,p,start,10)
 if err!=nil{t.Fatal(err)}
 if r.Entries["MSTR"]>1||r.Entries["MSTZ"]>1{t.Fatalf("repeated entry on same plateau: %+v",r.Entries)}
 for _,tr:=range r.Trades{if tr.Exit.Before(tr.Entry){t.Fatalf("exit before entry: %+v",tr)}}
}
func TestInvalidSettings(t *testing.T){
 _,err:=Run(nil,nil,Params{},time.Now(),10)
 if err==nil{t.Fatal("invalid params accepted")}
}
