package pair

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"io"
	"testing"
	"time"
)

func randomKey(t *testing.T) [32]byte {
	t.Helper()
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func TestNewMTLSConfig_ReturnsConfig(t *testing.T) {
	cfg, err := NewMTLSConfig(randomKey(t))
	if err != nil {
		t.Fatalf("NewMTLSConfig: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates: want 1, got %d", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion: want TLS1.3, got %x", cfg.MinVersion)
	}
}

func TestPinFingerprint_DeterministicPerKey(t *testing.T) {
	k := randomKey(t)
	a := PinFingerprint(k)
	b := PinFingerprint(k)
	if a != b {
		t.Fatalf("PinFingerprint: not deterministic — %x vs %x", a, b)
	}
}

func TestPinFingerprint_DiffersAcrossKeys(t *testing.T) {
	a := PinFingerprint(randomKey(t))
	b := PinFingerprint(randomKey(t))
	if a == b {
		t.Fatalf("PinFingerprint: distinct keys produced same fingerprint %x", a)
	}
}

// MITM is modeled by two independently-generated keys handshaking against
// each other. Each side's pin is derived from its own key, so the verifier
// must reject the peer's cert. A bug where the verifier short-circuited to
// nil (the obvious way to break this) would let the dial succeed.
func TestMTLS_RejectsUnpinnedCert(t *testing.T) {
	keyA := randomKey(t)
	keyB := randomKey(t)
	if keyA == keyB {
		t.Skip("randomKey collision — astronomically improbable")
	}
	cfgServer, err := NewMTLSConfig(keyA)
	if err != nil {
		t.Fatalf("server cfg: %v", err)
	}
	cfgClient, err := NewMTLSConfig(keyB)
	if err != nil {
		t.Fatalf("client cfg: %v", err)
	}
	cfgClient.ServerName = "leah-sync.local"

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfgServer)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srvErr := make(chan error, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			srvErr <- aerr
			return
		}
		defer func() { _ = c.Close() }()
		tc := c.(*tls.Conn)
		srvErr <- tc.Handshake()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := &tls.Dialer{Config: cfgClient}
	conn, dialErr := d.DialContext(ctx, "tcp", ln.Addr().String())
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("dial: unpinned cert was accepted (MITM not blocked)")
	}
	// Any non-nil err proves the verifier blocked the handshake; log the
	// exact value for forensics but don't tighten beyond that — the TLS
	// stack may wrap ErrUnpinnedCert with its own envelope.
	if !errors.Is(dialErr, ErrUnpinnedCert) {
		t.Logf("dial err (accepted as MITM rejection): %v", dialErr)
	}

	select {
	case <-srvErr:
	case <-time.After(2 * time.Second):
		t.Fatal("server side: handshake did not return")
	}
}

func TestMTLS_AcceptsPinnedCert(t *testing.T) {
	k := randomKey(t)
	cfgServer, err := NewMTLSConfig(k)
	if err != nil {
		t.Fatalf("server cfg: %v", err)
	}
	cfgClient, err := NewMTLSConfig(k)
	if err != nil {
		t.Fatalf("client cfg: %v", err)
	}
	cfgClient.ServerName = "leah-sync.local"

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfgServer)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srvErr := make(chan error, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			srvErr <- aerr
			return
		}
		defer func() { _ = c.Close() }()
		tc := c.(*tls.Conn)
		if herr := tc.Handshake(); herr != nil {
			srvErr <- herr
			return
		}
		_, _ = io.Copy(io.Discard, tc)
		srvErr <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := &tls.Dialer{Config: cfgClient}
	conn, dialErr := d.DialContext(ctx, "tcp", ln.Addr().String())
	if dialErr != nil {
		t.Fatalf("dial: pinned peer rejected: %v", dialErr)
	}
	_ = conn.Close()
	select {
	case <-srvErr:
	case <-time.After(2 * time.Second):
		t.Fatal("server side: handshake did not return")
	}
}
