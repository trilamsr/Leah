package inbound

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/attest"
)

// fakeAttestor records every scope it was asked to clear; returns deny when
// configured to so the table can prove the gate fails closed.
type fakeAttestor struct {
	calls []string
	deny  error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.calls = append(f.calls, scope)
	return f.deny
}

// staticResolver returns a fixed scope for the test; the real resolver in
// the daemon will key off the recommendation's tier/origin.
type staticResolver struct{ scope string }

func (s staticResolver) ScopeFor(_ string) string { return s.scope }

// Layer-2 attestation MUST use the recommendation's OWN scope when the rec is
// a self-build — remote origin never downgrades to `inbound-apply` (spec §4.2).
func TestAuthorize_SelfBuildDoesNotDowngrade(t *testing.T) {
	att := &fakeAttestor{}
	gate := &Gate{
		Attestor: att,
		Resolver: staticResolver{scope: attest.ScopeSelfBuild},
		Store:    NewMemoryEnrollStore(),
	}
	p := Pending{RecID: "rec-build-42", Channel: "discord", PeerID: "peer-A"}
	if err := gate.Authorize(context.Background(), p, IntentAccept); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if len(att.calls) != 1 || att.calls[0] != attest.ScopeSelfBuild {
		t.Fatalf("scope: got %v want [%s]", att.calls, attest.ScopeSelfBuild)
	}
}

// A TierConfirm rec with no special scope falls through to ScopeInboundApply.
func TestAuthorize_DefaultsToInboundApply(t *testing.T) {
	att := &fakeAttestor{}
	gate := &Gate{
		Attestor: att,
		Resolver: staticResolver{scope: attest.ScopeInboundApply},
		Store:    NewMemoryEnrollStore(),
	}
	if err := gate.Authorize(context.Background(), Pending{RecID: "rec-cron"}, IntentAccept); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if len(att.calls) != 1 || att.calls[0] != attest.ScopeInboundApply {
		t.Fatalf("scope: got %v want [%s]", att.calls, attest.ScopeInboundApply)
	}
}

// Denied attestation surfaces as a non-nil error so the router records
// attest_denied and SKIPS engine.Apply.
func TestAuthorize_DenialSurfacesError(t *testing.T) {
	deny := errors.New("operator declined")
	att := &fakeAttestor{deny: deny}
	gate := &Gate{
		Attestor: att,
		Resolver: staticResolver{scope: attest.ScopeInboundApply},
		Store:    NewMemoryEnrollStore(),
	}
	if err := gate.Authorize(context.Background(), Pending{RecID: "x"}, IntentAccept); !errors.Is(err, deny) {
		t.Fatalf("want denial err, got %v", err)
	}
}

// Reject/defer/unknown never attest — gating those would only train the
// operator to dismiss prompts (spec §4.3).
func TestAuthorize_NonAcceptIntentsSkipAttestation(t *testing.T) {
	for _, intent := range []Intent{IntentReject, IntentDefer, IntentUnknown} {
		att := &fakeAttestor{}
		gate := &Gate{
			Attestor: att,
			Resolver: staticResolver{scope: attest.ScopeInboundApply},
			Store:    NewMemoryEnrollStore(),
		}
		if err := gate.Authorize(context.Background(), Pending{RecID: "x"}, intent); err != nil {
			t.Fatalf("intent %v: %v", intent, err)
		}
		if len(att.calls) != 0 {
			t.Fatalf("intent %v: scope calls %v want none", intent, att.calls)
		}
	}
}

// Enrolled fails closed: an un-enrolled (channel,peer) returns false and the
// router drops the reply before reaching layer 2.
func TestEnrolled_FailsClosed(t *testing.T) {
	gate := &Gate{Store: NewMemoryEnrollStore()}
	ok, err := gate.Enrolled("discord", "peer-X")
	if err != nil {
		t.Fatalf("Enrolled: %v", err)
	}
	if ok {
		t.Fatalf("Enrolled returned true for un-enrolled pair (fail-closed violation)")
	}
}

// Enroll persists the pair through Put; a follow-up Enrolled returns true.
func TestEnroll_RoundTrip(t *testing.T) {
	store := NewMemoryEnrollStore()
	gate := &Gate{Store: store}
	if err := store.Enroll("discord", "peer-A"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	ok, err := gate.Enrolled("discord", "peer-A")
	if err != nil || !ok {
		t.Fatalf("Enrolled(after Enroll)=%v,%v want true,nil", ok, err)
	}
	// Other peer still fails closed.
	ok, _ = gate.Enrolled("discord", "peer-B")
	if ok {
		t.Fatalf("Enrolled(peer-B) leaked true from peer-A's enrollment")
	}
}

// FileEnrollStore persists to disk so a daemon restart preserves the trust
// grant — enrolling is a one-time act and forgetting it would re-prompt the
// operator on every restart.
func TestFileEnrollStore_PersistsAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enroll.json")

	s1, err := OpenFileEnrollStore(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := s1.Enroll("discord", "peer-A"); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	// Re-open from the same path; the prior enrollment must survive.
	s2, err := OpenFileEnrollStore(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	ok, err := s2.Enrolled("discord", "peer-A")
	if err != nil || !ok {
		t.Fatalf("Enrolled after re-open=%v,%v want true,nil", ok, err)
	}
}

// Flush failure MUST NOT commit to the in-memory set — otherwise a process
// that crashes after the failed write returns true from Enrolled() until
// restart, while the disk says the pair was never enrolled.
func TestFileEnrollStore_FlushFailureLeavesNoMemDiskSkew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enroll.json")
	s, err := OpenFileEnrollStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Make the parent directory unwritable so the atomic rename fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := s.Enroll("discord", "peer-A"); err == nil {
		t.Fatalf("Enroll succeeded despite RO parent dir")
	}
	ok, _ := s.Enrolled("discord", "peer-A")
	if ok {
		t.Fatalf("mem/disk skew: Enrolled=true after failed flush")
	}
}

// Repeated flushes write pairs in stable sorted order so the on-disk byte
// sequence doesn't churn with Go's randomized map iteration.
func TestFileEnrollStore_FlushOrderStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enroll.json")
	s, err := OpenFileEnrollStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, peer := range []string{"peer-C", "peer-A", "peer-B"} {
		if err := s.Enroll("discord", peer); err != nil {
			t.Fatalf("enroll %s: %v", peer, err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(b)
	idxA := bytesIndex(got, "peer-A")
	idxB := bytesIndex(got, "peer-B")
	idxC := bytesIndex(got, "peer-C")
	if idxA >= idxB || idxB >= idxC {
		t.Fatalf("pairs not sorted: A=%d B=%d C=%d (file=%s)", idxA, idxB, idxC, got)
	}
}

func bytesIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// FileEnrollStore mode bits — credentials-on-disk pattern: 0600.
func TestFileEnrollStore_FileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enroll.json")
	s, err := OpenFileEnrollStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Enroll("discord", "peer-A"); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Fatalf("perm = %v want 0600", mode)
	}
}

// Wired router integration: a reply from the wrong peer never reaches the
// gate (router peer-mismatch drop) — re-verified here against the concrete
// Gate to prove PR-2 wiring doesn't regress the PR-1 binding.
func TestRouter_PeerMismatchSkipsConcreteGate(t *testing.T) {
	att := &fakeAttestor{}
	store := NewMemoryEnrollStore()
	_ = store.Enroll("discord", "peer-real")
	gate := &Gate{
		Attestor: att,
		Resolver: staticResolver{scope: attest.ScopeInboundApply},
		Store:    store,
	}
	pending := NewMemoryPendingStore()
	_ = pending.Put(Pending{RecID: "rec-1", Channel: "discord", ConvID: "c1", PeerID: "peer-real"})

	r := &Router{
		Pending:  pending,
		Consent:  gate,
		Classify: stubClassifier{intent: IntentAccept},
		Engine:   countingEngine{},
	}
	if err := r.Handle(context.Background(), Reply{Channel: "discord", ConvID: "c1", PeerID: "peer-attacker", Text: "yes"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(att.calls) != 0 {
		t.Fatalf("attacker reply triggered attestation: %v", att.calls)
	}
	// Pending entry must remain — a peer-mismatch drop cannot consume it.
	if _, ok := pending.Peek("discord", "c1"); !ok {
		t.Fatalf("peer-mismatch drop consumed the pending entry (real operator can no longer reply)")
	}
}

type stubClassifier struct{ intent Intent }

func (s stubClassifier) Classify(_ context.Context, _ string) (Intent, error) {
	return s.intent, nil
}

type countingEngine struct{}

func (countingEngine) Accept(_ context.Context, _ string) error { return nil }
func (countingEngine) Reject(_ context.Context, _ string) error { return nil }
func (countingEngine) Apply(_ context.Context, _ string) error  { return nil }
