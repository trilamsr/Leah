package icloud

import (
	"context"
	"errors"
	"testing"
)

// TestNew_NonDarwin asserts non-darwin builds fail loudly rather than no-op.
// iCloud Keychain trust root MUST NOT silently degrade — §2.2 + §2.6.
func TestNew_NonDarwin(t *testing.T) {
	if hasDarwinKeychain {
		t.Skip("darwin build; covered by integration path")
	}
	_, err := New()
	if err == nil {
		t.Fatal("New(): want error on non-darwin, got nil")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("New(): want ErrUnsupported, got %v", err)
	}
}

// TestPut_ZeroDataRejected — empty payload would create a Keychain row with
// no secret and a spurious sync footprint. Reject at the boundary.
func TestPut_ZeroDataRejected(t *testing.T) {
	ks := newFake()
	err := ks.Put(context.Background(), "svc", "acct", nil, true)
	if !errors.Is(err, ErrEmptyData) {
		t.Fatalf("Put(nil): want ErrEmptyData, got %v", err)
	}
}

// TestPut_MissingServiceOrAccountRejected — SecItem queries with blank
// service+account match every row of the kind and would clobber across keys.
func TestPut_MissingServiceOrAccountRejected(t *testing.T) {
	ks := newFake()
	if err := ks.Put(context.Background(), "", "acct", []byte("x"), true); !errors.Is(err, ErrBadKey) {
		t.Fatalf("Put(empty service): want ErrBadKey, got %v", err)
	}
	if err := ks.Put(context.Background(), "svc", "", []byte("x"), true); !errors.Is(err, ErrBadKey) {
		t.Fatalf("Put(empty account): want ErrBadKey, got %v", err)
	}
}

// TestRoundTrip_SyncFlagPreserved — sync=true rows MUST report back via
// ListSynced; sync=false rows MUST NOT. Mis-classification leaks API keys to
// iCloud when operator opted out (§2.6).
func TestRoundTrip_SyncFlagPreserved(t *testing.T) {
	ks := newFake()
	ctx := context.Background()
	if err := ks.Put(ctx, "anthropic", "default", []byte("sk-synced"), true); err != nil {
		t.Fatalf("Put synced: %v", err)
	}
	if err := ks.Put(ctx, "voyage", "default", []byte("vk-local"), false); err != nil {
		t.Fatalf("Put local: %v", err)
	}

	got, err := ks.Get(ctx, "anthropic", "default")
	if err != nil || string(got) != "sk-synced" {
		t.Fatalf("Get synced: got=%q err=%v", got, err)
	}
	got, err = ks.Get(ctx, "voyage", "default")
	if err != nil || string(got) != "vk-local" {
		t.Fatalf("Get local: got=%q err=%v", got, err)
	}

	synced, err := ks.ListSynced(ctx)
	if err != nil {
		t.Fatalf("ListSynced: %v", err)
	}
	if len(synced) != 1 || synced[0] != "anthropic/default" {
		t.Fatalf("ListSynced: want [anthropic/default], got %v", synced)
	}
}

// TestGet_NotFound returns ErrNotFound — callers distinguish from transient
// IO error to decide whether to fall back to env var.
func TestGet_NotFound(t *testing.T) {
	ks := newFake()
	_, err := ks.Get(context.Background(), "svc", "acct")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing): want ErrNotFound, got %v", err)
	}
}

// TestDelete_RemovesFromListSynced — Unpair flow nukes the shared secret;
// ListSynced MUST reflect deletion or the Sync pane shows phantom rows.
func TestDelete_RemovesFromListSynced(t *testing.T) {
	ks := newFake()
	ctx := context.Background()
	_ = ks.Put(ctx, "anthropic", "default", []byte("x"), true)
	if err := ks.Delete(ctx, "anthropic", "default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	synced, _ := ks.ListSynced(ctx)
	if len(synced) != 0 {
		t.Fatalf("ListSynced after delete: want [], got %v", synced)
	}
}

// TestDelete_Idempotent — Unpair retries must not error on second call;
// idempotence keeps the UI from surfacing benign "not found" errors.
func TestDelete_Idempotent(t *testing.T) {
	ks := newFake()
	if err := ks.Delete(context.Background(), "svc", "acct"); err != nil {
		t.Fatalf("Delete(missing): want nil, got %v", err)
	}
}

// TestUpdate_OverwritesValue — re-Put on same (service, account) is the
// rotation path; must replace, not insert duplicate.
func TestUpdate_OverwritesValue(t *testing.T) {
	ks := newFake()
	ctx := context.Background()
	_ = ks.Put(ctx, "svc", "acct", []byte("v1"), true)
	_ = ks.Put(ctx, "svc", "acct", []byte("v2"), true)
	got, _ := ks.Get(ctx, "svc", "acct")
	if string(got) != "v2" {
		t.Fatalf("Put(rotate): want v2, got %s", got)
	}
	synced, _ := ks.ListSynced(ctx)
	if len(synced) != 1 {
		t.Fatalf("ListSynced after rotate: want 1 entry, got %d (%v)", len(synced), synced)
	}
}

// newFake returns an in-memory ICloudKeystore implementation for unit tests.
// The darwin SecItem backend is exercised separately by manual + integration
// runs; pure-Go tests use this fake.
func newFake() ICloudKeystore { return newMemKeystore() }
