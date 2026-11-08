package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "strings"
 "time"
 "github.com/marcgauthier/Crypto/internal/cache"
 "github.com/marcgauthier/Crypto/internal/market"
 "github.com/marcgauthier/Crypto/internal/stockrotation"
)
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive end")
 cutArg:=flag.String("train-until","2026-08-08","holdout begins")
 pairsArg:=flag.String("pairs","SOXL:SOXS,TQQQ:SQQQ,TNA:TZA,LABU:LABD,SPXL:SPXS,TECL:TECS,FAS:FAZ,ERX:ERY","long:short pairs")
 dataDir:=flag.String("data-dir","./data","metrics cache")
 flag.Parse()
 day,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 cut,err:=time.Parse("2006-01-02",*cutArg);if err!=nil{return err}
 if !start.Before(cut)||!cut.Before(end)||end.After(time.Now().UTC()){return fmt.Errorf("invalid dates")}
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Minute);defer cancel()
 db,err:=cache.OpenMetrics(*dataDir,3650);if err!=nil{return err};defer db.Close()
 fetch:=func(s string)([]market.Candle,error){v,_,e:=cache.Load(ctx,db,market.Yahoo{},s,start,end,time.Now().UTC());return v,e}
 
 type row struct{
  Pair string `json:"pair"`
  Mode string `json:"mode"`
  Reverse bool `json:"reverse"`
  Initial string `json:"initial"`
  Training stockrotation.Result `json:"training"`
  Holdout stockrotation.Result `json:"holdout"`
 }
 rows:=[]row{}
 for _,pairName:=range strings.Split(*pairsArg,","){
  tickers:=strings.Split(pairName,":");if len(tickers)!=2{return fmt.Errorf("invalid pair %s",pairName)}
  a,err:=fetch(tickers[0]);if err!=nil{return fmt.Errorf("%s: %w",tickers[0],err)}
  b,err:=fetch(tickers[1]);if err!=nil{return fmt.Errorf("%s: %w",tickers[1],err)}
 for _,mode:=range []string{"daily","hourly"}{
  for _,initial:=range []string{"MSTR","MSTZ"}{
   for _,reverse:=range []bool{false,true}{
   train,err:=stockrotation.RunWithDirection(a,b,start,cut,1,10,initial,mode,reverse);if err!=nil{return err}
   test,err:=stockrotation.RunWithDirection(a,b,cut,end,1,10,initial,mode,reverse);if err!=nil{return err}
   rows=append(rows,row{pairName,mode,reverse,initial,train,test})
  }
 }
 }
 }
 return json.NewEncoder(os.Stdout).Encode(struct{Results []row `json:"results"`}{rows})
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
