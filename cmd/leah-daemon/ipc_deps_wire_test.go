package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/ipc"
)

// TestA2AAdapter_NilServerReturnsNil covers the guard that keeps IPCDeps.A2A
// nil when the daemon boots without an a2a.Server (release build path in
// wirePhase4Producers where ed25519.GenerateKey failed). The dispatch switch
// treats a nil interface as "subsystem unavailable" and emits a structured
// error frame — the pre-BUG-8 regression was that the whole IPCDeps struct
// was passed empty regardless of producer availability.
func TestA2AAdapter_NilServerReturnsNil(t *testing.T) {
	if got := newA2AIPCAdapter(nil); got != nil {
		t.Fatalf("newA2AIPCAdapter(nil) = %v, want nil", got)
	}
}

// TestSyncAdapter_NilDepsReturnsNil covers the same guard for sync — the
// adapter refuses to bind when either the discovery engine or the sqlite
// handle is missing so the caller can rely on the dispatch nil-check.
func TestSyncAdapter_NilDepsReturnsNil(t *testing.T) {
	if got := newSyncIPCAdapter(nil, nil); got != nil {
		t.Fatalf("newSyncIPCAdapter(nil, nil) = %v, want nil", got)
	}
}

// TestA2AAdapter_LiveDispatchReachesHandler is the BUG-8 regression:
// pre-fix, main.go passed an empty IPCDeps{} so the dispatch switch's a2a
// case would nil-check and return "a2a unavailable" for every request.
// Binding the adapter must actually forward the frame into the a2a handler
// (which in turn errors on nil peer server — different error, proves
// the seam is live).
func TestA2AAdapter_LiveDispatchReachesHandler(t *testing.T) {
	stub := newStubA2A()
	adapter := newA2AIPCAdapter(stub)
	if adapter == nil {
		t.Fatal("adapter should be non-nil with a live a2aServer")
	}
	out, err := adapter.PeerList(context.Background(), ipc.Frame{
		Kind:    ipc.KindA2APeerList,
		TurnID:  "t1",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("PeerList: %v", err)
	}
	f := <-out
	// Handler wraps Peers() into a structured payload; presence of the "peers"
	// field proves the frame reached handleA2APeerList (rather than being
	// short-circuited by the pre-fix "unavailable" nil-check path).
	if !strings.Contains(string(f.Payload), "peers") {
		t.Fatalf("expected handler payload with peers key, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}
