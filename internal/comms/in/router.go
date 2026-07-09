package in

import (
	"context"
	"time"
)

type Reply struct {
	Channel  string
	PeerID   string
	ConvID   string
	Text     string
	Voice    []byte
	Received time.Time
}

type Intent int

const (
	IntentUnknown Intent = iota
	IntentAccept
	IntentReject
	IntentDefer
)

// ConsentGate is the load-bearing safety surface (spec §4). PR-1 depends on
// the interface only; the concrete two-layer gate lands in PR-2.
type ConsentGate interface {
	Enrolled(channel, peerID string) (bool, error)
	Authorize(ctx context.Context, p Pending, intent Intent) error
}

// Classifier bridges normalized text to an Intent (spec §5). Concrete regex +
// LLM-fallback implementation lands in PR-3.
type Classifier interface {
	Classify(ctx context.Context, text string) (Intent, error)
}

// Engine is the recommendation surface the router needs. The router stays
// decoupled from internal/recommend so PR-1 has no cross-package coupling;
// PR-4 supplies an adapter that resolves the RecID to a Recommendation.
type Engine interface {
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
	Apply(ctx context.Context, id string) error
}

type AuditRow struct {
	Channel string
	PeerID  string
	ConvID  string
	RecID   string
	Intent  Intent
	Outcome string // "applied" | "rejected" | "deferred" | "unknown" | "no_pending" | "peer_mismatch" | "unenrolled" | "attest_denied"
	At      time.Time
}

type Router struct {
	Pending  PendingStore
	Consent  ConsentGate
	Classify Classifier
	Engine   Engine
	Audit    func(AuditRow)
}

// Handle implements the spec §4.3 sequence. Drops are silent (return nil) and
// audited; only an active attestation denial or downstream engine error
// surfaces as a non-nil error.
func (r *Router) Handle(ctx context.Context, reply Reply) error {
	// Look up pending non-destructively first so a peer-mismatch / un-enrolled
	// reply does not consume the entry (the real operator's reply must still
	// land). Take only fires once both binding checks clear.
	p, ok := r.peekPending(reply.Channel, reply.ConvID)
	if !ok {
		r.audit(AuditRow{Channel: reply.Channel, PeerID: reply.PeerID, ConvID: reply.ConvID, Outcome: "no_pending"})
		return nil
	}
	if reply.PeerID != p.PeerID {
		r.audit(AuditRow{Channel: reply.Channel, PeerID: reply.PeerID, ConvID: reply.ConvID, RecID: p.RecID, Outcome: "peer_mismatch"})
		return nil
	}
	enrolled, err := r.Consent.Enrolled(reply.Channel, reply.PeerID)
	if err != nil || !enrolled {
		r.audit(AuditRow{Channel: reply.Channel, PeerID: reply.PeerID, ConvID: reply.ConvID, RecID: p.RecID, Outcome: "unenrolled"})
		return nil
	}

	// Bindings cleared — consume pending now (single-use).
	taken, ok := r.Pending.Take(reply.Channel, reply.ConvID)
	if !ok {
		r.audit(AuditRow{Channel: reply.Channel, PeerID: reply.PeerID, ConvID: reply.ConvID, Outcome: "no_pending"})
		return nil
	}
	p = taken

	intent, _ := r.Classify.Classify(ctx, reply.Text)
	row := AuditRow{Channel: reply.Channel, PeerID: reply.PeerID, ConvID: reply.ConvID, RecID: p.RecID, Intent: intent}

	switch intent {
	case IntentReject:
		row.Outcome = "rejected"
		err := r.Engine.Reject(ctx, p.RecID)
		r.audit(row)
		return err
	case IntentDefer:
		row.Outcome = "deferred"
		r.audit(row)
		return nil
	case IntentAccept:
		if err := r.Consent.Authorize(ctx, p, intent); err != nil {
			row.Outcome = "attest_denied"
			r.audit(row)
			return err
		}
		if err := r.Engine.Accept(ctx, p.RecID); err != nil {
			row.Outcome = "accept_failed"
			r.audit(row)
			return err
		}
		if err := r.Engine.Apply(ctx, p.RecID); err != nil {
			row.Outcome = "apply_failed"
			r.audit(row)
			return err
		}
		row.Outcome = "applied"
		r.audit(row)
		return nil
	default:
		row.Outcome = "unknown"
		r.audit(row)
		return nil
	}
}

// peekPending returns the entry without consuming it. Used for binding checks
// that must not consume on failure — only a fully-cleared reply takes.
func (r *Router) peekPending(channel, convID string) (Pending, bool) {
	if peeker, ok := r.Pending.(interface {
		Peek(channel, convID string) (Pending, bool)
	}); ok {
		return peeker.Peek(channel, convID)
	}
	// Fallback for stores that don't implement Peek: Take then Put back. Not
	// concurrency-safe under contention; the in-memory store provides Peek.
	p, ok := r.Pending.Take(channel, convID)
	if ok {
		_ = r.Pending.Put(p)
	}
	return p, ok
}

func (r *Router) audit(row AuditRow) {
	if r.Audit == nil {
		return
	}
	if row.At.IsZero() {
		row.At = time.Now()
	}
	r.Audit(row)
}
