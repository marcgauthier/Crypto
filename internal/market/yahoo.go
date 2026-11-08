package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Candle is a regular-session hourly OHLCV bar; Time is the start of the bar (UTC).
type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

type Provider interface {
	Fetch(context.Context, string, time.Time, time.Time) ([]Candle, error)
}

type Yahoo struct {
	Client  *http.Client
	BaseURL string
}

func (y Yahoo) Fetch(ctx context.Context, symbol string, start, end time.Time) ([]Candle, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || !start.Before(end) {
		return nil, errors.New("invalid symbol or time range")
	}
	base := y.BaseURL
	if base == "" {
		base = "https://query1.finance.yahoo.com"
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + "/v8/finance/chart/" + url.PathEscape(symbol))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("period1", fmt.Sprint(start.Unix()))
	q.Set("period2", fmt.Sprint(end.Unix()))
	q.Set("interval", "1h")
	q.Set("includePrePost", "false")
	q.Set("events", "div,splits")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CryptoResearch/1.0)")
	req.Header.Set("Accept", "application/json")
	client := y.Client
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Yahoo request %s: %w", symbol, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yahoo %s returned HTTP %d; provider may be rate-limiting or denying access", symbol, res.StatusCode)
	}
	var data struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*float64 `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 12<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode Yahoo %s: %w", symbol, err)
	}
	if data.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo %s: %s: %s", symbol, data.Chart.Error.Code, data.Chart.Error.Description)
	}
	if len(data.Chart.Result) != 1 || len(data.Chart.Result[0].Indicators.Quote) != 1 {
		return nil, fmt.Errorf("Yahoo %s returned no valid chart result", symbol)
	}
	result := data.Chart.Result[0]
	quote := result.Indicators.Quote[0]
	candles := make([]Candle, 0, len(result.Timestamp))
	for i, ts := range result.Timestamp {
		if i >= len(quote.Open) || i >= len(quote.High) || i >= len(quote.Low) || i >= len(quote.Close) {
			continue
		}
		a, b, c, d := quote.Open[i], quote.High[i], quote.Low[i], quote.Close[i]
		if a == nil || b == nil || c == nil || d == nil {
			continue
		}
		t := time.Unix(ts, 0).UTC()
		if t.Before(start) || !t.Before(end) {
			continue
		}
		v := 0.0
		if i < len(quote.Volume) && quote.Volume[i] != nil {
			v = *quote.Volume[i]
		}
		item := Candle{Time: t, Open: *a, High: *b, Low: *c, Close: *d, Volume: v}
		if !Valid(item) {
			continue
		}
		candles = append(candles, item)
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].Time.Before(candles[j].Time) })
	unique := candles[:0]
	for _, c := range candles {
		if len(unique) > 0 && unique[len(unique)-1].Time.Equal(c.Time) {
			unique[len(unique)-1] = c
		} else {
			unique = append(unique, c)
		}
	}
	return unique, nil
}

func Valid(c Candle) bool {
	values := []float64{c.Open, c.High, c.Low, c.Close, c.Volume}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return false
		}
	}
	return c.Open > 0 && c.High >= c.Open && c.High >= c.Close && c.Low <= c.Open && c.Low <= c.Close && c.Low > 0 && c.Volume >= 0
}
