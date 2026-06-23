package a2a

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"net"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE a2a_peer (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			paired_at    INTEGER NOT NULL,
			last_seen_at INTEGER,
			paused       INTEGER NOT NULL DEFAULT 0,
			scopes       TEXT NOT NULL
		);
		CREATE TABLE a2a_consent (
			id           INTEGER PRIMARY KEY,
			peer_id      TEXT NOT NULL,
			kind         TEXT NOT NULL,
			scope        TEXT NOT NULL CHECK(scope IN ('once','session','always')),
			granted_at   INTEGER NOT NULL,
			expires_at   INTEGER,
			UNIQUE(peer_id, kind)
		);
	`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

type fakeHandler struct {
	askCalled   bool
	memQ        string
	memK        int
	hits        []MemoryHit
	askPartials []string
	askFinal    string
	memErr      error
}

func (f *fakeHandler) Ask(ctx context.Context, peer PeerID, prompt string, out chan<- ReasonerEvent) {
	f.askCalled = true
	for _, p := range f.askPartials {
		out <- ReasonerEvent{Delta: p}
	}
	out <- ReasonerEvent{Delta: f.askFinal, Final: true}
}

func (f *fakeHandler) SearchMemory(ctx context.Context, peer PeerID, q string, k int) ([]MemoryHit, error) {
	f.memQ = q
	f.memK = k
	return f.hits, f.memErr
}

// pairedServerClient builds a Server + Client over net.Pipe with the client's
// pubkey already added to the server's paired-peers table.
func pairedServerClient(t *testing.T, h Handler, consent *ConsentStore) (*Server, *Client, net.Conn, net.Conn, PeerID) {
	t.Helper()
	serverPub, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("server key: %v", err)
	}
	_ = serverPub
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("client key: %v", err)
	}
	clientID := Fingerprint(clientPub)
	srv := NewServer(serverPriv, consent, nil, CapAsk|CapMemorySearch|CapConsentFlow, h)
	srv.AddPeer(A2APeer{
		ID:       clientID,
		Name:     "test",
		Pubkey:   clientPub,
		PairedAt: time.Now(),
	})
	cli := NewClient(clientPriv, CapAsk|CapMemorySearch, "client")
	srvConn, cliConn := net.Pipe()
	return srv, cli, srvConn, cliConn, clientID
}

func TestServerHandshakeAndAsk(t *testing.T) {
	h := &fakeHandler{askPartials: []string{"hel", "lo"}, askFinal: "."}
	db := newTestDB(t)
	consent := NewConsentStore(db)
	srv, cli, srvConn, cliConn, peerID := pairedServerClient(t, h, consent)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := consent.Grant(ctx, peerID, ConsentAsk, ScopeAlways); err != nil {
		t.Fatalf("grant: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.ServeConn(ctx, srvConn) }()

	sess, err := cli.DialConn(ctx, cliConn)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	caps, err := sess.Negotiate(ctx)
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if !caps.Has(CapAsk) {
		t.Fatalf("expected CapAsk negotiated, got %v", caps)
	}
	out, err := sess.Ask(ctx, "hi")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	got := ""
	final := false
	for ev := range out {
		if ev.Err != nil {
			t.Fatalf("event err: %v", ev.Err)
		}
		got += ev.Delta
		if ev.Final {
			final = true
			break
		}
	}
	if !final {
		t.Fatal("never saw final")
	}
	if got != "hello." {
		t.Fatalf("delta: got %q want %q", got, "hello.")
	}
	if !h.askCalled {
		t.Fatal("handler.Ask not called")
	}
	sess.Bye(ctx)
	select {
	case err := <-done:
		if err != nil && err.Error() != "io: read/write on closed pipe" {
			// Bye closes the conn — server returns EOF/closed which is fine.
			_ = err
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return")
	}
}

func TestServerRejectsUnpairedPeer(t *testing.T) {
	db := newTestDB(t)
	consent := NewConsentStore(db)
	h := &fakeHandler{}
	_, serverPriv, _ := ed25519.GenerateKey(rand.Reader)
	srv := NewServer(serverPriv, consent, nil, CapAsk, h)
	// Deliberately do NOT AddPeer.

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	cli := NewClient(clientPriv, CapAsk, "stranger")

	srvConn, cliConn := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.ServeConn(ctx, srvConn) }()

	if _, err := cli.DialConn(ctx, cliConn); err == nil {
		t.Fatal("expected dial to fail against unpaired peer")
	}
	select {
	case <-srvErr:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return")
	}
}

func TestAskWithoutConsentSurfacesRequire(t *testing.T) {
	h := &fakeHandler{}
	db := newTestDB(t)
	consent := NewConsentStore(db)
	srv, cli, srvConn, cliConn, _ := pairedServerClient(t, h, consent)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = srv.ServeConn(ctx, srvConn) }()

	sess, err := cli.DialConn(ctx, cliConn)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	out, err := sess.Ask(ctx, "hi")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	saw := false
	for ev := range out {
		if ev.Err != nil && errors.Is(ev.Err, ErrConsentNotGranted) {
			saw = true
			break
		}
		if ev.Final {
			break
		}
	}
	if !saw {
		t.Fatal("expected consent-not-granted error")
	}
	if h.askCalled {
		t.Fatal("handler must not run without consent")
	}
	sess.Bye(ctx)
}

func TestConsentGrantPersists(t *testing.T) {
	db := newTestDB(t)
	consent := NewConsentStore(db)
	peer := PeerID("abc")
	ctx := context.Background()

	if err := consent.Check(ctx, peer, ConsentAsk); !errors.Is(err, ErrConsentNotGranted) {
		t.Fatalf("expected ErrConsentNotGranted, got %v", err)
	}
	if err := consent.Grant(ctx, peer, ConsentAsk, ScopeAlways); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := consent.Check(ctx, peer, ConsentAsk); err != nil {
		t.Fatalf("check after grant: %v", err)
	}
	// Second check still works (ScopeAlways persists).
	if err := consent.Check(ctx, peer, ConsentAsk); err != nil {
		t.Fatalf("check #2: %v", err)
	}
}

func TestConsentOnceConsumed(t *testing.T) {
	db := newTestDB(t)
	consent := NewConsentStore(db)
	peer := PeerID("once")
	ctx := context.Background()
	if err := consent.Grant(ctx, peer, ConsentAsk, ScopeOnce); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// ScopeOnce — first Check passes and consumes; second Check rejects.
	if err := consent.Check(ctx, peer, ConsentAsk); err != nil {
		t.Fatalf("first check: %v", err)
	}
	if err := consent.Check(ctx, peer, ConsentAsk); !errors.Is(err, ErrConsentNotGranted) {
		t.Fatalf("expected ErrConsentNotGranted on second check, got %v", err)
	}
}

func TestBudgetExhaustionAbortsAsk(t *testing.T) {
	// Empty handler; the budget should trip before it runs.
	h := &fakeHandler{askFinal: "ignored"}
	db := newTestDB(t)
	consent := NewConsentStore(db)
	srv, cli, srvConn, cliConn, peerID := pairedServerClient(t, h, consent)
	srv.Budget = newSpentBudget(t, 0.5, 0.5) // already at ceiling
	srv.AskCost = 0.001                      // small charge still trips

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := consent.Grant(ctx, peerID, ConsentAsk, ScopeAlways); err != nil {
		t.Fatalf("grant: %v", err)
	}

	go func() { _ = srv.ServeConn(ctx, srvConn) }()

	sess, err := cli.DialConn(ctx, cliConn)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	out, err := sess.Ask(ctx, "hi")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	var lastEv ReasonerEvent
	for ev := range out {
		lastEv = ev
		if ev.Final {
			break
		}
	}
	if !lastEv.Final {
		t.Fatal("no final event")
	}
	if lastEv.Delta != "budget.exhausted" {
		t.Fatalf("delta: got %q want %q", lastEv.Delta, "budget.exhausted")
	}
	if h.askCalled {
		t.Fatal("handler must not run with budget exhausted")
	}
	sess.Bye(ctx)
}

func TestRevokeDropsPeer(t *testing.T) {
	db := newTestDB(t)
	consent := NewConsentStore(db)
	srv, _, _, _, peerID := pairedServerClient(t, &fakeHandler{}, consent)
	if got := len(srv.Peers()); got != 1 {
		t.Fatalf("peers: got %d want 1", got)
	}
	if err := srv.Revoke(context.Background(), peerID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := len(srv.Peers()); got != 0 {
		t.Fatalf("peers after revoke: got %d want 0", got)
	}
	if err := srv.Revoke(context.Background(), peerID); err == nil {
		t.Fatal("expected error re-revoking unknown peer")
	}
}
