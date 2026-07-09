package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/attest"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
	commsin "github.com/trilam/leah/internal/comms/in"
)

// runInbound dispatches `leah inbound <verb>`. Today only `enroll` is wired;
// the daemon-side router consumes the same file enrolled here.
func runInbound(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) || len(args) == 0 {
		printInboundUsage(w)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	switch args[0] {
	case "enroll":
		return runInboundEnroll(ctx, args[1:], w, newConnectAttestor())
	default:
		printInboundUsage(os.Stderr)
		return 2
	}
}

// runInboundEnroll attests at ScopeInboundEnroll then persists (channel, peer)
// to ~/.leah-state/inbound-enroll.json. The act of granting a remote surface
// the right to act is itself a local-loopback attestation (spec §4.4).
//
// Signature takes a connect.Attestor seam so tests can drive the success +
// denial paths without wiring stdin.
func runInboundEnroll(ctx context.Context, args []string, w io.Writer, att connect.Attestor) int {
	if shouldShowHelp(args) || len(args) < 2 {
		printInboundUsage(w)
		if shouldShowHelp(args) {
			return 0
		}
		return 2
	}
	channel := args[0]
	peerID := args[1]

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	if err := att.Attest(ctx, attest.ScopeInboundEnroll); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "inbound_enroll",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "attestation_denied",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah inbound enroll: %v\n", err)
		return 1
	}

	store, err := commsin.OpenFileEnrollStore(inboundEnrollPath())
	if err != nil {
		_ = a.Append(audit.Entry{Kind: "inbound_enroll", BlastRadius: 2, Outcome: "failed", Detail: "store_open_failed"})
		_, _ = fmt.Fprintf(os.Stderr, "leah inbound enroll: %v\n", err)
		return 1
	}
	if err := store.Enroll(channel, peerID); err != nil {
		_ = a.Append(audit.Entry{Kind: "inbound_enroll", BlastRadius: 2, Outcome: "failed", Detail: "store_write_failed"})
		_, _ = fmt.Fprintf(os.Stderr, "leah inbound enroll: %v\n", err)
		return 1
	}
	_ = a.Append(audit.Entry{Kind: "inbound_enroll", BlastRadius: 2, Outcome: "success"})
	_, _ = fmt.Fprintf(w, "ok: enrolled %s peer %s -> %s\n", channel, peerID, inboundEnrollPath())
	return 0
}

func inboundEnrollPath() string {
	return filepath.Join(stateDir(), "inbound-enroll.json")
}

func printInboundUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah inbound enroll <channel> <peerID>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "One-time loopback authorization: allows a remote (channel,peer) pair to")
	_, _ = fmt.Fprintln(w, "answer pushed recommendations. Per-action attestation still gates Apply.")
}
