package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestYahooFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/SOXL") || r.URL.Query().Get("interval") != "1h" || r.URL.Query().Get("includePrePost") != "false" {
			t.Errorf("invalid request %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"chart":{"result":[{"timestamp":[1000,4600,8200],"indicators":{"quote":[{"open":[10,11,null],"high":[11,12,null],"low":[9,10,null],"close":[10.5,11.5,null],"volume":[100,200,null]}]}}],"error":null}}`))
	}))
	defer srv.Close()
	bars, err := (Yahoo{BaseURL: srv.URL}).Fetch(context.Background(), "soxl", time.Unix(900, 0), time.Unix(9000, 0))
	if err != nil || len(bars) != 2 || bars[1].Close != 11.5 {
		t.Fatalf("bars=%+v err=%v", bars, err)
	}
}
func TestYahooError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "blocked", 429) }))
	defer srv.Close()
	_, err := (Yahoo{BaseURL: srv.URL}).Fetch(context.Background(), "SOXS", time.Unix(1, 0), time.Unix(200, 0))
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
}
