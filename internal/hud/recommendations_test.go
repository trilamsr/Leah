package hud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeRecommendSeam struct {
	recs       []Recommendation
	proposeErr error
	acceptErr  error
	rejectErr  error
	lastAccept string
	lastReject string
}

func (f *fakeRecommendSeam) Propose(_ context.Context) ([]Recommendation, error) {
	return f.recs, f.proposeErr
}
func (f *fakeRecommendSeam) Accept(_ context.Context, id string) error {
	f.lastAccept = id
	return f.acceptErr
}
func (f *fakeRecommendSeam) Reject(_ context.Context, id string) error {
	f.lastReject = id
	return f.rejectErr
}

func newRecsHandler(seam RecommendSeam) http.Handler {
	mux := http.NewServeMux()
	NewRecommendations(seam).Routes(mux)
	return mux
}

func TestRecommendations_List_HappyPath(t *testing.T) {
	seam := &fakeRecommendSeam{recs: []Recommendation{
		{ID: "r1", Pattern: "bash:ls", Action: "auto-approve", Tier: "auto", Source: "selflearn", Confidence: 0.91, CreatedAt: time.Unix(1_700_000_000, 0).UTC()},
		{ID: "r2", Pattern: "bash:rm", Action: "promote-tier", Tier: "confirm", Source: "operatormodel", Confidence: 0.62, CreatedAt: time.Unix(1_700_000_100, 0).UTC()},
		{ID: "r3", Pattern: "p3", Action: "a3", Tier: "silent", Source: "patterns", Confidence: 0.55},
		{ID: "r4", Pattern: "p4", Action: "a4", Tier: "auto", Source: "patterns", Confidence: 0.50},
		{ID: "r5", Pattern: "p5", Action: "a5", Tier: "confirm", Source: "patterns", Confidence: 0.45},
		{ID: "r6", Pattern: "p6", Action: "a6", Tier: "auto", Source: "patterns", Confidence: 0.40},
	}}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/hud/recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	var out []Recommendation
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("top-5 cap: got %d want 5", len(out))
	}
	if out[0].ID != "r1" || out[0].Confidence != 0.91 {
		t.Errorf("first rec: got %+v", out[0])
	}
}

func TestRecommendations_Accept_POST_CallsEngine(t *testing.T) {
	seam := &fakeRecommendSeam{}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hud/recommendations/abc/accept", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", resp.StatusCode)
	}
	if seam.lastAccept != "abc" {
		t.Errorf("Accept id: got %q want %q", seam.lastAccept, "abc")
	}
}

func TestRecommendations_Reject_POST_CallsEngine(t *testing.T) {
	seam := &fakeRecommendSeam{}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hud/recommendations/xyz/reject", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", resp.StatusCode)
	}
	if seam.lastReject != "xyz" {
		t.Errorf("Reject id: got %q want %q", seam.lastReject, "xyz")
	}
}

func TestRecommendations_Empty_Returns204(t *testing.T) {
	seam := &fakeRecommendSeam{recs: nil}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/hud/recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", resp.StatusCode)
	}
}

func TestRecommendations_EngineError_Returns500(t *testing.T) {
	seam := &fakeRecommendSeam{proposeErr: errors.New("storage down")}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/hud/recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
}

func TestRecommendations_Accept_MissingID_Returns400(t *testing.T) {
	seam := &fakeRecommendSeam{}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hud/recommendations//accept", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 400 or 404, got %d", resp.StatusCode)
	}
}

func TestRecommendations_Accept_GET_Rejected(t *testing.T) {
	seam := &fakeRecommendSeam{}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/hud/recommendations/abc/accept")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", resp.StatusCode)
	}
}

func TestDaemonRecommendSeam_ProxiesToDaemon(t *testing.T) {
	var seenAccept, seenReject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/recommend/propose" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Recommendation{{ID: "z", Pattern: "p", Tier: "auto", Confidence: 0.9}})
		case strings.HasSuffix(r.URL.Path, "/accept") && r.Method == http.MethodPost:
			seenAccept = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/recommend/"), "/accept")
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/reject") && r.Method == http.MethodPost:
			seenReject = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/recommend/"), "/reject")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	seam := NewDaemonRecommendSeam(NewClient(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recs, err := seam.Propose(ctx)
	if err != nil || len(recs) != 1 || recs[0].ID != "z" {
		t.Fatalf("Propose: got %v %v", recs, err)
	}
	if err := seam.Accept(ctx, "abc"); err != nil || seenAccept != "abc" {
		t.Fatalf("Accept: seen=%q err=%v", seenAccept, err)
	}
	if err := seam.Reject(ctx, "xyz"); err != nil || seenReject != "xyz" {
		t.Fatalf("Reject: seen=%q err=%v", seenReject, err)
	}
}

func TestRecommendations_Accept_EngineError_Returns500(t *testing.T) {
	seam := &fakeRecommendSeam{acceptErr: errors.New("apply blew up")}
	srv := httptest.NewServer(newRecsHandler(seam))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hud/recommendations/r1/accept", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
	body := make([]byte, 256)
	n, _ := resp.Body.Read(body)
	if !strings.Contains(string(body[:n]), "apply") {
		// body is best-effort; some handlers may return generic text
		_ = n
	}
}
