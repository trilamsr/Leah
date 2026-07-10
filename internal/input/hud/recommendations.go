package hud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RecommendSeam is the structural seam over internal/recommend.Engine — the
// HUD widget never imports recommend directly so the daemon owns the singleton.
type RecommendSeam interface {
	Propose(ctx context.Context) ([]Recommendation, error)
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
}

// Recommendation mirrors recommend.Recommendation minus the Action closure —
// only the fields the operator surface renders cross the seam.
type Recommendation struct {
	ID         string    `json:"id"`
	Pattern    string    `json:"pattern"`
	Action     string    `json:"action"`
	Tier       string    `json:"tier"`
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

// recsTopN caps the rendered card list so a runaway engine never floods the panel.
const recsTopN = 5

type Recommendations struct {
	Seam RecommendSeam
}

func NewRecommendations(seam RecommendSeam) *Recommendations {
	return &Recommendations{Seam: seam}
}

func (h *Recommendations) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/hud/recommendations", h.handleList)
	mux.HandleFunc("/hud/recommendations/", h.handleAction)
}

func (h *Recommendations) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	recs, err := h.Seam.Propose(r.Context())
	if err != nil {
		http.Error(w, "propose failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(recs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(recs) > recsTopN {
		recs = recs[:recsTopN]
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(recs)
}

func (h *Recommendations) handleAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/hud/recommendations/")
	id, verb, ok := strings.Cut(rest, "/")
	if !ok || id == "" || verb == "" {
		http.Error(w, "expected /hud/recommendations/<id>/{accept|reject}", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var err error
	switch verb {
	case "accept":
		err = h.Seam.Accept(r.Context(), id)
	case "reject":
		err = h.Seam.Reject(r.Context(), id)
	default:
		http.Error(w, "unknown verb: "+verb, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, verb+" failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// daemonRecommendSeam proxies HUD calls to the daemon — keeps the recommend
// singleton owned by leah-daemon while the HUD process stays a thin renderer.
type daemonRecommendSeam struct{ c *Client }

func NewDaemonRecommendSeam(c *Client) RecommendSeam { return &daemonRecommendSeam{c: c} }

func (d *daemonRecommendSeam) Propose(ctx context.Context) ([]Recommendation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.c.BaseURL+"/recommend/propose", nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hud: /recommend/propose status %d", resp.StatusCode)
	}
	var out []Recommendation
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *daemonRecommendSeam) Accept(ctx context.Context, id string) error {
	return d.post(ctx, id, "accept")
}
func (d *daemonRecommendSeam) Reject(ctx context.Context, id string) error {
	return d.post(ctx, id, "reject")
}
func (d *daemonRecommendSeam) post(ctx context.Context, id, verb string) error {
	url := d.c.BaseURL + "/recommend/" + id + "/" + verb
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := d.c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("hud: %s %s status %d", verb, id, resp.StatusCode)
	}
	return nil
}
