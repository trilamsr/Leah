package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/memory/sqlstore"
	"github.com/trilam/leah/internal/platform/sync/discovery"
)

type stubSyncEngine struct {
	peers        []discovery.Peer
	browseErr    error
	pairResult   PairResult
	pairErr      error
	gotOTP       string
	browseCalled int
}

func (s *stubSyncEngine) Browse(_ context.Context) ([]discovery.Peer, error) {
	s.browseCalled++
	return s.peers, s.browseErr
}

func (s *stubSyncEngine) PairStart(_ context.Context, otp string) (PairResult, error) {
	s.gotOTP = otp
	return s.pairResult, s.pairErr
}

func newSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlstore.OpenWAL(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlstore.MigrateUp(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func mustPeer(id, addr string, seenMS int64, status discovery.PeerStatus) discovery.Peer {
	return discovery.NewPeer(discovery.DeviceID(id), netip.MustParseAddrPort(addr), time.UnixMilli(seenMS), status)
}

func TestHandleSyncPeerList_EmitsSnapshot(t *testing.T) {
	eng := &stubSyncEngine{peers: []discovery.Peer{
		mustPeer("dev-A", "192.0.2.10:7777", 1700000000000, discovery.StatusOnline),
		mustPeer("dev-B", "192.0.2.11:7777", 1700000001000, discovery.StatusIdle),
	}}
	req := ipc.Frame{Kind: ipc.KindSyncPeerList, TurnID: "s1", Seq: 0}
	ch, err := handleSyncPeerList(context.Background(), req, eng)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindSyncPeerList {
		t.Fatalf("frames: %+v", frames)
	}
	var got struct {
		Peers []struct {
			ID       string `json:"id"`
			Addr     string `json:"addr"`
			LastSeen int64  `json:"lastSeen"`
			Status   string `json:"status"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(frames[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Peers) != 2 {
		t.Fatalf("peers: %+v", got.Peers)
	}
	if got.Peers[0].ID != "dev-A" || got.Peers[0].Addr != "192.0.2.10:7777" || got.Peers[0].Status != "online" || got.Peers[0].LastSeen != 1700000000000 {
		t.Errorf("peer[0]: %+v", got.Peers[0])
	}
	if got.Peers[1].Status != "idle" {
		t.Errorf("peer[1] status: %q", got.Peers[1].Status)
	}
}

func TestHandleSyncPeerList_NilEngineErrFrame(t *testing.T) {
	req := ipc.Frame{Kind: ipc.KindSyncPeerList, TurnID: "s1", Seq: 0}
	ch, err := handleSyncPeerList(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindError {
		t.Fatalf("frames: %+v", frames)
	}
}

func TestHandleSyncPairStart_StoresPendingAndReturnsPeerID(t *testing.T) {
	eng := &stubSyncEngine{pairResult: PairResult{PeerID: "dev-A", Name: "alice-mbp", Fingerprint: []byte{1, 2, 3}, Status: "paired"}}
	pending := newPendingPairs()
	req := ipc.Frame{Kind: ipc.KindSyncPairStart, TurnID: "s2", Seq: 0, Payload: json.RawMessage(`{"otp_string":"482-913"}`)}
	ch, err := handleSyncPairStart(context.Background(), req, eng, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindSyncPairStart {
		t.Fatalf("frames: %+v", frames)
	}
	if eng.gotOTP != "482-913" {
		t.Errorf("OTP not forwarded: %q", eng.gotOTP)
	}
	var got struct {
		PeerID string `json:"peer_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(frames[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PeerID != "dev-A" || got.Status != "paired" {
		t.Errorf("payload: %+v", got)
	}
	if _, ok := pending.snapshot("dev-A"); !ok {
		t.Errorf("PairResult not staged for ack")
	}
}

func TestHandleSyncPairStart_EmptyOTPErr(t *testing.T) {
	pending := newPendingPairs()
	eng := &stubSyncEngine{}
	req := ipc.Frame{Kind: ipc.KindSyncPairStart, TurnID: "s2", Seq: 0, Payload: json.RawMessage(`{"otp_string":""}`)}
	ch, err := handleSyncPairStart(context.Background(), req, eng, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindError {
		t.Fatalf("frames: %+v", frames)
	}
}

func TestHandleSyncPairStart_HandshakeErrSurfacedAsErrFrame(t *testing.T) {
	pending := newPendingPairs()
	eng := &stubSyncEngine{pairErr: errors.New("otp mismatch")}
	req := ipc.Frame{Kind: ipc.KindSyncPairStart, TurnID: "s2", Seq: 0, Payload: json.RawMessage(`{"otp_string":"000-000"}`)}
	ch, err := handleSyncPairStart(context.Background(), req, eng, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindError {
		t.Fatalf("frames: %+v", frames)
	}
}

func TestHandleSyncPairAck_InsertsRowAndReturnsOK(t *testing.T) {
	db := newSyncTestDB(t)
	pending := newPendingPairs()
	pending.put(PairResult{PeerID: "dev-A", Name: "alice-mbp", Fingerprint: []byte{0xAA, 0xBB}, Status: "paired"})
	req := ipc.Frame{Kind: ipc.KindSyncPairAck, TurnID: "s3", Seq: 0, Payload: json.RawMessage(`{"peer_id":"dev-A"}`)}
	ch, err := handleSyncPairAck(context.Background(), req, db, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindSyncPairAck {
		t.Fatalf("frames: %+v", frames)
	}
	var got struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(frames[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.OK {
		t.Errorf("ok=false")
	}
	var (
		id, name string
		fp       []byte
		paired   int64
	)
	if err := db.QueryRow(`SELECT id, name, paired_at, fingerprint FROM sync_peer WHERE id=?`, "dev-A").Scan(&id, &name, &paired, &fp); err != nil {
		t.Fatalf("select: %v", err)
	}
	if id != "dev-A" || name != "alice-mbp" || len(fp) != 2 || fp[0] != 0xAA || fp[1] != 0xBB {
		t.Errorf("row: id=%q name=%q fp=%x paired=%d", id, name, fp, paired)
	}
	if _, ok := pending.snapshot("dev-A"); ok {
		t.Errorf("pending not cleared on ack")
	}
}

func TestHandleSyncPairAck_RejectsUnknownPeerID(t *testing.T) {
	db := newSyncTestDB(t)
	pending := newPendingPairs()
	req := ipc.Frame{Kind: ipc.KindSyncPairAck, TurnID: "s3", Seq: 0, Payload: json.RawMessage(`{"peer_id":"ghost"}`)}
	ch, err := handleSyncPairAck(context.Background(), req, db, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindError {
		t.Fatalf("frames: %+v", frames)
	}
}

func TestHandleSyncPairAck_EmptyPeerIDErr(t *testing.T) {
	db := newSyncTestDB(t)
	pending := newPendingPairs()
	req := ipc.Frame{Kind: ipc.KindSyncPairAck, TurnID: "s3", Seq: 0, Payload: json.RawMessage(`{"peer_id":""}`)}
	ch, err := handleSyncPairAck(context.Background(), req, db, pending)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	frames := drainFrames(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindError {
		t.Fatalf("frames: %+v", frames)
	}
}

// TestSyncDispatchRoutedFromIPCHandler was retired in the fold: dispatch-wiring
// binds sync via SyncIPC in IPCDeps (PeerList/PairStart/PairAck on ipc.Frame),
// not via an engine arg on newIPCHandlerWithClassifyEnrich. The nil-deps
// routing for sync.* is still covered by TestIPCHandlerPhase4Dispatch; binding
// a concrete SyncIPC adapter is a follow-up.
