package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "math"
 "os"
 "sort"
 "time"
 "github.com/marcgauthier/Crypto/internal/market"
)
type bar struct{day string;open,close float64}
type week struct{Start string `json:"start"`;End string `json:"end"`;Days int `json:"days"`;HoldPct float64 `json:"hold_pct"`;SwitchPct float64 `json:"switch_pct"`;SwitchBeat bool `json:"switch_beat"`;CumulativeHoldPct float64 `json:"cumulative_hold_pct"`;CumulativeSwitchPct float64 `json:"cumulative_switch_pct"`;CumulativeBeat bool `json:"cumulative_beat"`}
type result struct{ReturnPct float64 `json:"return_pct"`;MaxDD float64 `json:"max_dd_pct"`;Switches int `json:"switches"`;LongTrades int `json:"long_trades"`;ShortTrades int `json:"short_trades"`}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func run()error{
 endArg:=flag.String("end","2026-11-07","last UTC date inclusive");flag.Parse()
 last,err:=time.Parse("2006-01-02",*endArg);if err!=nil{return err}
 end:=last.AddDate(0,0,1);start:=end.AddDate(0,-6,0)
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 cs,err:=(market.Yahoo{}).Fetch(ctx,"BTC-USD",start,end);if err!=nil{return err}
 sort.Slice(cs,func(i,j int)bool{return cs[i].Time.Before(cs[j].Time)})
 days:=[]bar{}
 for _,c:=range cs{if !market.Valid(c){continue};d:=c.Time.UTC().Format("2006-01-02");if len(days)==0||days[len(days)-1].day!=d{days=append(days,bar{day:d,open:c.Open,close:c.Close})}else{days[len(days)-1].close=c.Close}}
 if len(days)<100{return fmt.Errorf("only %d BTC days returned",len(days))}
 // 1x inverse exposure modeled with linear daily short P&L, no leverage or funding.
 // Rebalance inverse at each daily close, avoiding negative-price synthetic instruments.
 rate:=0.001
 hold:=1/(1+rate);holdPeak:=1.0;holdDD:=0.0;holdMarks:=make([]float64,0,len(days))
 for i,d:=range days{if i==0{hold*=d.close/d.open}else{hold*=d.close/days[i-1].close};v:=hold*(1-rate);if v>holdPeak{holdPeak=v};holdDD=math.Max(holdDD,(holdPeak-v)/holdPeak*100);holdMarks=append(holdMarks,v)}
 equity:=1/(1+rate);peak:=1.0;position:=1;switches:=0;longTrades:=0;shortTrades:=0
 for i,d:=range days{
  if i>=2{gain:=days[i-1].close/days[i-2].close-1;if position==1&&gain>=0.01||position==-1&&gain<=-0.01{equity*=1-rate;position= -position;equity/=1+rate;switches++;if position==1{shortTrades++}else{longTrades++}}}
  if i==0{equity*=1+float64(position)*(d.close/d.open-1)}else{equity*=1+float64(position)*(d.close/days[i-1].close-1)}
  if equity>peak{peak=equity}
 }
 // Rerun with proper open execution: apply prior-close to current open in old position,
 // then open-to-close in the new position.
 equity=1/(1+rate);peak=1;dd:=0.0;position=1;switches=0;longTrades=0;shortTrades=0;switchMarks:=make([]float64,0,len(days))
 for i,d:=range days{
  if i>0{equity*=1+float64(position)*(d.open/days[i-1].close-1)}
  if i>=2{gain:=days[i-1].close/days[i-2].close-1;if position==1&&gain>=0.01||position==-1&&gain<=-0.01{
    equity*=1-rate;equity/=1+rate;position= -position;switches++;if position==1{shortTrades++}else{longTrades++}
  }}
  equity*=1+float64(position)*(d.close/d.open-1)
  mark:=equity*(1-rate);if mark>peak{peak=mark};dd=math.Max(dd,(peak-mark)/peak*100);switchMarks=append(switchMarks,mark)
 }
 weeks:=[]week{};weeklyWins:=0;cumulativeWins:=0;fullWins:=0;fullCount:=0
 for begin:=0;begin<len(days);begin+=7{
  finish:=begin+7;if finish>len(days){finish=len(days)}
  hbase,sbase:=1.0,1.0
  if begin>0{hbase=holdMarks[begin-1];sbase=switchMarks[begin-1]}
  h:=100*(holdMarks[finish-1]/hbase-1);s:=100*(switchMarks[finish-1]/sbase-1)
  ch:=100*(holdMarks[finish-1]-1);cs:=100*(switchMarks[finish-1]-1)
  w:=week{days[begin].day,days[finish-1].day,finish-begin,h,s,s>h,ch,cs,cs>ch}
  weeks=append(weeks,w)
  if w.SwitchBeat{weeklyWins++};if w.CumulativeBeat{cumulativeWins++}
  if w.Days==7{fullCount++;if w.SwitchBeat{fullWins++}}
 }
 out:=struct{Start string `json:"start"`;End string `json:"end"`;Days int `json:"days"`;BuyHold result `json:"buy_hold"`;Switch result `json:"switch_1pct"`;Assumptions string `json:"assumptions"`;Weeks []week `json:"weeks"`;WeeklyWins int `json:"weekly_wins"`;CumulativeWins int `json:"cumulative_wins"`;FullWeekWins int `json:"full_week_wins"`;FullWeeks int `json:"full_weeks"`}{days[0].day,days[len(days)-1].day,len(days),result{ReturnPct:(hold*(1-rate)-1)*100,MaxDD:holdDD},result{ReturnPct:(equity*(1-rate)-1)*100,MaxDD:dd,Switches:switches,LongTrades:longTrades,ShortTrades:shortTrades},"UTC daily candles aggregated from Yahoo BTC-USD hourly; 1x linear inverse short, no funding, 10bps per side; next UTC day open execution",weeks,weeklyWins,cumulativeWins,fullWins,fullCount}
 fmt.Fprintf(os.Stderr,"WEEKLY_RESULT %d/%d weeks; full weeks %d/%d; cumulative %d/%d\\n",weeklyWins,len(weeks),fullWins,fullCount,cumulativeWins,len(weeks))
 for _,w:=range weeks{fmt.Fprintf(os.Stderr,"WEEK %s %s HOLD %+.2f SWITCH %+.2f BEAT %t\\n",w.Start,w.End,w.HoldPct,w.SwitchPct,w.SwitchBeat)}
 return json.NewEncoder(os.Stdout).Encode(out)
}
