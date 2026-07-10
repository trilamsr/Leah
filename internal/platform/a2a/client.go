package a2a

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// A2AClient is the §5.5 outbound-side contract.
type A2AClient interface {
	Dial(ctx context.Context, addr netip.AddrPort, pubkey ed25519.PublicKey) (A2ASession, error)
}

// Client dials a remote Server. Identity is the local Ed25519 key — Dial
// signs the server's nonce with it to complete the handshake.
type Client struct {
	Identity ed25519.PrivateKey
	Caps     CapabilitySet
	Name     string

	// DialTimeout caps the §5.9 1.2 s handshake budget; tests override.
	DialTimeout time.Duration
}

// NewClient builds a Client with the supplied identity.
func NewClient(id ed25519.PrivateKey, caps CapabilitySet, name string) *Client {
	return &Client{Identity: id, Caps: caps, Name: name, DialTimeout: 1200 * time.Millisecond}
}

// Dial completes the §5.4.1 handshake and returns a session ready for Ask /
// SearchMemory / Bye.
func (c *Client) Dial(ctx context.Context, addr netip.AddrPort, _ ed25519.PublicKey) (A2ASession, error) {
	d := net.Dialer{Timeout: c.DialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, fmt.Errorf("a2a: dial %s: %w", addr, err)
	}
	return c.start(ctx, conn)
}

// DialConn runs the handshake over a pre-established conn (test seam).
func (c *Client) DialConn(ctx context.Context, conn net.Conn) (A2ASession, error) {
	return c.start(ctx, conn)
}

func (c *Client) start(ctx context.Context, conn net.Conn) (A2ASession, error) {
	sess := &session{conn: conn}
	if err := c.handshake(ctx, sess); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return sess, nil
}

func (c *Client) handshake(ctx context.Context, s *session) error {
	pub := c.Identity.Public().(ed25519.PublicKey)
	offer := helloPayload{
		Pubkey: pub,
		Caps:   uint64(c.Caps),
		Name:   c.Name,
	}
	if err := writeFrame(s.conn, Frame{
		ID:      NewID(),
		Kind:    KindHelloOffer,
		Payload: mustMarshal(offer),
	}); err != nil {
		return err
	}
	ack, err := readFrame(s.conn)
	if err != nil {
		return err
	}
	if ack.Kind != KindHelloAck {
		return fmt.Errorf("a2a: expected hello.ack got %s", ack.Kind)
	}
	var ackBody helloAckPayload
	if err := unmarshalPayload(ack.Payload, &ackBody); err != nil {
		return err
	}
	s.caps = CapabilitySet(ackBody.Caps)
	s.peerPubkey = ed25519.PublicKey(ackBody.Pubkey)

	// Sign the server's nonce.
	sig := Prove(c.Identity, ackBody.Nonce)
	if err := writeFrame(s.conn, Frame{
		ID:      NewID(),
		Kind:    KindIdentityProve,
		Payload: sig,
	}); err != nil {
		return err
	}
	verify, err := readFrame(s.conn)
	if err != nil {
		return err
	}
	if verify.Kind != KindIdentityVerify {
		return fmt.Errorf("a2a: expected identity.verify got %s", verify.Kind)
	}
	return nil
}

type session struct {
	conn       net.Conn
	caps       CapabilitySet
	peerPubkey ed25519.PublicKey
	mu         sync.Mutex
	closed     bool
}

func (s *session) Negotiate(_ context.Context) (CapabilitySet, error) {
	return s.caps, nil
}

// Ask streams partials until ask.end. Caller ranges the channel.
func (s *session) Ask(ctx context.Context, prompt string) (<-chan ReasonerEvent, error) {
	if err := writeFrame(s.conn, Frame{
		ID:      NewID(),
		Kind:    KindAskRequest,
		Payload: mustMarshal(askRequestPayload{Prompt: prompt}),
	}); err != nil {
		return nil, err
	}
	out := make(chan ReasonerEvent, 16)
	go func() {
		defer close(out)
		send := func(ev ReasonerEvent) bool {
			select {
			case <-ctx.Done():
				return false
			case out <- ev:
				return true
			}
		}
		for {
			if ctx.Err() != nil {
				return
			}
			f, err := readFrame(s.conn)
			if err != nil {
				send(ReasonerEvent{Err: err})
				return
			}
			switch f.Kind {
			case KindAskPartial:
				if !send(ReasonerEvent{Delta: string(f.Payload)}) {
					return
				}
			case KindAskEnd:
				send(ReasonerEvent{Delta: string(f.Payload), Final: true})
				return
			case KindConsentRequire:
				send(ReasonerEvent{Err: fmt.Errorf("%w: %s", ErrConsentNotGranted, string(f.Payload))})
				return
			default:
				send(ReasonerEvent{Err: fmt.Errorf("a2a: unexpected ask response %s", f.Kind)})
				return
			}
		}
	}()
	return out, nil
}

func (s *session) SearchMemory(_ context.Context, q string, k int) ([]MemoryHit, error) {
	if err := writeFrame(s.conn, Frame{
		ID:      NewID(),
		Kind:    KindMemorySearch,
		Payload: mustMarshal(memorySearchPayload{Query: q, K: k}),
	}); err != nil {
		return nil, err
	}
	f, err := readFrame(s.conn)
	if err != nil {
		return nil, err
	}
	switch f.Kind {
	case KindMemoryResult:
		var body memoryResultPayload
		if err := unmarshalPayload(f.Payload, &body); err != nil {
			return nil, err
		}
		if body.Err != "" {
			return nil, errors.New(body.Err)
		}
		return body.Hits, nil
	case KindConsentRequire:
		return nil, fmt.Errorf("%w: %s", ErrConsentNotGranted, string(f.Payload))
	default:
		return nil, fmt.Errorf("a2a: unexpected memory response %s", f.Kind)
	}
}

func (s *session) Bye(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	_ = writeFrame(s.conn, Frame{ID: NewID(), Kind: KindBye, Payload: []byte{}})
	_ = s.conn.Close()
}

func mustMarshal(v any) []byte {
	b, err := cbor.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("a2a: marshal payload: %v", err))
	}
	return b
}

func unmarshalPayload(b []byte, v any) error {
	return cbor.Unmarshal(b, v)
}
