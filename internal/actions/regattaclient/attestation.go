package regattaclient

import (
	"context"

	"github.com/trilam/leah/internal/platform/contracts"
)

// Scope strings the GatedClient passes to the operator-attestation prompt.
// Status is intentionally NOT gated — read-only.
const (
	ScopeShip   = "regatta:ship"
	ScopeReview = "regatta:review"
)

// ShipRequest / ShipResponse / ReviewRequest / ReviewResponse / Status are the
// structural surface the gated wrapper exposes. Concrete fields are filled by
// the transport implementer; the seam only needs the shape.
type ShipRequest struct {
	Branch string
}

type ShipResponse struct {
	AgentID string
}

type ReviewRequest struct {
	AgentID string
}

type ReviewResponse struct {
	Approved bool
}

type Status struct {
	Mode      string
	Healthy   bool
	Version   string
	LatencyMS int64
}

// ClientInterface is the structural seam the GatedClient wraps. Production wires
// the real regatta-backed client; tests inject a fake. Keeps GatedClient
// transport-agnostic so cloud/docker/socket modes plug in unchanged.
type ClientInterface interface {
	Ship(ctx context.Context, req ShipRequest) (ShipResponse, error)
	Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error)
	Status(ctx context.Context) (Status, error)
}

// AuditRow is the local audit projection. Kept narrow on purpose: no payload
// bytes — only kind + success, per spec, so failure modes never leak operator
// branches or review text into the audit log.
type AuditRow struct {
	Kind    string
	Success bool
}

// AuditSink is the seam the gated client writes through. Production wires
// audit.Logger; tests inject a recorder.
type AuditSink interface {
	Record(row AuditRow)
}

// GatedClient gates regatta write ops behind operator attestation and records
// one audit row per attempted call. Status passes through unattested — it is
// the daemon's 30s health probe and must not require operator consent.
type GatedClient struct {
	inner    ClientInterface
	attestor contracts.Attestor
	audit    AuditSink
}

// NewGated returns a GatedClient. All three deps are required — nil at the seam
// is a programmer error caught by the type system at the call site.
func NewGated(inner ClientInterface, attestor contracts.Attestor, audit AuditSink) *GatedClient {
	return &GatedClient{inner: inner, attestor: attestor, audit: audit}
}

// Ship gates the write op behind ScopeShip. Denial short-circuits before the
// inner call AND before audit emission — denied attestations are the attestor's
// row to log, not ours.
func (g *GatedClient) Ship(ctx context.Context, req ShipRequest) (ShipResponse, error) {
	if err := g.attestor.Attest(ctx, ScopeShip); err != nil {
		return ShipResponse{}, err
	}
	resp, err := g.inner.Ship(ctx, req)
	g.audit.Record(AuditRow{Kind: "regatta_ship", Success: err == nil})
	return resp, err
}

// Review gates the write op behind ScopeReview. Same denial semantics as Ship.
func (g *GatedClient) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	if err := g.attestor.Attest(ctx, ScopeReview); err != nil {
		return ReviewResponse{}, err
	}
	resp, err := g.inner.Review(ctx, req)
	g.audit.Record(AuditRow{Kind: "regatta_review", Success: err == nil})
	return resp, err
}

// Status is the read-only probe — no attestation, no audit row. Called every
// 30s by the daemon and must stay cheap.
func (g *GatedClient) Status(ctx context.Context) (Status, error) {
	return g.inner.Status(ctx)
}
