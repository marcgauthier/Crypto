package main
import (
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "time"
 "github.com/marcgauthier/Crypto/internal/cache"
 "github.com/marcgauthier/Crypto/internal/market"
 "github.com/marcgauthier/Crypto/internal/rotation"
)
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive UTC end date")
 cutoffArg:=flag.String("train-until","2026-08-08","UTC holdout start")
 dataDir:=flag.String("data-dir","./data","MetricsDB cache")
 flag.Parse()
 d,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=d.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 cutoff,err:=time.Parse("2006-01-02",*cutoffArg);if err!=nil{return err}
 db,err:=cache.OpenMetrics(*dataDir,3650);if err!=nil{return err};defer db.Close()
 ctx:=context.Background();provider:=market.Yahoo{}
 a,_,err:=cache.Load(ctx,db,provider,"MSTR",start,end,time.Now().UTC());if err!=nil{return err}
 b,_,err:=cache.Load(ctx,db,provider,"MSTZ",start,end,time.Now().UTC());if err!=nil{return err}
 if len(a)<100||len(b)<100{return fmt.Errorf("missing stock candles")}
 best:=rotation.Result{};found:=false;trials:=0
 // Optimize ONLY on candles before cutoff, with a strict training boundary.
 ta,tb:=[]market.Candle{},[]market.Candle{}
 for _,c:=range a{if c.Time.Before(cutoff){ta=append(ta,c)}}
 for _,c:=range b{if c.Time.Before(cutoff){tb=append(tb,c)}}
 for _,look:=range []int{12,24,36}{
 for _,stable:=range []int{2,3,4}{
 for _,width:=range []float64{1,2,3}{
 for _,rebound:=range []float64{0.2,0.5,1}{
 for _,stop:=range []float64{4,8,12}{
  p:=rotation.Params{Lookback:look,StableBars:stable,MaxRangePct:width,ReboundPct:rebound,StopPct:stop}
  r,e:=rotation.Run(ta,tb,p,ta[look+stable].Time,10);if e!=nil{return e}
  trials++
  if len(r.Trades)<2{continue}
  if !found||r.ReturnPct>best.ReturnPct{best=r;found=true}
 }}}}}
 if !found{return fmt.Errorf("no candidate has 2 training trades")}
 test,err:=rotation.Run(a,b,best.Params,cutoff,10);if err!=nil{return err}
 fixed:=rotation.Params{Lookback:24,StableBars:3,MaxRangePct:2,ReboundPct:0.5,StopPct:8}
 fixedTest,err:=rotation.Run(a,b,fixed,cutoff,10);if err!=nil{return err}
 return json.NewEncoder(os.Stdout).Encode(struct{
  TrainStart time.Time `json:"train_start"`
  TestStart time.Time `json:"test_start"`
  End time.Time `json:"end"`
  Trials int `json:"train_trials"`
  Training rotation.Result `json:"training"`
  Holdout rotation.Result `json:"holdout"`
  FixedHoldout rotation.Result `json:"fixed_holdout"`
 }{start,cutoff,end,trials,best,test,fixedTest})
}
