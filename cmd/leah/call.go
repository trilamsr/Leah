package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/trilam/leah/internal/actions/adapters/facetime"
	"github.com/trilam/leah/internal/actions/adapters/imessage"
	"github.com/trilam/leah/internal/platform/contracts"
)

// Distinct from the ship-tool scope so granted consent never cross-authorizes (anti-habituation).
const (
	scopeShipIMessage = "cli:imessage:send"
	scopeCall         = "cli:facetime:call"
)

// nativeExec streams any script via stdin so it never lands in argv.
type nativeExec struct{}

func (nativeExec) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return out, err
}

func printCallUsage() {
	fmt.Fprintln(os.Stderr, "usage: leah call <callee> [--audio]")
}

func runCall(ctx context.Context, args []string) int {
	audio := false
	var callee string
	for _, a := range args {
		switch {
		case a == "--audio":
			audio = true
		case a == "-h" || a == "--help":
			printCallUsage()
			return 0
		case callee == "":
			callee = a
		}
	}
	if callee == "" {
		printCallUsage()
		return 2
	}
	return runCallWith(ctx, newConnectAttestor(), nativeExec{}, callee, audio)
}

func runCallWith(ctx context.Context, att contracts.Attestor, ex facetime.OSExec, callee string, audio bool) int {
	if err := att.Attest(ctx, scopeCall); err != nil {
		fmt.Fprintf(os.Stderr, "leah call: %v\n", err)
		return 1
	}
	ad, err := facetime.New(facetime.Config{Attestor: noopAttestor{}, OSExec: ex})
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah call: %v\n", err)
		return 1
	}
	if audio {
		err = ad.InitiateAudio(ctx, callee)
	} else {
		err = ad.InitiateVideo(ctx, callee)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah call: %v\n", err)
		return 1
	}
	return 0
}

func runShipIMessageWith(ctx context.Context, att contracts.Attestor, ex imessage.OSExec, to, body string) int {
	if err := att.Attest(ctx, scopeShipIMessage); err != nil {
		fmt.Fprintf(os.Stderr, "leah ship --imessage: %v\n", err)
		return 1
	}
	ad, err := imessage.New(imessage.Config{Attestor: noopAttestor{}, OSExec: ex})
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah ship --imessage: %v\n", err)
		return 1
	}
	if err := ad.Send(ctx, imessage.Message{To: to, Body: body}); err != nil {
		fmt.Fprintf(os.Stderr, "leah ship --imessage: %v\n", err)
		return 1
	}
	return 0
}
