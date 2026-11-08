package dashboard

const page = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Crypto | Strategy Stability</title>
<style>
:root{color-scheme:dark;--bg:#0c1321;--panel:#142032;--muted:#9eb2c9;--border:#273a50;--text:#e8f3ff;--cyan:#43d4cf;--amber:#ffbd65;--red:#fc8d91}
*{box-sizing:border-box}body{margin:0;font:15px/1.55 system-ui,-apple-system,Segoe UI,Arial,sans-serif;background:var(--bg);color:var(--text)}
main{width:min(1300px,96vw);margin:34px auto 72px}.eyebrow{font-size:.75rem;text-transform:uppercase;letter-spacing:.17em;color:var(--cyan);font-weight:700}h1{margin:8px 0 4px;font-size:clamp(1.8rem,4vw,2.5rem);line-height:1.2}p{margin:.35em 0 1em;color:var(--muted)}
.toolbar{background:var(--panel);border:1px solid var(--border);border-radius:14px;padding:16px 20px;margin:25px 0;display:flex;gap:20px;align-items:end;flex-wrap:wrap}.toolbar>div{flex:1 1 330px}label{display:block;color:var(--muted);font-size:.81rem;margin-bottom:7px}select{width:100%;border:1px solid #41536a;border-radius:9px;background:#0d1726;color:var(--text);padding:11px 12px;font-size:14px}button{color:var(--bg);background:var(--cyan);border:0;border-radius:9px;padding:11px 17px;font-weight:700;cursor:pointer}.muted{color:var(--muted)}
.cards{display:grid;grid-template-columns:repeat(4,minmax(170px,1fr));gap:14px;margin-bottom:20px}.card,.panel{background:var(--panel);border:1px solid var(--border);border-radius:14px;padding:18px}.card .label{color:var(--muted);font-size:.81rem}.card .value{font-weight:760;font-size:1.7rem;letter-spacing:-.03em;margin:4px 0}.card .sub{color:var(--muted);font-size:.77rem}
.charts{display:grid;grid-template-columns:1fr 1fr;gap:16px}.panel h2{font-size:1rem;margin:0 0 12px}.panel svg{width:100%;height:auto;display:block;overflow:visible}.legend{display:flex;gap:15px;font-size:.8rem;color:var(--muted);flex-wrap:wrap}.dot{width:9px;height:9px;display:inline-block;border-radius:100%;margin-right:5px}
.note{border-left:3px solid var(--amber);background:#1d2631;padding:10px 15px;border-radius:0 9px 9px 0;margin:22px 0;color:#dfd4bd}.tablepanel{margin-top:20px;overflow:auto}.tablepanel h2{margin:0 0 12px;font-size:1.05rem}table{width:100%;border-collapse:collapse;white-space:nowrap;text-align:right;font-variant-numeric:tabular-nums}th,td{padding:11px 12px;border-bottom:1px solid var(--border);font-size:.87rem}th{font-size:.75rem;color:var(--muted);font-weight:600}th:first-child,td:first-child{text-align:left}tbody tr:hover{background:#1c2c41}.pos{color:var(--cyan)}.neg{color:var(--red)}
#status{min-height:22px}
.livepanel{margin:20px 0}.livepanel h2{margin:0 0 4px;font-size:1.15rem}.livepanel .scroll{overflow:auto;margin-top:10px}.livepanel td:first-child,.livepanel th:first-child{text-align:left}.livepanel td{font-size:.85rem}.liveflag{display:inline-block;border-radius:8px;border:1px solid var(--border);padding:2px 8px;font-size:.78rem}.liveflag.open{border-color:var(--amber);color:var(--amber)}.liveflag.buy{border-color:var(--cyan);color:var(--cyan)}.liveflag.sell{border-color:var(--red);color:var(--red)}
.sr-only{position:absolute;left:-9999px}@media(max-width:900px){.charts{grid-template-columns:1fr}.cards{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:480px){.cards{grid-template-columns:1fr}.toolbar{padding:12px}.card .value{font-size:1.4rem}}
</style></head><body><main>
<div class="eyebrow">Continuous monitoring · 00:00 UTC daily optimization · every 5 minutes</div>
<h1>Live strategy signals</h1>
<p>SOXL / SOXS and MSTR / MSTZ are independent. Both legs can be OPEN simultaneously. BUY of an inverse ETF is bearish exposure, not a margin short. <strong>No orders are submitted.</strong> Prices are indicative completed 5-minute Yahoo candles, which may be delayed.</p>
<section class="panel livepanel" aria-label="Active monitoring rules"><h2>Current rules and paper positions</h2><p class="muted" id="live-updated">Loading status from SQLite…</p>
<div class="scroll"><table><thead><tr><th>Pair / Symbol</th><th>Side</th><th>Buy after drop</th><th>Sell after gain</th><th>Price</th><th>5m bar UTC</th><th>Drop now</th><th>Position</th><th>Entry / target</th><th>State</th><th>Evaluated (UTC)</th></tr></thead><tbody id="live-rows"><tr><td colspan="11">No active rules yet.</td></tr></tbody></table></div></section>
<section class="panel livepanel" aria-label="Recent buy and sell signals"><h2>Recent BUY / SELL signals</h2><p>Signals are recorded in SQLite with indicative prices. One action maximum per symbol per completed 5-minute candle, including across restarts.</p>
<div class="scroll"><table><thead><tr><th>Detected UTC</th><th>Pair / Symbol</th><th>Side</th><th>Action</th><th>5m bar UTC</th><th>Price</th><th>Buy drop</th><th>Sell gain</th><th>Realized paper return</th></tr></thead><tbody id="signals-rows"><tr><td colspan="9">No live signals recorded.</td></tr></tbody></table></div></section>
<div class="eyebrow">Historical research · Local dashboard</div>
<h1>Strategy consistency</h1>
<p>How stable are the optimized buy-after-drop and sell-after-gain thresholds from one scan date to the next?</p>
<section class="toolbar"><div><label for="group">Symbol / pair / window / strategy settings</label><select id="group" aria-label="Select historical strategy"><option>Loading SQLite history...</option></select></div><button id="refresh">Refresh</button></section>
<div id="status" role="status" class="muted"></div>
<div class="cards">
<div class="card"><div class="label">Recorded scan days</div><div class="value" id="count">—</div><div class="sub">Same window and settings only</div></div>
<div class="card"><div class="label">Average buy after drop</div><div class="value" id="buy">—</div><div class="sub" id="buy-std">—</div></div>
<div class="card"><div class="label">Average sell after gain</div><div class="value" id="sell">—</div><div class="sub" id="sell-std">—</div></div>
<div class="card"><div class="label">Latest out-of-sample result</div><div class="value" id="result">—</div><div class="sub" id="result-meta">—</div></div>
</div>
<section class="charts">
<div class="panel"><h2>Optimized thresholds over time</h2><svg role="img" aria-label="Time series of the optimal buy drop and sell gain thresholds" id="threshold-chart" viewBox="0 0 640 280"></svg><div class="legend"><span><i class="dot" style="background:var(--cyan)"></i>Buy drop %</span><span><i class="dot" style="background:var(--amber)"></i>Sell gain %</span></div></div>
<div class="panel"><h2>Out-of-sample returns over time</h2><svg role="img" aria-label="Time series of the strategy and buy-and-hold test returns" id="return-chart" viewBox="0 0 640 280"></svg><div class="legend"><span><i class="dot" style="background:var(--cyan)"></i>Strategy return %</span><span><i class="dot" style="background:var(--amber)"></i>Buy &amp; hold %</span></div></div>
</section>
<div class="note" id="note">Stability describes repeated historical fits, not future profitability. Nearby rolling windows overlap strongly; identical thresholds can still be overfitted.</div>
<section class="panel tablepanel"><h2>Scan history</h2><table><thead><tr><th>UTC date</th><th>Buy drop</th><th>Sell gain</th><th>Training return</th><th>Test return</th><th>Test buy/hold</th><th>Max drawdown</th><th>Trades</th><th>Win rate</th><th>Pair return</th></tr></thead><tbody id="history"><tr><td colspan="10">No scans yet.</td></tr></tbody></table></section>
<p style="font-size:.77rem;margin-top:18px">Source: local SQLite findings. Hourly market candles remain separately cached in MetricsDB. Test results include marked-to-market open positions. No trades are executed.</p>
</main><script>
(() => {
'use strict';
const sel=document.getElementById('group'),status=document.getElementById('status');let groups=[];
const $=id=>document.getElementById(id);
const pct=n=>Number.isFinite(n)?n.toFixed(2)+'%':'—';
const signed=n=>Number.isFinite(n)?(n>=0?'+':'')+n.toFixed(2)+'%':'—';
const ns='http://www.w3.org/2000/svg';
function el(tag,attrs,parent,text){const node=document.createElementNS(ns,tag);Object.entries(attrs||{}).forEach(([k,v])=>node.setAttribute(k,String(v)));if(text!==undefined)node.textContent=text;parent.appendChild(node);return node}
function chart(id,rows,keys){const svg=$(id);svg.replaceChildren();if(!rows.length)return;
 const left=57,right=15,top=13,bottom=38,w=640,h=280,plotW=w-left-right,plotH=h-top-bottom;
 const vals=rows.flatMap(r=>keys.map(k=>Number(r[k.key]))).filter(Number.isFinite);
 if(!vals.length)return;let lo=Math.min(...vals),hi=Math.max(...vals);if(id==='return-chart'){lo=Math.min(0,lo);hi=Math.max(0,hi)}
 const span=hi-lo||2,pad=span*.14;lo-=pad;hi+=pad;
 const X=i=>left+plotW*(rows.length===1?.5:i/(rows.length-1));const Y=v=>top+plotH*(hi-v)/(hi-lo);
 for(let i=0;i<=4;i++){const v=lo+(hi-lo)*(1-i/4),y=top+plotH*i;el('line',{x1:left,y1:y,x2:w-right,y2:y,stroke:'#304359','stroke-width':1},svg);el('text',{x:left-8,y:y+4,'text-anchor':'end',fill:'#91a8be','font-size':12},svg,v.toFixed(1)+'%')}
 keys.forEach(k=>{const points=rows.map((r,i)=>X(i).toFixed(1)+','+Y(r[k.key]).toFixed(1)).join(' ');el('polyline',{points,fill:'none',stroke:k.color,'stroke-width':3,'stroke-linecap':'round','stroke-linejoin':'round'},svg);
 rows.forEach((r,i)=>{const dot=el('circle',{cx:X(i),cy:Y(r[k.key]),r:rows.length<70?3.2:2.0,fill:k.color},svg);el('title',{},dot,r.analysis_day+' · '+k.label+' '+pct(r[k.key]))})});
 const first=rows[0].analysis_day,last=rows.at(-1).analysis_day;el('text',{x:left,y:h-12,fill:'#91a8be','font-size':12},svg,first);el('text',{x:w-right,y:h-12,'text-anchor':'end',fill:'#91a8be','font-size':12},svg,last);
}
function cell(row,value,cl=''){const td=document.createElement('td');td.textContent=value;if(cl)td.className=cl;row.appendChild(td)}
function paint(payload){const rows=payload.rows,st=payload.stability;status.textContent=rows.length?payload.group.symbol+' · '+payload.group.window_days+' calendar days · '+payload.group.lookback_bars+'-bar lookback · '+payload.group.settings_key:'No matching scans.';
 $('count').textContent=st.runs;$('buy').textContent=pct(st.mean_buy_drop_pct);$('sell').textContent=pct(st.mean_sell_gain_pct);
 $('buy-std').textContent='Std. dev. '+st.stddev_buy_drop_pct.toFixed(2)+' percentage points · range '+pct(st.min_buy_drop_pct)+'–'+pct(st.max_buy_drop_pct);
 $('sell-std').textContent='Std. dev. '+st.stddev_sell_gain_pct.toFixed(2)+' percentage points · range '+pct(st.min_sell_gain_pct)+'–'+pct(st.max_sell_gain_pct);
 $('result').textContent=rows.length?signed(rows[0].test_return_pct):'—';$('result').className='value '+(rows.length?(rows[0].test_return_pct>=0?'pos':'neg'):'');
 $('result-meta').textContent=rows.length?rows[0].analysis_day+' · '+rows[0].test_trades+' completed trades':'No data yet';$('note').textContent=st.note;
 const ascending=rows.slice().reverse();chart('threshold-chart',ascending,[{key:'buy_drop_pct',label:'Buy drop',color:'#43d4cf'},{key:'sell_gain_pct',label:'Sell gain',color:'#ffbd65'}]);chart('return-chart',ascending,[{key:'test_return_pct',label:'Test return',color:'#43d4cf'},{key:'test_buy_hold_pct',label:'Buy & hold',color:'#ffbd65'}]);
 const tbody=$('history');tbody.replaceChildren();for(const f of rows){const tr=document.createElement('tr');cell(tr,f.analysis_day);cell(tr,pct(f.buy_drop_pct));cell(tr,pct(f.sell_gain_pct));cell(tr,signed(f.train_return_pct),f.train_return_pct>=0?'pos':'neg');cell(tr,signed(f.test_return_pct),f.test_return_pct>=0?'pos':'neg');cell(tr,signed(f.test_buy_hold_pct));cell(tr,pct(f.test_max_drawdown_pct));cell(tr,String(f.test_trades));cell(tr,pct(f.test_win_rate_pct));cell(tr,signed(f.portfolio_return_pct));tbody.appendChild(tr)}if(!rows.length){const tr=document.createElement('tr');const td=document.createElement('td');td.colSpan=10;td.textContent='No results for this filter.';tr.appendChild(td);tbody.appendChild(tr)}
}
async function load(){const i=Number(sel.value),g=groups[i];if(!g)return;const q=new URLSearchParams({pair:g.pair,symbol:g.symbol,days:g.window_days,lookback:g.lookback_bars,settings:g.settings_key});const response=await fetch('/api/history?'+q);if(!response.ok)throw new Error('Unable to load history ('+response.status+')');paint(await response.json())}
async function init(){try{const response=await fetch('/api/groups');if(!response.ok)throw Error('Unable to read SQLite groups');groups=await response.json();sel.replaceChildren();if(!groups.length){const option=document.createElement('option');option.textContent='No runs recorded — run cryptoscan first';sel.appendChild(option);status.textContent='Run: go run ./cmd/cryptoscan --pair SOXL:SOXS --days 180';return}groups.forEach((g,i)=>{const o=document.createElement('option');o.value=String(i);o.textContent=g.pair+' · '+g.symbol+' · '+g.window_days+'d · '+g.lookback_bars+' bars · '+g.runs+' scans';sel.appendChild(o)});await load()}catch(e){status.textContent=String(e)}}
function cleanTime(raw){if(!raw)return '—';let d=new Date(raw);return Number.isFinite(d.valueOf())?d.toISOString().replace('T',' ').slice(0,16):raw}
async function loadLive(){
 try{
  const [a,b]=await Promise.all([fetch('/api/live'),fetch('/api/signals?limit=100')]);if(!a.ok||!b.ok)throw Error('Live endpoints unavailable');
  const states=await a.json(),signals=await b.json();
  $('live-updated').textContent='Checked '+new Date().toISOString().replace('T',' ').slice(0,19)+' UTC · '+states.length+' configured symbols · Refreshes every 30 seconds';
  const out=$('live-rows');out.replaceChildren();for(const s of states){
   const tr=document.createElement('tr');cell(tr,s.pair+' / '+s.symbol);cell(tr,s.side);cell(tr,pct(s.drop_pct));cell(tr,pct(s.gain_pct));
   cell(tr,s.last_price?s.last_price.toFixed(2):'—');cell(tr,cleanTime(s.last_bar_at));cell(tr,s.recent_peak?pct(s.last_drop_pct):'—');
   cell(tr,s.position);cell(tr,s.position==='OPEN'?'$'+s.entry_price.toFixed(2)+' / $'+(s.entry_price*(1+s.gain_pct/100)).toFixed(2):'—');
   cell(tr,s.status+(s.error?' · '+s.error:''));cell(tr,s.evaluation_day||'—');out.appendChild(tr);
  }if(!states.length){const tr=document.createElement('tr');cell(tr,'No rules yet. The service evaluates available data at startup and 00:00 UTC.');out.appendChild(tr)}
  const logs=$('signals-rows');logs.replaceChildren();for(const s of signals){
   const tr=document.createElement('tr');cell(tr,cleanTime(s.detected_at));cell(tr,s.pair+' / '+s.symbol);cell(tr,s.side);cell(tr,s.action,s.action==='BUY'?'pos':'neg');
   cell(tr,cleanTime(s.bar_closed_at));cell(tr,'$'+s.price.toFixed(2));cell(tr,pct(s.drop_pct));cell(tr,pct(s.gain_pct));cell(tr,s.action==='SELL'?signed(s.return_pct):'—',s.return_pct>=0?'pos':'neg');logs.appendChild(tr);
  }if(!signals.length){const tr=document.createElement('tr');cell(tr,'No BUY or SELL signals yet.');logs.appendChild(tr)}
 }catch(e){$('live-updated').textContent=String(e)}
}
loadLive();setInterval(loadLive,30000);
sel.addEventListener('change',()=>load().catch(e=>status.textContent=String(e)));
$('refresh').addEventListener('click',()=>init());init();
})();
</script></body></html>`
