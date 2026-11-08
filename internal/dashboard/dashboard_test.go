package dashboard

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marcgauthier/Crypto/internal/findings"
)

func TestDashboardAndAPI(t *testing.T) {
	db, err := findings.Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	end := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	if err := db.Save(context.Background(), []findings.Finding{{
		Pair: "MSTR:MSTZ", Symbol: "MSTR", Side: "bullish", ScannedAt: end, WindowDays: 180, WindowStart: end.AddDate(0, 0, -180), WindowEnd: end,
		LookbackBars: 24, SettingsKey: "grid-test", BuyDropPct: 4, SellGainPct: 7, TestReturnPct: 9, TestTrades: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	for _, tc := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/", 200, "Strategy consistency"},
		{"/api/groups", 200, "MSTR:MSTZ"},
		{"/api/history?pair=MSTR:MSTZ&symbol=MSTR&days=180&lookback=24&settings=grid-test", 200, "buy_drop_pct"},
		{"/api/history?pair=MSTR:MSTZ&symbol=MSTR&days=foo&lookback=24&settings=grid-test", 400, "invalid days"},
		{"/bogus", 404, "404"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Errorf("%s: status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/groups", nil))
	var groups []findings.Group
	if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil || len(groups) != 1 || groups[0].Runs != 1 {
		t.Fatalf("groups %v %v", groups, err)
	}
}
