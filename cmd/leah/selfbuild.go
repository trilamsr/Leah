package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/reasoner"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/watchdog"
)

// runSelfBuild wires SelfBuild against trilamsr/Leah (repo locked in
// dispatcher; CLI cannot override).
func runSelfBuild(intent string) {
	ctx := context.Background()
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
	b := budget.New()

	promptPath := filepath.Join(promptDir(), "self-build-feature.md")
	sysPrompt, err := os.ReadFile(promptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read self-build prompt: %v\n", err)
		os.Exit(1)
	}

	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(sysPrompt)}

	tmp, err := os.MkdirTemp("", "leah-selfbuild-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	sb := &dispatcher.SelfBuild{
		Reasoner:                 r,
		GH:                       ghclient.New(),
		Audit:                    a,
		Budget:                   b,
		Out:                      os.Stdout,
		TmpDir:                   tmp,
		PromptPath:               promptPath,
		AttestationQuestionsPath: filepath.Join(promptDir(), "self-build-attestations.txt"),
		Watch:                    true,
		Regatta:                  regattaclient.New(),
		Heartbeat:                watchdog.New(),
		Notify:                   notify.NewDesktop(),
		PollEvery:                30 * time.Second,
		MaxPolls:                 120,
	}

	if err := sb.Run(ctx, intent); err != nil {
		if errors.Is(err, dispatcher.ErrSelfBuildClarify) {
			// Reasoner printed questions; non-fatal exit code 0.
			return
		}
		fmt.Fprintf(os.Stderr, "leah self-build: %v\n", err)
		os.Exit(1)
	}
}
