package main
import("context";"encoding/json";"fmt";"os";"sort";"time";"github.com/marcgauthier/Crypto/internal/market")
type day struct{date string;open,high,low,close float64;time time.Time}
type row struct{Date string `json:"date"`;EntryDate string `json:"entry_date"`;Entry float64 `json:"entry"`;SignalLow float64 `json:"signal_low"`;Prev30Low float64 `json:"previous_30_low"`;DaysObserved int `json:"days_observed"`;NoLowerLow5 bool `json:"no_lower_low_5"`;NoLowerLow10 bool `json:"no_lower_low_10"`;Hit2Pct5 bool `json:"hit_2pct_5"`;Hit5Pct10 bool `json:"hit_5pct_10"`;Hit10Pct20 bool `json:"hit_10pct_20"`;First2BeforeLower10 bool `json:"first_2_before_lower_10"`;First5BeforeLower20 bool `json:"first_5_before_lower_20"`;Return5Pct float64 `json:"return_5_pct"`;Return10Pct float64 `json:"return_10_pct"`}
func run()error{
 end:=time.Date(2026,11,8,0,0,0,0,time.UTC);start:=end.AddDate(0,-3,0)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 bars,err:=(market.Yahoo{}).Fetch(ctx,"MSTR",start.AddDate(0,0,-65),end);if err!=nil{return err}
 loc,_:=time.LoadLocation("America/New_York")
 days:=[]day{}
 for _,c:=range bars{
  date:=c.Time.In(loc).Format("2006-01-02")
  if len(days)==0||days[len(days)-1].date!=date{days=append(days,day{date,c.Open,c.High,c.Low,c.Close,c.Time})}else{
   d:=&days[len(days)-1];if c.High>d.high{d.high=c.High};if c.Low<d.low{d.low=c.Low};d.close=c.Close
  }
 }
 sort.Slice(days,func(i,j int)bool{return days[i].time.Before(days[j].time)})
 out:=[]row{}
 for i:=7;i+1<len(days);i++{
  d:=days[i];if d.time.Before(start)||!d.time.Before(end){continue}
  prev:=days[i-7].low;for j:=i-6;j<i;j++{if days[j].low<prev{prev=days[j].low}}
  if d.low>prev*1.01||d.close<d.open*0.995{continue}
  r:=row{Date:d.date,EntryDate:days[i+1].date,Entry:days[i+1].open,SignalLow:d.low,Prev30Low:prev,NoLowerLow5:true,NoLowerLow10:true}
  lower10:=false;lower20:=false
  for j:=i+1;j<len(days)&&j<=i+20;j++{
   h:=j-i;r.DaysObserved=h;x:=days[j]
   if h<=5{if x.low<d.low{r.NoLowerLow5=false};if x.high>=r.Entry*1.02{r.Hit2Pct5=true}}
   if h<=10{if x.low<d.low{r.NoLowerLow10=false};if x.high>=r.Entry*1.05{r.Hit5Pct10=true}}
   if h<=20&&x.high>=r.Entry*1.10{r.Hit10Pct20=true}
   // On same-day target and lower low, count as ambiguous, not a win.
   if h<=10&&!lower10{if x.low<d.low{lower10=true}else if x.high>=r.Entry*1.02{r.First2BeforeLower10=true}}
   if h<=20&&!lower20{if x.low<d.low{lower20=true}else if x.high>=r.Entry*1.05{r.First5BeforeLower20=true}}
   if h==5{r.Return5Pct=(x.close/r.Entry-1)*100}
   if h==10{r.Return10Pct=(x.close/r.Entry-1)*100}
  }
  out=append(out,r)
 }
 output:=struct{Start,End string;TradingDays int;Signals int;Results []row}{start.Format("2006-01-02"),end.AddDate(0,0,-1).Format("2006-01-02"),len(days),len(out),out}
 return json.NewEncoder(os.Stdout).Encode(output)
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
