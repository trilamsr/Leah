package regattaclient

import (
	"context"
	"errors"
	"testing"
)

type fakeAttestor struct {
	calls []string
	err   error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.calls = append(f.calls, scope)
	return f.err
}

type fakeAuditSink struct {
	rows []AuditRow
}

func (f *fakeAuditSink) Record(row AuditRow) { f.rows = append(f.rows, row) }

type fakeInner struct {
	shipCalls   int
	reviewCalls int
	statusCalls int
	shipErr     error
	reviewErr   error
	statusErr   error
	shipResp    ShipResponse
	reviewResp  ReviewResponse
	statusResp  Status
}

func (f *fakeInner) Ship(_ context.Context, _ ShipRequest) (ShipResponse, error) {
	f.shipCalls++
	return f.shipResp, f.shipErr
}

func (f *fakeInner) Review(_ context.Context, _ ReviewRequest) (ReviewResponse, error) {
	f.reviewCalls++
	return f.reviewResp, f.reviewErr
}

func (f *fakeInner) Status(_ context.Context) (Status, error) {
	f.statusCalls++
	return f.statusResp, f.statusErr
}

func TestGatedShip_AttestationDenied_NoCall(t *testing.T) {
	inner := &fakeInner{}
	att := &fakeAttestor{err: errors.New("denied")}
	audit := &fakeAuditSink{}
	g := NewGated(inner, att, audit)

	if _, err := g.Ship(context.Background(), ShipRequest{Branch: "feat/x"}); err == nil {
		t.Fatal("want error on denial")
	}
	if inner.shipCalls != 0 {
		t.Errorf("inner.Ship invoked despite denial: calls=%d", inner.shipCalls)
	}
	if len(audit.rows) != 0 {
		t.Errorf("audit row emitted on denial: %+v", audit.rows)
	}
	if len(att.calls) != 1 || att.calls[0] != ScopeShip {
		t.Errorf("scope mismatch: %v", att.calls)
	}
}

func TestGatedShip_HappyPath_AuditRowEmitted(t *testing.T) {
	inner := &fakeInner{shipResp: ShipResponse{AgentID: "a1"}}
	att := &fakeAttestor{}
	audit := &fakeAuditSink{}
	g := NewGated(inner, att, audit)

	resp, err := g.Ship(context.Background(), ShipRequest{Branch: "feat/x"})
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if resp.AgentID != "a1" {
		t.Errorf("resp.AgentID: %q", resp.AgentID)
	}
	if inner.shipCalls != 1 {
		t.Errorf("inner.Ship calls: %d", inner.shipCalls)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("want 1 audit row, got %d", len(audit.rows))
	}
	if audit.rows[0].Kind != "regatta_ship" || !audit.rows[0].Success {
		t.Errorf("audit row: %+v", audit.rows[0])
	}
}

func TestGatedReview_AttestationDenied_NoCall(t *testing.T) {
	inner := &fakeInner{}
	att := &fakeAttestor{err: errors.New("denied")}
	audit := &fakeAuditSink{}
	g := NewGated(inner, att, audit)

	if _, err := g.Review(context.Background(), ReviewRequest{AgentID: "a1"}); err == nil {
		t.Fatal("want error on denial")
	}
	if inner.reviewCalls != 0 {
		t.Errorf("inner.Review invoked despite denial: calls=%d", inner.reviewCalls)
	}
	if len(audit.rows) != 0 {
		t.Errorf("audit row emitted on denial: %+v", audit.rows)
	}
	if len(att.calls) != 1 || att.calls[0] != ScopeReview {
		t.Errorf("scope mismatch: %v", att.calls)
	}
}

func TestGatedStatus_NoAttestationRequired(t *testing.T) {
	inner := &fakeInner{statusResp: Status{Healthy: true, Mode: "docker"}}
	att := &fakeAttestor{}
	audit := &fakeAuditSink{}
	g := NewGated(inner, att, audit)

	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Healthy || st.Mode != "docker" {
		t.Errorf("status: %+v", st)
	}
	if len(att.calls) != 0 {
		t.Errorf("Status MUST NOT attest, got: %v", att.calls)
	}
	if inner.statusCalls != 1 {
		t.Errorf("inner.Status calls: %d", inner.statusCalls)
	}
	if len(audit.rows) != 0 {
		t.Errorf("Status MUST NOT emit audit, got: %+v", audit.rows)
	}
}

func TestGatedShip_InnerFailure_AuditSuccessFalse(t *testing.T) {
	inner := &fakeInner{shipErr: errors.New("upstream down")}
	att := &fakeAttestor{}
	audit := &fakeAuditSink{}
	g := NewGated(inner, att, audit)

	if _, err := g.Ship(context.Background(), ShipRequest{Branch: "feat/x"}); err == nil {
		t.Fatal("want error from inner")
	}
	if len(audit.rows) != 1 {
		t.Fatalf("want 1 audit row, got %d", len(audit.rows))
	}
	if audit.rows[0].Kind != "regatta_ship" || audit.rows[0].Success {
		t.Errorf("audit row should mark failure: %+v", audit.rows[0])
	}
}
