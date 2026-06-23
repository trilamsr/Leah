package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/trilam/leah/internal/a2a"
	"github.com/trilam/leah/internal/ipc"
)

type stubA2A struct {
	peers      map[a2a.PeerID]a2a.A2APeer
	revokeErr  error
	revokedIDs []a2a.PeerID
}

func newStubA2A() *stubA2A {
	return &stubA2A{peers: map[a2a.PeerID]a2a.A2APeer{}}
}

func (s *stubA2A) Peers() []a2a.A2APeer {
	out := make([]a2a.A2APeer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	return out
}

func (s *stubA2A) AddPeer(p a2a.A2APeer) { s.peers[p.ID] = p }

func (s *stubA2A) Revoke(_ context.Context, id a2a.PeerID) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revokedIDs = append(s.revokedIDs, id)
	delete(s.peers, id)
	return nil
}

func a2aFrame(t *testing.T, kind string, payload any) ipc.Frame {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return ipc.Frame{Kind: kind, TurnID: "t1", Seq: 1, Payload: b}
}

func drainA2AFrames(ch <-chan ipc.Frame) []ipc.Frame {
	var out []ipc.Frame
	for f := range ch {
		out = append(out, f)
	}
	return out
}

func mkPeer(t *testing.T, name string, paused bool) a2a.A2APeer {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return a2a.A2APeer{
		ID:       a2a.Fingerprint(pub),
		Name:     name,
		Pubkey:   pub,
		PairedAt: time.Unix(1_700_000_000, 0).UTC(),
		Paused:   paused,
		Scopes:   []string{"leah.ask"},
	}
}

func TestHandleA2APeerList_EmptyReturnsEmptyList(t *testing.T) {
	srv := newStubA2A()
	ch, err := handleA2APeerList(context.Background(), a2aFrame(t, ipc.KindA2APeerList, nil), srv)
	if err != nil {
		t.Fatalf("handleA2APeerList: %v", err)
	}
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindA2APeerList {
		t.Fatalf("expected single peer.list frame, got %+v", got)
	}
	var resp struct {
		Peers []peerWire `json:"peers"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Peers) != 0 {
		t.Fatalf("expected zero peers, got %d", len(resp.Peers))
	}
}

func TestHandleA2APeerList_RendersPeers(t *testing.T) {
	srv := newStubA2A()
	p := mkPeer(t, "laptop", false)
	srv.AddPeer(p)
	ch, _ := handleA2APeerList(context.Background(), a2aFrame(t, ipc.KindA2APeerList, nil), srv)
	got := drainA2AFrames(ch)
	var resp struct {
		Peers []peerWire `json:"peers"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Peers) != 1 || resp.Peers[0].ID != string(p.ID) || resp.Peers[0].Name != "laptop" {
		t.Fatalf("peer mismatch: %+v", resp.Peers)
	}
	if resp.Peers[0].PubkeyFP == "" {
		t.Fatalf("pubkey_fp empty")
	}
}

func TestHandleA2APairStart_ValidatesPayload(t *testing.T) {
	srv := newStubA2A()
	ch, err := handleA2APairStart(context.Background(), a2aFrame(t, ipc.KindA2APairStart, map[string]string{"otp_string": "", "peer_addr": ""}), srv)
	if err != nil {
		t.Fatalf("handleA2APairStart: %v", err)
	}
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame for empty otp, got %+v", got)
	}
}

func TestHandleA2APairStart_AcceptsValidPayload(t *testing.T) {
	srv := newStubA2A()
	ch, _ := handleA2APairStart(context.Background(), a2aFrame(t, ipc.KindA2APairStart, map[string]string{
		"otp_string": "123456",
		"peer_addr":  "127.0.0.1:7766",
	}), srv)
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindA2APairStart {
		t.Fatalf("expected pair.start ack, got %+v", got)
	}
	var resp struct {
		PeerID string `json:"peer_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "pending" {
		t.Fatalf("expected status=pending, got %q", resp.Status)
	}
}

func TestHandleA2APeerPause_TogglesState(t *testing.T) {
	srv := newStubA2A()
	p := mkPeer(t, "phone", false)
	srv.AddPeer(p)
	ch, _ := handleA2APeerPause(context.Background(), a2aFrame(t, ipc.KindA2APeerPause, map[string]any{
		"peer_id": string(p.ID),
		"paused":  true,
	}), srv)
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindA2APeerPause {
		t.Fatalf("expected peer.pause ack, got %+v", got)
	}
	stored, ok := srv.peers[p.ID]
	if !ok {
		t.Fatalf("peer dropped from store")
	}
	if !stored.Paused {
		t.Fatalf("expected Paused=true, got false")
	}
	ch2, _ := handleA2APeerPause(context.Background(), a2aFrame(t, ipc.KindA2APeerPause, map[string]any{
		"peer_id": string(p.ID),
		"paused":  false,
	}), srv)
	_ = drainA2AFrames(ch2)
	if srv.peers[p.ID].Paused {
		t.Fatalf("expected Paused=false after toggle, got true")
	}
}

func TestHandleA2APeerPause_UnknownPeerIsErr(t *testing.T) {
	srv := newStubA2A()
	ch, _ := handleA2APeerPause(context.Background(), a2aFrame(t, ipc.KindA2APeerPause, map[string]any{
		"peer_id": "deadbeef",
		"paused":  true,
	}), srv)
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame, got %+v", got)
	}
}

func TestHandleA2APeerUnpair_CallsRevoke(t *testing.T) {
	srv := newStubA2A()
	p := mkPeer(t, "laptop", false)
	srv.AddPeer(p)
	ch, _ := handleA2APeerUnpair(context.Background(), a2aFrame(t, ipc.KindA2APeerUnpair, map[string]string{
		"peer_id": string(p.ID),
	}), srv)
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindA2APeerUnpair {
		t.Fatalf("expected peer.unpair ack, got %+v", got)
	}
	if len(srv.revokedIDs) != 1 || srv.revokedIDs[0] != p.ID {
		t.Fatalf("revoke not called for %s; got %+v", p.ID, srv.revokedIDs)
	}
}

func TestHandleA2APeerUnpair_EmptyPeerIDIsErr(t *testing.T) {
	srv := newStubA2A()
	ch, _ := handleA2APeerUnpair(context.Background(), a2aFrame(t, ipc.KindA2APeerUnpair, map[string]string{"peer_id": ""}), srv)
	got := drainA2AFrames(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame, got %+v", got)
	}
}

func TestHandleA2A_NilServerErrFrames(t *testing.T) {
	cases := []struct {
		name string
		fn   func(ctx context.Context, req ipc.Frame, srv a2aServer) (<-chan ipc.Frame, error)
		kind string
	}{
		{"list", handleA2APeerList, ipc.KindA2APeerList},
		{"pair", handleA2APairStart, ipc.KindA2APairStart},
		{"pause", handleA2APeerPause, ipc.KindA2APeerPause},
		{"unpair", handleA2APeerUnpair, ipc.KindA2APeerUnpair},
	}
	for _, c := range cases {
		ch, err := c.fn(context.Background(), ipc.Frame{Kind: c.kind, TurnID: "t", Seq: 1}, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := drainA2AFrames(ch)
		if len(got) != 1 || got[0].Kind != ipc.KindError {
			t.Fatalf("%s: expected single error frame, got %+v", c.name, got)
		}
	}
}
