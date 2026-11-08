package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "time"
 "github.com/marcgauthier/Crypto/internal/alwaysbtc"
 "github.com/marcgauthier/Crypto/internal/cache"
 "github.com/marcgauthier/Crypto/internal/market"
)
func run()error{
 endText:=flag.String("end","2026-11-07","inclusive historical UTC end")
 cutoffText:=flag.String("train-until","2026-08-08","holdout start")
 dir:=flag.String("data-dir","./data","metrics cache")
 flag.Parse()
 day,err:=time.Parse("2006-01-02",*endText);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 cutoff,err:=time.Parse("2006-01-02",*cutoffText);if err!=nil{return err}
 if !start.Before(cutoff)||!cutoff.Before(end)||end.After(time.Now().UTC()){return fmt.Errorf("invalid historical period")}
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Minute);defer cancel()
 db,err:=cache.OpenMetrics(*dir,3650);if err!=nil{return err};defer db.Close()
 fetch:=func(symbol string,from time.Time)([]market.Candle,error){bars,_,err:=cache.Load(ctx,db,market.Yahoo{},symbol,from,end,time.Now().UTC());return bars,err}
 a,err:=fetch("MSTR",start);if err!=nil{return err}
 b,err:=fetch("MSTZ",start);if err!=nil{return err}
 btc,err:=fetch("BTC-USD",start.AddDate(0,0,-8));if err!=nil{return err}
 train,err:=alwaysbtc.Optimize(a,b,btc,alwaysbtc.Config{Start:start,End:cutoff,CostBps:10});if err!=nil{return err}
 test,err:=alwaysbtc.Run(a,b,btc,train.Params,alwaysbtc.Config{Start:cutoff,End:end,CostBps:10});if err!=nil{return err}
 fixed,err:=alwaysbtc.Run(a,b,btc,alwaysbtc.Params{DayWeight:1,SwitchThreshold:2,ConfirmDays:2},alwaysbtc.Config{Start:cutoff,End:end,CostBps:10});if err!=nil{return err}
 train.Equity=nil;test.Equity=nil;fixed.Equity=nil
 return json.NewEncoder(os.Stdout).Encode(struct{
  Training alwaysbtc.Result `json:"training"`
  Holdout alwaysbtc.Result `json:"holdout"`
  FixedHoldout alwaysbtc.Result `json:"fixed_holdout"`
  BTCBars int `json:"btc_bars"`
  MSTRBars int `json:"mstr_bars"`
  MSTZBars int `json:"mstz_bars"`
 }{train,test,fixed,len(btc),len(a),len(b)})
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
