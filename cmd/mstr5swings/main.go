package main
import("context";"encoding/json";"fmt";"os";"sort";"time";"github.com/marcgauthier/Crypto/internal/market")
type rise struct{LowDate string `json:"low_date"`;HitDate string `json:"hit_date"`;Low float64 `json:"low"`;Hit float64 `json:"hit"`;GainPct float64 `json:"gain_pct"`;Hours float64 `json:"hours"`}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func run()error{
 end:=time.Date(2026,11,8,0,0,0,0,time.UTC);start:=end.AddDate(0,0,-90)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 bars,err:=(market.Yahoo{}).Fetch(ctx,"MSTR",start,end);if err!=nil{return err}
 sort.Slice(bars,func(i,j int)bool{return bars[i].Time.Before(bars[j].Time)})
 if len(bars)==0{return fmt.Errorf("no bars")}
 loc,_:=time.LoadLocation("America/New_York")
 trough:=bars[0].Low;troughTime:=bars[0].Time
 armed:=true
 results:=[]rise{}
 for _,c:=range bars{
  if !market.Valid(c){continue}
  if !armed{trough=c.Low;troughTime=c.Time;armed=true;continue}
  // An hourly candle cannot establish whether its low preceded its high.
  // Only count a rise when its target is hit on a later candle.
  if c.Time.After(troughTime)&&c.High>=trough*1.05{
   results=append(results,rise{troughTime.In(loc).Format("2006-01-02 15:04"),c.Time.In(loc).Format("2006-01-02 15:04"),trough,c.High,(c.High/trough-1)*100,c.Time.Sub(troughTime).Hours()})
   armed=false;continue
  }
  if c.Low<trough{trough=c.Low;troughTime=c.Time}
 }
 return json.NewEncoder(os.Stdout).Encode(struct{Start,End string;Candles int;Count int;Rises []rise}{start.Format("2006-01-02"),end.Format("2006-01-02"),len(bars),len(results),results})
}
