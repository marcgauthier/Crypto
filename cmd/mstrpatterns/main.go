package main
import("context";"encoding/json";"fmt";"math";"os";"sort";"time";"github.com/marcgauthier/Crypto/internal/market")
type event struct{Time string `json:"time"`;Low float64 `json:"low"`;WaitHours float64 `json:"wait_hours"`;Trend1,Trend3,Trend7 float64;Support7,Support30,Support90 float64;Range7,Range30,Range90 float64;RSI14 float64;MA20Dist float64;DayChange float64;DayRange float64;VolumeRatio float64}
type day struct{date string;open,high,low,close,volume float64}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func run()error{
 end:=time.Date(2026,11,8,0,0,0,0,time.UTC);start:=end.AddDate(0,0,-90)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 bars,err:=(market.Yahoo{}).Fetch(ctx,"MSTR",start.AddDate(0,0,-160),end);if err!=nil{return err}
 sort.Slice(bars,func(i,j int)bool{return bars[i].Time.Before(bars[j].Time)})
 loc,_:=time.LoadLocation("America/New_York")
 ds:=[]day{};di:=[]int{}
 for _,c:=range bars{if !market.Valid(c){continue};date:=c.Time.In(loc).Format("2006-01-02")
  if len(ds)==0||ds[len(ds)-1].date!=date{ds=append(ds,day{date,c.Open,c.High,c.Low,c.Close,float64(c.Volume)})}else{d:=&ds[len(ds)-1];d.high=math.Max(d.high,c.High);d.low=math.Min(d.low,c.Low);d.close=c.Close;d.volume+=float64(c.Volume)}
  di=append(di,len(ds)-1)
 }
 // Align the filtered bars and their daily index.
 clean:=[]market.Candle{};for _,c:=range bars{if market.Valid(c){clean=append(clean,c)}}
 bars=clean
 calc:=func(i int,low float64,wait float64)event{
  k:=di[i];e:=event{Time:bars[i].Time.In(loc).Format("2006-01-02 15:04"),Low:low,WaitHours:wait}
  tr:=func(n int)float64{if k<n{return math.NaN()};return (low/ds[k-n].close-1)*100}
  e.Trend1=tr(1);e.Trend3=tr(3);e.Trend7=tr(7)
  for z,n:=range []int{7,30,90}{if k<n{continue};lo,hi:=ds[k-n].low,ds[k-n].high;for j:=k-n+1;j<k;j++{lo=math.Min(lo,ds[j].low);hi=math.Max(hi,ds[j].high)}
   sup:=(low/lo-1)*100;rg:=100*(low-lo)/(hi-lo);if hi==lo{rg=50}
   switch z{case 0:e.Support7=sup;e.Range7=rg;case 1:e.Support30=sup;e.Range30=rg;case 2:e.Support90=sup;e.Range90=rg}
  }
  if k>=20{sum:=0.0;for j:=k-20;j<k;j++{sum+=ds[j].close};e.MA20Dist=(low/(sum/20)-1)*100}
  if k>=15{up,down:=0.0,0.0;for j:=k-14;j<k;j++{diff:=ds[j].close-ds[j-1].close;if diff>0{up+=diff}else{down-=diff}};if down==0{e.RSI14=100}else{e.RSI14=100-100/(1+up/down)}}
  e.DayChange=(low/ds[k].open-1)*100;e.DayRange=(ds[k].high/ds[k].low-1)*100
  if k>=20{v:=0.0;for j:=k-20;j<k;j++{v+=ds[j].volume};if v>0{e.VolumeRatio=ds[k].volume/(v/20)}}
  return e
 }
 // Exactly same segmentation as previous 35-swing scan.
 first:=0;for first<len(bars)&&bars[first].Time.Before(start){first++}
 trough:=bars[first].Low;ti:=first;armed:=true;ev:=[]event{}
 for i:=first;i<len(bars);i++{c:=bars[i]
  if !armed{trough=c.Low;ti=i;armed=true;continue}
  if i>ti&&c.High>=trough*1.05{ev=append(ev,calc(ti,trough,c.Time.Sub(bars[ti].Time).Hours()));armed=false;continue}
  if c.Low<trough{trough=c.Low;ti=i}
 }
 // Baseline: all hourly bars in period (not only the 35 successful lows).
 base:=[]event{};for i:=first;i<len(bars);i++{base=append(base,calc(i,bars[i].Low,0))}
 output:=struct{Start string;Events []event;Baseline []event}{start.Format("2006-01-02"),ev,base}
 return json.NewEncoder(os.Stdout).Encode(output)
}
