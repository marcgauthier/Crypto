// Package dashboard serves a read-only, dependency-free localhost dashboard.
// Its charts use native SVG, so the website works without internet/CDNs.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/marcgauthier/Crypto/internal/findings"
)

type Handler struct{ db *findings.Store }

func New(db *findings.Store) http.Handler {
	h := &Handler{db: db}
	m := http.NewServeMux()
	m.HandleFunc("GET /", h.index)
	m.HandleFunc("GET /api/groups", h.groups)
	m.HandleFunc("GET /api/history", h.history)
	return m
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'")
	fmt.Fprint(w, page)
}
func jsonResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.db.Groups(r.Context())
	if err != nil {
		http.Error(w, "cannot load groups", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, groups)
}
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days, err := strconv.Atoi(q.Get("days"))
	if err != nil || days < 1 || days > 36500 {
		http.Error(w, "invalid days", http.StatusBadRequest)
		return
	}
	lookback, err := strconv.Atoi(q.Get("lookback"))
	if err != nil || lookback < 2 || lookback > 100000 {
		http.Error(w, "invalid lookback", http.StatusBadRequest)
		return
	}
	group := findings.Group{Pair: q.Get("pair"), Symbol: q.Get("symbol"), WindowDays: days, LookbackBars: lookback, SettingsKey: q.Get("settings")}
	rows, err := h.db.History(r.Context(), group, 2000)
	if err != nil {
		http.Error(w, "cannot load history: "+err.Error(), http.StatusBadRequest)
		return
	}
	jsonResponse(w, struct {
		Group     findings.Group     `json:"group"`
		Rows      []findings.Finding `json:"rows"`
		Stability findings.Stability `json:"stability"`
	}{group, rows, findings.Summarize(rows)})
}
