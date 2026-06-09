package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/watchdog"
)

func main() {
	pollEvery := 30 * time.Second
	if v := os.Getenv("LEAH_DAEMON_POLL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pollEvery = time.Duration(n) * time.Second
		}
	}

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath}

	loop := daemonloop.New(
		regattaclient.New(),
		watchdog.New(),
		notify.NewDesktop(),
		a,
		os.Stdout,
		pollEvery,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := loop.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
		os.Exit(1)
	}
}

func stateDir() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir state dir: %v\n", err)
		os.Exit(1)
	}
	return d
}
