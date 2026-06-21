package inbound

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeGate records every call so tests can assert which layer fired.
type fakeGate struct {
	enrolled     map[string]bool // "channel|peer" -> true
	enrolledErr  error
	authorizeErr error
	enrolledCalls,
	authorizeCalls int
	lastAuthIntent Intent
	lastAuthPeer   string
}

func (g *fakeGate) Enrolled(channel, peer string) (bool, error) {
	g.enrolledCalls++
	if g.enrolledErr != nil {
		return false, g.enrolledErr
	}
	return g.enrolled[channel+"|"+peer], nil
}
func (g *fakeGate) Authorize(ctx context.Context, p Pending, intent Intent) error {
	g.authorizeCalls++
	g.lastAuthIntent = intent
	g.lastAuthPeer = p.PeerID
	return g.authorizeErr
}

type fakeClassifier struct {
	out Intent
	err error
}

func (c *fakeClassifier) Classify(ctx context.Context, text string) (Intent, error) {
	return c.out, c.err
}

type fakeEngine struct {
	acceptID, rejectID, applyID string
	acceptCalls, rejectCalls, applyCalls int
	acceptErr, applyErr error
}

func (e *fakeEngine) Accept(ctx context.Context, id string) error {
	e.acceptCalls++
	e.acceptID = id
	return e.acceptErr
}
func (e *fakeEngine) Reject(ctx context.Context, id string) error {
	e.rejectCalls++
	e.rejectID = id
	return nil
}
func (e *fakeEngine) Apply(ctx context.Context, id string) error {
	e.applyCalls++
	e.applyID = id
	return e.applyErr
}

func newRouter(gate *fakeGate, cls *fakeClassifier, eng *fakeEngine, store PendingStore, audit *[]AuditRow) *Router {
	return &Router{
		Pending:  store,
		Consent:  gate,
		Classify: cls,
		Engine:   eng,
		Audit: func(row AuditRow) {
			*audit = append(*audit, row)
		},
	}
}

func mkReply(text string) Reply {
	return Reply{Channel: "discord", PeerID: "u1", ConvID: "c1", Text: text, Received: time.Now()}
}

func seedPending(t *testing.T, s PendingStore) {
	t.Helper()
	if err := s.Put(Pending{RecID: "rec-1", Channel: "discord", ConvID: "c1", PeerID: "u1", SentAt: time.Now()}); err != nil {
		t.Fatalf("seed Put: %v", err)
	}
}

// Reject path does NOT clear the per-action gate — declining is always safe (spec §4.3).
func TestRouter_RejectSkipsAuthorize(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentReject}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)

	if err := r.Handle(context.Background(), mkReply("no")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 0 {
		t.Fatalf("Authorize called on reject: %d", gate.authorizeCalls)
	}
	if eng.rejectCalls != 1 || eng.rejectID != "rec-1" {
		t.Fatalf("Reject not invoked: calls=%d id=%q", eng.rejectCalls, eng.rejectID)
	}
	if eng.applyCalls != 0 {
		t.Fatalf("Apply called on reject: %d", eng.applyCalls)
	}
}

// Defer path: no Authorize, no Engine mutation. Pending was consumed (single-use),
// but spec §5 says defer "does not delete the pending rec" — that re-Put is the
// classifier's job in PR-3; PR-1 router just routes the intent.
func TestRouter_DeferSkipsAuthorizeAndApply(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentDefer}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("later")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 0 || eng.applyCalls != 0 || eng.acceptCalls != 0 {
		t.Fatalf("defer should not gate/apply/accept: auth=%d apply=%d accept=%d",
			gate.authorizeCalls, eng.applyCalls, eng.acceptCalls)
	}
}

// Unknown intent: no Authorize, no mutation. Audit records the unknown branch.
func TestRouter_UnknownSkipsAuthorizeAndAudits(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentUnknown}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("zzz")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 0 || eng.applyCalls+eng.acceptCalls+eng.rejectCalls != 0 {
		t.Fatalf("unknown should not gate/mutate")
	}
	if len(audit) == 0 || audit[len(audit)-1].Outcome != "unknown" {
		t.Fatalf("audit outcome=%v, want unknown", audit)
	}
}

// Accept path: Authorize fires before Engine.Accept; Apply only on success.
func TestRouter_AcceptClearsGateThenAcceptsAndApplies(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentAccept}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("yes")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 1 || gate.lastAuthIntent != IntentAccept {
		t.Fatalf("Authorize: calls=%d intent=%v", gate.authorizeCalls, gate.lastAuthIntent)
	}
	if eng.acceptCalls != 1 || eng.applyCalls != 1 || eng.acceptID != "rec-1" || eng.applyID != "rec-1" {
		t.Fatalf("accept/apply not invoked: accept=%d apply=%d ids=%q/%q",
			eng.acceptCalls, eng.applyCalls, eng.acceptID, eng.applyID)
	}
}

// Denied per-action attestation leaves the rec unapplied (spec §7 load-bearing test).
func TestRouter_AcceptDeniedAttestLeavesUnapplied(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}, authorizeErr: errors.New("denied")}
	cls := &fakeClassifier{out: IntentAccept}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("yes")); err == nil {
		t.Fatalf("Handle: want error on denied attest")
	}
	if eng.acceptCalls != 0 || eng.applyCalls != 0 {
		t.Fatalf("denied attest must not Accept/Apply: accept=%d apply=%d", eng.acceptCalls, eng.applyCalls)
	}
	if len(audit) == 0 || audit[len(audit)-1].Outcome != "attest_denied" {
		t.Fatalf("audit outcome=%v, want attest_denied", audit)
	}
}

// Addressee binding: PeerID != Pending.PeerID is dropped before Enrolled (spec §3.2).
func TestRouter_PeerIDMismatchDroppedBeforeGate(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentAccept}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	bad := mkReply("yes")
	bad.PeerID = "u2" // different peer
	if err := r.Handle(context.Background(), bad); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.enrolledCalls != 0 {
		t.Fatalf("Enrolled called on peer mismatch: %d", gate.enrolledCalls)
	}
	// Pending stays untaken — a wrong-peer reply must not consume the pending entry.
	if _, ok := store.Take("discord", "c1"); !ok {
		t.Fatalf("pending consumed by wrong-peer reply (should remain)")
	}
}

// Un-enrolled (channel, peer) drops before Take/attest (fail-closed, spec §4.2 layer 1).
func TestRouter_UnenrolledDroppedBeforeTake(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{}} // not enrolled
	cls := &fakeClassifier{out: IntentAccept}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("yes")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 0 {
		t.Fatalf("Authorize called on un-enrolled: %d", gate.authorizeCalls)
	}
	if _, ok := store.Take("discord", "c1"); !ok {
		t.Fatalf("pending consumed by un-enrolled reply (should remain)")
	}
}

// No pending rec for (channel, convID): drop before Authorize ("no pending" per §4.3).
func TestRouter_NoPendingDrops(t *testing.T) {
	store := NewMemoryPendingStore()
	// no seed
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentAccept}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("yes")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gate.authorizeCalls != 0 || eng.acceptCalls != 0 || eng.applyCalls != 0 {
		t.Fatalf("no-pending should not gate/mutate")
	}
	if len(audit) == 0 || audit[len(audit)-1].Outcome != "no_pending" {
		t.Fatalf("audit outcome=%v, want no_pending", audit)
	}
}

// Replay defense: a second identical reply finds no pending rec.
func TestRouter_TakeIsSingleUseReplayDefense(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentReject}
	eng := &fakeEngine{}
	var audit []AuditRow
	r := newRouter(gate, cls, eng, store, &audit)
	if err := r.Handle(context.Background(), mkReply("no")); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	// second identical reply
	if err := r.Handle(context.Background(), mkReply("no")); err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if eng.rejectCalls != 1 {
		t.Fatalf("replay should not re-fire Reject: calls=%d", eng.rejectCalls)
	}
}

// Nil audit hook must not panic — keeps unit-test setup minimal.
func TestRouter_NilAuditIsSafe(t *testing.T) {
	store := NewMemoryPendingStore()
	seedPending(t, store)
	gate := &fakeGate{enrolled: map[string]bool{"discord|u1": true}}
	cls := &fakeClassifier{out: IntentReject}
	eng := &fakeEngine{}
	r := &Router{Pending: store, Consent: gate, Classify: cls, Engine: eng}
	if err := r.Handle(context.Background(), mkReply("no")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}
