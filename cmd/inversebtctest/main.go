package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "time"
 "github.com/marcgauthier/Crypto/internal/cache"
 "github.com/marcgauthier/Crypto/internal/market"
 "github.com/marcgauthier/Crypto/internal/simplebtc"
)
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive UTC end")
 cutArg:=flag.String("train-until","2026-08-08","holdout start")
 dataDir:=flag.String("data-dir","./data","metrics cache")
 flag.Parse()
 day,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 cut,err:=time.Parse("2006-01-02",*cutArg);if err!=nil{return err}
 if !start.Before(cut)||!cut.Before(end)||end.After(time.Now().UTC()){return fmt.Errorf("invalid dates")}
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Minute);defer cancel()
 db,err:=cache.OpenMetrics(*dataDir,3650);if err!=nil{return err};defer db.Close()
 fetch:=func(s string,from time.Time)([]market.Candle,error){v,_,e:=cache.Load(ctx,db,market.Yahoo{},s,from,end,time.Now().UTC());return v,e}
 a,err:=fetch("MSTR",start);if err!=nil{return err}
 b,err:=fetch("MSTZ",start);if err!=nil{return err}
 btc,err:=fetch("BTC-USD",start.AddDate(0,0,-8));if err!=nil{return err}
 train,err:=simplebtc.Run(b,a,btc,start,cut,1,10);if err!=nil{return err}
 hold,err:=simplebtc.Run(b,a,btc,cut,end,1,10);if err!=nil{return err}
 return json.NewEncoder(os.Stdout).Encode(struct{
  Training simplebtc.Result `json:"training"`
  Holdout simplebtc.Result `json:"holdout"`
  BTCBars int `json:"btc_bars"`
  MSTRBars int `json:"mstr_bars"`
  MSTZBars int `json:"mstz_bars"`
 }{train,hold,len(btc),len(a),len(b)})
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
