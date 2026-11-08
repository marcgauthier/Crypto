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
 "github.com/marcgauthier/Crypto/internal/stockrotation"
)
func run()error{
 endArg:=flag.String("end","2026-11-07","inclusive end")
 cutArg:=flag.String("train-until","2026-08-08","holdout begins")
 dataDir:=flag.String("data-dir","./data","metrics cache")
 flag.Parse()
 day,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=day.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 cut,err:=time.Parse("2006-01-02",*cutArg);if err!=nil{return err}
 if !start.Before(cut)||!cut.Before(end)||end.After(time.Now().UTC()){return fmt.Errorf("invalid dates")}
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Minute);defer cancel()
 db,err:=cache.OpenMetrics(*dataDir,3650);if err!=nil{return err};defer db.Close()
 fetch:=func(s string)([]market.Candle,error){v,_,e:=cache.Load(ctx,db,market.Yahoo{},s,start,end,time.Now().UTC());return v,e}
 a,err:=fetch("MSTR");if err!=nil{return err}
 b,err:=fetch("MSTZ");if err!=nil{return err}
 type row struct{
  Mode string `json:"mode"`
  Reverse bool `json:"reverse"`
  Initial string `json:"initial"`
  Training stockrotation.Result `json:"training"`
  Holdout stockrotation.Result `json:"holdout"`
 }
 rows:=[]row{}
 for _,mode:=range []string{"daily","hourly"}{
  for _,initial:=range []string{"MSTR","MSTZ"}{
   for _,reverse:=range []bool{false,true}{
   train,err:=stockrotation.RunWithDirection(a,b,start,cut,1,10,initial,mode,reverse);if err!=nil{return err}
   test,err:=stockrotation.RunWithDirection(a,b,cut,end,1,10,initial,mode,reverse);if err!=nil{return err}
   rows=append(rows,row{mode,reverse,initial,train,test})
  }
 }
 }
 return json.NewEncoder(os.Stdout).Encode(struct{Results []row `json:"results"`; MSTRBars int `json:"mstr_bars"`; MSTZBars int `json:"mstz_bars"`}{rows,len(a),len(b)})
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
