package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trilam/leah/internal/platform/a2a"
	"github.com/trilam/leah/internal/platform/ipc"
)

// a2aServer is the daemon-side view of internal/a2a.Server — narrow so tests
// can drive the handlers without spinning a TCP listener. AddPeer is the seam
// pause toggling rides on (a2a.Server lacks SetPaused; re-Add overwrites the
// map entry).
type a2aServer interface {
	Peers() []a2a.A2APeer
	AddPeer(p a2a.A2APeer)
	Revoke(ctx context.Context, id a2a.PeerID) error
}

// peerWire is the JSON shape the ConnectionsPane consumes. Pubkey full-hex
// stays out of the panel — fingerprint (first 16 chars of ID) is what humans
// compare on pair.
type peerWire struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	PubkeyFP string   `json:"pubkey_fp"`
	PairedAt int64    `json:"paired_at"`
	Paused   bool     `json:"paused"`
	Scopes   []string `json:"scopes"`
}

func handleA2APeerList(_ context.Context, req ipc.Frame, srv a2aServer) (<-chan ipc.Frame, error) {
	if srv == nil {
		return a2aErr(req, "a2a server unavailable"), nil
	}
	peers := srv.Peers()
	wire := make([]peerWire, 0, len(peers))
	for _, p := range peers {
		wire = append(wire, peerWire{
			ID:       string(p.ID),
			Name:     p.Name,
			PubkeyFP: shortFP(p.ID),
			PairedAt: p.PairedAt.Unix(),
			Paused:   p.Paused,
			Scopes:   p.Scopes,
		})
	}
	payload, _ := json.Marshal(map[string]any{"peers": wire})
	return ackFrame(req, ipc.KindA2APeerList, payload), nil
}

func handleA2APairStart(_ context.Context, req ipc.Frame, srv a2aServer) (<-chan ipc.Frame, error) {
	if srv == nil {
		return a2aErr(req, "a2a server unavailable"), nil
	}
	var p struct {
		OTP      string `json:"otp_string"`
		PeerAddr string `json:"peer_addr"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return a2aErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.OTP == "" || p.PeerAddr == "" {
		return a2aErr(req, "otp_string and peer_addr required"), nil
	}
	// Inbound pairing is operator-driven and completes asynchronously over
	// the §5.4 hello.* TCP handshake; the daemon stub acks pending so the
	// ConnectionsPane can render the spinner. AddPeer fires from the
	// handshake path after OTP confirm — out of scope for this IPC frame.
	payload, _ := json.Marshal(map[string]string{"peer_id": "", "status": "pending"})
	return ackFrame(req, ipc.KindA2APairStart, payload), nil
}

func handleA2APeerPause(_ context.Context, req ipc.Frame, srv a2aServer) (<-chan ipc.Frame, error) {
	if srv == nil {
		return a2aErr(req, "a2a server unavailable"), nil
	}
	var p struct {
		PeerID string `json:"peer_id"`
		Paused bool   `json:"paused"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return a2aErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.PeerID == "" {
		return a2aErr(req, "peer_id required"), nil
	}
	id := a2a.PeerID(p.PeerID)
	var found *a2a.A2APeer
	for _, peer := range srv.Peers() {
		if peer.ID == id {
			peer.Paused = p.Paused
			found = &peer
			break
		}
	}
	if found == nil {
		return a2aErr(req, fmt.Sprintf("unknown peer %s", p.PeerID)), nil
	}
	srv.AddPeer(*found)
	payload, _ := json.Marshal(map[string]any{"peer_id": p.PeerID, "paused": p.Paused})
	return ackFrame(req, ipc.KindA2APeerPause, payload), nil
}

func handleA2APeerUnpair(ctx context.Context, req ipc.Frame, srv a2aServer) (<-chan ipc.Frame, error) {
	if srv == nil {
		return a2aErr(req, "a2a server unavailable"), nil
	}
	var p struct {
		PeerID string `json:"peer_id"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return a2aErr(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.PeerID == "" {
		return a2aErr(req, "peer_id required"), nil
	}
	if err := srv.Revoke(ctx, a2a.PeerID(p.PeerID)); err != nil {
		return a2aErr(req, err.Error()), nil
	}
	payload, _ := json.Marshal(map[string]string{"peer_id": p.PeerID})
	return ackFrame(req, ipc.KindA2APeerUnpair, payload), nil
}

func ackFrame(req ipc.Frame, kind string, payload json.RawMessage) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: kind, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

func a2aErr(req ipc.Frame, msg string) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	out <- ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

func shortFP(id a2a.PeerID) string {
	s := string(id)
	if len(s) <= 16 {
		return s
	}
	return s[:16]
}
