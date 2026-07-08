package a2a

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/trilam/leah/internal/budget"
)

// A2APeer is the §5.4.1 peering record. Pubkey + Fingerprint(Pubkey) ==
// a2a_peer.id; declared name is operator-overridable at pair time per §5.4.1.
type A2APeer struct {
	ID       PeerID
	Name     string
	Pubkey   ed25519.PublicKey
	PairedAt time.Time
	Paused   bool
	Scopes   []string
}

// A2AServer is the §5.5 inbound-side contract.
type A2AServer interface {
	Listen(ctx context.Context, addr netip.AddrPort) error
	Stop(ctx context.Context) error
	Peers() []A2APeer
	Revoke(ctx context.Context, peerID PeerID) error
}

// ReasonerEvent is what a peer's Ask call streams back. Mirrors the
// internal/reasoner stream shape so the A2A proxy adds no new type tax.
type ReasonerEvent struct {
	Delta string
	Final bool
	Err   error
}

// MemoryHit is a §5.7-bounded memory.search row. share: external rows only.
type MemoryHit struct {
	ID    string
	Text  string
	Score float64
}

// A2ASession is the §5.5 per-peer session. Negotiate is the §5.4 hello.*
// handshake; the remaining methods are direct passthroughs gated by Consent.
type A2ASession interface {
	Negotiate(ctx context.Context) (CapabilitySet, error)
	Ask(ctx context.Context, prompt string) (<-chan ReasonerEvent, error)
	SearchMemory(ctx context.Context, q string, k int) ([]MemoryHit, error)
	Bye(ctx context.Context)
}

// Handler is the server-side application contract. The protocol layer hands
// off after consent + budget gates clear. Implementation lives in the
// daemon — kept minimal here so the protocol package doesn't pull in
// reasoner or memory.
type Handler interface {
	Ask(ctx context.Context, peer PeerID, prompt string, out chan<- ReasonerEvent)
	SearchMemory(ctx context.Context, peer PeerID, q string, k int) ([]MemoryHit, error)
}

// Server is the protocol-layer implementation of A2AServer. Identity, consent,
// and budget are injected; the network listener lives behind Listen so tests
// can drive it via net.Pipe.
type Server struct {
	Identity ed25519.PrivateKey
	Consent  *ConsentStore
	Budget   *budget.Budget
	Caps     CapabilitySet
	Handler  Handler

	// AskCost is the dollar charge per inbound ask.request, billed against the
	// per-process Budget. §5.7: peer-driven calls bill the operator.
	AskCost float64

	mu       sync.Mutex
	peers    map[PeerID]A2APeer
	listener net.Listener
}

// NewServer wires the protocol layer. Caller supplies identity + dependencies.
func NewServer(id ed25519.PrivateKey, consent *ConsentStore, b *budget.Budget, caps CapabilitySet, h Handler) *Server {
	return &Server{
		Identity: id,
		Consent:  consent,
		Budget:   b,
		Caps:     caps,
		Handler:  h,
		AskCost:  0.001,
		peers:    map[PeerID]A2APeer{},
	}
}

// AddPeer registers a paired peer — the operator-driven pair flow calls this
// after OTP confirmation.
func (s *Server) AddPeer(p A2APeer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[p.ID] = p
}

// Peers returns a snapshot of paired peers.
func (s *Server) Peers() []A2APeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]A2APeer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	return out
}

// Revoke drops a peer. §5.5 — operator-initiated.
func (s *Server) Revoke(ctx context.Context, peerID PeerID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.peers[peerID]; !ok {
		return fmt.Errorf("a2a: unknown peer %s", peerID)
	}
	delete(s.peers, peerID)
	return nil
}

// Listen starts a TCP listener on addr and serves until ctx is done. §5.7
// defaults to 127.0.0.1 — callers binding to a public address must pass an
// explicit non-loopback netip.Addr.
func (s *Server) Listen(ctx context.Context, addr netip.AddrPort) error {
	l, err := net.Listen("tcp", addr.String())
	if err != nil {
		return fmt.Errorf("a2a: listen %s: %w", addr, err)
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("a2a: accept: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

// Stop closes the listener; concurrent Listen returns from Accept.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	l := s.listener
	s.listener = nil
	s.mu.Unlock()
	if l != nil {
		return l.Close()
	}
	return nil
}

// handleConn is the per-connection loop. ServeConn closes conn on return.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	if err := s.ServeConn(ctx, conn); err != nil && !errors.Is(err, io.EOF) {
		// §5.8 — drop session, log; do not retry.
		_ = err
	}
}

// ServeConn runs the protocol loop on conn until Bye or error. The conn is
// always closed before return — A2A sessions are single-shot (§5.4 hello.*
// establishes one session per connection).
func (s *Server) ServeConn(ctx context.Context, conn net.Conn) error {
	defer func() { _ = conn.Close() }()
	peer, caps, err := s.serverHandshake(ctx, conn)
	if err != nil {
		return err
	}
	_ = caps
	for {
		f, err := readFrame(conn)
		if err != nil {
			return err
		}
		if err := CheckTaskDepth(f); err != nil {
			return err
		}
		if err := s.dispatch(ctx, conn, peer, f); err != nil {
			return err
		}
		if f.Kind == KindBye {
			return nil
		}
	}
}

func (s *Server) serverHandshake(ctx context.Context, conn net.Conn) (PeerID, CapabilitySet, error) {
	// Inbound hello.offer carries proposed caps + claimed pubkey.
	offer, err := readFrame(conn)
	if err != nil {
		return "", 0, err
	}
	if offer.Kind != KindHelloOffer {
		return "", 0, fmt.Errorf("a2a: expected hello.offer got %s", offer.Kind)
	}
	var hello helloPayload
	if err := unmarshalPayload(offer.Payload, &hello); err != nil {
		return "", 0, err
	}
	if len(hello.Pubkey) != ed25519.PublicKeySize {
		return "", 0, fmt.Errorf("a2a: bad pubkey length %d", len(hello.Pubkey))
	}
	peerID := Fingerprint(hello.Pubkey)
	s.mu.Lock()
	peer, known := s.peers[peerID]
	s.mu.Unlock()
	if !known {
		return "", 0, fmt.Errorf("a2a: peer %s not paired", peerID)
	}
	if peer.Paused {
		return "", 0, fmt.Errorf("a2a: peer %s paused", peerID)
	}

	// Caps are the AND of what we and the peer advertise — §5.4 graceful
	// degradation across versions.
	negotiated := s.Caps & CapabilitySet(hello.Caps)

	// hello.ack with our pubkey + negotiated caps + a nonce for identity.prove.
	nonce, err := NewNonce()
	if err != nil {
		return "", 0, err
	}
	ackBody := helloAckPayload{
		Pubkey: s.Identity.Public().(ed25519.PublicKey),
		Caps:   uint64(negotiated),
		Nonce:  nonce,
	}
	if err := writeFrame(conn, Frame{
		ID:      NewID(),
		Kind:    KindHelloAck,
		Payload: mustMarshal(ackBody),
	}); err != nil {
		return "", 0, err
	}

	// identity.prove from peer signed over our nonce.
	prove, err := readFrame(conn)
	if err != nil {
		return "", 0, err
	}
	if prove.Kind != KindIdentityProve {
		return "", 0, fmt.Errorf("a2a: expected identity.prove got %s", prove.Kind)
	}
	if err := Verify(hello.Pubkey, nonce, prove.Payload); err != nil {
		return "", 0, err
	}

	// identity.verify ack so the peer knows we accepted.
	if err := writeFrame(conn, Frame{
		ID:      NewID(),
		Kind:    KindIdentityVerify,
		Payload: []byte{},
	}); err != nil {
		return "", 0, err
	}
	return peerID, negotiated, nil
}

func (s *Server) dispatch(ctx context.Context, conn net.Conn, peer PeerID, f Frame) error {
	switch f.Kind {
	case KindAskRequest:
		return s.handleAsk(ctx, conn, peer, f)
	case KindMemorySearch:
		return s.handleMemorySearch(ctx, conn, peer, f)
	case KindBye:
		return nil
	default:
		// Unknown-in-context — log and continue. §5.8: "tool.unknown" semantics.
		return nil
	}
}

func (s *Server) handleAsk(ctx context.Context, conn net.Conn, peer PeerID, f Frame) error {
	if err := s.Consent.Check(ctx, peer, ConsentAsk); err != nil {
		denyFrame := Frame{ID: NewID(), Kind: KindConsentRequire, Payload: []byte(ConsentAsk)}
		return writeFrame(conn, denyFrame)
	}
	if s.Budget != nil {
		if err := s.Budget.Charge(s.AskCost); err != nil {
			return writeFrame(conn, Frame{ID: NewID(), Kind: KindAskEnd, Payload: []byte("budget.exhausted")})
		}
	}
	var req askRequestPayload
	if err := unmarshalPayload(f.Payload, &req); err != nil {
		return err
	}
	// Cancel + drain on early exit so Handler.Ask can't park on out<-
	// once the 16-slot buffer fills.
	hctx, hcancel := context.WithCancel(ctx)
	out := make(chan ReasonerEvent, 16)
	go func() {
		defer close(out)
		if s.Handler != nil {
			s.Handler.Ask(hctx, peer, req.Prompt, out)
		}
	}()
	drain := func() {
		hcancel()
		for range out {
		}
	}
	for ev := range out {
		if ev.Err != nil {
			hcancel()
			return writeFrame(conn, Frame{ID: NewID(), Kind: KindAskEnd, Payload: []byte(ev.Err.Error())})
		}
		kind := KindAskPartial
		if ev.Final {
			kind = KindAskEnd
		}
		if err := writeFrame(conn, Frame{ID: NewID(), Kind: kind, Payload: []byte(ev.Delta)}); err != nil {
			drain()
			return err
		}
		if ev.Final {
			hcancel()
			return nil
		}
	}
	hcancel()
	return writeFrame(conn, Frame{ID: NewID(), Kind: KindAskEnd, Payload: []byte{}})
}

func (s *Server) handleMemorySearch(ctx context.Context, conn net.Conn, peer PeerID, f Frame) error {
	if err := s.Consent.Check(ctx, peer, ConsentMemory); err != nil {
		return writeFrame(conn, Frame{ID: NewID(), Kind: KindConsentRequire, Payload: []byte(ConsentMemory)})
	}
	var req memorySearchPayload
	if err := unmarshalPayload(f.Payload, &req); err != nil {
		return err
	}
	hits, err := s.Handler.SearchMemory(ctx, peer, req.Query, req.K)
	if err != nil {
		return writeFrame(conn, Frame{ID: NewID(), Kind: KindMemoryResult, Payload: mustMarshal(memoryResultPayload{Err: err.Error()})})
	}
	return writeFrame(conn, Frame{
		ID:      NewID(),
		Kind:    KindMemoryResult,
		Payload: mustMarshal(memoryResultPayload{Hits: hits}),
	})
}

// helloPayload is the hello.offer body.
type helloPayload struct {
	Pubkey []byte `cbor:"pubkey"`
	Caps   uint64 `cbor:"caps"`
	Name   string `cbor:"name"`
}

type helloAckPayload struct {
	Pubkey []byte `cbor:"pubkey"`
	Caps   uint64 `cbor:"caps"`
	Nonce  []byte `cbor:"nonce"`
}

type askRequestPayload struct {
	Prompt string `cbor:"prompt"`
	Depth  int    `cbor:"depth,omitempty"`
}

type memorySearchPayload struct {
	Query string `cbor:"q"`
	K     int    `cbor:"k"`
}

type memoryResultPayload struct {
	Hits []MemoryHit `cbor:"hits"`
	Err  string      `cbor:"err,omitempty"`
}

// Wire framing: 4-byte big-endian length + CBOR-encoded Frame. Lets the reader
// know how many bytes to consume up front so a Frame.Payload can carry a
// memory.result without TCP fragmentation breaking the decode.
func writeFrame(w io.Writer, f Frame) error {
	b, err := Encode(f)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readFrame(r io.Reader) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameBytes {
		return Frame{}, ErrFrameTooLarge
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return Frame{}, err
	}
	return Decode(buf)
}
