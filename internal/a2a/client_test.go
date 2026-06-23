package a2a

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

func TestSearchMemoryRoundtrip(t *testing.T) {
	hits := []MemoryHit{{ID: "m1", Text: "hello", Score: 0.42}}
	h := &fakeHandler{hits: hits}
	db := newTestDB(t)
	consent := NewConsentStore(db)
	srv, cli, srvConn, cliConn, peerID := pairedServerClient(t, h, consent)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := consent.Grant(ctx, peerID, ConsentMemory, ScopeAlways); err != nil {
		t.Fatalf("grant: %v", err)
	}

	go func() { _ = srv.ServeConn(ctx, srvConn) }()
	sess, err := cli.DialConn(ctx, cliConn)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	got, err := sess.SearchMemory(ctx, "q", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 || got[0].ID != "m1" || got[0].Text != "hello" {
		t.Fatalf("hits: %+v", got)
	}
	if h.memQ != "q" || h.memK != 5 {
		t.Fatalf("handler args: q=%q k=%d", h.memQ, h.memK)
	}
	sess.Bye(ctx)
}

func TestSearchMemoryConsentMissing(t *testing.T) {
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
	_, err = sess.SearchMemory(ctx, "q", 5)
	if !errors.Is(err, ErrConsentNotGranted) {
		t.Fatalf("expected ErrConsentNotGranted, got %v", err)
	}
	sess.Bye(ctx)
}

func TestByeClosesConn(t *testing.T) {
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
	sess.Bye(ctx)
	// Second Bye must be a safe no-op.
	sess.Bye(ctx)
}

func TestIdentityRoundtrip(t *testing.T) {
	dir := t.TempDir()
	store := &FileIdentityStore{Path: dir + "/id.key"}
	priv, err := EnsureIdentity(store)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	priv2, err := EnsureIdentity(store)
	if err != nil {
		t.Fatalf("ensure2: %v", err)
	}
	if string(priv) != string(priv2) {
		t.Fatal("identity not stable across loads")
	}
	nonce, err := NewNonce()
	if err != nil {
		t.Fatalf("nonce: %v", err)
	}
	sig := Prove(priv, nonce)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatal("priv.Public() is not ed25519.PublicKey")
	}
	if err := Verify(pub, nonce, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Tampered nonce must fail.
	nonce[0] ^= 0xff
	if err := Verify(pub, nonce, sig); err == nil {
		t.Fatal("tampered nonce verified")
	}
}
